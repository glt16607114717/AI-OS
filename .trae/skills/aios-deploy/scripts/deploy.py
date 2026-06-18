import subprocess
import os
import sys
import time

# ── 服务器配置 ──
SERVER_HOST = '8.163.127.182'
SERVER_PORT = 443
SERVER_USER = 'root'
SERVER_PASS = 'glt01054717@'
DEPLOY_PORT = '18731'
REMOTE_PATH = '/opt/ai-os/ai-os-server'


def run_command(args, timeout=60):
    try:
        result = subprocess.run(
            args,
            capture_output=True,
            timeout=timeout
        )
        try:
            stdout = result.stdout.decode('utf-8').strip()
        except:
            stdout = result.stdout.decode('gbk', errors='ignore').strip()
        try:
            stderr = result.stderr.decode('utf-8').strip()
        except:
            stderr = result.stderr.decode('gbk', errors='ignore').strip()
        return result.returncode, stdout, stderr
    except subprocess.TimeoutExpired:
        return -1, "", "命令超时"


def ssh_exec(cmd, timeout=60):
    """通过 paramiko 执行远程命令"""
    import paramiko
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(SERVER_HOST, port=SERVER_PORT, username=SERVER_USER,
                   password=SERVER_PASS, timeout=15)
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode()
    err = stderr.read().decode()
    client.close()
    return out, err


def main():
    action = 'deploy'
    if len(sys.argv) > 1:
        action = sys.argv[1].lower()

    # 基于脚本位置自动推导项目根目录（跨机器兼容）
    script_dir = os.path.dirname(os.path.abspath(__file__))
    project_root = os.path.abspath(os.path.join(script_dir, '..', '..', '..', '..'))
    go_dir = os.path.join(project_root, 'go-backend')
    output_file = 'ai-os-server'

    print(f"=== AI-OS Go 后端部署 ===")
    print(f"操作类型: {action}")
    print(f"项目根目录: {project_root}")
    print(f"Go 目录: {go_dir}")
    print(f"服务器: {SERVER_USER}@{SERVER_HOST}:{SERVER_PORT}")

    if action in ['build', 'deploy']:
        os.chdir(go_dir)

        print("\n1. 交叉编译 Go 代码...")
        os.environ['GOOS'] = 'linux'
        os.environ['GOARCH'] = 'amd64'
        os.environ['CGO_ENABLED'] = '0'

        returncode, stdout, stderr = run_command(['go', 'build', '-o', output_file, '.'], timeout=120)

        if stdout:
            print("STDOUT:", stdout)
        if stderr:
            print("STDERR:", stderr)

        if returncode != 0:
            print(f"编译失败，退出码: {returncode}")
            return

        local_file = os.path.join(go_dir, output_file)
        size_mb = os.path.getsize(local_file) / 1024 / 1024
        print(f"编译成功 ({size_mb:.1f} MB)")

    if action == 'deploy':
        import paramiko

        print("\n2. 上传到服务器...")
        local_file = os.path.join(go_dir, output_file)

        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        client.connect(SERVER_HOST, port=SERVER_PORT, username=SERVER_USER,
                       password=SERVER_PASS, timeout=15)

        # 先停服务（释放文件占用）
        print("3. 停止现有服务...")
        ssh_exec('systemctl stop ai-os && sleep 2 && echo OK')

        # 强制释放端口
        ssh_exec(f'kill -9 $(lsof -ti:{DEPLOY_PORT}) 2>/dev/null; sleep 1; echo DONE')

        # 备份+上传
        print("4. 备份旧版本并上传新版本...")
        ssh_exec(f'cp {REMOTE_PATH} {REMOTE_PATH}.bak 2>/dev/null; echo BACKUP_DONE')

        sftp = client.open_sftp()
        sftp.put(local_file, REMOTE_PATH)
        sftp.close()
        client.close()
        print(f"上传完成 ({size_mb:.1f} MB)")

        # 启动
        print("\n5. 启动服务...")
        out, err = ssh_exec(
            f'chmod +x {REMOTE_PATH} && systemctl start ai-os && sleep 3 && '
            f'systemctl is-active ai-os && ss -tlnp | grep {DEPLOY_PORT}',
            timeout=30
        )
        print(out)

        if 'active' in out:
            # 健康检查
            out2, _ = ssh_exec(f'curl -s -m 3 http://127.0.0.1:{DEPLOY_PORT}/api/health')
            print(f"健康检查: {out2}")
            print("\n=== 部署完成 ===")
        else:
            print("\n=== 部署失败 ===")
            out3, _ = ssh_exec('journalctl -u ai-os --no-pager -n 20')
            print(out3)

        # 清理本地二进制
        os.remove(local_file)

    elif action == 'restart':
        print("\n1. 停止服务...")
        out, _ = ssh_exec('systemctl stop ai-os && sleep 2 && echo STOPPED')
        print(out)

        print("2. 释放端口...")
        out, _ = ssh_exec(f'kill -9 $(lsof -ti:{DEPLOY_PORT}) 2>/dev/null; sleep 1; echo DONE')
        print(out)

        print("3. 启动服务...")
        out, _ = ssh_exec(
            f'systemctl start ai-os && sleep 3 && systemctl is-active ai-os && '
            f'curl -s -m 3 http://127.0.0.1:{DEPLOY_PORT}/api/health',
            timeout=30
        )
        print(out)
        print("\n=== 重启完成 ===")


if __name__ == "__main__":
    main()

"""AI-OS Go 后端部署
用原生 ssh / scp 命令，不依赖 paramiko。
"""
import subprocess
import os
import sys

SSH = ['ssh', '-p', '443', '-o', 'StrictHostKeyChecking=no', 'root@8.163.127.182']
SCP = ['scp', '-P', '443', '-o', 'StrictHostKeyChecking=no']
DEPLOY_PORT = '18731'
REMOTE_PATH = '/opt/ai-os/ai-os-server'


def run(args, timeout=120):
    result = subprocess.run(args, capture_output=True, timeout=timeout)
    out = result.stdout.decode('utf-8', errors='replace').strip()
    err = result.stderr.decode('utf-8', errors='replace').strip()
    return result.returncode, out, err


def ssh(cmd, timeout=60):
    return run(SSH + [cmd], timeout)


def main():
    action = sys.argv[1].lower() if len(sys.argv) > 1 else 'deploy'

    script_dir = os.path.dirname(os.path.abspath(__file__))
    project_root = os.path.abspath(os.path.join(script_dir, '..', '..', '..', '..'))
    go_dir = os.path.join(project_root, 'go-backend')
    binary = 'ai-os-server'

    print(f"=== AI-OS Go 后端部署 ===")
    print(f"操作: {action}  |  服务器: root@8.163.127.182:443")

    if action in ('build', 'deploy'):
        os.chdir(go_dir)
        os.environ['GOOS'] = 'linux'
        os.environ['GOARCH'] = 'amd64'
        os.environ['CGO_ENABLED'] = '0'

        print("\n1. 交叉编译...")
        rc, out, err = run(['go', 'build', '-o', binary, '.'], timeout=120)
        if rc != 0:
            print(f"编译失败: {err}")
            return
        size_mb = os.path.getsize(os.path.join(go_dir, binary)) / 1024 / 1024
        print(f"编译成功 ({size_mb:.1f} MB)")

    if action == 'deploy':
        local_file = os.path.join(go_dir, binary)

        print("\n2. 停止服务...")
        ssh('systemctl stop ai-os && sleep 2 && echo STOPPED')

        print("3. 上传...")
        rc, out, err = run(SCP + [local_file, f'root@8.163.127.182:{REMOTE_PATH}'], timeout=60)
        if rc != 0:
            print(f"上传失败: {err}")
            return
        print(f"上传完成 ({os.path.getsize(local_file) / 1024 / 1024:.1f} MB)")

        print("4. 启动服务...")
        rc, out, err = ssh(
            f'chmod +x {REMOTE_PATH} && systemctl start ai-os && sleep 3 && '
            f'systemctl is-active ai-os && ss -tlnp | grep {DEPLOY_PORT}',
            timeout=30
        )
        print(out)

        if 'active' in out:
            rc2, health, _ = ssh(f'curl -s -m 3 http://127.0.0.1:{DEPLOY_PORT}/api/health')
            print(f"健康检查: {health}")
            print("\n=== 部署完成 ===")
        else:
            print("\n=== 部署失败，查看日志 ===")
            _, log, _ = ssh('journalctl -u ai-os --no-pager -n 20')
            print(log)

        os.remove(local_file)

    elif action == 'restart':
        print("\n1. 重启服务...")
        rc, out, err = ssh(
            f'systemctl stop ai-os && sleep 2 && '
            f'systemctl start ai-os && sleep 3 && '
            f'systemctl is-active ai-os && '
            f'curl -s -m 3 http://127.0.0.1:{DEPLOY_PORT}/api/health',
            timeout=30
        )
        print(out)
        print("\n=== 重启完成 ===")


if __name__ == '__main__':
    main()

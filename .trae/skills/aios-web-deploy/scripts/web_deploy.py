import subprocess
import os
import sys
import shutil
import paramiko
import stat

# ── 服务器配置 ──
SERVER_HOST = '8.163.127.182'
SERVER_PORT = 443
SERVER_USER = 'root'
SERVER_PASS = 'glt01054717@'
REMOTE_WEB_PATH = '/www/wwwroot/ai-os-web'

def run_command(args, timeout=120):
    """执行本地命令"""
    try:
        # Windows 下查找 npm
        if args[0] == 'npm' and os.name == 'nt':
            args = ['npm.cmd'] + args[1:]

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
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(SERVER_HOST, port=SERVER_PORT, username=SERVER_USER,
                   password=SERVER_PASS, timeout=15)
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode()
    err = stderr.read().decode()
    client.close()
    return out, err


def create_nginx_config():
    """创建 Nginx 配置文件 (已配置，此函数保留但不使用)"""
    # Nginx 配置已完成，此函数仅保留备用
    return ""


def main():
    action = 'deploy'
    if len(sys.argv) > 1:
        action = sys.argv[1].lower()

    # 基于脚本位置自动推导项目根目录
    script_dir = os.path.dirname(os.path.abspath(__file__))
    project_root = os.path.abspath(os.path.join(script_dir, '..', '..', '..', '..'))
    web_dir = os.path.join(project_root, 'web')
    dist_dir = os.path.join(web_dir, 'dist')

    print(f"=== AI-OS Web 前端部署 ===")
    print(f"操作类型: {action}")
    print(f"项目根目录: {project_root}")
    print(f"Web 目录: {web_dir}")
    print(f"服务器: {SERVER_USER}@{SERVER_HOST}:{SERVER_PORT}")

    if action in ['build', 'deploy']:
        os.chdir(web_dir)

        # 检查 node_modules
        if not os.path.exists('node_modules'):
            print("\n1. 安装依赖...")
            returncode, stdout, stderr = run_command(['npm', 'install'], timeout=300)
            if returncode != 0:
                print(f"依赖安装失败: {stderr}")
                return

        print("\n2. 清理旧的构建文件...")
        if os.path.exists(dist_dir):
            shutil.rmtree(dist_dir)
            print(f"已删除 {dist_dir}")

        print("\n3. 构建前端项目...")
        returncode, stdout, stderr = run_command(['npm', 'run', 'build'], timeout=300)

        if stdout:
            print("STDOUT:", stdout)
        if stderr:
            print("STDERR:", stderr)

        if returncode != 0:
            print(f"构建失败，退出码: {returncode}")
            return

        if not os.path.exists(dist_dir):
            print(f"构建失败：dist 目录不存在")
            return

        # 计算构建产物大小
        total_size = 0
        for root, dirs, files in os.walk(dist_dir):
            for file in files:
                total_size += os.path.getsize(os.path.join(root, file))
        size_mb = total_size / 1024 / 1024
        print(f"构建成功 ({size_mb:.2f} MB)")

    if action == 'deploy':
        print("\n4. 连接服务器...")
        client = paramiko.SSHClient()
        client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
        client.connect(SERVER_HOST, port=SERVER_PORT, username=SERVER_USER,
                       password=SERVER_PASS, timeout=15)

        # 创建远程目录
        print("5. 准备远程目录...")
        ssh_exec(f'mkdir -p {REMOTE_WEB_PATH}')

        # 备份旧版本
        print("6. 备份旧版本...")
        ssh_exec(f'cp -r {REMOTE_WEB_PATH} {REMOTE_WEB_PATH}.bak 2>/dev/null; echo BACKUP_DONE')

        # 删除旧文件
        print("7. 删除旧文件...")
        ssh_exec(f'rm -rf {REMOTE_WEB_PATH}/*')

        # 上传新文件
        print("8. 上传新文件...")
        sftp = client.open_sftp()

        def upload_files(local_path, remote_path):
            """递归上传目录"""
            for item in os.listdir(local_path):
                local_item = os.path.join(local_path, item)
                remote_item = f"{remote_path}/{item}"  # Linux路径使用正斜杠

                if os.path.isfile(local_item):
                    sftp.put(local_item, remote_item)
                    print(f"上传: {item}")
                elif os.path.isdir(local_item):
                    try:
                        sftp.mkdir(remote_item)
                    except:
                        pass  # 目录可能已存在
                    upload_files(local_item, remote_item)

        upload_files(dist_dir, REMOTE_WEB_PATH)
        sftp.close()
        client.close()
        print(f"上传完成 ({size_mb:.2f} MB)")

        print("\n=== 部署完成 ===")
        print(f"前端文件已上传到: {REMOTE_WEB_PATH}")
        print(f"访问地址: http://{SERVER_HOST}/")


if __name__ == "__main__":
    main()
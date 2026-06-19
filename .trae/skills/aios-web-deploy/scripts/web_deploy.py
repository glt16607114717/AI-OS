"""AI-OS Web 前端部署
用原生 ssh / scp 命令，不依赖 paramiko。
"""
import subprocess
import os
import sys
import shutil

SSH = ['ssh', '-p', '443', '-o', 'StrictHostKeyChecking=no', 'root@8.163.127.182']
SCP = ['scp', '-P', '443', '-o', 'StrictHostKeyChecking=no', '-r']
REMOTE_WEB_PATH = '/www/wwwroot/ai-os-web'


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
    web_dir = os.path.join(project_root, 'web')
    dist_dir = os.path.join(web_dir, 'dist')

    print(f"=== AI-OS Web 前端部署 ===")
    print(f"操作: {action}  |  服务器: root@8.163.127.182:443")

    if action in ('build', 'deploy'):
        os.chdir(web_dir)

        if not os.path.exists('node_modules'):
            print("\n1. 安装依赖...")
            npm = 'npm.cmd' if os.name == 'nt' else 'npm'
            rc, out, err = run([npm, 'install'], timeout=300)
            if rc != 0:
                print(f"安装失败: {err}")
                return

        print("\n2. 构建...")
        if os.path.exists(dist_dir):
            shutil.rmtree(dist_dir)
        npm = 'npm.cmd' if os.name == 'nt' else 'npm'
        rc, out, err = run([npm, 'run', 'build'], timeout=300)
        if rc != 0:
            print(f"构建失败: {err}")
            return
        print("构建成功")

    if action == 'deploy':
        print("\n3. 上传...")
        ssh(f'rm -rf {REMOTE_WEB_PATH}/*')
        rc, out, err = run(SCP + [f'{dist_dir}/*', f'root@8.163.127.182:{REMOTE_WEB_PATH}/'], timeout=120)
        if rc != 0:
            print(f"上传失败: {err}")
            return
        print(f"上传完成")
        print(f"\n=== 部署完成 ===")
        print(f"访问: http://8.163.127.182/")


if __name__ == '__main__':
    main()

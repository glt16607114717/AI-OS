"""配置 SSH 免密登录（把本机公钥推送到服务器）
注意：首次运行需要输入服务器密码一次，之后免密。
"""
import os
import subprocess
import sys

HOST = 'root@8.163.127.182'
PORT = '443'
KEY_TYPES = ['id_ed25519', 'id_rsa']

if __name__ == '__main__':
    ssh_dir = os.path.expanduser('~/.ssh')
    found = False
    for kt in KEY_TYPES:
        pub = os.path.join(ssh_dir, f'{kt}.pub')
        if os.path.exists(pub):
            found = True
            print(f"推送公钥: {pub}")
            with open(pub, 'r') as f:
                pubkey = f.read().strip()
            cmd = f'mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo "{pubkey}" >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && echo KEY_ADDED'
            result = subprocess.run(
                ['ssh', '-p', PORT, '-o', 'StrictHostKeyChecking=no', HOST, cmd],
                capture_output=True, timeout=30
            )
            print(result.stdout.decode('utf-8', errors='replace'))

    if not found:
        print("未找到公钥，请先生成: ssh-keygen -t ed25519")
        sys.exit(1)

    print("\n验证免密...")
    result = subprocess.run(
        ['ssh', '-p', PORT, '-o', 'StrictHostKeyChecking=no', HOST, 'echo SSH_OK'],
        capture_output=True, timeout=15
    )
    out = result.stdout.decode('utf-8', errors='replace').strip()
    print(out if out else "失败，仍需密码")

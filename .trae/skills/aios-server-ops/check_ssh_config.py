"""检查 SSH 配置"""
import subprocess

SSH = ['ssh', '-p', '443', '-o', 'StrictHostKeyChecking=no', 'root@8.163.127.182']

def run(cmd):
    result = subprocess.run(SSH + [cmd], capture_output=True, timeout=30)
    print(result.stdout.decode('utf-8', errors='replace'))

if __name__ == '__main__':
    print("=== SSH配置 ===")
    run('grep -E "PubkeyAuthentication|AuthorizedKeysFile|Port|PermitRootLogin" /etc/ssh/sshd_config')
    print("\n=== .ssh目录 ===")
    run('ls -la ~/.ssh')
    print("\n=== SSH服务状态 ===")
    run('systemctl status sshd | head -5')

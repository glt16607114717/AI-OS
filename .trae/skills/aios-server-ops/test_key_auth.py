"""测试 SSH 密钥认证"""
import subprocess

SSH = ['ssh', '-p', '443', '-o', 'StrictHostKeyChecking=no', 'root@8.163.127.182']

if __name__ == '__main__':
    print("=== 测试 SSH 免密 ===")
    result = subprocess.run(SSH + ['echo "免密登录测试成功" && whoami'], capture_output=True, timeout=15)
    out = result.stdout.decode('utf-8', errors='replace').strip()
    err = result.stderr.decode('utf-8', errors='replace').strip()
    if out:
        print(out)
    if err:
        print(f"失败: {err}")

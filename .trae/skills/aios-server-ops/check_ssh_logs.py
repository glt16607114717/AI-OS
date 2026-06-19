"""检查 SSH 日志"""
import subprocess

SSH = ['ssh', '-p', '443', '-o', 'StrictHostKeyChecking=no', 'root@8.163.127.182']

def run(cmd):
    result = subprocess.run(SSH + [cmd], capture_output=True, timeout=30)
    print(result.stdout.decode('utf-8', errors='replace'))

if __name__ == '__main__':
    print("=== 最近SSH日志 ===")
    run('journalctl -u ssh -n 30 --no-pager')

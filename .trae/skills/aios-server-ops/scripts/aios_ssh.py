"""AIOS SSH 工具：用原生 ssh 命令执行远程命令
依赖 ~/.ssh/config 或默认密钥（id_rsa / id_ed25519 等），无需任何参数。
"""
import subprocess
import sys

HOST = 'root@8.163.127.182'
PORT = '443'

def run(cmd):
    result = subprocess.run(
        ['ssh', '-p', PORT, '-o', 'StrictHostKeyChecking=no', HOST, cmd],
        capture_output=True, timeout=120
    )
    out = result.stdout.decode('utf-8', errors='replace')
    err = result.stderr.decode('utf-8', errors='replace')
    if out:
        print(out, end='')
    if err:
        print('STDERR:', err, end='')

if __name__ == '__main__':
    cmd = ' '.join(sys.argv[1:]) if len(sys.argv) > 1 else 'echo OK'
    run(cmd)

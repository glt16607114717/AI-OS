"""AIOS SSH 工具：免密执行远程命令"""
import paramiko
import sys

def run(cmd):
    c = paramiko.SSHClient()
    c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    # 用密钥文件认证（无passphrase）
    import os
    key_path = os.path.expanduser('~/.ssh/aios_ed25519')
    pkey = paramiko.RSAKey.from_private_key_file(key_path)
    c.connect('8.163.127.182', port=443, username='root', pkey=pkey)
    stdin, stdout, stderr = c.exec_command(cmd)
    out = stdout.read().decode()
    err = stderr.read().decode()
    c.close()
    if out:
        print(out, end='')
    if err:
        print('STDERR:', err, end='')

if __name__ == '__main__':
    cmd = ' '.join(sys.argv[1:]) if len(sys.argv) > 1 else 'echo OK'
    run(cmd)

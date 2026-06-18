#!/usr/bin/env python
# -*- coding: utf-8 -*-
import paramiko

# ── 服务器配置 ──
SERVER_HOST = '8.163.127.182'
SERVER_PORT = 443
SERVER_USER = 'root'
SERVER_PASS = 'glt01054717@'

# ── 公钥内容 ──
PUBLIC_KEY = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDIBLYf3hgGVFjsBKTQv0GTWKjIMNkWwGmfi5NVcgQ+6DKeBs9B/6PouTZ6WyY24PV/bv9PgHQEnR53KW1e8AjTc5eQtZUZ/DW2Vb69ORKmXuSf0yon/o8iexo7jtC2wQ+gKNLLpuz+gUxBjw/fDmi0bauJclflQn73WpKAgAzuUCs853oJy9PDIdpDZfDlEuN5B4t/zJWzgtnf2r9ACHyVqOrgY+qLbAp5sn5E0sll5/IqDpts5+fbSdiM3WflfLVqYXCtAcvwDAxREeG45GmtTT89VwiJFN8unhjgBZw13/BPkoFp3aSctEbur5C8cyFX5hCQVd3VKqFUI3/VGo0n aios"

def setup_ssh_key():
    """配置SSH免密登录"""
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(SERVER_HOST, port=SERVER_PORT, username=SERVER_USER, password=SERVER_PASS, timeout=15)

    # 创建.ssh目录
    client.exec_command('mkdir -p ~/.ssh && chmod 700 ~/.ssh')

    # 添加公钥到authorized_keys
    command = f'echo "{PUBLIC_KEY}" >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys'
    stdin, stdout, stderr = client.exec_command(command)
    error = stderr.read().decode()

    client.close()

    if error and 'exists' not in error:
        print(f"错误: {error}")
        return False
    else:
        print("✅ SSH免密登录配置完成")
        return True

if __name__ == "__main__":
    print("=== 配置SSH免密登录 ===")
    setup_ssh_key()
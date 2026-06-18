#!/usr/bin/env python
# -*- coding: utf-8 -*-
import paramiko

# ── 服务器配置 ──
SERVER_HOST = '8.163.127.182'
SERVER_PORT = 443
SERVER_USER = 'root'
SERVER_PASS = 'glt01054717@'

def check_ssh_config():
    """检查SSH配置"""
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(SERVER_HOST, port=SERVER_PORT, username=SERVER_USER, password=SERVER_PASS, timeout=15)

    # 检查SSH配置
    stdin, stdout, stderr = client.exec_command('cat /etc/ssh/sshd_config | grep -E "PubkeyAuthentication|AuthorizedKeysFile|Port|PermitRootLogin"')
    ssh_config = stdout.read().decode()
    print("=== SSH配置 ===")
    print(ssh_config)

    # 检查.ssh目录权限
    stdin, stdout, stderr = client.exec_command('ls -la ~/.ssh')
    ssh_dir = stdout.read().decode()
    print("=== .ssh目录权限 ===")
    print(ssh_dir)

    # 检查authorized_keys文件权限
    stdin, stdout, stderr = client.exec_command('ls -la ~/.ssh/authorized_keys')
    auth_keys = stdout.read().decode()
    print("=== authorized_keys文件权限 ===")
    print(auth_keys)

    # 检查SSH服务状态
    stdin, stdout, stderr = client.exec_command('systemctl status sshd | head -10')
    sshd_status = stdout.read().decode()
    print("=== SSH服务状态 ===")
    print(sshd_status)

    client.close()

if __name__ == "__main__":
    check_ssh_config()
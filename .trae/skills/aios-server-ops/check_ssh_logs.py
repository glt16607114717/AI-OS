#!/usr/bin/env python
# -*- coding: utf-8 -*-
import paramiko

# ── 服务器配置 ──
SERVER_HOST = '8.163.127.182'
SERVER_PORT = 443
SERVER_USER = 'root'
SERVER_PASS = 'glt01054717@'

def check_ssh_logs():
    """检查SSH日志"""
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(SERVER_HOST, port=SERVER_PORT, username=SERVER_USER, password=SERVER_PASS, timeout=15)

    # 检查最近的SSH日志
    stdin, stdout, stderr = client.exec_command('journalctl -u ssh -n 20 --no-pager')
    ssh_logs = stdout.read().decode()
    print("=== SSH日志 ===")
    print(ssh_logs)

    # 尝试测试SSH密钥连接
    print("\n=== 测试SSH密钥验证 ===")
    stdin, stdout, stderr = client.exec_command('echo "测试"')
    result = stdout.read().decode()
    print(f"密码登录成功: {result}")

    client.close()

if __name__ == "__main__":
    check_ssh_logs()
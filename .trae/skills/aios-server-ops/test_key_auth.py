#!/usr/bin/env python
# -*- coding: utf-8 -*-
import paramiko
import os

# ── 服务器配置 ──
SERVER_HOST = '8.163.127.182'
SERVER_PORT = 443
SERVER_USER = 'root'

# ── 密钥路径 ──
PRIVATE_KEY_PATH = r"C:\Users\guilt\.ssh\aios_ed25519"

def test_key_auth():
    """测试密钥认证"""
    print("=== 测试SSH密钥认证 ===")

    # 检查密钥文件是否存在
    if not os.path.exists(PRIVATE_KEY_PATH):
        print(f"❌ 私钥文件不存在: {PRIVATE_KEY_PATH}")
        return False

    print(f"✅ 私钥文件存在: {PRIVATE_KEY_PATH}")

    # 读取私钥
    try:
        private_key = paramiko.RSAKey.from_private_key_file(PRIVATE_KEY_PATH)
        print("✅ 私钥加载成功")
    except Exception as e:
        print(f"❌ 私钥加载失败: {e}")
        return False

    # 尝试使用密钥连接
    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())

    try:
        client.connect(SERVER_HOST, port=SERVER_PORT, username=SERVER_USER, pkey=private_key, timeout=15)
        print("✅ SSH密钥认证成功")

        # 测试执行命令
        stdin, stdout, stderr = client.exec_command('echo "免密登录测试成功"')
        result = stdout.read().decode()
        print(f"✅ 命令执行结果: {result}")

        client.close()
        return True

    except Exception as e:
        print(f"❌ SSH密钥认证失败: {e}")
        return False

if __name__ == "__main__":
    test_key_auth()
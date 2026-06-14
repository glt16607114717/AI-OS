#!/usr/bin/env python3
"""AIOS Go 后端接口测试脚本"""

import json
import sys
import os
import requests

BASE_URL = "http://124.221.220.89:18731"
DEFAULT_USERNAME = "桂良涛"
DEFAULT_PASSWORD = "admin123"


def read_input():
    """从文件或stdin读取输入"""
    for arg in sys.argv[1:]:
        if os.path.isfile(arg):
            with open(arg, "r", encoding="utf-8-sig") as f:
                return f.read().strip()
    if not sys.stdin.isatty():
        data = sys.stdin.read().strip()
        if data:
            return data
    return ""


def login(username, password):
    """登录获取 JWT Token"""
    login_url = f"{BASE_URL}/api/login"
    print(f"====== 1. 登录 ======")
    print(f"POST {login_url}")
    print(f"账号: {username}")

    try:
        resp = requests.post(
            login_url,
            json={"username": username, "password": password},
            timeout=15,
        )
    except Exception as e:
        print(f"[错误] 连接失败: {e}")
        sys.exit(1)

    print(f"状态码: {resp.status_code}")

    try:
        result = resp.json()
    except Exception:
        print(f"[错误] 返回非JSON: {resp.text[:500]}")
        sys.exit(1)

    if not result.get("ok"):
        print(f"[错误] 登录失败: {json.dumps(result, ensure_ascii=False)}")
        sys.exit(1)

    token = result.get("data", {}).get("token", "")
    if not token:
        print(f"[错误] 未获取到 token: {json.dumps(result, ensure_ascii=False)}")
        sys.exit(1)

    print(f"登录成功, token: {token[:20]}...")
    return token


def main():
    raw = read_input()
    if not raw:
        print("AIOS 接口测试工具")
        print("")
        print("用法: python aios_api_test.py <params.json>")
        print("")
        print("JSON 参数:")
        print('  {"url": "/api/users", "method": "GET", "params": {}}')
        print('  {"url": "/api/users/create", "method": "POST", "params": {"username":"x","password":"y"}}')
        print('  {"url": "/api/health", "no_auth": true}')
        sys.exit(1)

    try:
        p = json.loads(raw)
    except json.JSONDecodeError as e:
        print(f"[错误] JSON 解析失败: {e}")
        sys.exit(1)

    url = p.get("url", "")
    method = p.get("method", "GET").upper()
    params = p.get("params", {})
    username = p.get("username", DEFAULT_USERNAME)
    password = p.get("password", DEFAULT_PASSWORD)
    no_auth = p.get("no_auth", False)

    if not url:
        print("[错误] 必须提供 url 参数")
        sys.exit(1)

    # 登录获取 Token（除非公开接口）
    headers = {}
    if not no_auth:
        token = login(username, password)
        headers["Authorization"] = token

    # 请求目标接口
    full_url = BASE_URL + url
    print(f"\n====== 2. 请求接口 ======")
    print(f"{method} {full_url}")

    if method == "GET":
        print(f"Query: {json.dumps(params, ensure_ascii=False)}")
        resp = requests.get(full_url, params=params, headers=headers, timeout=30)
    else:
        print(f"Body: {json.dumps(params, ensure_ascii=False)}")
        headers["Content-Type"] = "application/json"
        resp = requests.post(full_url, json=params, headers=headers, timeout=30)

    print(f"\n====== 3. 响应结果 ======")
    print(f"状态码: {resp.status_code}")
    try:
        result = resp.json()
        print(json.dumps(result, ensure_ascii=False, indent=2))
    except Exception:
        print(resp.text[:2000])


if __name__ == "__main__":
    main()

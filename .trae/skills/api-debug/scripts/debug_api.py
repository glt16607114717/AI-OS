import json
import sys
import os
import re
import requests


def read_input() -> str:
    for arg in sys.argv[1:]:
        if os.path.isfile(arg):
            with open(arg, "r", encoding="utf-8-sig") as f:
                return f.read().strip()
    return ""


def main():
    raw = read_input()
    if not raw:
        print("usage: python debug_api.py <params.json>")
        print("")
        print("JSON params:")
        print('  {"base_url": "http://rmp-api-a.me/admin.php",')
        print('   "account": "ca-admin", "password": "123456",')
        print('   "url": "/api/xxx", "method": "POST", "params": {...}}')
        print('   "files": {"file": "C:/path/to/file.xlsx"}}')
        sys.exit(1)

    try:
        p = json.loads(raw)
    except json.JSONDecodeError as e:
        print(f"JSON parse error: {e}")
        sys.exit(1)

    base_url = p.get("base_url", "")
    login_url = p.get("login_url", base_url + "/api/admin/login")
    account = p.get("account", "")
    password = p.get("password", "")
    url = p.get("url", "")
    method = p.get("method", "GET").upper()
    params = p.get("params", {})
    files_config = p.get("files", {})

    if not all([base_url, account, password, url]):
        print("缺少必要参数: base_url, account, password, url")
        sys.exit(1)

    print("====== 1. 登录获取access_token ======")
    print(f"登录请求URL: {login_url}")

    login_data = {
        "account": account,
        "password": password,
        "identity_type": 0,
        "is_pc_login": 1,
        "RSApassword": 0,
        "fingerprint": "debug_fingerprint_123",
    }

    try:
        login_resp = requests.post(
            login_url,
            json=login_data,
            headers={"Content-Type": "application/json", "x-request-source": "pc"},
            verify=False,
            timeout=30,
        )
    except Exception as e:
        print(f"登录请求异常: {e}")
        sys.exit(1)

    print(f"登录返回状态码: {login_resp.status_code}")

    try:
        login_result = login_resp.json()
    except Exception:
        print(f"登录返回非JSON: {login_resp.text[:500]}")
        sys.exit(1)

    if not login_result or "data" not in login_result or "access_token" not in login_result.get("data", {}):
        print(f"登录失败: {json.dumps(login_result, ensure_ascii=False)}")
        sys.exit(1)

    access_token = login_result["data"]["access_token"]
    print(f"登录成功，access_token: {access_token}\n")

    print("====== 2. 请求目标接口 ======")
    full_url = base_url + url
    print(f"请求URL: {full_url}")
    print(f"请求方式: {method}")

    headers = {
        "Authorization": access_token,
        "x-request-source": "pc",
    }

    has_files = bool(files_config)

    if has_files:
        print(f"文件上传: {json.dumps(files_config, ensure_ascii=False)}")
        if params:
            print(f"附加参数: {json.dumps(params, ensure_ascii=False)}")
        print()
        opened_files = []
        files_multi = {}
        try:
            for field_name, file_path in files_config.items():
                file_path = os.path.abspath(file_path)
                if not os.path.isfile(file_path):
                    print(f"文件不存在: {file_path}")
                    sys.exit(1)
                f = open(file_path, "rb")
                opened_files.append(f)
                filename = os.path.basename(file_path)
                files_multi[field_name] = (filename, f)

            form_data = {k: str(v) for k, v in params.items()} if params else None
            resp = requests.post(
                full_url,
                data=form_data,
                files=files_multi,
                headers=headers,
                verify=False,
                timeout=60,
            )
        finally:
            for f in opened_files:
                f.close()
    else:
        print(f"请求参数: {json.dumps(params, ensure_ascii=False)}\n")
        headers["Content-Type"] = "application/json"
        try:
            if method == "GET":
                resp = requests.get(full_url, params=params, headers=headers, verify=False, timeout=30)
            elif method == "PUT":
                resp = requests.put(full_url, json=params, headers=headers, verify=False, timeout=30)
            elif method == "DELETE":
                resp = requests.delete(full_url, json=params, headers=headers, verify=False, timeout=30)
            else:
                resp = requests.post(full_url, json=params, headers=headers, verify=False, timeout=30)
        except Exception as e:
            print(f"请求异常: {e}")
            sys.exit(1)

    print("====== 3. 响应结果 ======")

    content_type = resp.headers.get("Content-Type", "")
    if "html" in content_type or resp.text.strip().startswith("<"):
        cleaned = re.sub(r"<(style|script)[^>]*>.*?</\1>", "", resp.text, flags=re.DOTALL | re.IGNORECASE)
        cleaned = re.sub(r"\s+", " ", re.sub(r"<[^>]+>", "", cleaned)).strip()
        if len(cleaned) > 300:
            cleaned = cleaned[:300] + "..."
        print(f"[接口错误] {cleaned}")
    else:
        print(resp.text)


if __name__ == "__main__":
    main()

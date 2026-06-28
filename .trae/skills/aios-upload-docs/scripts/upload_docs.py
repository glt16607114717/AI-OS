#!/usr/bin/env python3
"""批量上传本地文档到 AI-OS 知识库

用法：
    python upload_docs.py --dir <目录路径> [--exclude README.md,xxx.md]
"""
import requests, os, sys, time, argparse

# === 配置（不要改）===
API_BASE = "http://8.163.127.182:18731"
USERNAME = "桂良涛"
PASSWORD = "glt01054717"
DEFAULT_EXCLUDE = {"README.md"}
SUPPORTED_EXT = {
    ".md", ".txt", ".pdf", ".docx", ".xlsx",
    ".go", ".py", ".js", ".ts", ".vue", ".java", ".c", ".cpp", ".h", ".rs",
    ".sql", ".yaml", ".yml", ".json", ".xml", ".html", ".css",
    ".sh", ".bat", ".ini", ".cfg", ".toml",
}


def login():
    r = requests.post(
        f"{API_BASE}/api/login",
        json={"username": USERNAME, "password": PASSWORD},
        timeout=10,
    )
    data = r.json()
    if not data.get("ok"):
        print(f"登录失败: {data.get('error', '')}")
        sys.exit(1)
    token = data["data"]["token"]
    print(f"登录成功, token: {token[:16]}...")
    return token


def upload_file(token, filepath, timeout=600):
    filename = os.path.basename(filepath)
    with open(filepath, "rb") as f:
        files = {"file": (filename, f)}
        r = requests.post(
            f"{API_BASE}/api/rag/upload",
            headers={"Authorization": f"Bearer {token}"},
            files=files,
            timeout=timeout,
        )
    return r.json()


def collect_files(directory, exclude_set):
    files = []
    for root, dirs, fnames in os.walk(directory):
        for fname in sorted(fnames):
            ext = os.path.splitext(fname)[1].lower()
            if ext not in SUPPORTED_EXT:
                continue
            if fname in exclude_set:
                continue
            fpath = os.path.join(root, fname)
            files.append((fpath, os.path.relpath(fpath, directory)))
    return files


def main():
    parser = argparse.ArgumentParser(description="批量上传文档到 AI-OS 知识库")
    parser.add_argument("--dir", required=True, help="要上传的目录路径")
    parser.add_argument("--exclude", default="", help="排除的文件名（逗号分隔）")
    args = parser.parse_args()

    directory = args.dir
    if not os.path.isdir(directory):
        print(f"目录不存在: {directory}")
        sys.exit(1)

    exclude_set = set(DEFAULT_EXCLUDE)
    if args.exclude:
        exclude_set.update(x.strip() for x in args.exclude.split(","))

    token = login()
    files = collect_files(directory, exclude_set)
    print(f"共发现 {len(files)} 个文档（排除了 {', '.join(sorted(exclude_set))}）")

    success = fail = 0
    failed_files = []
    for fpath, relpath in files:
        try:
            data = upload_file(token, fpath)
            if data.get("ok"):
                info = data["data"]
                success += 1
                print(f"  [OK] {relpath} → {info['chunks']}个分块 ({info['size']}字符)")
            else:
                fail += 1
                failed_files.append(relpath)
                print(f"  [NG] {relpath}: {data.get('error', '')}")
        except Exception as e:
            fail += 1
            failed_files.append(relpath)
            print(f"  [NG] {relpath}: {e}")
        time.sleep(0.5)

    print(f"\n完成: 成功={success}, 失败={fail}")
    if failed_files:
        print(f"失败文件:")
        for f in failed_files:
            print(f"  {f}")


if __name__ == "__main__":
    main()

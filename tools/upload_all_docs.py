"""批量上传知识库文档到 RAG"""
import requests
import glob
import os
import sys

BASE_URL = "http://8.163.127.182:18731"

# 先登录
print("🔐 登录中...")
resp = requests.post(f"{BASE_URL}/api/login", json={
    "username": "桂良涛",
    "password": "glt01054717"
})
if resp.status_code != 200:
    print(f"❌ 登录失败: {resp.status_code} {resp.text}")
    sys.exit(1)

data = resp.json()
token = data.get("data", {}).get("token", "")
if not token:
    print(f"❌ 未获取到 token: {data}")
    sys.exit(1)
print(f"✅ 登录成功")

# 找到所有文档
doc_dir = r"d:\wwwroot\ai-os"
files = glob.glob(os.path.join(doc_dir, "知识库-*.md")) + glob.glob(os.path.join(doc_dir, "操作手册-*.md"))
print(f"\n📄 共找到 {len(files)} 个文档\n")

headers = {"Authorization": f"Bearer {token}"}
success = 0
fail = 0

for f in sorted(files):
    fname = os.path.basename(f)
    fsize = os.path.getsize(f)
    print(f"  ⏳ {fname} ({fsize/1024:.0f}KB)...", end=" ")
    
    try:
        with open(f, "rb") as fh:
            resp = requests.post(
                f"{BASE_URL}/api/rag/upload",
                files={"file": (fname, fh, "text/markdown")},
                headers=headers,
                timeout=120
            )
        if resp.status_code == 200:
            d = resp.json()
            chunks = d.get("data", {}).get("chunks", 0)
            print(f"✅ {chunks} 块")
            success += 1
        else:
            print(f"❌ {resp.status_code} {resp.text[:100]}")
            fail += 1
    except Exception as e:
        print(f"❌ {e}")
        fail += 1

print(f"\n{'='*50}")
print(f"完成: 成功 {success}, 失败 {fail}")

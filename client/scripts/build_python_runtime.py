import urllib.request
import os
import zipfile
import shutil
import subprocess
import sys

BASE_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
REQUIREMENTS = os.path.join(BASE_DIR, "backend", "python", "requirements.txt")
DEST_ZIP = os.path.join(BASE_DIR, "resources", "python-runtime.zip")

EMBED_URL = "https://npmmirror.com/mirrors/python/3.12.10/python-3.12.10-embed-amd64.zip"
NUGET_URL = "https://registry.npmmirror.com/-/binary/python/3.12.10/python-3.12.10-amd64.zip"
PIP_INDEX = "https://mirrors.aliyun.com/pypi/simple"

CACHE_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), ".cache")
os.makedirs(CACHE_DIR, exist_ok=True)

tmp = os.path.join(os.environ["TEMP"], "ai-os-python-build")
if os.path.exists(tmp):
    shutil.rmtree(tmp, ignore_errors=True)
os.makedirs(tmp, exist_ok=True)


def cached_download(url, filename):
    cache_path = os.path.join(CACHE_DIR, filename)
    if os.path.exists(cache_path):
        size_mb = os.path.getsize(cache_path) / 1024 / 1024
        print(f"  缓存命中: {filename} ({size_mb:.1f} MB)")
        return cache_path
    print(f"  下载: {url}")
    try:
        urllib.request.urlretrieve(url, cache_path)
        size_mb = os.path.getsize(cache_path) / 1024 / 1024
        print(f"  下载完成: {filename} ({size_mb:.1f} MB)")
        return cache_path
    except Exception as e:
        print(f"  下载失败: {e}")
        print(f"  请手动下载: {url}")
        print(f"  保存到: {cache_path}")
        sys.exit(1)


def step(n, total, msg):
    print(f"\n=== [{n}/{total}] {msg} ===")


step(1, 9, "下载 Python 3.12.10 embed 包")
embed_path = cached_download(EMBED_URL, "python-3.12.10-embed-amd64.zip")

step(2, 9, "下载 Python 3.12.10 nuget 完整包（含标准库）")
nuget_path = cached_download(NUGET_URL, "python-3.12.10-amd64.zip")

step(3, 9, "解压并合并")
python_dir = os.path.join(tmp, "python")
nuget_dir = os.path.join(tmp, "nuget")

print("  解压 embed 包...")
with zipfile.ZipFile(embed_path) as z:
    z.extractall(python_dir)

print("  解压 nuget 包...")
with zipfile.ZipFile(nuget_path) as z:
    z.extractall(nuget_dir)

lib_src = os.path.join(nuget_dir, "Lib")
lib_dst = os.path.join(python_dir, "Lib")
os.makedirs(lib_dst, exist_ok=True)

print("  合并标准库...")
for item in os.listdir(lib_src):
    s, d = os.path.join(lib_src, item), os.path.join(lib_dst, item)
    if os.path.isdir(s):
        if os.path.exists(d):
            shutil.rmtree(d)
        shutil.copytree(s, d)
    else:
        shutil.copy2(s, d)

print("  合并完成")

step(4, 9, "写入 _pth 配置")
pth_path = os.path.join(python_dir, "python312._pth")
with open(pth_path, "w") as f:
    f.write("Lib\nLib\\site-packages\n.\nimport site\n")
print(f"  写入: {pth_path}")

step(5, 9, "创建 site-packages 目录")
sp_dir = os.path.join(lib_dst, "site-packages")
os.makedirs(sp_dir, exist_ok=True)
print(f"  创建: {sp_dir}")

py = os.path.join(python_dir, "python.exe")
ver = subprocess.check_output([py, "--version"]).decode().strip()
print(f"  Python 版本: {ver}")

step(6, 9, "安装 pip")
subprocess.run(
    [py, os.path.join(nuget_dir, "Lib", "ensurepip", "__main__.py")],
    check=True,
)
subprocess.run(
    [py, "-m", "pip", "install", "-i", PIP_INDEX, "setuptools", "--no-warn-script-location"],
    check=True,
)
print("  pip 安装完成")

step(7, 9, "安装 requirements.txt 依赖")
subprocess.run(
    [py, "-m", "pip", "install", "-i", PIP_INDEX, "-r", REQUIREMENTS, "--no-warn-script-location"],
    check=True,
)
print("  依赖安装完成")

step(8, 9, "清理无用文件")
clean_dirs = [
    "Lib\\test",
    "Lib\\unittest",
    "Lib\\tkinter",
    "Lib\\turtledemo",
    "Lib\\idlelib",
    "Lib\\pydoc_data",
]
for pattern in clean_dirs:
    p = os.path.join(python_dir, pattern)
    if os.path.exists(p):
        shutil.rmtree(p, ignore_errors=True)
        print(f"  清理: {pattern}")

for root, dirs, files in os.walk(python_dir, topdown=False):
    for d in dirs:
        if d == "__pycache__":
            shutil.rmtree(os.path.join(root, d), ignore_errors=True)

total = sum(
    os.path.getsize(os.path.join(dp, f))
    for dp, dn, fn in os.walk(python_dir)
    for f in fn
)
print(f"  清理后目录总大小: {total / 1024 / 1024:.1f} MB")

step(9, 9, "打包 python-runtime.zip")
if os.path.exists(DEST_ZIP):
    os.remove(DEST_ZIP)

file_count = 0
with zipfile.ZipFile(DEST_ZIP, "w", zipfile.ZIP_DEFLATED) as zf:
    for root, dirs, files in os.walk(python_dir):
        for f in files:
            fp = os.path.join(root, f)
            zf.write(fp, os.path.relpath(fp, python_dir))
            file_count += 1

zip_size = os.path.getsize(DEST_ZIP) / 1024 / 1024
print(f"  文件数: {file_count}")
print(f"\n输出: {DEST_ZIP}")
print(f"大小: {zip_size:.1f} MB")
print("=== 完成! ===")

#!/usr/bin/env python3
"""AIOS Go 后端打包部署脚本"""

import subprocess
import sys
import os
import time
import shutil

# 配置
GO_SOURCE = r"d:\wwwroot\ai-os\go-backend"
BINARY_NAME = "aios-server"
SERVER_USER = "root"
SERVER_HOST = "8.163.127.182"
SERVER_PATH = f"/opt/ai-os"
SERVICE_NAME = "aios-server"
HEALTH_URL = f"http://{SERVER_HOST}:18731/api/health"


def run(cmd, cwd=None, check=True, shell=False):
    """执行命令并打印输出"""
    print(f"[执行] {cmd}")
    result = subprocess.run(
        cmd, cwd=cwd, shell=shell, capture_output=True, text=True
    )
    if result.stdout.strip():
        print(result.stdout.strip())
    if result.stderr.strip():
        print(f"[stderr] {result.stderr.strip()}")
    if check and result.returncode != 0:
        print(f"[错误] 命令执行失败，返回码: {result.returncode}")
        sys.exit(1)
    return result


# SSH 公共参数：重试 + 保活 + 指定端口 443，对抗丢包
SSH_PORT = "443"
SSH_OPTS = [
    "-p", SSH_PORT,
    "-o", "ConnectTimeout=30",
    "-o", "ServerAliveInterval=10",
    "-o", "ServerAliveCountMax=6",
    "-o", "ConnectionAttempts=5",
]
# SCP 用大写 -P 指定端口
SCP_OPTS = [
    "-P", SSH_PORT,
    "-o", "ConnectTimeout=30",
    "-o", "ServerAliveInterval=10",
    "-o", "ServerAliveCountMax=6",
    "-o", "ConnectionAttempts=5",
]


def ssh_run(cmd, check=True):
    """在远程服务器执行命令（带重试保活）"""
    return run(
        ["ssh"] + SSH_OPTS + [f"{SERVER_USER}@{SERVER_HOST}", cmd],
        check=check,
    )


def scp_upload(local, remote, max_retries=5):
    """SCP 上传文件（自动重试，对抗丢包）"""
    for attempt in range(1, max_retries + 1):
        result = run(
            ["scp"] + SCP_OPTS + [local, f"{SERVER_USER}@{SERVER_HOST}:{remote}"],
            check=False,
        )
        if result.returncode == 0:
            if attempt > 1:
                print(f"[成功] 上传完成（第{attempt}次重试成功）")
            return result
        if attempt < max_retries:
            wait = attempt * 3
            print(f"[重试] 上传失败，{wait}秒后第{attempt+1}次尝试（共{max_retries}次）...")
            time.sleep(wait)
    print(f"[错误] 上传失败，已重试{max_retries}次")
    sys.exit(1)


def build():
    """步骤1: 本地交叉编译"""
    print("\n========== 步骤1: Go 交叉编译 ==========")

    # go mod tidy
    print("[信息] 执行 go mod tidy...")
    run(["go", "mod", "tidy"], cwd=GO_SOURCE, check=False)

    # 编译
    binary_path = os.path.join(GO_SOURCE, BINARY_NAME)
    if os.path.exists(binary_path):
        os.remove(binary_path)

    env = os.environ.copy()
    env["GOOS"] = "linux"
    env["GOARCH"] = "amd64"
    env["CGO_ENABLED"] = "0"

    print(f"[信息] 编译为 linux/amd64: {binary_path}")
    result = subprocess.run(
        ["go", "build", "-o", BINARY_NAME, "."],
        cwd=GO_SOURCE,
        env=env,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print(f"[错误] 编译失败:\n{result.stderr}")
        sys.exit(1)

    size_mb = os.path.getsize(binary_path) / (1024 * 1024)
    print(f"[成功] 编译完成: {binary_path} ({size_mb:.1f} MB)")
    return binary_path


def deploy():
    """完整部署: 编译 + 上传 + 重启（SSH 连接优化：6次→2次）"""
    binary_path = build()

    # 步骤2: SSH 复用（mkdir + 备份 + 停止，一条命令搞定）
    print("\n========== 步骤2: 准备远程环境 ==========")
    prep_cmd = (
        f"mkdir -p {SERVER_PATH} && "
        f"if [ -f {SERVER_PATH}/{BINARY_NAME} ]; then cp {SERVER_PATH}/{BINARY_NAME} {SERVER_PATH}/{BINARY_NAME}.bak; fi && "
        f"sudo systemctl stop {SERVICE_NAME} 2>/dev/null; echo PREP_DONE"
    )
    ssh_run(prep_cmd, check=False)

    # 步骤3: SCP 上传
    print("[信息] 上传新二进制...")
    scp_upload(binary_path, f"{SERVER_PATH}/{BINARY_NAME}")

    # 步骤4: SSH 复用（chmod + restart，一条命令搞定）
    print("\n========== 步骤3: 启动服务 ==========")
    start_cmd = (
        f"chmod +x {SERVER_PATH}/{BINARY_NAME} && "
        f"sudo systemctl restart {SERVICE_NAME} && echo RESTART_OK"
    )
    ssh_run(start_cmd)

    # 健康检查
    print("\n========== 健康检查 ==========")
    health_check()


def restart_service():
    """重启远程服务"""
    print("[信息] 重启 systemd 服务...")
    ssh_run(f"sudo systemctl restart {SERVICE_NAME}")
    print("[成功] 服务已重启")


def status():
    """查看服务状态"""
    print("\n========== 服务状态 ==========")
    ssh_run(f"sudo systemctl status {SERVICE_NAME} --no-pager -l", check=False)


def logs():
    """查看服务日志"""
    print("\n========== 最近日志 ==========")
    ssh_run(f"sudo journalctl -u {SERVICE_NAME} -n 50 --no-pager", check=False)


def health_check():
    """健康检查"""
    print("[信息] 等待服务启动...")
    time.sleep(3)

    for i in range(5):
        try:
            import urllib.request

            req = urllib.request.Request(f"{HEALTH_URL}", method="GET")
            resp = urllib.request.urlopen(req, timeout=5)
            if resp.status == 200:
                print(f"[成功] 健康检查通过 ✓ (尝试 {i+1}/5)")
                return True
        except Exception:
            print(f"[等待] 服务尚未就绪... ({i+1}/5)")
            time.sleep(2)

    print("[警告] 健康检查失败！服务可能未正常启动")
    print("[信息] 正在回滚到上一个版本...")
    rollback()
    return False


def rollback():
    """回滚到上一个版本"""
    print("[信息] 回滚到 .bak 版本...")
    ssh_run(f"sudo systemctl stop {SERVICE_NAME}", check=False)
    ssh_run(
        f"if [ -f {SERVER_PATH}/{BINARY_NAME}.bak ]; then cp {SERVER_PATH}/{BINARY_NAME}.bak {SERVER_PATH}/{BINARY_NAME}; fi",
        check=False,
    )
    ssh_run(f"sudo systemctl start {SERVICE_NAME}", check=False)
    print("[信息] 回滚完成，请检查服务状态")


def main():
    action = sys.argv[1] if len(sys.argv) > 1 else "deploy"

    if action == "deploy":
        deploy()
    elif action == "build":
        build()
    elif action == "restart":
        restart_service()
    elif action == "status":
        status()
    elif action == "logs":
        logs()
    else:
        print("AIOS Go 后端部署工具")
        print("")
        print("用法:")
        print("  python aios_deploy.py deploy   - 完整部署（编译+上传+重启）")
        print("  python aios_deploy.py build    - 仅编译")
        print("  python aios_deploy.py restart  - 仅重启服务")
        print("  python aios_deploy.py status   - 查看服务状态")
        print("  python aios_deploy.py logs     - 查看服务日志")


if __name__ == "__main__":
    main()

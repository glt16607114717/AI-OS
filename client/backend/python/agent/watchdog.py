"""
AI-OS 看门狗脚本
检查 main.py 进程是否在运行，如果不在就启动它。
由计划任务 AI-OS-Watchdog 每 2 分钟调用一次。
"""
import subprocess
import os
import sys

NO_WINDOW = 0x08000000  # CREATE_NO_WINDOW

def find_python_dir():
    """查找 Python 运行时目录"""
    # 优先从环境变量获取
    python_dir = os.environ.get("AIOS_PYTHON_DIR", "")
    if python_dir and os.path.exists(os.path.join(python_dir, "pythonw.exe")):
        return python_dir

    # 从脚本位置推断
    script_dir = os.path.dirname(os.path.abspath(__file__))
    # watchdog.py 在 backend/python/agent/ 下，往上 3 级到 app 根目录
    app_root = os.path.normpath(os.path.join(script_dir, "..", "..", ".."))
    candidates = [
        os.path.join(app_root, "runtime", "python"),
        r"C:\ProgramData\AI-OS\runtime\python",
    ]
    for c in candidates:
        if os.path.exists(os.path.join(c, "pythonw.exe")):
            return c

    return ""

def is_agent_running():
    """检查 main.py 进程是否在运行（排除 watchdog 自己）"""
    try:
        # 用 WMIC 检查 pythonw.exe 的命令行参数是否包含 main.py
        result = subprocess.run(
            ["wmic", "process", "where", "name='pythonw.exe'", "get",
             "CommandLine", "/VALUE"],
            capture_output=True, timeout=10,
            creationflags=NO_WINDOW,
            stdin=subprocess.DEVNULL,
        )
        output = result.stdout.decode("latin-1", errors="replace")
        # 检查是否有命令行包含 main.py 的 pythonw 进程
        for line in output.split("\n"):
            line = line.strip()
            if "main.py" in line.lower():
                return True
        return False
    except Exception:
        return False

def start_agent(python_dir, main_script):
    """启动 main.py"""
    pythonw_exe = os.path.join(python_dir, "pythonw.exe")
    subprocess.Popen(
        [pythonw_exe, "-X", "utf8", main_script],
        cwd=python_dir,
        creationflags=NO_WINDOW | 0x00000008,  # CREATE_NO_WINDOW | DETACHED_PROCESS
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

def main():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    main_script = os.path.join(script_dir, "main.py")
    python_dir = find_python_dir()

    # 写日志到 C:\ProgramData\AI-OS\logs\watchdog.log
    log_dir = r"C:\ProgramData\AI-OS\logs"
    log_file = os.path.join(log_dir, "watchdog.log")
    try:
        os.makedirs(log_dir, exist_ok=True)
        with open(log_file, "a", encoding="utf-8") as f:
            from datetime import datetime
            ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            f.write(f"[{ts}] python_dir={python_dir}\n")
    except Exception:
        pass

    if not python_dir:
        try:
            with open(log_file, "a", encoding="utf-8") as f:
                f.write(f"  ERROR: python_dir not found\n")
        except Exception:
            pass
        return

    running = is_agent_running()
    try:
        with open(log_file, "a", encoding="utf-8") as f:
            f.write(f"  running={running}\n")
    except Exception:
        pass

    if not running:
        start_agent(python_dir, main_script)
        try:
            with open(log_file, "a", encoding="utf-8") as f:
                f.write(f"  started agent\n")
        except Exception:
            pass

if __name__ == "__main__":
    main()

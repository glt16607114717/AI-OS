import os
import sys
import time
import signal
import logging
import traceback
import threading
import json
import urllib.request
import urllib.error
from datetime import datetime
from pathlib import Path

# Ensure voice module can be imported
BACKEND_DIR = Path(__file__).resolve().parent
if str(BACKEND_DIR) not in sys.path:
    sys.path.insert(0, str(BACKEND_DIR))
VOICE_DIR = BACKEND_DIR.parent
if str(VOICE_DIR) not in sys.path:
    sys.path.insert(0, str(VOICE_DIR))

import uvicorn
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel
from typing import Optional, Any

START_TIME = time.time()
VERSION = "0.1.0"
SHUTDOWN_DELAY = 5
VOICE_WORKER_PORT = 18732
VOICE_WORKER_URL = f"http://127.0.0.1:{VOICE_WORKER_PORT}"
WATCHDOG_INTERVAL = 10  # 秒

# Debug mode: True = show full error info, False = show generic error
DEBUG = os.environ.get("AI_OS_DEBUG", "true").lower() == "true"

# Voice worker 进程状态
_worker_pid = None  # 当前 voice_worker 的 PID（由 CreateProcessAsUser 返回）
_watchdog_stop = threading.Event()


def load_build_info():
    build_info_path = os.path.join(os.path.dirname(os.path.dirname(__file__)), 'build_info.json')
    with open(build_info_path, 'r', encoding='utf-8') as f:
        return json.load(f)


BUILD_INFO = load_build_info()

if sys.stdout is None:
    sys.stdout = open(os.devnull, 'w', encoding='utf-8')
if sys.stderr is None:
    sys.stderr = open(os.devnull, 'w', encoding='utf-8')

logging.basicConfig(
    level=logging.DEBUG if DEBUG else logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    handlers=[logging.StreamHandler(sys.stdout)],
)
logger = logging.getLogger("ai-os-agent")

app = FastAPI(title="AI-OS Agent", version=VERSION)


# --- Global Exception Handler ---

@app.exception_handler(Exception)
async def global_exception_handler(request: Request, exc: Exception):
    logger.error(f"Unhandled exception on {request.method} {request.url.path}: {exc}")
    logger.debug(traceback.format_exc())
    if DEBUG:
        return JSONResponse(
            status_code=500,
            content={"ok": False, "error": str(exc), "traceback": traceback.format_exc()},
        )
    return JSONResponse(
        status_code=500,
        content={"ok": False, "error": "Internal Server Error"},
    )


# --- Routes ---

@app.get("/health")
async def health():
    return {
        "ok": True,
        "uptime": int(time.time() - START_TIME),
        "version": VERSION,
        "pid": os.getpid(),
        "started_at": datetime.fromtimestamp(START_TIME).isoformat(),
        "build_time": BUILD_INFO.get("build_time", "unknown"),
        "voice_worker_pid": _worker_pid,
    }


@app.post("/shutdown")
async def shutdown():
    logger.info("Received shutdown request, shutting down gracefully in {} seconds...".format(SHUTDOWN_DELAY))

    def delayed_exit():
        time.sleep(SHUTDOWN_DELAY)
        logger.info("Executing shutdown...")
        sys.exit(0)

    thread = threading.Thread(target=delayed_exit, daemon=True)
    thread.start()

    return {
        "ok": True,
        "message": "Graceful shutdown initiated",
        "delay_seconds": SHUTDOWN_DELAY
    }


# ── Voice Worker 管理 ────────────────────────────────────

def _find_python_executable() -> str:
    """
    查找 voice_worker 应使用的 Python 可执行文件。
    优先使用环境变量 AI_OS_PYTHON 指定的路径，
    其次用当前解释器（sys.executable）。
    生产环境用 pythonw.exe（无控制台窗口），开发调试用 python.exe。
    """
    env_python = os.environ.get("AI_OS_PYTHON")
    if env_python and os.path.isfile(env_python):
        return env_python

    exe = sys.executable
    # 生产环境：用 pythonw.exe 避免弹黑框
    # 调试时：用 python.exe 以便看到错误输出
    exe_dir = os.path.dirname(exe)
    exe_name = os.path.basename(exe)
    if exe_name.lower() == "python.exe":
        pythonw = os.path.join(exe_dir, "pythonw.exe")
        if os.path.isfile(pythonw) and not DEBUG:
            return pythonw
    return exe


def _spawn_voice_worker() -> int | None:
    """在用户 Session 1 中创建 voice_worker 子进程，返回 PID。"""
    from session_spawn import spawn_in_user_session, is_user_session_active

    if not is_user_session_active():
        logger.debug("用户桌面未活跃（无 explorer.exe），跳过 spawn")
        return None

    python_exe = _find_python_executable()
    worker_script = str(Path(__file__).resolve().parent / "voice_worker.py")
    cmd_line = f'"{python_exe}" "{worker_script}"'
    working_dir = str(Path(__file__).resolve().parent)

    pid = spawn_in_user_session(cmd_line, working_dir=working_dir)
    if pid:
        logger.info(f"Voice Worker 已在用户会话启动, PID={pid}")
    return pid


def _check_worker_alive() -> bool:
    """检查 voice_worker 进程是否存活（通过 HTTP 健康检查）。"""
    try:
        req = urllib.request.Request(f"{VOICE_WORKER_URL}/health", method="GET")
        with urllib.request.urlopen(req, timeout=3) as resp:
            data = json.loads(resp.read())
            return data.get("ok", False)
    except Exception:
        return False


def _watchdog_loop() -> None:
    """
    Watchdog 主循环：每 10 秒检查 voice_worker 存活状态。
    - 不存活 → 检查用户桌面是否活跃 → 是则重新 spawn
    - 用户未登录 → 跳过，等下次检查
    """
    global _worker_pid

    # 首次 spawn 前等 2 秒，让主服务自身初始化完成
    time.sleep(2)

    # 首次 spawn
    if not _check_worker_alive():
        _worker_pid = _spawn_voice_worker()

    while not _watchdog_stop.is_set():
        _watchdog_stop.wait(WATCHDOG_INTERVAL)
        if _watchdog_stop.is_set():
            break

        if _check_worker_alive():
            continue

        # Worker 挂了，尝试重新 spawn
        logger.warning("Voice Worker 无响应，尝试重新启动...")
        _worker_pid = _spawn_voice_worker()


@app.on_event("startup")
async def on_startup():
    global _worker_pid
    build_time = BUILD_INFO.get("build_time", "unknown")
    logger.info(f"AI-OS Agent v{VERSION} started (pid={os.getpid()}, build_time={build_time})")
    logger.info(f"Debug mode: {DEBUG}")

    # 启动 watchdog 线程（负责 spawn + 保活 voice_worker）
    watchdog_thread = threading.Thread(target=_watchdog_loop, daemon=True, name="VoiceWorkerWatchdog")
    watchdog_thread.start()
    logger.info("Voice Worker watchdog 已启动")


# ── Voice API（转发到 voice_worker）──────────────────────

class VoiceActionRequest(BaseModel):
    action: str
    payload: dict = {}


def _forward_to_worker(path: str, method: str = "POST", data: dict | None = None) -> dict:
    """将请求转发到 voice_worker（Session 1 进程）。"""
    url = f"{VOICE_WORKER_URL}{path}"
    body = None
    if data is not None:
        body = json.dumps(data).encode("utf-8")

    req = urllib.request.Request(
        url,
        data=body,
        method=method,
        headers={"Content-Type": "application/json"} if body else {},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return json.loads(resp.read())
    except urllib.error.URLError:
        return {"ok": False, "error": "Voice Worker 未响应，可能正在启动中"}
    except Exception as e:
        return {"ok": False, "error": f"转发请求到 Voice Worker 失败: {e}"}


@app.post("/api/voice")
async def voice_api(req: VoiceActionRequest):
    """转发所有 voice 请求到 voice_worker。"""
    return _forward_to_worker("/api/voice", data={"action": req.action, "payload": req.payload})


@app.post("/api/voice/download-model")
async def voice_download_model():
    """转发模型下载请求到 voice_worker。"""
    return _forward_to_worker("/api/voice/download-model", method="POST")


@app.get("/api/voice/model-status")
async def voice_model_status():
    """转发模型状态查询到 voice_worker。"""
    return _forward_to_worker("/api/voice/model-status", method="GET")


# ── 信号处理 ─────────────────────────────────────────────

def handle_shutdown(signum, frame):
    sig_name = signal.Signals(signum).name
    logger.info(f"Received {sig_name}, shutting down gracefully...")
    _watchdog_stop.set()
    sys.exit(0)


signal.signal(signal.SIGTERM, handle_shutdown)
signal.signal(signal.SIGINT, handle_shutdown)

if __name__ == "__main__":
    port = int(os.environ.get("AIOS_PORT", "18731"))
    logger.info(f"Starting AI-OS Agent on port {port}")
    uvicorn.run(app, host="127.0.0.1", port=port, log_level="info")

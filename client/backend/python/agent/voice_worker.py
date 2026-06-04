"""
语音助手 Worker（Session 1 子进程）

由主服务（WinSW, Session 0）通过 CreateProcessAsUser 在用户会话中拉起。
本进程跑在用户桌面（Session 1），能正常读取键鼠输入。

职责：
  - 语音监听（Vosk + sounddevice）
  - 鼠标位置标定（空格确认）
  - 键鼠录制（F9 停止）+ 回放
  - 指令 CRUD

端口：18732（固定，主服务通过此端口转发请求）
主服务通过 /api/voice 代理到 http://127.0.0.1:18732/api/voice

作者：桂良涛，邮箱：桂良涛@nndrobot.com
"""

import os
import sys
import time
import signal
import logging
import threading
import traceback
import json
import ctypes
import ctypes.wintypes as wintypes
from datetime import datetime
from pathlib import Path

# ── 路径修正 ─────────────────────────────────────────────
# voice_worker.py 在 agent/ 目录下，需要能 import voice 模块
BACKEND_DIR = Path(__file__).resolve().parent.parent  # python/
if str(BACKEND_DIR) not in sys.path:
    sys.path.insert(0, str(BACKEND_DIR))

import uvicorn
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel
from typing import Optional

START_TIME = time.time()
WORKER_PORT = 18732

# ── 日志 ─────────────────────────────────────────────────
logging.basicConfig(
    level=logging.DEBUG,
    format="%(asctime)s [%(levelname)s] [VOICE-WORKER] %(message)s",
    handlers=[logging.StreamHandler(sys.stdout)],
)
logger = logging.getLogger("voice-worker")

app = FastAPI(title="AI-OS Voice Worker", version="0.1.0")


# ── 全局异常处理 ─────────────────────────────────────────

@app.exception_handler(Exception)
async def global_exception_handler(request: Request, exc: Exception):
    logger.error(f"Unhandled: {request.method} {request.url.path}: {exc}")
    logger.debug(traceback.format_exc())
    return JSONResponse(
        status_code=500,
        content={"ok": False, "error": str(exc), "traceback": traceback.format_exc()},
    )


# ── 健康检查 ─────────────────────────────────────────────

@app.get("/health")
async def health():
    return {
        "ok": True,
        "uptime": int(time.time() - START_TIME),
        "pid": os.getpid(),
        "session": "voice-worker",
    }


# ── Voice 模块状态 ───────────────────────────────────────
# 直接导入 voice 模块，所有键鼠操作在 Session 1 运行

voice_module = None


def ensure_voice():
    global voice_module
    if voice_module is not None:
        return True
    try:
        from voice import init_voice
        init_voice()
        import voice as vm
        voice_module = vm
        logger.info("Voice 模块已加载")
        return True
    except Exception as e:
        logger.error(f"Voice 模块加载失败: {e}")
        return False


# ── API 路由 ─────────────────────────────────────────────

class VoiceActionRequest(BaseModel):
    action: str
    payload: dict = {}


@app.post("/api/voice")
async def voice_api(req: VoiceActionRequest):
    action = req.action
    payload = req.payload

    # voice_status 不需要 ensure_voice
    if action == "voice_status":
        if not ensure_voice():
            from voice.config import find_vosk_model
            return {"ok": True, "listening": False, "model_ready": find_vosk_model() is not None,
                    "enabled": False, "commands": [], "calibrating": False, "calibration_index": None,
                    "mouse_pos": [0, 0], "recording": False, "recording_action_count": 0,
                    "recording_index": None}
        return {"ok": True, **voice_module.get_status()}

    if not ensure_voice():
        return {"ok": False, "error": "Voice 模块未加载"}

    if action == "voice_set_enabled":
        voice_module.set_enabled(payload.get("enabled", False))
        return {"ok": True}
    elif action == "voice_start":
        voice_module.start_voice()
        return {"ok": True}
    elif action == "voice_stop":
        voice_module.stop_voice()
        return {"ok": True}
    elif action == "voice_add_command":
        voice_module.add_command(payload.get("phrase", ""))
        return {"ok": True}
    elif action == "voice_update_command":
        voice_module.update_command(
            payload.get("index", 0),
            phrase=payload.get("phrase"),
            position=payload.get("position"),
            enabled=payload.get("enabled"),
        )
        return {"ok": True}
    elif action == "voice_remove_command":
        voice_module.remove_command(payload.get("index", 0))
        return {"ok": True}
    elif action == "voice_start_calibration":
        voice_module.start_calibration(payload.get("index", 0))
        return {"ok": True}
    elif action == "voice_cancel_calibration":
        voice_module.cancel_calibration()
        return {"ok": True}
    elif action == "voice_start_recording":
        return voice_module.start_recording(payload.get("index", 0))
    elif action == "voice_stop_recording":
        return voice_module.stop_recording()
    elif action == "voice_save_recording":
        voice_module.save_recorded_actions(
            payload.get("index", 0),
            payload.get("actions", []),
        )
        return {"ok": True}
    elif action == "voice_recording_status":
        return {"ok": True, **voice_module.get_recording_status()}
    else:
        return {"ok": False, "error": f"Unknown action: {action}"}


@app.post("/api/voice/download-model")
async def voice_download_model():
    """下载 Vosk 模型"""
    def do_download():
        from voice.config import download_vosk_model
        path = download_vosk_model()
        logger.info(f"Vosk model downloaded to {path}")
        ensure_voice()

    thread = threading.Thread(target=do_download, daemon=True)
    thread.start()
    return {"ok": True, "message": "Download started"}


@app.get("/api/voice/model-status")
async def voice_model_status():
    from voice.config import find_vosk_model
    path = find_vosk_model()
    return {"ok": True, "installed": path is not None, "path": path}


# ── 信号处理 ─────────────────────────────────────────────

def handle_shutdown(signum, frame):
    sig_name = signal.Signals(signum).name
    logger.info(f"Received {sig_name}, shutting down...")
    sys.exit(0)


signal.signal(signal.SIGTERM, handle_shutdown)
signal.signal(signal.SIGINT, handle_shutdown)


# ── 启动 ─────────────────────────────────────────────────

@app.on_event("startup")
async def on_startup():
    logger.info(f"Voice Worker started (pid={os.getpid()}, port={WORKER_PORT})")
    ensure_voice()


if __name__ == "__main__":
    logger.info(f"Starting Voice Worker on port {WORKER_PORT}")
    uvicorn.run(app, host="127.0.0.1", port=WORKER_PORT, log_level="info")

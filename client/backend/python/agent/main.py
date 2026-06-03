import os
import sys
import time
import signal
import logging
import threading
import json
from datetime import datetime
from pathlib import Path

# Ensure voice module can be imported
BACKEND_DIR = Path(__file__).resolve().parent
if str(BACKEND_DIR) not in sys.path:
    sys.path.insert(0, str(BACKEND_DIR))
# Also add parent for 'voice' package
VOICE_DIR = BACKEND_DIR.parent
if str(VOICE_DIR) not in sys.path:
    sys.path.insert(0, str(VOICE_DIR))

import uvicorn
from fastapi import FastAPI
from pydantic import BaseModel
from typing import Optional, Any

START_TIME = time.time()
VERSION = "0.1.0"
SHUTDOWN_DELAY = 5

# Voice module (lazy import to avoid crash if vosk not installed)
voice = None


def load_build_info():
    build_info_path = os.path.join(os.path.dirname(os.path.dirname(__file__)), 'build_info.json')
    try:
        with open(build_info_path, 'r', encoding='utf-8') as f:
            return json.load(f)
    except (FileNotFoundError, json.JSONDecodeError, OSError):
        return {}


BUILD_INFO = load_build_info()

if sys.stdout is None:
    sys.stdout = open(os.devnull, 'w', encoding='utf-8')
if sys.stderr is None:
    sys.stderr = open(os.devnull, 'w', encoding='utf-8')

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    handlers=[logging.StreamHandler(sys.stdout)],
)
logger = logging.getLogger("ai-os-agent")

app = FastAPI(title="AI-OS Agent", version=VERSION)


@app.get("/health")
async def health():
    return {
        "ok": True,
        "uptime": int(time.time() - START_TIME),
        "version": VERSION,
        "pid": os.getpid(),
        "started_at": datetime.fromtimestamp(START_TIME).isoformat(),
        "build_time": BUILD_INFO.get("build_time", "unknown"),
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


@app.on_event("startup")
async def on_startup():
    global voice
    build_time = BUILD_INFO.get("build_time", "unknown")
    logger.info(f"AI-OS Agent v{VERSION} started (pid={os.getpid()}, build_time={build_time})")
    try:
        from voice import init_voice
        voice = init_voice()
        if voice:
            logger.info("Voice module initialized")
    except Exception as e:
        logger.warning(f"Voice module not available: {e}")


# --- Voice API ---

class VoiceActionRequest(BaseModel):
    action: str
    payload: dict = {}

def ensure_voice():
    """Lazy init voice module if model became available after startup."""
    global voice
    if voice is not None:
        return True
    try:
        from voice import init_voice
        init_voice()
        # init_voice returns None, check if module loaded by importing it
        import voice as voice_mod
        voice = voice_mod
        logger.info("Voice module lazy-initialized")
        return True
    except Exception as e:
        logger.warning(f"Voice lazy-init failed: {e}")
    return False


@app.post("/api/voice")
async def voice_api(req: VoiceActionRequest):
    if req.action == "voice_status":
        # Always allow status check, auto-init if possible
        ensure_voice()
        if voice is None:
            from voice.config import find_vosk_model
            return {"ok": True, "listening": False, "model_ready": find_vosk_model() is not None,
                    "enabled": False, "commands": [], "calibrating": False, "calibration_index": None,
                    "mouse_pos": [0, 0]}
        status = voice.get_status()
        return {"ok": True, **status}

    if not ensure_voice():
        return {"ok": False, "error": "Voice module not available"}

    action = req.action
    payload = req.payload

    try:
        if action == "voice_set_enabled":
            voice.set_enabled(payload.get("enabled", False))
            return {"ok": True}
        elif action == "voice_start":
            voice.start_voice()
            return {"ok": True}
        elif action == "voice_stop":
            voice.stop_voice()
            return {"ok": True}
        elif action == "voice_add_command":
            voice.add_command(payload.get("phrase", ""))
            return {"ok": True}
        elif action == "voice_update_command":
            voice.update_command(
                payload.get("index", 0),
                phrase=payload.get("phrase"),
                position=payload.get("position"),
                enabled=payload.get("enabled"),
            )
            return {"ok": True}
        elif action == "voice_remove_command":
            voice.remove_command(payload.get("index", 0))
            return {"ok": True}
        elif action == "voice_start_calibration":
            voice.start_calibration(payload.get("index", 0))
            return {"ok": True}
        elif action == "voice_cancel_calibration":
            voice.cancel_calibration()
            return {"ok": True}
        else:
            return {"ok": False, "error": f"Unknown action: {action}"}
    except Exception as e:
        logger.error(f"Voice API error: {e}")
        return {"ok": False, "error": str(e)}


@app.post("/api/voice/download-model")
async def voice_download_model():
    """下载 Vosk 模型（在后台线程中执行）"""
    import threading

    def do_download():
        try:
            from voice.config import download_vosk_model
            path = download_vosk_model()
            logger.info(f"Vosk model downloaded to {path}")
            # Re-init voice with new model
            global voice
            if voice is None:
                from voice import init_voice
                voice = init_voice()
        except Exception as e:
            logger.error(f"Vosk model download failed: {e}")

    thread = threading.Thread(target=do_download, daemon=True)
    thread.start()
    return {"ok": True, "message": "Download started"}


@app.get("/api/voice/model-status")
async def voice_model_status():
    """检查 Vosk 模型是否已下载"""
    try:
        from voice.config import find_vosk_model
        path = find_vosk_model()
        return {"ok": True, "installed": path is not None, "path": path}
    except Exception as e:
        return {"ok": True, "installed": False, "error": str(e)}


def handle_shutdown(signum, frame):
    sig_name = signal.Signals(signum).name
    logger.info(f"Received {sig_name}, shutting down gracefully...")
    sys.exit(0)


signal.signal(signal.SIGTERM, handle_shutdown)
signal.signal(signal.SIGINT, handle_shutdown)

if __name__ == "__main__":
    port = int(os.environ.get("AIOS_PORT", "18731"))
    logger.info(f"Starting AI-OS Agent on port {port}")
    uvicorn.run(app, host="127.0.0.1", port=port, log_level="info")

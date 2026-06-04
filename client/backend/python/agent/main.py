import os
import sys
import time
import signal
import logging
import traceback
import json
from datetime import datetime
from pathlib import Path

# Force UTF-8 for pythonw.exe (no console, default encoding may be ascii)
os.environ["PYTHONIOENCODING"] = "utf-8"
os.environ["PYTHONUTF8"] = "1"
if sys.stdout and hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding="utf-8")
if sys.stderr and hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding="utf-8")

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

START_TIME = time.time()
VERSION = "0.1.2"
SHUTDOWN_DELAY = 5

# Debug mode: True = show full error info, False = show generic error
DEBUG = os.environ.get("AI_OS_DEBUG", "true").lower() == "true"

# Voice module (lazy import to avoid crash if vosk not installed)
voice = None


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
    }


@app.post("/shutdown")
async def shutdown():
    logger.info("Received shutdown request, shutting down gracefully in {} seconds...".format(SHUTDOWN_DELAY))

    def delayed_exit():
        time.sleep(SHUTDOWN_DELAY)
        logger.info("Executing shutdown...")
        sys.exit(0)

    import threading
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
    logger.info(f"Debug mode: {DEBUG}")
    try:
        from voice import init_voice
        init_voice()
        import voice as voice_mod
        voice = voice_mod
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
    from voice import init_voice
    init_voice()
    import voice as voice_mod
    voice = voice_mod
    logger.info("Voice module lazy-initialized")
    return True


@app.post("/api/voice")
async def voice_api(req: VoiceActionRequest):
    if req.action == "voice_status":
        ensure_voice()
        if voice is None:
            from voice.config import find_vosk_model
            return {"ok": True, "listening": False, "model_ready": find_vosk_model() is not None,
                    "enabled": False, "commands": [], "calibrating": False, "calibration_index": None,
                    "mouse_pos": [0, 0]}
        status = voice.get_status()
        return {"ok": True, **status}

    ensure_voice()

    action = req.action
    payload = req.payload

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
            {k: v for k, v in payload.items() if k != "index"},
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
    elif action == "voice_start_recording":
        result = voice.start_recording(payload.get("index", 0))
        return {"ok": True, **result}
    elif action == "voice_stop_recording":
        result = voice.stop_recording()
        return {"ok": True, **result}
    elif action == "voice_save_recording":
        voice.save_recorded_actions(payload.get("index", 0), payload.get("actions", []))
        return {"ok": True}
    elif action == "voice_recording_status":
        status = voice.get_recording_status()
        return {"ok": True, **status}
    elif action == "voice_recognize_log":
        logs = voice.get_recognize_log()
        return {"ok": True, "logs": logs}
    elif action == "voice_clear_recognize_log":
        voice.clear_recognize_log()
        return {"ok": True}
    else:
        return {"ok": False, "error": f"Unknown action: {action}"}


@app.post("/api/voice/download-model")
async def voice_download_model():
    """下载 Vosk 模型（在后台线程中执行）"""
    import threading

    def do_download():
        from voice.config import download_vosk_model
        path = download_vosk_model()
        logger.info(f"Vosk model downloaded to {path}")
        global voice
        if voice is None:
            from voice import init_voice
            init_voice()
            import voice as voice_mod
            voice = voice_mod

    thread = threading.Thread(target=do_download, daemon=True)
    thread.start()
    return {"ok": True, "message": "Download started"}


@app.get("/api/voice/model-status")
async def voice_model_status():
    """检查 Vosk 模型是否已下载"""
    from voice.config import find_vosk_model
    path = find_vosk_model()
    return {"ok": True, "installed": path is not None, "path": path}


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

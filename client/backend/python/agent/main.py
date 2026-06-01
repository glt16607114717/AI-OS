import os
import sys
import time
import signal
import logging
import threading
from datetime import datetime

import uvicorn
from fastapi import FastAPI

START_TIME = time.time()
VERSION = "0.1.0"
SHUTDOWN_DELAY = 5

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
    logger.info(f"AI-OS Agent v{VERSION} started (pid={os.getpid()})")


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

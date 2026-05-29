"""
AI-OS Agent - Minimal shell version
Just a health check endpoint to prove the service is alive.
"""
import os
import sys
import time
import logging
from datetime import datetime

import uvicorn
from fastapi import FastAPI

START_TIME = time.time()
VERSION = "0.1.0"

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


@app.on_event("startup")
async def on_startup():
    logger.info(f"AI-OS Agent v{VERSION} started (pid={os.getpid()})")


if __name__ == "__main__":
    port = int(os.environ.get("AIOS_PORT", "18731"))
    logger.info(f"Starting AI-OS Agent on port {port}")
    uvicorn.run(app, host="127.0.0.1", port=port, log_level="info")

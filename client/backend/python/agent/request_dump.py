"""
请求转储模块。

将 LLM 请求和响应完整保存到文件，用于调试和分析。
每个请求一个文件，按时间戳命名。

存储路径：%PROGRAMDATA%/AI-OS/logs/requests/
"""

import json
import logging
import os
from datetime import datetime
from pathlib import Path

logger = logging.getLogger("llm")

DUMP_DIR = Path(os.environ.get("PROGRAMDATA", "C:\\ProgramData")) / "AI-OS" / "logs" / "requests"
DUMP_DIR.mkdir(parents=True, exist_ok=True)


def dump_request(body: dict, response_body: dict = None, vendor_info: dict = None, error: str = None):
    """保存一次完整的请求/响应到文件。"""
    try:
        ts = datetime.now().strftime("%Y%m%d_%H%M%S_%f")
        filename = DUMP_DIR / f"{ts}.json"

        record = {
            "timestamp": datetime.now().isoformat(),
            "request": body,
        }

        if vendor_info:
            record["vendor"] = {
                "id": vendor_info.get("vendor_id"),
                "name": vendor_info.get("vendor_name"),
                "model": vendor_info.get("model_id"),
            }

        if response_body:
            record["response"] = response_body

        if error:
            record["error"] = error

        filename.write_text(json.dumps(record, ensure_ascii=False, indent=2), encoding="utf-8")
        logger.info(f"[RequestDump] saved: {filename.name}")
    except Exception as e:
        logger.error(f"[RequestDump] dump failed: {e}")

"""
LLM 操作日志模块。

存储大模型请求、路由决策、降级、故障转移、用量监控等事件到 SQLite。
前端 1 秒轮询读取，人可读格式。

作者：桂良涛
"""

import logging
import sqlite3
import threading
import time
from datetime import datetime
from pathlib import Path

logger = logging.getLogger("llm")

DB_DIR = Path("C:/ProgramData/AI-OS/data")
DB_DIR.mkdir(parents=True, exist_ok=True)
DB_PATH = DB_DIR / "llm_log.db"

_lock = threading.Lock()
_db: sqlite3.Connection | None = None

MAX_LOGS = 1000  # 最多保留条数，超过自动清理


def _get_db() -> sqlite3.Connection:
    global _db
    if _db is None:
        _db = sqlite3.connect(str(DB_PATH), check_same_thread=False)
        _db.execute("PRAGMA journal_mode=WAL")
        _db.execute("""
            CREATE TABLE IF NOT EXISTS llm_log (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                ts TEXT NOT NULL,
                category TEXT NOT NULL,
                level TEXT NOT NULL DEFAULT 'info',
                message TEXT NOT NULL,
                detail TEXT DEFAULT ''
            )
        """)
        _db.execute("CREATE INDEX IF NOT EXISTS idx_ts ON llm_log(ts DESC)")
        _db.commit()
    return _db


def write_log(category: str, message: str, level: str = "info", detail: str = ""):
    """
    写入一条日志。
    category: request / route / downgrade / quota / failover / config / error
    level: info / warning / error
    message: 人可读的一句话
    detail: 额外信息（模型、token、错误码等）
    """
    ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    with _lock:
        db = _get_db()
        db.execute(
            "INSERT INTO llm_log (ts, category, level, message, detail) VALUES (?, ?, ?, ?, ?)",
            (ts, category, level, message, detail),
        )
        # 自动清理超出条数
        count = db.execute("SELECT COUNT(*) FROM llm_log").fetchone()[0]
        if count > MAX_LOGS:
            db.execute(
                "DELETE FROM llm_log WHERE id IN (SELECT id FROM llm_log ORDER BY ts ASC LIMIT ?)",
                (count - MAX_LOGS,),
            )
        db.commit()


def get_logs(limit: int = 200, category: str = "", after_id: int = 0) -> list[dict]:
    """
    查询日志。
    after_id: 只返回 id > after_id 的记录（用于增量刷新）。
    """
    with _lock:
        db = _get_db()
        if after_id > 0:
            if category:
                rows = db.execute(
                    "SELECT id, ts, category, level, message, detail FROM llm_log WHERE id > ? AND category = ? ORDER BY id DESC LIMIT ?",
                    (after_id, category, limit),
                ).fetchall()
            else:
                rows = db.execute(
                    "SELECT id, ts, category, level, message, detail FROM llm_log WHERE id > ? ORDER BY id DESC LIMIT ?",
                    (after_id, limit),
                ).fetchall()
        else:
            if category:
                rows = db.execute(
                    "SELECT id, ts, category, level, message, detail FROM llm_log WHERE category = ? ORDER BY id DESC LIMIT ?",
                    (category, limit),
                ).fetchall()
            else:
                rows = db.execute(
                    "SELECT id, ts, category, level, message, detail FROM llm_log ORDER BY id DESC LIMIT ?",
                    (limit,),
                ).fetchall()

    return [
        {
            "id": r[0],
            "ts": r[1],
            "category": r[2],
            "level": r[3],
            "message": r[4],
            "detail": r[5],
        }
        for r in rows
    ]


def clear_logs():
    """清空所有日志。"""
    with _lock:
        db = _get_db()
        db.execute("DELETE FROM llm_log")
        db.execute("DELETE FROM sqlite_sequence WHERE name='llm_log'")
        db.commit()
    logger.info("[Log] all logs cleared")


def get_max_id() -> int:
    """获取当前最大 id。"""
    with _lock:
        db = _get_db()
        row = db.execute("SELECT MAX(id) FROM llm_log").fetchone()
        return row[0] or 0

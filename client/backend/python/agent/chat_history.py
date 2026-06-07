"""
聊天历史记录模块。

存储工作台对话到 SQLite，支持前端缓存恢复。

作者：桂良涛
"""

import logging
import sqlite3
import threading
from datetime import datetime
from pathlib import Path

logger = logging.getLogger("llm")

DB_DIR = Path("C:/ProgramData/AI-OS/data")
DB_DIR.mkdir(parents=True, exist_ok=True)
DB_PATH = DB_DIR / "chat_history.db"

_lock = threading.Lock()
_db: sqlite3.Connection | None = None

MAX_MESSAGES = 10000  # 最多保留条数，超过自动清理


def _get_db() -> sqlite3.Connection:
    global _db
    if _db is None:
        _db = sqlite3.connect(str(DB_PATH), check_same_thread=False)
        _db.execute("PRAGMA journal_mode=WAL")
        _db.execute("""
            CREATE TABLE IF NOT EXISTS chat_history (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                created_at TEXT NOT NULL,
                role TEXT NOT NULL,
                content TEXT NOT NULL
            )
        """)
        _db.execute("CREATE INDEX IF NOT EXISTS idx_created_at ON chat_history(created_at DESC)")
        _db.commit()
    return _db


def add_message(role: str, content: str):
    """
    添加一条聊天记录。
    role: 'user' | 'assistant'
    """
    created_at = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    with _lock:
        db = _get_db()
        db.execute(
            "INSERT INTO chat_history (created_at, role, content) VALUES (?, ?, ?)",
            (created_at, role, content),
        )
        # 自动清理超出条数
        count = db.execute("SELECT COUNT(*) FROM chat_history").fetchone()[0]
        if count > MAX_MESSAGES:
            db.execute(
                "DELETE FROM chat_history WHERE id IN (SELECT id FROM chat_history ORDER BY created_at ASC LIMIT ?)",
                (count - MAX_MESSAGES,),
            )
        db.commit()
    logger.info(f"[Chat] message saved: {role}, len={len(content)}")


def get_history(limit: int = 1000) -> list[dict]:
    """查询最近的聊天记录（按时间正序）。"""
    with _lock:
        db = _get_db()
        rows = db.execute(
            "SELECT id, created_at, role, content FROM chat_history ORDER BY created_at ASC LIMIT ?",
            (limit,),
        ).fetchall()

    return [
        {
            "id": r[0],
            "created_at": r[1],
            "role": r[2],
            "content": r[3],
        }
        for r in rows
    ]


def clear_history():
    """清空所有聊天记录。"""
    with _lock:
        db = _get_db()
        db.execute("DELETE FROM chat_history")
        db.execute("DELETE FROM sqlite_sequence WHERE name='chat_history'")
        db.commit()
    logger.info("[Chat] history cleared")


def get_max_id() -> int:
    """获取当前最大 id（用于增量同步）。"""
    with _lock:
        db = _get_db()
        row = db.execute("SELECT MAX(id) FROM chat_history").fetchone()
        return row[0] or 0
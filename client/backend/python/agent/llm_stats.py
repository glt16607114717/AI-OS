"""
LLM 用量统计模块（SQLite 版）

每次请求记录一条统计：vendor、model、token 用量、延迟、成功/失败。
每天 12:00 自动清理 30 天前的数据。
查询用 SQL 聚合，毫秒级。
"""

import os
import sqlite3
import threading
from pathlib import Path
from datetime import datetime, timedelta

_DATA_DIR = Path(os.environ.get("AIOS_DATA_DIR", "C:/ProgramData/AI-OS")) / "data"
_DB_FILE = _DATA_DIR / "llm_stats.db"
_RETENTION_DAYS = 90
_lock = threading.Lock()


def _get_conn() -> sqlite3.Connection:
    """获取数据库连接（带 WAL 模式提升并发性能）"""
    _DATA_DIR.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(str(_DB_FILE), check_same_thread=False)
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA synchronous=NORMAL")
    return conn


def _init_db():
    """初始化表和索引"""
    conn = _get_conn()
    conn.execute("""
        CREATE TABLE IF NOT EXISTS llm_requests (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            ts TEXT NOT NULL,
            vendor_id TEXT NOT NULL,
            vendor_name TEXT NOT NULL,
            model_id TEXT NOT NULL,
            prompt_tokens INTEGER DEFAULT 0,
            completion_tokens INTEGER DEFAULT 0,
            total_tokens INTEGER DEFAULT 0,
            latency_ms INTEGER DEFAULT 0,
            success INTEGER DEFAULT 1,
            error TEXT DEFAULT ''
        )
    """)
    conn.execute("CREATE INDEX IF NOT EXISTS idx_ts ON llm_requests(ts)")
    conn.execute("CREATE INDEX IF NOT EXISTS idx_vendor ON llm_requests(vendor_id)")
    conn.execute("CREATE INDEX IF NOT EXISTS idx_model ON llm_requests(model_id)")
    conn.commit()
    conn.close()


# 模块加载时初始化
_init_db()


def record(
    vendor_id: str,
    vendor_name: str,
    model_id: str,
    prompt_tokens: int = 0,
    completion_tokens: int = 0,
    total_tokens: int = 0,
    latency_ms: int = 0,
    success: bool = True,
    error: str = "",
) -> None:
    """记录一次 LLM 请求的统计数据"""
    with _lock:
        conn = _get_conn()
        conn.execute(
            """INSERT INTO llm_requests
               (ts, vendor_id, vendor_name, model_id, prompt_tokens,
                completion_tokens, total_tokens, latency_ms, success, error)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (
                datetime.now().isoformat(timespec="seconds"),
                vendor_id,
                vendor_name,
                model_id,
                prompt_tokens,
                completion_tokens,
                total_tokens,
                latency_ms,
                1 if success else 0,
                error[:200] if error else "",
            ),
        )
        conn.commit()
        conn.close()


def cleanup() -> int:
    """清理超过 30 天的记录，返回删除条数"""
    cutoff = (datetime.now() - timedelta(days=_RETENTION_DAYS)).strftime("%Y-%m-%dT%H:%M:%S")
    with _lock:
        conn = _get_conn()
        cur = conn.execute("DELETE FROM llm_requests WHERE ts < ?", (cutoff,))
        removed = cur.rowcount
        conn.commit()
        conn.close()
    return removed


def get_summary(days: int = 30) -> dict:
    """
    获取最近 N 天的汇总统计（全部用 SQL 聚合）。
    """
    cutoff = (datetime.now() - timedelta(days=days)).strftime("%Y-%m-%dT%H:%M:%S")

    conn = _get_conn()
    conn.row_factory = sqlite3.Row

    # 总览
    row = conn.execute("""
        SELECT
            COUNT(*) as total_requests,
            COALESCE(SUM(total_tokens), 0) as total_tokens,
            COALESCE(SUM(prompt_tokens), 0) as total_prompt_tokens,
            COALESCE(SUM(completion_tokens), 0) as total_completion_tokens,
            COALESCE(AVG(CASE WHEN success=1 THEN latency_ms END), 0) as avg_latency_ms,
            COALESCE(SUM(success), 0) as success_count
        FROM llm_requests WHERE ts >= ?
    """, (cutoff,)).fetchone()

    total_requests = row["total_requests"]
    success_count = row["success_count"]

    # 按厂商聚合
    by_vendor = {}
    for r in conn.execute("""
        SELECT
            vendor_id,
            MAX(vendor_name) as name,
            COUNT(*) as requests,
            COALESCE(SUM(total_tokens), 0) as tokens,
            COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
            COALESCE(SUM(completion_tokens), 0) as completion_tokens,
            COALESCE(AVG(CASE WHEN success=1 THEN latency_ms END), 0) as avg_latency_ms,
            SUM(success) as success_count,
            SUM(CASE WHEN success=0 THEN 1 ELSE 0 END) as errors
        FROM llm_requests WHERE ts >= ?
        GROUP BY vendor_id
    """, (cutoff,)):
        by_vendor[r["vendor_id"]] = {
            "name": r["name"],
            "requests": r["requests"],
            "tokens": r["tokens"],
            "prompt_tokens": r["prompt_tokens"],
            "completion_tokens": r["completion_tokens"],
            "avg_latency_ms": int(r["avg_latency_ms"]),
            "success_count": r["success_count"],
            "errors": r["errors"],
        }

    # 按模型聚合
    by_model = {}
    for r in conn.execute("""
        SELECT
            model_id,
            COUNT(*) as requests,
            COALESCE(SUM(total_tokens), 0) as tokens,
            COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
            COALESCE(SUM(completion_tokens), 0) as completion_tokens,
            COALESCE(AVG(CASE WHEN success=1 THEN latency_ms END), 0) as avg_latency_ms,
            SUM(success) as success_count,
            SUM(CASE WHEN success=0 THEN 1 ELSE 0 END) as errors
        FROM llm_requests WHERE ts >= ?
        GROUP BY model_id
    """, (cutoff,)):
        by_model[r["model_id"]] = {
            "requests": r["requests"],
            "tokens": r["tokens"],
            "prompt_tokens": r["prompt_tokens"],
            "completion_tokens": r["completion_tokens"],
            "avg_latency_ms": int(r["avg_latency_ms"]),
            "success_count": r["success_count"],
            "errors": r["errors"],
        }

    # 按天聚合
    daily = {}
    for r in conn.execute("""
        SELECT
            SUBSTR(ts, 1, 10) as day,
            COUNT(*) as requests,
            COALESCE(SUM(total_tokens), 0) as tokens,
            COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
            COALESCE(SUM(completion_tokens), 0) as completion_tokens,
            SUM(success) as success_count,
            SUM(CASE WHEN success=0 THEN 1 ELSE 0 END) as errors
        FROM llm_requests WHERE ts >= ?
        GROUP BY SUBSTR(ts, 1, 10)
        ORDER BY day DESC
    """, (cutoff,)):
        daily[r["day"]] = {
            "requests": r["requests"],
            "tokens": r["tokens"],
            "prompt_tokens": r["prompt_tokens"],
            "completion_tokens": r["completion_tokens"],
            "success_count": r["success_count"],
            "errors": r["errors"],
        }

    conn.close()

    return {
        "total_requests": total_requests,
        "total_tokens": row["total_tokens"],
        "total_prompt_tokens": row["total_prompt_tokens"],
        "total_completion_tokens": row["total_completion_tokens"],
        "avg_latency_ms": int(row["avg_latency_ms"]),
        "success_rate": round(success_count / total_requests, 4) if total_requests else 0,
        "by_vendor": by_vendor,
        "by_model": by_model,
        "daily": daily,
    }


def get_recent_errors(limit: int = 20) -> list[dict]:
    """获取最近的错误记录"""
    cutoff = (datetime.now() - timedelta(days=7)).strftime("%Y-%m-%dT%H:%M:%S")
    with _lock:
        conn = _get_conn()
        conn.row_factory = sqlite3.Row
        rows = conn.execute(
            """SELECT ts, vendor_name, model_id, latency_ms, error
               FROM llm_requests
               WHERE ts >= ? AND success = 0
               ORDER BY ts DESC LIMIT ?""",
            (cutoff, limit),
        ).fetchall()
        conn.close()
    return [dict(r) for r in rows]

"""
LLM 用量统计模块（MySQL 版）

每次请求记录一条统计：vendor、model、token 用量、延迟、成功/失败。
支持按用户维度聚合。
保留 90 天数据，自动清理。

@author 桂良涛
"""

import logging
import threading
from datetime import datetime, timedelta

logger = logging.getLogger("agent")

try:
    import pymysql
except ImportError:
    pymysql = None

# 远程 MySQL 配置（与 user_api.py 保持一致）
DB_CONFIG = {
    "host": "124.221.220.89",
    "port": 23306,
    "user": "root",
    "password": "glt01054717@",
    "charset": "utf8mb4",
    "connect_timeout": 10,
    "read_timeout": 10,
}
DB_NAME = "ai_os"
TABLE_NAME = "sys_llm_stats"

_RETENTION_DAYS = 90
_lock = threading.Lock()


def _get_conn():
    """获取 MySQL 连接"""
    if pymysql is None:
        raise RuntimeError("pymysql 未安装，无法使用 LLM 统计")
    cfg = dict(DB_CONFIG)
    cfg["database"] = DB_NAME
    return pymysql.connect(**cfg, cursorclass=pymysql.cursors.DictCursor)


def _init_db():
    """首次连接时自动建库建表"""
    try:
        conn = pymysql.connect(**DB_CONFIG, cursorclass=pymysql.cursors.DictCursor)
        with conn:
            cur = conn.cursor()
            cur.execute(f"CREATE DATABASE IF NOT EXISTS `{DB_NAME}` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_general_ci")
            cur.execute(f"USE `{DB_NAME}`")
            cur.execute(f"""
                CREATE TABLE IF NOT EXISTS `{TABLE_NAME}` (
                    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
                    `ts` DATETIME NOT NULL,
                    `user_id` INT UNSIGNED DEFAULT NULL COMMENT '关联用户ID',
                    `username` VARCHAR(64) DEFAULT '' COMMENT '冗余用户名',
                    `vendor_id` VARCHAR(64) NOT NULL,
                    `vendor_name` VARCHAR(128) NOT NULL DEFAULT '',
                    `model_id` VARCHAR(128) NOT NULL,
                    `prompt_tokens` INT DEFAULT 0,
                    `completion_tokens` INT DEFAULT 0,
                    `total_tokens` INT DEFAULT 0,
                    `latency_ms` INT DEFAULT 0,
                    `success` TINYINT DEFAULT 1,
                    `error` VARCHAR(512) DEFAULT '',
                    INDEX `idx_ts` (`ts`),
                    INDEX `idx_vendor` (`vendor_id`),
                    INDEX `idx_model` (`model_id`),
                    INDEX `idx_user` (`user_id`),
                    INDEX `idx_user_date` (`user_id`, `ts`)
                ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
            """)
        logger.info(f"[LLM统计] 数据库和表已就绪: {DB_NAME}.{TABLE_NAME}")
    except Exception as e:
        logger.error(f"[LLM统计] 建库建表失败: {e}")


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
    user_id: int | None = None,
    username: str = "",
) -> None:
    """记录一次 LLM 请求的统计数据"""
    with _lock:
        try:
            conn = _get_conn()
            with conn:
                cur = conn.cursor()
                cur.execute(
                    f"""INSERT INTO `{TABLE_NAME}`
                       (ts, user_id, username, vendor_id, vendor_name, model_id,
                        prompt_tokens, completion_tokens, total_tokens,
                        latency_ms, success, error)
                       VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)""",
                    (
                        datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
                        user_id,
                        username,
                        vendor_id,
                        vendor_name,
                        model_id,
                        prompt_tokens,
                        completion_tokens,
                        total_tokens,
                        latency_ms,
                        1 if success else 0,
                        error[:512] if error else "",
                    ),
                )
                conn.commit()
        except Exception as e:
            logger.error(f"[LLM统计] record 写入失败: {e}")


def cleanup() -> int:
    """清理超过保留天数的记录，返回删除条数"""
    cutoff = (datetime.now() - timedelta(days=_RETENTION_DAYS)).strftime("%Y-%m-%d %H:%M:%S")
    with _lock:
        try:
            conn = _get_conn()
            with conn:
                cur = conn.cursor()
                cur.execute(f"DELETE FROM `{TABLE_NAME}` WHERE ts < %s", (cutoff,))
                removed = cur.rowcount
                conn.commit()
                return removed
        except Exception as e:
            logger.error(f"[LLM统计] cleanup 失败: {e}")
            return 0


def get_summary(days: int = 30) -> dict:
    """
    获取最近 N 天的汇总统计（全部用 SQL 聚合）。
    返回值包含 by_vendor / by_model / by_user / daily 四个维度。
    """
    cutoff = (datetime.now() - timedelta(days=days)).strftime("%Y-%m-%d %H:%M:%S")

    try:
        conn = _get_conn()
    except Exception as e:
        logger.error(f"[LLM统计] get_summary 连接失败: {e}")
        return {}

    try:
        with conn:
            cur = conn.cursor()

            # 总览
            cur.execute(f"""
                SELECT
                    COUNT(*) as total_requests,
                    COALESCE(SUM(total_tokens), 0) as total_tokens,
                    COALESCE(SUM(prompt_tokens), 0) as total_prompt_tokens,
                    COALESCE(SUM(completion_tokens), 0) as total_completion_tokens,
                    COALESCE(AVG(CASE WHEN success=1 THEN latency_ms END), 0) as avg_latency_ms,
                    COALESCE(SUM(success), 0) as success_count
                FROM `{TABLE_NAME}` WHERE ts >= %s
            """, (cutoff,))
            row = cur.fetchone()

            total_requests = row["total_requests"]
            success_count = row["success_count"]

            # 按厂商聚合
            by_vendor = {}
            cur.execute(f"""
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
                FROM `{TABLE_NAME}` WHERE ts >= %s
                GROUP BY vendor_id
            """, (cutoff,))
            for r in cur.fetchall():
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
            cur.execute(f"""
                SELECT
                    model_id,
                    COUNT(*) as requests,
                    COALESCE(SUM(total_tokens), 0) as tokens,
                    COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
                    COALESCE(SUM(completion_tokens), 0) as completion_tokens,
                    COALESCE(AVG(CASE WHEN success=1 THEN latency_ms END), 0) as avg_latency_ms,
                    SUM(success) as success_count,
                    SUM(CASE WHEN success=0 THEN 1 ELSE 0 END) as errors
                FROM `{TABLE_NAME}` WHERE ts >= %s
                GROUP BY model_id
            """, (cutoff,))
            for r in cur.fetchall():
                by_model[r["model_id"]] = {
                    "requests": r["requests"],
                    "tokens": r["tokens"],
                    "prompt_tokens": r["prompt_tokens"],
                    "completion_tokens": r["completion_tokens"],
                    "avg_latency_ms": int(r["avg_latency_ms"]),
                    "success_count": r["success_count"],
                    "errors": r["errors"],
                }

            # 按用户聚合
            by_user = {}
            cur.execute(f"""
                SELECT
                    user_id,
                    MAX(username) as username,
                    COUNT(*) as requests,
                    COALESCE(SUM(total_tokens), 0) as tokens,
                    COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
                    COALESCE(SUM(completion_tokens), 0) as completion_tokens,
                    COALESCE(AVG(CASE WHEN success=1 THEN latency_ms END), 0) as avg_latency_ms,
                    SUM(success) as success_count,
                    SUM(CASE WHEN success=0 THEN 1 ELSE 0 END) as errors
                FROM `{TABLE_NAME}` WHERE ts >= %s AND user_id IS NOT NULL
                GROUP BY user_id
            """, (cutoff,))
            for r in cur.fetchall():
                by_user[str(r["user_id"])] = {
                    "username": r["username"],
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
            cur.execute(f"""
                SELECT
                    DATE(ts) as day,
                    COUNT(*) as requests,
                    COALESCE(SUM(total_tokens), 0) as tokens,
                    COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
                    COALESCE(SUM(completion_tokens), 0) as completion_tokens,
                    SUM(success) as success_count,
                    SUM(CASE WHEN success=0 THEN 1 ELSE 0 END) as errors
                FROM `{TABLE_NAME}` WHERE ts >= %s
                GROUP BY DATE(ts)
                ORDER BY day DESC
            """, (cutoff,))
            for r in cur.fetchall():
                day_key = str(r["day"])
                daily[day_key] = {
                    "requests": r["requests"],
                    "tokens": r["tokens"],
                    "prompt_tokens": r["prompt_tokens"],
                    "completion_tokens": r["completion_tokens"],
                    "success_count": r["success_count"],
                    "errors": r["errors"],
                }

        return {
            "total_requests": total_requests,
            "total_tokens": row["total_tokens"],
            "total_prompt_tokens": row["total_prompt_tokens"],
            "total_completion_tokens": row["total_completion_tokens"],
            "avg_latency_ms": int(row["avg_latency_ms"]),
            "success_rate": round(success_count / total_requests, 4) if total_requests else 0,
            "by_vendor": by_vendor,
            "by_model": by_model,
            "by_user": by_user,
            "daily": daily,
        }
    except Exception as e:
        logger.error(f"[LLM统计] get_summary 查询失败: {e}")
        return {}
    finally:
        try:
            conn.close()
        except Exception:
            pass


def get_recent_errors(limit: int = 20) -> list[dict]:
    """获取最近的错误记录"""
    cutoff = (datetime.now() - timedelta(days=7)).strftime("%Y-%m-%d %H:%M:%S")
    with _lock:
        try:
            conn = _get_conn()
            with conn:
                cur = conn.cursor()
                cur.execute(
                    f"""SELECT ts, vendor_name, model_id, latency_ms, error
                       FROM `{TABLE_NAME}`
                       WHERE ts >= %s AND success = 0
                       ORDER BY ts DESC LIMIT %s""",
                    (cutoff, limit),
                )
                rows = cur.fetchall()
                # 统一 ts 为字符串格式
                for r in rows:
                    if hasattr(r["ts"], "isoformat"):
                        r["ts"] = r["ts"].isoformat()
                return rows
        except Exception as e:
            logger.error(f"[LLM统计] get_recent_errors 查询失败: {e}")
            return []

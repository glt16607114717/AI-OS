"""
MySQL 只读查询工具（工作台专用）

测试环境配置硬编码，仅允许 SELECT 语句。
自动追加 LIMIT，超时 10 秒，最多返回 200 行。

@author 桂良涛
"""

import json
import re
import traceback

try:
    import pymysql
except ImportError:
    pymysql = None

# 测试环境配置
DB_CONFIG = {
    "host": "gz-cdb-morch28x.sql.tencentcdb.com",
    "port": 28833,
    "user": "robot_test",
    "password": "nnd@1234TEST",
    "database": "nnd_robot_test",
    "charset": "utf8mb4",
    "connect_timeout": 10,
    "read_timeout": 10,
}

MAX_ROWS = 200
DEFAULT_LIMIT = 50


def execute(sql: str, database: str = None) -> dict:
    """
    执行只读 SQL 查询。

    @param sql SQL 语句（仅允许 SELECT）
    @param database 可选，切换数据库
    @return {"ok": True, "data": [...], "columns": [...], "rows": int} 或 {"ok": False, "error": "..."}
    """
    if pymysql is None:
        return {"ok": False, "error": "pymysql 未安装"}

    # 安全校验：只允许 SELECT
    stripped = sql.strip().upper()
    if not stripped.startswith("SELECT") and not stripped.startswith("SHOW") and not stripped.startswith("DESCRIBE") and not stripped.startswith("EXPLAIN"):
        return {"ok": False, "error": "仅允许 SELECT / SHOW / DESCRIBE / EXPLAIN 语句"}

    # 自动追加 LIMIT（如果没有的话，仅对 SELECT）
    if stripped.startswith("SELECT") and "LIMIT" not in stripped:
        limit_match = re.search(r'\bLIMIT\s+(\d+)\s*$', sql, re.IGNORECASE)
        if not limit_match:
            sql = sql.rstrip(";") + f" LIMIT {DEFAULT_LIMIT}"

    config = dict(DB_CONFIG)
    if database:
        config["database"] = database

    try:
        conn = pymysql.connect(**config)
        try:
            with conn.cursor(pymysql.cursors.DictCursor) as cursor:
                cursor.execute(sql)

                # 拦截过大结果集
                if cursor.rowcount > MAX_ROWS:
                    return {
                        "ok": False,
                        "error": f"结果集过大（{cursor.rowcount} 行），请添加更严格的 WHERE 条件或减小 LIMIT",
                    }

                rows = cursor.fetchall()
                columns = [desc[0] for desc in cursor.description] if cursor.description else []
                return {
                    "ok": True,
                    "data": rows,
                    "columns": columns,
                    "rows": len(rows),
                }
        finally:
            conn.close()
    except Exception as e:
        return {"ok": False, "error": f"MySQL 错误: {type(e).__name__}: {str(e)}"}


def list_tables(database: str = None) -> dict:
    """列出所有表名"""
    return execute("SHOW TABLES", database)


def describe_table(table: str) -> dict:
    """查看表结构"""
    # 防注入：表名只允许字母数字下划线
    if not re.match(r'^[a-zA-Z0-9_]+$', table):
        return {"ok": False, "error": "非法表名"}
    return execute(f"DESCRIBE `{table}`")

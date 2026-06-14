#!/usr/bin/env python3
"""AIOS 数据库查询脚本"""

import json
import sys
import os
import re
import time

# 数据库配置
DB_CONFIG = {
    "host": "124.221.220.89",
    "port": 23306,
    "database": "ai_os",
    "username": "root",
    "password": "glt01054717@",
}

# 危险关键词
WRITE_KEYWORDS = ["INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER", "TRUNCATE", "GRANT", "REVOKE"]
# 敏感字段（结果中脱敏）
SENSITIVE_FIELDS = {"password_hash", "password", "api_key", "secret"}


def read_input():
    """从文件或stdin读取输入"""
    for arg in sys.argv[1:]:
        if os.path.isfile(arg):
            with open(arg, "r", encoding="utf-8-sig") as f:
                return f.read().strip()
    # 尝试从stdin读取
    if not sys.stdin.isatty():
        data = sys.stdin.read().strip()
        if data:
            return data
    return ""


def get_sql_type(sql):
    """获取SQL类型"""
    sql_stripped = sql.strip().upper()
    match = re.match(
        r"^(SELECT|SHOW|DESCRIBE|EXPLAIN|INSERT|UPDATE|DELETE|DROP|CREATE|ALTER|TRUNCATE|GRANT|REVOKE)",
        sql_stripped,
    )
    return match.group(1) if match else "UNKNOWN"


def mask_sensitive(data):
    """脱敏敏感字段"""
    if not isinstance(data, list):
        return data
    for row in data:
        if isinstance(row, dict):
            for key in list(row.keys()):
                if key.lower() in SENSITIVE_FIELDS and row[key]:
                    row[key] = "***MASKED***"
    return data


def execute_sql(sql, read_only=True, limit=100):
    """执行SQL"""
    try:
        import pymysql
    except ImportError:
        print("[错误] 缺少 pymysql 库，请安装: pip install pymysql")
        sys.exit(1)

    sql_type = get_sql_type(sql)

    # 只读模式检查
    if read_only and sql_type in WRITE_KEYWORDS:
        print(f"[错误] 只读模式下不允许执行 {sql_type} 操作")
        print('[提示] 如需写入，请在参数中设置 "read_only": false')
        sys.exit(1)

    # SELECT 自动加 LIMIT（如果没有）
    if sql_type == "SELECT" and "LIMIT" not in sql.upper():
        sql = sql.rstrip(";") + f" LIMIT {limit}"

    conn = pymysql.connect(
        host=DB_CONFIG["host"],
        port=DB_CONFIG["port"],
        user=DB_CONFIG["username"],
        password=DB_CONFIG["password"],
        database=DB_CONFIG["database"],
        charset="utf8mb4",
        cursorclass=pymysql.cursors.DictCursor,
    )

    try:
        start = time.time()
        with conn.cursor() as cursor:
            cursor.execute(sql)

            if sql_type in ("SELECT", "SHOW", "DESCRIBE", "EXPLAIN"):
                data = cursor.fetchall()
                data = mask_sensitive(list(data))
                elapsed = round((time.time() - start) * 1000, 2)
                return {
                    "ok": True,
                    "sql_type": sql_type,
                    "rows": len(data),
                    "execution_time_ms": elapsed,
                    "data": data,
                }
            else:
                conn.commit()
                elapsed = round((time.time() - start) * 1000, 2)
                return {
                    "ok": True,
                    "sql_type": sql_type,
                    "affected_rows": cursor.rowcount,
                    "execution_time_ms": elapsed,
                    "message": f"{sql_type} 执行成功，影响 {cursor.rowcount} 行",
                }
    except Exception as e:
        return {"ok": False, "error": f"MySQL 错误: {type(e).__name__}: {e}"}
    finally:
        conn.close()


def main():
    raw = read_input()
    if not raw:
        print("usage: python aios_db.py <params.json>")
        print("")
        print('JSON params: {"sql": "SELECT ...", "read_only": true}')
        sys.exit(1)

    try:
        params = json.loads(raw)
    except json.JSONDecodeError as e:
        print(f"[错误] JSON 解析失败: {e}")
        sys.exit(1)

    sql = params.get("sql", "").strip()
    if not sql:
        print("[错误] 参数必须包含 sql 字段")
        sys.exit(1)

    read_only = params.get("read_only", True)

    print(f"[信息] 数据库: {DB_CONFIG['host']}:{DB_CONFIG['port']}/{DB_CONFIG['database']}")
    print(f"[信息] SQL: {sql[:200]}{'...' if len(sql) > 200 else ''}")
    print(f"[信息] 模式: {'只读' if read_only else '读写'}")

    result = execute_sql(sql, read_only=read_only)
    print(json.dumps(result, ensure_ascii=False, default=str))


if __name__ == "__main__":
    main()

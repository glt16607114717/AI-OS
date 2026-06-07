import json
import sys
import os
import re

CONFIG_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "mysql_query_config.json")
DANGEROUS_KEYWORDS = ["DROP", "DELETE", "UPDATE", "INSERT", "ALTER", "CREATE", "TRUNCATE", "GRANT", "REVOKE"]


def read_input() -> str:
    for arg in sys.argv[1:]:
        if os.path.isfile(arg):
            with open(arg, "r", encoding="utf-8-sig") as f:
                return f.read().strip()
    return ""


def load_config(environment: str, database_group: str = "main") -> dict:
    with open(CONFIG_FILE, "r", encoding="utf-8") as f:
        all_config = json.load(f)
    group_config = all_config.get(database_group)
    if not group_config:
        print(f"[错误] 数据库分组不存在: {database_group}")
        print(f"可用分组: {', '.join(all_config.keys())}")
        sys.exit(1)
    env_config = group_config.get(environment)
    if not env_config:
        print(f"[错误] 环境不存在: {environment}")
        print(f"可用环境: {', '.join(group_config.keys())}")
        sys.exit(1)
    return env_config


def get_sql_type(sql: str) -> str:
    sql_stripped = sql.strip().upper()
    match = re.match(r"^(SELECT|SHOW|DESCRIBE|EXPLAIN|INSERT|UPDATE|DELETE|DROP|CREATE|ALTER|TRUNCATE|GRANT|REVOKE)", sql_stripped)
    return match.group(1) if match else "UNKNOWN"


def check_permissions(sql: str, read_only: bool):
    if not read_only:
        return
    sql_type = get_sql_type(sql)
    if sql_type in DANGEROUS_KEYWORDS:
        print(f"[错误] 当前环境为只读，不允许执行 {sql_type} 操作")
        sys.exit(1)


def execute_sql(config: dict, sql: str) -> dict:
    try:
        import pymysql
    except ImportError:
        print("[错误] 缺少 pymysql 库，请安装: pip install pymysql")
        sys.exit(1)
    connection = pymysql.connect(
        host=config["host"],
        port=config["port"],
        user=config["username"],
        password=config["password"],
        database=config["database"],
        charset="utf8mb4",
        cursorclass=pymysql.cursors.DictCursor,
    )
    try:
        import time
        start_time = time.time()
        with connection.cursor() as cursor:
            cursor.execute(sql)
            sql_type = get_sql_type(sql)
            if sql_type in ("SELECT", "SHOW", "DESCRIBE", "EXPLAIN"):
                data = cursor.fetchall()
            else:
                connection.commit()
                data = [{"affected_rows": cursor.rowcount, "message": "SQL 执行成功"}]
        execution_time = round((time.time() - start_time) * 1000, 2)
        return {
            "success": True,
            "database": config["database"],
            "sql_type": sql_type,
            "execution_time_ms": execution_time,
            "read_only": config.get("read_only", True),
            "data": data,
        }
    finally:
        connection.close()


def main():
    environment = None
    database_group = "main"
    params_file = None

    for arg in sys.argv[1:]:
        if arg in ("dev", "test", "gray", "prod"):
            environment = arg
        elif arg in ("main", "bi"):
            database_group = arg
        elif os.path.isfile(arg):
            params_file = arg

    if not environment:
        print("[error] environment required: dev/test/gray/prod")
        sys.exit(1)

    raw = read_input()
    if not raw:
        print("usage: python mysql_query.py <env> [main|bi] <params.json>")
        print("")
        print("JSON params: {\"sql\": \"SELECT ...\"}")
        sys.exit(1)

    try:
        params = json.loads(raw)
    except json.JSONDecodeError as e:
        print(f"[error] JSON parse error: {e}")
        sys.exit(1)

    sql = params.get("sql", "").strip()

    if not sql:
        print("[错误] JSON 参数必须包含 sql 字段")
        sys.exit(1)

    print(f"[信息] 环境: {environment} | 数据库分组: {database_group}")
    print(f"[信息] SQL: {sql[:200]}{'...' if len(sql) > 200 else ''}")

    config = load_config(environment, database_group)
    check_permissions(sql, config.get("read_only", True))

    result = execute_sql(config, sql)
    print(json.dumps(result, ensure_ascii=False, default=str))


if __name__ == "__main__":
    main()

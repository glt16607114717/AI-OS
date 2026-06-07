import json
import sys
import os
from datetime import datetime, date

CONFIG_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "mongodb.json")
WRITE_COMMANDS = [
    "INSERT", "UPDATE", "DELETE", "REPLACE",
    "CREATE", "DROP", "RENAME", "ALTER",
    "CREATEINDEX", "DROPINDEX", "CREATEROLE", "DROPROLE",
    "GRANTROLE", "REVOKE", "CREATEUSER", "DROPUSER",
]


class DateTimeEncoder(json.JSONEncoder):
    def default(self, obj):
        if isinstance(obj, datetime):
            return obj.strftime("%Y-%m-%d %H:%M:%S")
        if isinstance(obj, date):
            return obj.strftime("%Y-%m-%d")
        if hasattr(obj, '__str__'):
            return str(obj)
        return super().default(obj)


def read_input() -> str:
    for arg in sys.argv[1:]:
        if os.path.isfile(arg):
            with open(arg, "r", encoding="utf-8-sig") as f:
                return f.read().strip()
    return ""


def load_config(environment: str) -> dict:
    with open(CONFIG_FILE, "r", encoding="utf-8") as f:
        all_config = json.load(f)
    for group_name, group_config in all_config.items():
        env_config = group_config.get(environment)
        if env_config:
            return env_config
    print(f"[错误] 环境不存在: {environment}")
    sys.exit(1)


def check_permissions(command: dict, read_only: bool):
    if not read_only:
        return
    command_name = list(command.keys())[0].upper() if command else ""
    if command_name in WRITE_COMMANDS:
        print(f"[错误] 当前环境为只读，不允许执行 {command_name} 命令")
        sys.exit(1)


def execute_command(config: dict, command: dict) -> dict:
    try:
        from pymongo import MongoClient
        from urllib.parse import quote_plus
    except ImportError:
        print("[错误] 缺少 pymongo 库，请安装: uv pip install pymongo")
        sys.exit(1)
    username = quote_plus(config["username"])
    password = quote_plus(config["password"])
    auth_source = config.get("auth_source", "admin")
    uri = f"mongodb://{username}:{password}@{config['host']}:{config['port']}/{config['database']}?authSource={auth_source}"
    client = MongoClient(uri, serverSelectionTimeoutMS=5000)
    db = client[config["database"]]
    import time
    start_time = time.time()
    result = db.command(command)
    execution_time = round((time.time() - start_time) * 1000, 2)
    formatted_result = []
    if isinstance(result, dict):
        ok = result.get("ok", 0)
        if ok == 1:
            if "cursor" in result:
                first_batch = result["cursor"].get("firstBatch", [])
                formatted_result = first_batch
            elif "result" in result:
                formatted_result = result["result"]
            elif "value" in result:
                formatted_result = result["value"]
            elif "n" in result:
                formatted_result = [{"count": result["n"]}]
            else:
                clean = {k: v for k, v in result.items() if k != "ok"}
                formatted_result = [clean] if clean else [result]
        else:
            error_msg = result.get("errmsg", "未知错误")
            print(f"[错误] MongoDB 命令执行失败: {error_msg}")
            sys.exit(1)
    else:
        formatted_result = result
    client.close()
    return {
        "success": True,
        "database": config["database"],
        "execution_time_ms": execution_time,
        "data": formatted_result,
    }


def main():
    environment = None
    for arg in sys.argv[1:]:
        if arg in ("dev", "test", "gray", "prod"):
            environment = arg

    if not environment:
        print("[error] environment required: dev/test/gray/prod")
        sys.exit(1)

    raw = read_input()
    if not raw:
        print("usage: python mongo_query.py <env> <params.json>")
        print("")
        print("JSON params (MongoDB command):")
        print('  {"find": "api_request_log", "filter": {"status": 200}, "limit": 10}')
        sys.exit(1)

    try:
        command = json.loads(raw)
    except json.JSONDecodeError as e:
        print(f"[error] JSON parse error: {e}")
        sys.exit(1)

    if not command:
        print("[错误] MongoDB 命令为空")
        sys.exit(1)

    print(f"[信息] 环境: {environment}")

    config = load_config(environment)
    read_only = environment != "test"
    check_permissions(command, read_only)

    result = execute_command(config, command)
    print(json.dumps(result, ensure_ascii=False, cls=DateTimeEncoder))


if __name__ == "__main__":
    main()

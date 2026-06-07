"""
工具注册中心（工作台专用）

管理所有可被大模型调用的工具。
负责：工具定义（发给大模型）、参数校验、执行分发。

@author 桂良涛
"""

import json
import logging
from typing import Callable

from . import mysql_tool

logger = logging.getLogger("agent")

# ── 工具定义（OpenAI Function Calling 格式） ──

TOOL_DEFINITIONS = [
    {
        "type": "function",
        "function": {
            "name": "db_query",
            "description": "执行只读 SQL 查询（SELECT），返回查询结果。数据库：nnd_robot_test（测试环境）",
            "parameters": {
                "type": "object",
                "properties": {
                    "sql": {
                        "type": "string",
                        "description": "要执行的 SQL 语句（仅 SELECT）",
                    },
                },
                "required": ["sql"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "db_list_tables",
            "description": "列出数据库中所有表名",
            "parameters": {
                "type": "object",
                "properties": {},
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "db_describe_table",
            "description": "查看指定表的字段结构（字段名、类型、是否可空、键、默认值等）",
            "parameters": {
                "type": "object",
                "properties": {
                    "table": {
                        "type": "string",
                        "description": "表名",
                    },
                },
                "required": ["table"],
            },
        },
    },
]


# ── 工具执行分发 ──

def execute(name: str, arguments: str) -> str:
    """
    执行工具调用。

    @param name 工具名称
    @param arguments JSON 格式的参数字符串
    @return JSON 格式的执行结果字符串
    """
    try:
        args = json.loads(arguments) if arguments else {}
    except json.JSONDecodeError:
        return json.dumps({"ok": False, "error": f"参数解析失败: {arguments}"}, ensure_ascii=False)

    logger.info(f"[Tools] 执行工具: {name}({json.dumps(args, ensure_ascii=False)})")

    try:
        if name == "db_query":
            result = mysql_tool.execute(args.get("sql", ""))
        elif name == "db_list_tables":
            result = mysql_tool.list_tables()
        elif name == "db_describe_table":
            result = mysql_tool.describe_table(args.get("table", ""))
        else:
            result = {"ok": False, "error": f"未知工具: {name}"}
    except Exception as e:
        logger.error(f"[Tools] 工具执行异常: {name}: {e}")
        result = {"ok": False, "error": f"工具执行异常: {type(e).__name__}: {str(e)}"}

    # 大模型返回结果可能很大，截断
    result_str = json.dumps(result, ensure_ascii=False, default=str)
    if len(result_str) > 8000:
        result_str = result_str[:8000] + "...（结果已截断）"

    logger.info(f"[Tools] 工具结果: {name} -> {len(result_str)} 字符")
    return result_str

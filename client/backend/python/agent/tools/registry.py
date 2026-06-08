"""
工具注册中心（工作台专用）

管理所有可被大模型调用的工具。
负责：工具定义（发给大模型）、参数校验、执行分发。

目录结构：
  tools/base/    — 底层基础设施（MySQL 查询等），不对用户直接暴露
  tools/skills/  — 用户可调用的技能（skills.json），AI 优先匹配技能

@author 桂良涛
"""

import json
import logging
import os

from .base import mysql_tool

logger = logging.getLogger("agent")

# ── 技能加载 ──

_SKILLS: dict[str, dict] = {}  # skill_id → skill_config


def _load_skills():
    """从 tools/skills/skills.json 加载所有技能定义。"""
    global _SKILLS
    if _SKILLS:
        return
    skills_file = os.path.join(os.path.dirname(__file__), "skills", "skills.json")
    if not os.path.exists(skills_file):
        return
    try:
        with open(skills_file, "r", encoding="utf-8") as f:
            skills_list = json.load(f)
        for skill in skills_list:
            _SKILLS[skill["id"]] = skill
        logger.info("[Skills] 已加载 %d 个技能", len(_SKILLS))
    except Exception as e:
        logger.error("[Skills] 加载技能失败: %s", e)


def get_skill_list() -> list[dict]:
    """获取技能列表（供前端展示）。"""
    _load_skills()
    return [
        {
            "id": s["id"],
            "name": s["name"],
            "description": s["description"],
            "example_queries": s.get("example_queries", []),
        }
        for s in _SKILLS.values()
    ]


# ── 工具定义（OpenAI Function Calling 格式） ──

def _build_tool_definitions() -> list[dict]:
    """
    构建工具定义列表。
    只暴露技能给 AI，底层 db 工具不直接暴露。
    """
    tools = []

    # 动态加载技能作为工具
    _load_skills()
    for skill_id, skill in _SKILLS.items():
        tools.append({
            "type": "function",
            "function": {
                "name": f"skill_{skill_id}",
                "description": skill["description"],
                "parameters": {
                    "type": "object",
                    "properties": {},
                },
            },
        })

    return tools


# 延迟构建，首次访问时初始化
TOOL_DEFINITIONS = []


def get_tool_definitions() -> list[dict]:
    """获取工具定义列表（延迟初始化）。"""
    global TOOL_DEFINITIONS
    if not TOOL_DEFINITIONS:
        TOOL_DEFINITIONS = _build_tool_definitions()
    return TOOL_DEFINITIONS


# ── 技能执行 ──

def _execute_skill(skill_id: str, user_args: dict) -> dict:
    """执行技能：执行多条 SQL，返回所有结果 + AI 计算指引。"""
    _load_skills()
    skill = _SKILLS.get(skill_id)
    if not skill:
        return {"ok": False, "error": f"未知技能: {skill_id}"}

    queries = skill.get("queries", [])
    if not queries:
        return {"ok": False, "error": f"技能 {skill_id} 没有定义 queries"}

    # 执行每条 SQL
    results = {}
    for q in queries:
        sql = q["sql"]
        logger.info(f"[Skills] 执行 [{q['name']}]: {sql[:200]}")
        result = mysql_tool.execute(sql)
        if not result.get("ok"):
            return {"ok": False, "error": f"查询 {q['name']} 失败: {result.get('error')}"}
        results[q["name"]] = {
            "label": q["label"],
            "data": result.get("data", []),
            "rows": result.get("rows", 0),
        }

    # 组装返回：技能名 + 各查询结果 + AI 计算指引
    return {
        "ok": True,
        "skill": skill.get("name", skill_id),
        "query_results": results,
        "post_instruction": skill.get("post_instruction", ""),
    }


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
        if name.startswith("skill_"):
            skill_id = name[6:]  # 去掉 "skill_" 前缀
            result = _execute_skill(skill_id, args)
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

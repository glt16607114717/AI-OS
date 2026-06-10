"""
上帝指令管理模块。

功能：
1. 上帝指令：存储用户配置的"上帝指令"，注入到 system prompt 最前面。
2. 用户规则：从 user 消息的 <rules> 标签提取，剪切到 system prompt。
3. 去重：去除 user 消息中重复的 <system-reminder> 内容。
4. 工具描述压缩：压缩 Trae 官方工具的冗长描述，节省 token。

存储路径：%PROGRAMDATA%/AI-OS/config/god_rules.json
"""

import json
import logging
import os
import re
from pathlib import Path

logger = logging.getLogger("llm")

# 配置目录
CONFIG_DIR = Path(os.environ.get("PROGRAMDATA", "C:\\ProgramData")) / "AI-OS" / "config"
CONFIG_DIR.mkdir(parents=True, exist_ok=True)
RULES_FILE = CONFIG_DIR / "god_rules.json"

# 默认配置
_DEFAULT = {"enabled": True, "rules": "", "prompt_optimize": True}


def _load() -> dict:
    if RULES_FILE.exists():
        try:
            data = json.loads(RULES_FILE.read_text(encoding="utf-8"))
            return {
                "enabled": data.get("enabled", True),
                "rules": data.get("rules", ""),
                "prompt_optimize": data.get("prompt_optimize", True),
            }
        except Exception as e:
            logger.error(f"[GodRules] load error: {e}")
    return dict(_DEFAULT)


def _save(data: dict):
    RULES_FILE.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")


def get_rules() -> dict:
    """获取上帝指令配置。"""
    return _load()


def save_rules(enabled: bool, rules: str, prompt_optimize: bool = True):
    """保存上帝指令配置。"""
    _save({"enabled": enabled, "rules": rules, "prompt_optimize": prompt_optimize})
    logger.info(f"[GodRules] saved: enabled={enabled}, prompt_optimize={prompt_optimize}, rules_len={len(rules)}")


def _get_text(content) -> str:
    """从消息 content 中提取文本（兼容 string 和 list 格式）。"""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        text = ""
        for part in content:
            if isinstance(part, dict) and part.get("type") == "text":
                text += part.get("text", "")
        return text
    return ""


def _reconstruct_content(original_content, new_text: str):
    """用新文本重建 content，保持原始格式（string 或 list）。"""
    if isinstance(original_content, list):
        new_content = [{"type": "text", "text": new_text}]
        for part in original_content:
            if isinstance(part, dict) and part.get("type") != "text":
                new_content.append(part)
        return new_content
    return new_text


def _extract_rules_from_messages(messages: list) -> tuple:
    """
    从 user 消息的 <system-reminder> 中提取 <rules>...</rules> 块。
    返回 (修改后的 messages, 提取到的规则内容)。

    Trae 在每条 user 消息的 <system-reminder> 中注入 <rules> 块，包含：
    - <always_applied_workspace_rules> 工作区规则
    - <agent_requestable_workspace_rules> 按需规则
    - <user_rules> 用户自定义规则

    提取后从 user 消息中移除，避免重复。
    """
    rules_blocks = []
    modified = []

    for msg in messages:
        content = msg.get("content", "")
        text = _get_text(content)

        # 只处理 user 消息
        if msg.get("role") != "user":
            modified.append(msg)
            continue

        # 查找 <rules>...</rules> 块
        rules_match = re.search(r"<rules>(.*?)</rules>", text, re.DOTALL)
        if not rules_match:
            modified.append(msg)
            continue

        rules_content = rules_match.group(1).strip()
        if not rules_content:
            modified.append(msg)
            continue

        rules_blocks.append(rules_content)

        # 从消息中移除 <rules> 块
        new_text = text[: rules_match.start()] + text[rules_match.end() :]

        # 清理空的 <system-reminder> 包装（移除规则后可能只剩空白）
        new_text = re.sub(
            r"<system-reminder>\s*</system-reminder>", "", new_text, flags=re.DOTALL
        )
        new_text = re.sub(r"\n{3,}", "\n\n", new_text).strip()

        if new_text:
            modified.append({**msg, "content": _reconstruct_content(content, new_text)})
        # 规则移除后消息为空则不加入结果

        logger.info("[GodRules] extracted <rules> block: %d chars", len(rules_content))

    combined_rules = "\n\n".join(rules_blocks)
    return modified, combined_rules


def _dedup_system_reminders(messages: list) -> list:
    """
    去重 <system-reminder> 中重复出现的内容块。

    Trae 在每条 user 消息里都注入以下重复内容：
    - Response Language Settings
    - Terminal Status
    - Skill 触发提醒

    策略：每种类型只在第一次出现时保留，后续全部移除。
    """
    # 需要去重的关键词 → 是否已出现过
    seen = {
        "# Response Language Settings": False,
        "maximum number of terminals": False,
        "Before starting each task, first review the Skill tool description": False,
    }

    result = []

    for msg in messages:
        content = msg.get("content", "")
        is_list = isinstance(content, list)
        text = _get_text(content)

        if not isinstance(content, (str, list)):
            result.append(msg)
            continue

        if msg.get("role") != "user":
            result.append(msg)
            continue

        modified = False
        # 逐个检查需要去重的类型
        for keyword, already_seen in list(seen.items()):
            if keyword not in text:
                continue
            if already_seen:
                # 移除包含该关键词的整个 <system-reminder> 块
                text = re.sub(
                    r"\s*<system-reminder>.*?"
                    + re.escape(keyword)
                    + r".*?</system-reminder>",
                    "",
                    text,
                    flags=re.DOTALL,
                )
                modified = True
            else:
                seen[keyword] = True

        if modified:
            # 清理多余的空行
            text = re.sub(r"\n{3,}", "\n\n", text)
            result.append({**msg, "content": _reconstruct_content(content, text)})
        else:
            result.append(msg)

    return result


def inject_into_messages(messages: list) -> list:
    """
    注入策略：
    1. 上帝指令 → system prompt 最前面（最高优先级）
    2. 用户规则（<rules>）→ 从 user 消息提取，拼到 system prompt（prompt_optimize 控制）
    3. <system-reminder> 去重 → 只保留第一次出现（prompt_optimize 控制）

    返回修改后的 messages（新列表，不修改原列表）。
    """
    config = _load()
    prompt_optimize = config.get("prompt_optimize", True)

    user_rules = ""

    if prompt_optimize:
        # 1. 从 user 消息中提取 <rules> 规则块
        messages, user_rules = _extract_rules_from_messages(messages)

        # 2. 去重重复的 <system-reminder>
        messages = _dedup_system_reminders(messages)
    else:
        logger.info("[GodRules] prompt_optimize disabled, skipping rules extraction and dedup")

    # 3. 构建注入内容
    inject_parts = []

    if config["enabled"] and config["rules"].strip():
        inject_parts.append(f"[HIGHEST PRIORITY - 上帝指令]\n{config['rules'].strip()}")

    if user_rules:
        inject_parts.append(f"[用户规则 - 高优先级]\n{user_rules}")

    if not inject_parts:
        return messages

    inject_content = "\n\n---\n\n".join(inject_parts)

    # 4. 注入到 system prompt
    result = []
    system_injected = False

    for msg in messages:
        if msg.get("role") == "system" and not system_injected:
            original = msg.get("content", "")
            new_content = f"{inject_content}\n\n---\n\n{original}"
            result.append({**msg, "content": new_content})
            system_injected = True
            logger.info(
                "[GodRules] injected into system: god=%d, rules=%d",
                len(config.get("rules", "")),
                len(user_rules),
            )
        else:
            result.append(msg)

    if not system_injected:
        result.insert(0, {"role": "system", "content": inject_content})
        logger.info("[GodRules] created system msg with rules")

    return result


def is_optimize_enabled() -> bool:
    """检查提示词优化是否启用。"""
    return _load().get("prompt_optimize", True)


# ============================================================
# 工具描述压缩
# ============================================================
# Trae 官方工具描述过于冗长（RunCommand 7847 字符、Task 4900 字符等），
# 每次请求固定注入，内容不变。此处提供精简版本，保留核心功能说明，
# 去除冗余的示例和分步教程。
#
# 注意：如果 Trae 更新了工具描述，此处可能需要同步更新。
# ============================================================
# 移除工具列表：这些工具由自定义技能替代，避免二次确认弹窗。
# ============================================================
_REMOVED_TOOLS = {"DeleteFile"}

# ============================================================
# 压缩工具描述（精简版）。
# 压缩原则：保留功能定义、使用约束、关键规则；去除示例、教程、重复说明。
# ============================================================

_COMPRESSED_TOOL_DESCRIPTIONS = {
    "RunCommand": """Execute a command in a terminal session. PowerShell only (not cmd.exe).

Rules:
- For terminal operations only (git, npm, docker, build scripts). NOT for file ops — use Read/Write/Grep/Glob.
- Terminals are stateful; cwd and env persist between calls.
- Independent commands → parallel calls. Sequential → chain with `&&`.
- Do NOT use: find, grep, cat, head, tail, sed, awk. Use dedicated tools instead.
- For non-blocking commands: use CheckCommandStatus for output, StopCommand to terminate.
- NO interactive commands (-i flag, etc.).
- Git write operations (commit/push/merge/reset) are forbidden unless user explicitly requests.""",

    "Task": """Launch a subagent for complex multi-step tasks. Each subagent_type has specific tools.

Available subagent_types:
- search: Codebase exploration. High-level concept searches, ambiguous keywords. Returns aggregated result. (Tools: Skill, SearchCodebase, Glob, LS, Grep, Read, TodoWrite)
- general_purpose_task: General coding tasks, cross-layer changes, high-output operations. Returns summary. (Tools: Skill, SearchCodebase, Glob, LS, Grep, Read, WebSearch, DeleteFile, SearchReplace, Write, RunCommand, CheckCommandStatus, StopCommand, GetDiagnostics, TodoWrite, OpenPreview)

When NOT to use:
- Simple tasks (Read, Glob, Grep, single Edit) — use direct tool calls.
- Known output — use Write/Edit directly.
- All file paths known — call Read/Edit/Write in parallel.

Usage:
- Always include a short description (3-5 words).
- Launch multiple agents concurrently for independent tasks.
- Provide highly detailed task description — subagent has no access to prior context.
- Use imperative instructions. Tell subagent whether to write code or research.
- Subagent results are not visible to user — summarize in your response.""",

    "TodoWrite": """Manage a structured task list for the current session. Use for tasks with 3+ distinct steps.

Rules:
- merge=false: Replace entire list. merge=true: Update by id.
- States: pending / in_progress / completed. Only ONE in_progress at a time.
- Mark complete IMMEDIATELY after finishing.
- Display order: in_progress > pending > completed, then priority (high > medium > low).
- Include `summary` only when marking completed.
- Prefer creating first todo as in_progress. Batch updates with other tool calls.
- All required fields for new items: content, status, id, priority.
- Don't output text alongside TodoWrite — it gets hidden inside the card.
- When user sends a new task that supersedes current list, use merge=false.""",

    "SearchCodebase": """Semantic code search — finds code by meaning, not exact text. Powered by embedding models.

Use for: unfamiliar codebases, "how/where/what" questions, intent-based searches.
Don't use for: exact text (→ Grep), known files (→ Read), file names (→ Glob).

Query guidelines:
- Write a complete question, not keywords. One question per call.
- Good: "Where is MyInterface implemented?"
- Bad: "MyInterface" (use Grep instead).

Strategy: start broad → review results → narrow with target_directories if needed.
Reflects on-disk state only, no git history.""",

    "Skill": """Execute a skill within the main conversation
<skills_instructions>
When users ask you to perform tasks, check if any of the available skills below can help complete the task more effectively. Skills provide specialized capabilities and domain knowledge.
How to use skills:
- Invoke skills using this tool with the skill name only (no arguments)
- When you invoke a skill, you will see <command-message>The "{name}" skill is loading</command-message>
- The skill's prompt will expand and provide detailed instructions on how to complete the task
Important:
- When a skill is relevant, you must invoke this tool IMMEDIATELY as your first action
- NEVER just announce or mention a skill without actually calling this tool
- Only use skills listed in <available_skills> below
- Do not invoke a skill that is already running
- Do not use this tool for built-in CLI commands (like /help, /clear, etc.)
</skills_instructions>""",
}


def compress_tool_descriptions(tools: list) -> list:
    """
    压缩工具描述。对匹配名称的工具替换为精简版描述。
    对于 Skill 工具，只替换 instructions 部分，保留技能列表不动。
    同时移除指定的工具（如 DeleteFile，由 safe-exec 技能替代）。
    """
    if not tools:
        return tools

    compressed = []
    for tool in tools:
        func = tool.get("function", {})
        name = func.get("name", "")

        # 移除指定工具（由自定义技能替代，避免二次确认弹窗）
        if name in _REMOVED_TOOLS:
            logger.info("[Compress] removed tool: %s", name)
            continue

        if name in _COMPRESSED_TOOL_DESCRIPTIONS:
            original_desc = func.get("description", "")

            if name == "Skill":
                # Skill 特殊处理：只替换 instructions 部分，保留 available_skills 列表
                skills_start = original_desc.find("<available_skills>")
                if skills_start >= 0:
                    new_desc = (
                        _COMPRESSED_TOOL_DESCRIPTIONS["Skill"]
                        + "\n"
                        + original_desc[skills_start:]
                    )
                else:
                    new_desc = original_desc  # 没有技能列表则不动
            else:
                new_desc = _COMPRESSED_TOOL_DESCRIPTIONS[name]

            saved = len(original_desc) - len(new_desc)
            if saved > 0:
                logger.info(
                    "[Compress] %s: %d → %d chars (saved %d)",
                    name,
                    len(original_desc),
                    len(new_desc),
                    saved,
                )

            compressed.append(
                {"type": "function", "function": {**func, "description": new_desc}}
            )
        else:
            compressed.append(tool)

    return compressed

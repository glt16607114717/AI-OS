"""
RAG 蒸馏器：用 GLM-4-Flash 对 AI 回答进行知识蒸馏/压缩

流程：
1. 主模型回答完成后，将问答对发送给 GLM-4-Flash
2. GLM-4-Flash 提取核心知识点，生成精炼版本
3. 精炼版存入向量库（而非原始长回答）
4. 蒸馏失败时 fallback 存原文

蒸馏模型：智谱 GLM-4-Flash（永久免费、不限量、30并发）
@author 桂良涛
"""

import json
import logging
import os
import threading
import time
import urllib.request
from pathlib import Path

logger = logging.getLogger("agent")

# 蒸馏模型配置
DISTILL_MODEL = "glm-4-flash"
DISTILL_BASE_URL = "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions"
DISTILL_TIMEOUT = 30  # 秒

# 蒸馏提示词
DISTILL_SYSTEM_PROMPT = """你是一个知识蒸馏助手。你的任务是从问答对话中提取核心知识点。

核心原则：不要保留原文结构，用自己的话重新概括核心知识。

规则：
1. 不要保留原文结构，用自己的话重新概括核心知识
2. 去掉所有过程、代码片段、思考链、废话、格式符号
3. 去掉所有 HTML/XML 标签（如 <system-reminder>、<user_input> 等）
4. 去掉 markdown 格式符号（如 **、`、```、# 等）
5. 英文内容翻译为中文
6. 如果是数字/日期类的简单回答，直接保留原意即可
7. 用简洁的中文陈述句输出，不超过 300 字"""

DISTILL_USER_TEMPLATE = """请从以下最新的问答中提取核心知识点。

{history_block}---
【需要蒸馏的最新问答】
用户问题：{question}

AI 回答：{answer}

注意：上面的历史对话仅用于理解语境，你只需要从"最新问答"中提取知识。
请输出精炼后的知识点（不超过 300 字）："""


# API Key 缓存
_cached_api_key = None
_cache_time = 0


def _get_zhipu_api_key() -> str:
    """从配置文件读取智谱 API Key（带缓存）"""
    global _cached_api_key, _cache_time

    # 缓存 60 秒
    if _cached_api_key and (time.time() - _cache_time) < 60:
        return _cached_api_key

    try:
        keys_path = Path(os.environ.get("PROGRAMDATA", "C:\\ProgramData")) / "AI-OS" / "config" / "llm_keys.json"
        if not keys_path.exists():
            logger.warning("[蒸馏] llm_keys.json 不存在")
            return ""

        with open(keys_path, "r", encoding="utf-8") as f:
            data = json.load(f)

        zhipu_keys = data.get("keys", {}).get("zhipu", [])
        for k in zhipu_keys:
            if k.get("enabled", False) and k.get("api_key"):
                _cached_api_key = k["api_key"]
                _cache_time = time.time()
                return _cached_api_key

        logger.warning("[蒸馏] 未找到可用的智谱 API Key")
        return ""

    except Exception as e:
        logger.warning(f"[蒸馏] 读取 API Key 失败: {e}")
        return ""


def _build_history_block(messages: list[dict]) -> str:
    """从消息列表中构建最近5轮对话的语境文本"""
    if not messages:
        return ""

    # 提取 user/assistant 消息（去掉 system 和 tool）
    conversation = []
    for m in messages:
        role = m.get("role", "")
        if role not in ("user", "assistant"):
            continue
        content = m.get("content", "")
        if isinstance(content, list):
            content = " ".join(p.get("text", "") for p in content if isinstance(p, dict))
        content = str(content).strip()
        if not content:
            continue
        label = "用户" if role == "user" else "AI"
        conversation.append(f"{label}：{content}")

    # 取最后 10 条（约5轮），去掉最后一条（就是当前问答）
    history = conversation[-11:-1] if len(conversation) > 1 else []
    if not history:
        return ""

    # 每条截取 200 字，避免太长
    trimmed = []
    for h in history:
        trimmed.append(h[:200] + ("..." if len(h) > 200 else ""))

    return "【历史对话（仅供理解语境，不需要蒸馏）】\n" + "\n".join(trimmed) + "\n\n"


def _call_distill_model(question: str, answer: str, history_block: str = "") -> str:
    """
    同步调用 GLM-4-Flash 进行蒸馏。

    @param question 用户问题
    @param answer AI 回答
    @param history_block 历史对话语境文本
    @return 蒸馏后的文本，失败返回空字符串
    """
    api_key = _get_zhipu_api_key()
    if not api_key:
        return ""

    # 截断过长的输入（避免 token 超限）
    max_answer_len = 20000  # 约 1 万字中文
    if len(answer) > max_answer_len:
        answer = answer[:max_answer_len] + "...(截断)"

    user_msg = DISTILL_USER_TEMPLATE.format(
        history_block=history_block,
        question=question,
        answer=answer,
    )

    payload = json.dumps({
        "model": DISTILL_MODEL,
        "messages": [
            {"role": "system", "content": DISTILL_SYSTEM_PROMPT},
            {"role": "user", "content": user_msg},
        ],
        "temperature": 0.3,
        "max_tokens": 800,
    }).encode("utf-8")

    req = urllib.request.Request(
        DISTILL_BASE_URL,
        data=payload,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}",
        },
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=DISTILL_TIMEOUT) as resp:
            result = json.loads(resp.read().decode("utf-8"))
            content = result.get("choices", [{}])[0].get("message", {}).get("content", "")
            return content.strip() if content else ""
    except Exception as e:
        logger.warning(f"[蒸馏] GLM-4-Flash 调用失败: {e}")
        return ""


def distill_async(question: str, answer: str, callback, messages: list[dict] = None):
    """
    异步蒸馏：在后台线程调用蒸馏模型，完成后通过 callback 返回结果。

    @param question 用户问题
    @param answer AI 原始回答
    @param callback 回调函数 callback(distilled_text: str)
                     distilled_text 为空字符串表示蒸馏失败
    @param messages 完整对话历史（可选，用于构建语境）
    """
    history_block = _build_history_block(messages or [])

    def _worker():
        try:
            result = _call_distill_model(question, answer, history_block)
            if result:
                logger.info(f"[蒸馏] 成功: {len(answer)} 字 → {len(result)} 字")
            else:
                logger.info("[蒸馏] 失败，将 fallback 存原文")
            callback(result)
        except Exception as e:
            logger.warning(f"[蒸馏] 线程异常: {e}")
            callback("")

    thread = threading.Thread(target=_worker, daemon=True, name="rag-distill")
    thread.start()
    return thread

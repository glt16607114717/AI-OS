"""
RAG 增强器：所有 LLM 交互都走 RAG 增强

在每次大模型请求前：
1. 提取用户消息的关键内容
2. 在向量库中搜索相关知识
3. 将搜索结果作为上下文注入 messages

在每次大模型回答后：
1. 将有价值的回答存入向量库
2. 供未来的查询检索使用

@author 桂良涛
"""

import logging
import time

logger = logging.getLogger("agent")


def _extract_text(content) -> str:
    """从 content 中提取纯文本，兼容 string 和 list 格式"""
    if isinstance(content, list):
        text = " ".join(
            p.get("text", "") for p in content if isinstance(p, dict)
        ).strip()
    else:
        text = str(content).strip()

    # 从 Trae 代理消息中提取 <user_input> 标签内容
    import re
    m = re.search(r"<user_input>\s*(.*?)\s*</user_input>", text, re.DOTALL)
    if m:
        return m.group(1).strip()

    return text


def should_enhance(messages: list[dict]) -> bool:
    """
    判断是否需要 RAG 增强。
    - 消息太少（仅 system + 1条 user）时不增强（缺乏上下文）
    - 短问候语不增强
    """
    # 找最后一条用户消息
    user_msgs = [m for m in messages if m.get("role") == "user"]
    if not user_msgs:
        return False

    last_user = _extract_text(user_msgs[-1].get("content", ""))

    # 太短的消息不需要检索
    if len(last_user) < 6:
        return False

    return True


def enhance_messages(messages: list[dict], top_k: int = 3, min_similarity: float = 0.3) -> tuple[list[dict], list[dict]]:
    """
    RAG 增强：在用户消息之前注入检索到的相关上下文。

    @param messages 原始消息列表
    @param top_k 检索条数
    @param min_similarity 最低相似度阈值
    @return (增强后的消息列表, 引用列表)
    """
    if not should_enhance(messages):
        return messages, []

    try:
        from rag.vector_store import search

        # 提取用户消息
        user_msgs = [m for m in messages if m.get("role") == "user"]
        query = _extract_text(user_msgs[-1].get("content", ""))
        logger.info(f"[RAG] 增强: query='{query[:50]}'")

        # 向量检索（多取一些，然后按时间排序取最新）
        results = search(query, n_results=top_k * 3)

        if not results:
            return messages, []

        # 过滤低相似度结果
        relevant = [r for r in results if r["similarity"] >= min_similarity]
        if not relevant:
            return messages, []

        # 按时间倒序（最新的优先），取 top_k 条
        relevant.sort(key=lambda r: r["metadata"].get("timestamp", 0), reverse=True)
        relevant = relevant[:top_k]

        # 构建 RAG 上下文
        context_parts = []
        references = []
        for i, r in enumerate(relevant):
            source = r["metadata"].get("source", "unknown")
            sim = r["similarity"]
            text = r["text"]
            context_parts.append(f"[参考资料{i+1}] (来源:{source}, 相关度:{sim:.2f})\n{text}")
            # 截取前 200 字作为摘要
            preview = text[:200] + ("..." if len(text) > 200 else "")
            references.append({
                "index": i + 1,
                "source": source,
                "similarity": round(sim * 100, 1),
                "text": preview,
            })

        context_text = "\n\n".join(context_parts)

        # 在 system 消息后、用户消息前插入 RAG 上下文
        enhanced = []
        inserted = False
        for msg in messages:
            enhanced.append(msg)
            if msg.get("role") == "system" and not inserted:
                enhanced.append({
                    "role": "system",
                    "content": f"以下是来自知识库的相关参考信息，请结合这些信息回答用户问题：\n\n{context_text}\n\n如果参考资料与用户问题不相关，请忽略并直接回答。",
                })
                inserted = True

        logger.info(f"[RAG] 增强: query='{query[:30]}...' → {len(relevant)} 条参考资料注入")
        return enhanced, references

    except ImportError:
        # RAG 模块不可用，静默跳过
        return messages, []
    except Exception as e:
        logger.warning(f"[RAG] 增强失败，使用原始消息: {e}")
        return messages, []


def _do_store(text: str, source: str, user_msg: str):
    """实际写入向量库"""
    try:
        from rag.vector_store import store_texts
        store_texts(
            texts=[text],
            metadatas=[{
                "source": source,
                "type": "qa_pair",
                "timestamp": int(time.time()),
                "distilled": "蒸馏" in text[:10],
            }],
        )
        logger.info(f"[RAG] 存储问答: source={source}, user='{user_msg[:30]}...', len={len(text)}")
    except ImportError:
        pass
    except Exception as e:
        logger.warning(f"[RAG] 存储问答失败: {e}")


def store_assistant_response(user_msg: str, assistant_msg: str, source: str = "workspace", messages: list[dict] = None):
    """
    存储 assistant 的回答到向量库，供未来检索。
    先尝试用 GLM-4-Flash 蒸馏压缩，失败则存原文。

    @param user_msg 用户问题（可能是原始格式，需清洗）
    @param assistant_msg AI 回答
    @param source 来源标识（workspace / proxy）
    @param messages 完整对话历史（可选，蒸馏时用作语境）
    """
    if not assistant_msg or len(assistant_msg.strip()) < 20:
        return  # 太短的回答不存储

    # 清洗用户问题：提取 <user_input> 标签内容，去掉 list 格式和标签
    clean_msg = _extract_text(user_msg) if user_msg else ""

    try:
        from rag.distiller import distill_async

        def _on_distilled(distilled_text: str):
            if distilled_text:
                text = f"【蒸馏】问：{clean_msg}\n答：{distilled_text}"
            else:
                # fallback 存原文
                text = f"问：{clean_msg}\n答：{assistant_msg}"
            _do_store(text, source, clean_msg)

        distill_async(clean_msg, assistant_msg, _on_distilled, messages=messages)

    except ImportError:
        # 蒸馏模块不可用，直接存原文
        text = f"问：{clean_msg}\n答：{assistant_msg}"
        _do_store(text, source, clean_msg)
    except Exception as e:
        logger.warning(f"[RAG] 蒸馏调度失败，存原文: {e}")
        text = f"问：{clean_msg}\n答：{assistant_msg}"
        _do_store(text, source, clean_msg)

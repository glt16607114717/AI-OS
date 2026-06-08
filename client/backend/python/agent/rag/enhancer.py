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

    last_user = user_msgs[-1].get("content", "").strip()

    # 太短的消息不需要检索
    if len(last_user) < 6:
        return False

    return True


def enhance_messages(messages: list[dict], top_k: int = 3, min_similarity: float = 0.3) -> list[dict]:
    """
    RAG 增强：在用户消息之前注入检索到的相关上下文。

    @param messages 原始消息列表
    @param top_k 检索条数
    @param min_similarity 最低相似度阈值
    @return 增强后的消息列表（不修改原列表）
    """
    if not should_enhance(messages):
        return messages

    try:
        from rag.vector_store import search

        # 提取用户消息
        user_msgs = [m for m in messages if m.get("role") == "user"]
        query = user_msgs[-1].get("content", "").strip()

        # 向量检索
        results = search(query, n_results=top_k)

        if not results:
            return messages

        # 过滤低相似度结果
        relevant = [r for r in results if r["similarity"] >= min_similarity]
        if not relevant:
            return messages

        # 构建 RAG 上下文
        context_parts = []
        for i, r in enumerate(relevant):
            source = r["metadata"].get("source", "unknown")
            context_parts.append(f"[参考资料{i+1}] (来源:{source}, 相关度:{r['similarity']:.2f})\n{r['text']}")

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
        return enhanced

    except ImportError:
        # RAG 模块不可用，静默跳过
        return messages
    except Exception as e:
        logger.warning(f"[RAG] 增强失败，使用原始消息: {e}")
        return messages


def store_assistant_response(user_msg: str, assistant_msg: str, source: str = "workspace"):
    """
    存储 assistant 的回答到向量库，供未来检索。

    @param user_msg 用户问题
    @param assistant_msg AI 回答
    @param source 来源标识（workspace / proxy）
    """
    if not assistant_msg or len(assistant_msg.strip()) < 20:
        return  # 太短的回答不存储

    try:
        from rag.vector_store import store_texts

        # 将问答对合并为一条文本存储
        qa_text = f"问：{user_msg}\n答：{assistant_msg}"

        store_texts(
            texts=[qa_text],
            metadatas=[{
                "source": source,
                "type": "qa_pair",
                "timestamp": int(time.time()),
            }],
        )

        logger.info(f"[RAG] 存储问答: source={source}, user='{user_msg[:30]}...'")

    except ImportError:
        pass
    except Exception as e:
        logger.warning(f"[RAG] 存储问答失败: {e}")

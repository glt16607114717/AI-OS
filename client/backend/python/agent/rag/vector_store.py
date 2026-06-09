"""
ChromaDB 向量存储服务

使用 ChromaDB 做本地持久化向量存储，
配合 rag.embedding 做文本向量化。

@author 桂良涛
"""

import logging
import os

logger = logging.getLogger("agent")

AGENT_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CHROMA_DIR = os.path.join(os.environ.get("PROGRAMDATA", "C:\\ProgramData"), "AI-OS", "data", "chroma_db")

# 全局客户端
_client = None
_collection = None


def _get_or_create_collection():
    """获取或创建默认 collection"""
    global _client, _collection

    if _collection is not None:
        return _collection

    try:
        import chromadb

        os.makedirs(CHROMA_DIR, exist_ok=True)
        _client = chromadb.PersistentClient(path=CHROMA_DIR)

        # 使用自定义 embedding function（调用我们的 BGE-M3）
        _collection = _client.get_or_create_collection(
            name="aios_knowledge",
            metadata={"hnsw:space": "cosine"},
        )

        logger.info(f"[RAG] ChromaDB 初始化成功: {CHROMA_DIR}, 文档数={_collection.count()}")
        return _collection

    except ImportError:
        logger.warning("[RAG] chromadb 未安装，向量存储不可用")
        return None
    except Exception as e:
        logger.error(f"[RAG] ChromaDB 初始化失败: {e}")
        return None


def store_texts(texts: list[str], metadatas: list[dict] | None = None, ids: list[str] | None = None):
    """
    存储文本到向量库。

    @param texts 文本列表
    @param metadatas 元数据列表（如 [{"source": "workspace", "role": "assistant"}]）
    @param ids 唯一 ID 列表（不传则自动生成）
    """
    collection = _get_or_create_collection()
    if collection is None:
        return False

    try:
        from rag.embedding import embed_texts

        embeddings = embed_texts(texts)
        if embeddings is None:
            logger.warning("[RAG] 向量化失败，跳过存储")
            return False

        if ids is None:
            import uuid
            ids = [str(uuid.uuid4()) for _ in texts]

        if metadatas is None:
            metadatas = [{}] * len(texts)

        collection.add(
            ids=ids,
            embeddings=embeddings,
            documents=texts,
            metadatas=metadatas,
        )

        logger.info(f"[RAG] 存储 {len(texts)} 条文本到向量库")
        return True

    except Exception as e:
        logger.error(f"[RAG] 存储文本失败: {e}")
        return False


def search(query: str, n_results: int = 3, where: dict | None = None) -> list[dict]:
    """
    语义搜索：根据查询文本返回最相关的文档。

    @param query 查询文本
    @param n_results 返回条数
    @param where 元数据过滤条件
    @return [{"text": "...", "metadata": {...}, "distance": 0.23}, ...]
    """
    collection = _get_or_create_collection()
    if collection is None:
        return []

    try:
        from rag.embedding import embed_single

        query_embedding = embed_single(query)
        if query_embedding is None:
            return []

        kwargs = {
            "query_embeddings": [query_embedding],
            "n_results": n_results,
        }
        if where:
            kwargs["where"] = where

        results = collection.query(**kwargs)

        # 格式化结果
        documents = results.get("documents", [[]])[0]
        metadatas = results.get("metadatas", [[]])[0]
        distances = results.get("distances", [[]])[0]

        output = []
        for doc, meta, dist in zip(documents, metadatas, distances):
            output.append({
                "text": doc,
                "metadata": meta or {},
                "distance": dist,
                "similarity": round(1 - dist, 4),  # cosine distance → similarity
            })

        logger.info(f"[RAG] 搜索 '{query[:30]}...' → {len(output)} 条结果")
        return output

    except Exception as e:
        logger.error(f"[RAG] 搜索失败: {e}")
        return []


def get_collection_info() -> dict:
    """获取向量库信息"""
    collection = _get_or_create_collection()
    if collection is None:
        return {"ok": False, "error": "ChromaDB 未初始化"}

    return {
        "ok": True,
        "count": collection.count(),
        "name": collection.name,
        "path": CHROMA_DIR,
    }


def get_all_documents(limit: int = 1000, offset: int = 0) -> dict:
    """获取向量库全部文档（分页）"""
    collection = _get_or_create_collection()
    if collection is None:
        return {"ok": False, "error": "ChromaDB 未初始化", "documents": [], "total": 0}

    try:
        total = collection.count()
        results = collection.get(
            include=["documents", "metadatas"],
            limit=limit,
            offset=offset,
        )
        documents = []
        for doc_id, doc_text, meta in zip(
            results.get("ids", []),
            results.get("documents", []),
            results.get("metadatas", []),
        ):
            documents.append({
                "id": doc_id,
                "text": doc_text,
                "metadata": meta or {},
            })
        # 按时间倒序：最新的在前面
        documents.sort(key=lambda d: d["metadata"].get("timestamp", 0), reverse=True)
        return {"ok": True, "documents": documents, "total": total}
    except Exception as e:
        logger.error(f"[RAG] 获取文档列表失败: {e}")
        return {"ok": False, "error": str(e), "documents": [], "total": 0}


def reset_collection():
    """清空向量库"""
    global _client, _collection

    try:
        if _client is not None:
            _client.delete_collection("aios_knowledge")
            logger.info("[RAG] 向量库已清空")
        _collection = None
        _get_or_create_collection()  # 重新创建空库
        return True
    except Exception as e:
        logger.error(f"[RAG] 清空向量库失败: {e}")
        return False

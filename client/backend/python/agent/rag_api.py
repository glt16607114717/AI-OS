"""
RAG 管理 API

提供：
- rag_status：环境检测（模型、依赖、向量库状态）
- rag_search：手动测试语义搜索
- rag_reset：清空向量库

@author 桂良涛
"""

import logging
from fastapi import APIRouter, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel

logger = logging.getLogger("agent")

router = APIRouter(tags=["rag"])


class RagActionRequest(BaseModel):
    action: str
    payload: dict = {}


@router.post("/api/rag")
async def rag_action(req: RagActionRequest):
    action = req.action
    payload = req.payload

    if action == "rag_status":
        return _get_rag_status()

    elif action == "rag_search":
        query = payload.get("query", "").strip()
        top_k = payload.get("top_k", 3)
        if not query:
            return {"ok": False, "error": "查询内容不能为空"}

        try:
            from rag.vector_store import search
            results = search(query, n_results=top_k)
            return {"ok": True, "results": results}
        except Exception as e:
            return {"ok": False, "error": str(e)}

    elif action == "rag_store":
        texts = payload.get("texts", [])
        source = payload.get("source", "manual")
        if not texts:
            return {"ok": False, "error": "文本不能为空"}

        try:
            from rag.vector_store import store_texts
            ok = store_texts(texts, metadatas=[{"source": source}] * len(texts))
            return {"ok": ok}
        except Exception as e:
            return {"ok": False, "error": str(e)}

    elif action == "rag_reset":
        try:
            from rag.vector_store import reset_collection
            ok = reset_collection()
            return {"ok": ok}
        except Exception as e:
            return {"ok": False, "error": str(e)}

    elif action == "rag_test_embed":
        """测试向量化功能"""
        text = payload.get("text", "测试文本")
        try:
            from rag.embedding import embed_single
            vec = embed_single(text)
            if vec is None:
                return {"ok": False, "error": "向量化失败"}
            return {"ok": True, "dimension": len(vec), "preview": vec[:5]}
        except Exception as e:
            return {"ok": False, "error": str(e)}

    else:
        return {"ok": False, "error": f"Unknown action: {action}"}


def _get_rag_status() -> dict:
    """获取 RAG 环境完整状态"""
    status = {
        "ok": True,
        "components": {},
    }

    # 1. 检查 Python 依赖
    deps = {}
    for dep in ["onnxruntime", "chromadb", "transformers", "numpy"]:
        try:
            mod = __import__(dep)
            version = getattr(mod, "__version__", "unknown")
            deps[dep] = {"installed": True, "version": version}
        except ImportError:
            deps[dep] = {"installed": False, "version": None}
    status["components"]["dependencies"] = deps

    # 2. 检查模型文件
    try:
        from rag.embedding import get_status as embed_status
        status["components"]["model"] = embed_status()
    except Exception as e:
        status["components"]["model"] = {"model_ready": False, "error": str(e)}

    # 3. 检查向量库
    try:
        from rag.vector_store import get_collection_info
        status["components"]["vector_store"] = get_collection_info()
    except Exception as e:
        status["components"]["vector_store"] = {"ok": False, "error": str(e)}

    # 4. 总体就绪状态
    all_deps_ok = all(d["installed"] for d in deps.values())
    model_ready = status["components"].get("model", {}).get("model_ready", False)
    vector_ok = status["components"].get("vector_store", {}).get("ok", False)

    status["ready"] = all_deps_ok and model_ready and vector_ok

    # 5. 模型下载指引
    status["download_guide"] = {
        "model_dir": "backend/python/agent/models/bge-m3-onnx-int8/",
        "files": [
            {"name": "model_quantized.onnx", "size": "558MB", "required": True},
            {"name": "tokenizer.json", "size": "17MB", "required": True},
            {"name": "tokenizer_config.json", "size": "1KB", "required": True},
            {"name": "sentencepiece.bpe.model", "size": "5MB", "required": True},
            {"name": "config.json", "size": "<1KB", "required": False},
            {"name": "special_tokens_map.json", "size": "<1KB", "required": False},
        ],
        "download_urls": {
            "official": "https://huggingface.co/gpahal/bge-m3-onnx-int8/tree/main",
            "mirror": "https://hf-mirror.com/gpahal/bge-m3-onnx-int8/tree/main",
            "direct_files": {
                "model_quantized.onnx": "https://hf-mirror.com/gpahal/bge-m3-onnx-int8/resolve/main/model_quantized.onnx",
                "tokenizer.json": "https://hf-mirror.com/gpahal/bge-m3-onnx-int8/resolve/main/tokenizer.json",
                "tokenizer_config.json": "https://hf-mirror.com/gpahal/bge-m3-onnx-int8/resolve/main/tokenizer_config.json",
                "sentencepiece.bpe.model": "https://hf-mirror.com/gpahal/bge-m3-onnx-int8/resolve/main/sentencepiece.bpe.model",
                "config.json": "https://hf-mirror.com/gpahal/bge-m3-onnx-int8/resolve/main/config.json",
                "special_tokens_map.json": "https://hf-mirror.com/gpahal/bge-m3-onnx-int8/resolve/main/special_tokens_map.json",
            },
        },
        "pip_deps": ["onnxruntime", "chromadb", "transformers", "numpy"],
    }

    return status

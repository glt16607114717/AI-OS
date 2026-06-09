"""
BGE-M3 ONNX int8 本地向量化服务

使用 onnxruntime 加载量化后的 BGE-M3 模型，
将文本转为 1024 维向量（稠密检索），
无需 GPU、无需 PyTorch、无需云端 API。

@author 桂良涛
"""

import logging
import os
import numpy as np

logger = logging.getLogger("agent")

# ── 模型路径 ──
# 小文件（tokenizer、config）打包在安装目录
# 大文件（onnx）放在 ProgramData（卸载不删）
AGENT_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
_INSTALL_MODEL_DIR = os.path.join(AGENT_DIR, "models", "bge-m3-onnx-int8")
_PROGRAMDATA_MODEL_DIR = os.path.join(
    os.environ.get("PROGRAMDATA", "C:\\ProgramData"),
    "AI-OS", "models", "bge-m3-onnx-int8"
)


def _find_file(filename: str) -> str | None:
    """在安装目录和 ProgramData 中查找文件，优先安装目录"""
    install_path = os.path.join(_INSTALL_MODEL_DIR, filename)
    if os.path.exists(install_path):
        return install_path
    pd_path = os.path.join(_PROGRAMDATA_MODEL_DIR, filename)
    if os.path.exists(pd_path):
        return pd_path
    return None


def _get_model_dir_for(filename: str) -> str:
    """获取文件所在目录"""
    path = _find_file(filename)
    if path:
        return os.path.dirname(path)
    # 默认返回 ProgramData（大文件下载位置）
    return _PROGRAMDATA_MODEL_DIR


# ── 全局单例 ──

_session = None
_tokenizer = None


def _check_model_files() -> dict:
    """检查模型文件是否齐全，返回缺失文件列表"""
    required = [
        "model_quantized.onnx",
        "tokenizer.json",
        "tokenizer_config.json",
        "sentencepiece.bpe.model",
    ]
    missing = []
    found = []
    for f in required:
        path = _find_file(f)
        if path:
            size = os.path.getsize(path)
            found.append({"file": f, "size_mb": round(size / 1024 / 1024, 1)})
        else:
            missing.append(f)
    return {
        "found": found,
        "missing": missing,
        "model_dir": _PROGRAMDATA_MODEL_DIR,
        "install_dir": _INSTALL_MODEL_DIR,
    }


def is_model_ready() -> bool:
    """模型文件是否就绪"""
    status = _check_model_files()
    return len(status["missing"]) == 0


def _load_model():
    """懒加载 ONNX Runtime 会话和分词器"""
    global _session, _tokenizer

    if _session is not None:
        return True

    if not is_model_ready():
        logger.warning("[RAG] 模型文件不完整，跳过加载")
        return False

    try:
        import onnxruntime as ort

        model_path = _find_file("model_quantized.onnx")

        # CPU 优化选项
        sess_options = ort.SessionOptions()
        sess_options.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL
        sess_options.intra_op_num_threads = 4
        sess_options.inter_op_num_threads = 2

        _session = ort.InferenceSession(model_path, sess_options)
        logger.info(f"[RAG] ONNX 模型加载成功: {model_path}")

        # 加载分词器（用 transformers 的 tokenizer，轻量）
        from transformers import AutoTokenizer
        tokenizer_dir = _get_model_dir_for("tokenizer.json")
        _tokenizer = AutoTokenizer.from_pretrained(tokenizer_dir)
        logger.info("[RAG] Tokenizer 加载成功")

        return True

    except ImportError as e:
        logger.warning(f"[RAG] 依赖缺失，跳过模型加载: {e}")
        return False
    except Exception as e:
        logger.error(f"[RAG] 模型加载失败: {e}")
        _session = None
        _tokenizer = None
        return False


def _mean_pooling(last_hidden_state: np.ndarray, attention_mask: np.ndarray) -> np.ndarray:
    """Mean Pooling：根据 attention_mask 对 hidden_state 做加权平均"""
    mask_expanded = np.expand_dims(attention_mask, -1).astype(np.float32)
    sum_embeddings = np.sum(last_hidden_state * mask_expanded, axis=1)
    sum_mask = np.clip(mask_expanded.sum(axis=1), a_min=1e-9, a_max=None)
    return sum_embeddings / sum_mask


def embed_texts(texts: list[str], batch_size: int = 16) -> list[list[float]] | None:
    """
    将一组文本转为向量。

    @param texts 文本列表
    @param batch_size 批处理大小
    @return 向量列表（每个 1024 维），失败返回 None
    """
    if not _load_model():
        return None

    try:
        all_embeddings = []

        for i in range(0, len(texts), batch_size):
            batch = texts[i:i + batch_size]

            # 分词
            encoded = _tokenizer(
                batch,
                padding=True,
                truncation=True,
                max_length=512,
                return_tensors="np",
            )

            # ONNX 推理
            inputs = {
                "input_ids": encoded["input_ids"].astype(np.int64),
                "attention_mask": encoded["attention_mask"].astype(np.int64),
            }
            # 有些模型还需要 token_type_ids
            if "token_type_ids" in encoded:
                inputs["token_type_ids"] = encoded["token_type_ids"].astype(np.int64)

            outputs = _session.run(None, inputs)
            last_hidden = outputs[0]  # (batch, seq_len, hidden_dim)

            # Mean Pooling + L2 归一化
            embeddings = _mean_pooling(last_hidden, encoded["attention_mask"].astype(np.float32))
            norms = np.linalg.norm(embeddings, axis=1, keepdims=True)
            embeddings = embeddings / np.clip(norms, a_min=1e-9, a_max=None)

            all_embeddings.extend(embeddings.tolist())

        return all_embeddings

    except Exception as e:
        logger.error(f"[RAG] 向量化失败: {e}")
        return None


def embed_single(text: str) -> list[float] | None:
    """单条文本向量化"""
    result = embed_texts([text])
    return result[0] if result else None


def get_status() -> dict:
    """获取 RAG embedding 服务状态"""
    files_status = _check_model_files()
    return {
        "model_ready": is_model_ready(),
        "model_loaded": _session is not None,
        "model_dir": _PROGRAMDATA_MODEL_DIR,
        "install_dir": _INSTALL_MODEL_DIR,
        "files": files_status,
    }

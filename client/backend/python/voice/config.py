"""
语音助手配置管理模块
管理 voice_config.json 的读写及路径定义

作者：桂良涛，邮箱：桂良涛@nndrobot.com
"""

import json
import os
from pathlib import Path

# ── 路径定义 ──────────────────────────────────────────────
_BASE_DIR = Path(os.environ.get("AI_OS_BASE_DIR", r"C:\ProgramData\AI-OS"))
CONFIG_FILE = _BASE_DIR / "config" / "voice_config.json"
LOG_DIR = _BASE_DIR / "logs"
MODEL_SEARCH_DIRS = [
    _BASE_DIR / "models" / "vosk",
    Path(__file__).parent.parent / "models",
]


def load_config() -> dict:
    """加载语音配置，文件不存在时返回默认值。"""
    if CONFIG_FILE.exists():
        try:
            return json.loads(CONFIG_FILE.read_text(encoding="utf-8"))
        except Exception:
            pass
    return {"enabled": False, "commands": []}


def save_config(cfg: dict) -> None:
    """保存语音配置到文件。"""
    CONFIG_FILE.parent.mkdir(parents=True, exist_ok=True)
    CONFIG_FILE.write_text(
        json.dumps(cfg, ensure_ascii=False, indent=2), encoding="utf-8"
    )
    # 确保普通用户可写
    _ensure_writable(CONFIG_FILE)
    _ensure_writable(CONFIG_FILE.parent)


def _ensure_writable(path: Path) -> None:
    """确保文件/目录对 Users 组可写（Windows only）。"""
    if os.name != "nt":
        return
    try:
        import subprocess
        target = str(path)
        subprocess.run(
            ["icacls", target, "/grant", "Users:(M)", "/c"],
            capture_output=True, timeout=5,
            creationflags=0x08000000,  # CREATE_NO_WINDOW，防止闪黑框
        )
    except Exception:
        pass


def find_vosk_model() -> str | None:
    """
    搜索 Vosk 中文模型目录。
    优先匹配标准模型（vosk-model-cn），其次小模型（vosk-model-small-cn）。
    验证目录包含 am/final.mdl。
    """
    # 优先匹配的前缀列表（标准模型 > 小模型）
    prefixes = ["vosk-model-cn-0.2", "vosk-model-cn-kaldi", "vosk-model-small-cn"]
    for base in MODEL_SEARCH_DIRS:
        if not base.exists():
            continue
        for entry in sorted(base.iterdir()):
            if not entry.is_dir():
                continue
            for prefix in prefixes:
                if entry.name.startswith(prefix):
                    if (entry / "am" / "final.mdl").exists():
                        return str(entry)
    return None


VOSK_MODEL_URLS = [
    "https://alphacephei.com/vosk/models/vosk-model-small-cn-0.22.zip",
    "https://github.com/alphacep/vosk-api/releases/download/v0.3.42/vosk-model-small-cn-0.22.zip",
]
MODEL_DIR = _BASE_DIR / "models" / "vosk"


def download_vosk_model(on_progress=None) -> str:
    """
    下载 Vosk 中文模型并解压。依次尝试多个下载源。
    on_progress: callback(downloaded, total) 用于进度回调
    返回模型目录路径
    """
    import urllib.request
    import zipfile

    existing = find_vosk_model()
    if existing:
        return existing

    MODEL_DIR.mkdir(parents=True, exist_ok=True)
    zip_path = MODEL_DIR / "vosk-model-small-cn-0.22.zip"

    last_error = None
    for url in VOSK_MODEL_URLS:
        try:
            urllib.request.urlretrieve(
                url,
                str(zip_path),
                reporthook=lambda block, block_size, total_size: (
                    on_progress(block * block_size, total_size) if on_progress and total_size > 0 else None
                ),
            )
            last_error = None
            break
        except Exception as e:
            last_error = e
            continue

    if last_error and not zip_path.exists():
        raise RuntimeError(f"所有下载源均失败: {last_error}")

    # 解压模型
    with zipfile.ZipFile(str(zip_path), "r") as zf:
        zf.extractall(str(MODEL_DIR))

    # 清理 zip 文件
    zip_path.unlink(missing_ok=True)

    result = find_vosk_model()
    if not result:
        raise RuntimeError("模型解压失败")
    return result

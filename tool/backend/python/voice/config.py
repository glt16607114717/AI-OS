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

# FunASR 模型缓存目录
MODELSCOPE_CACHE = _BASE_DIR / "models" / "funasr"

# Paraformer 模型名称（ModelScope 标识符）
PARAFORMER_MODEL = "paraformer-zh"


def setup_model_env() -> None:
    """设置 ModelScope 缓存路径环境变量（必须在 import funasr 之前调用）。"""
    MODELSCOPE_CACHE.mkdir(parents=True, exist_ok=True)
    os.environ["MODELSCOPE_CACHE"] = str(MODELSCOPE_CACHE)


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
            creationflags=0x08000000,
        )
    except Exception:
        pass


def find_asr_model() -> str | None:
    """
    检查 FunASR Paraformer 模型是否已下载且完整。
    必须同时存在 model.pt（权重）和 config.yaml（配置）才算完整。
    返回模型目录路径，未下载或残缺返回 None。
    """
    # 关键文件：缺任一个都算残缺
    required_files = ["model.pt", "config.yaml", "am.mvn"]

    def _check_dir(dir_path: Path) -> bool:
        if not dir_path.exists():
            return False
        return all((dir_path / f).is_file() and (dir_path / f).stat().st_size > 0
                   for f in required_files)

    # 优先检查自定义缓存目录
    cache_dir = MODELSCOPE_CACHE / "iic"
    if cache_dir.exists():
        for entry in cache_dir.iterdir():
            if entry.is_dir() and "paraformer" in entry.name.lower() and _check_dir(entry):
                return str(entry)

    # 兜底：检查 ModelScope 默认缓存（用户目录）
    default_cache = Path.home() / ".cache" / "modelscope" / "hub" / "iic"
    if default_cache.exists():
        for entry in default_cache.iterdir():
            if entry.is_dir() and "paraformer" in entry.name.lower() and _check_dir(entry):
                return str(entry)

    return None


def is_model_ready() -> bool:
    """检查 FunASR 模型是否已下载就绪。"""
    return find_asr_model() is not None


def download_asr_model(on_progress=None) -> str:
    """
    下载 FunASR Paraformer 语音识别模型。
    使用 modelscope SDK 从国内 CDN 下载到 MODELSCOPE_CACHE 目录。
    on_progress(percent, message) 回调用于报告进度（percent: 0-100）。
    返回模型目录路径。
    """
    setup_model_env()

    model_id = "iic/speech_paraformer-large_asr_nat-zh-cn-16k-common-vocab8404-pytorch"
    target_dir = MODELSCOPE_CACHE / "iic" / "speech_paraformer-large_asr_nat-zh-cn-16k-common-vocab8404-pytorch"

    def _report(pct, msg):
        if on_progress:
            try:
                on_progress(pct, msg)
            except Exception:
                pass

    _report(1, "正在连接 ModelScope ...")

    try:
        from modelscope.hub.snapshot_download import snapshot_download
    except Exception as e:
        raise RuntimeError(f"modelscope 未安装: {e}")

    _report(5, "开始下载模型文件 ...")

    # modelscope 的 snapshot_download 不直接提供进度回调，
    # 用 local_dir 模式下载（文件直接落到目标目录），通过 hooks 估算进度
    # 9 个文件，按文件数分摊进度（5% ~ 95%）
    try:
        # 尝试用 HookCallbacks（较新版本 modelscope 支持）
        from modelscope.hub.api import HubApi
        from modelscope.utils.logger import get_logger

        class _ProgressHook:
            def __init__(self):
                self.total = 9  # 该模型的文件数
                self.done = 0

            def __call__(self, *args, **kwargs):
                # modelscope 内部 hook 签名不固定，用宽松处理
                pass

        # 简单可靠的方式：直接 snapshot_download，它在下载过程中会输出日志
        # 我们通过监听目标目录文件数来估算进度
        import threading

        stop_monitor = threading.Event()

        def monitor():
            import time
            known_files = {
                "config.yaml", "configuration.json", "am.mvn", "model.pt",
                "README.md", "seg_dict", "tokens.json",
            }
            while not stop_monitor.is_set():
                try:
                    existing = set()
                    if target_dir.exists():
                        for f in target_dir.rglob("*"):
                            if f.is_file():
                                existing.add(f.name)
                    done = len(existing & known_files)
                    pct = 5 + int(done / len(known_files) * 90)
                    _report(min(pct, 95), f"下载中（{done}/{len(known_files)} 文件）...")
                except Exception:
                    pass
                time.sleep(2)

        mon_thread = threading.Thread(target=monitor, daemon=True)
        mon_thread.start()

        try:
            snapshot_download(model_id, local_dir=str(target_dir))
        finally:
            stop_monitor.set()

        _report(100, "下载完成")

    except Exception as e:
        raise RuntimeError(f"模型下载失败（modelscope）: {e}")

    result = find_asr_model()
    if not result:
        raise RuntimeError("FunASR 模型下载失败：文件未就位")
    return result

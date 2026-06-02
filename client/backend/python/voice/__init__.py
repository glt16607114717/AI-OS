"""
语音助手模块
使用 Vosk 离线中文语音识别 + sounddevice 采集音频，实时匹配预配置指令并模拟鼠标点击。

对外接口：
    init_voice()                      初始化，若 enabled 则自动启动
    start_voice() / stop_voice()      启动/停止监听
    get_status()                      返回运行状态
    set_enabled(bool)                 开关
    add_command(phrase)               添加指令
    update_command(index, data)       更新指令
    remove_command(index)             删除指令
    start_calibration(index)          开始标定（空格确认位置）
    cancel_calibration()              取消标定

作者：桂良涛，邮箱：桂良涛@nndrobot.com
"""

import json
import threading
import time
import ctypes
from datetime import datetime

from .config import load_config, save_config, find_vosk_model, LOG_DIR

# ── 模块状态 ──────────────────────────────────────────────
_voice_thread: threading.Thread | None = None
_stop_event = threading.Event()
_running = False
_calibrating = False
_calibration_index = -1
_mouse_pos = (0, 0)

# ── 日志 ──────────────────────────────────────────────────

def log(msg: str, tag: str = "VOICE") -> None:
    ts = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    line = f"[{ts}] [{tag}] {msg}"
    print(line)
    try:
        LOG_DIR.mkdir(parents=True, exist_ok=True)
        log_file = LOG_DIR / "voice.log"
        with open(str(log_file), "a", encoding="utf-8") as f:
            f.write(line + "\n")
    except Exception:
        pass

# ── 鼠标点击 ──────────────────────────────────────────────

def _click_point(x, y) -> None:
    user32 = ctypes.windll.user32
    ix, iy = int(x), int(y)
    user32.SetCursorPos(ix, iy)
    time.sleep(0.1)
    MOUSEEVENTF_LEFTDOWN = 0x0002
    MOUSEEVENTF_LEFTUP = 0x0004
    user32.mouse_event(MOUSEEVENTF_LEFTDOWN, 0, 0, 0, 0)
    time.sleep(0.03)
    user32.mouse_event(MOUSEEVENTF_LEFTUP, 0, 0, 0, 0)
    time.sleep(0.2)

# ── 语音监听主循环（sounddevice 回调模式）──────────────────

def _voice_loop() -> None:
    global _running

    model_path = find_vosk_model()
    if not model_path:
        log("Vosk 中文模型未找到，语音唤醒不可用", "ERROR")
        return

    try:
        from vosk import Model, KaldiRecognizer
    except ImportError:
        log("vosk 未安装，语音唤醒不可用", "ERROR")
        return

    try:
        import sounddevice as sd
    except ImportError:
        log("sounddevice 未安装，语音唤醒不可用", "ERROR")
        return

    try:
        model = Model(model_path)
        rec = KaldiRecognizer(model, 16000)
        rec.SetWords(True)
    except Exception as e:
        log(f"Vosk 模型加载失败: {e}", "ERROR")
        return

    # 使用队列在线程间传递音频数据
    import queue
    audio_queue: queue.Queue[bytes | None] = queue.Queue()

    def _audio_callback(indata, frames, time_info, status):
        """sounddevice 输入流回调，将音频数据送入队列。"""
        if status:
            log(f"音频状态: {status}", "VOICE")
        audio_queue.put(bytes(indata))

    try:
        stream = sd.RawInputStream(
            samplerate=16000,
            blocksize=4000,
            dtype="int16",
            channels=1,
            callback=_audio_callback,
        )
        stream.start()
    except Exception as e:
        log(f"音频设备初始化失败: {e}", "ERROR")
        return

    log("语音指令监听启动（sounddevice）", "VOICE")
    _running = True
    read_count = 0
    recognize_count = 0

    try:
        while not _stop_event.is_set():
            try:
                data = audio_queue.get(timeout=0.5)
            except Exception:
                # queue.get 超时，继续循环检查 _stop_event
                continue

            if data is None:
                break

            read_count += 1
            if read_count % 50 == 1:
                log(f"语音监听中... 已读取{read_count}帧, 识别{recognize_count}次", "VOICE")

            # 每次循环热加载配置
            try:
                cfg = load_config()
            except Exception:
                cfg = {"enabled": True, "commands": []}

            commands = cfg.get("commands", [])
            enabled_cmds = [
                c for c in commands
                if c.get("enabled", True) and c.get("phrase") and c.get("position")
            ]
            if not enabled_cmds:
                continue

            recognize_count += 1
            detected_text = ""
            try:
                if rec.AcceptWaveform(data):
                    result = json.loads(rec.Result())
                    text = result.get("text", "").strip()
                    if text:
                        log(f"语音识别: {text}", "VOICE")
                        detected_text = text
            except Exception as e:
                log(f"识别异常: {e}", "VOICE")
                continue

            if detected_text:
                for cmd in commands:
                    if not cmd.get("enabled", True):
                        continue
                    phrase = cmd.get("phrase", "")
                    pos = cmd.get("position")
                    if not phrase or not pos or len(pos) != 2:
                        continue
                    if phrase in detected_text:
                        log(f"语音指令 [{phrase}] -> 点击 ({pos[0]},{pos[1]})", "VOICE")
                        _click_point(pos[0], pos[1])
    except Exception as e:
        log(f"语音线程异常退出: {e}", "ERROR")
    finally:
        try:
            stream.stop()
            stream.close()
        except Exception:
            pass
        _running = False
        log("语音指令监听已停止", "VOICE")

# ── 标定 ──────────────────────────────────────────────────

def _calibration_mouse_tracker() -> None:
    global _mouse_pos, _calibrating
    user32 = ctypes.windll.user32
    VK_SPACE = 0x20
    prev = False
    while _calibrating and not _stop_event.is_set():
        try:
            pt = ctypes.wintypes.POINT()
            user32.GetCursorPos(ctypes.byref(pt))
            _mouse_pos = (pt.x, pt.y)
            cur = bool(user32.GetAsyncKeyState(VK_SPACE) & 0x8000)
            if cur and not prev:
                cfg = load_config()
                commands = cfg.get("commands", [])
                if 0 <= _calibration_index < len(commands):
                    commands[_calibration_index]["position"] = list(_mouse_pos)
                    cfg["commands"] = commands
                    save_config(cfg)
                    log(
                        f"标定完成: 指令[{commands[_calibration_index]['phrase']}] "
                        f"-> ({_mouse_pos[0]},{_mouse_pos[1]})",
                        "VOICE",
                    )
                _calibrating = False
                return
            prev = cur
        except Exception:
            pass
        time.sleep(0.02)


def start_calibration(index: int) -> None:
    """开始标定指定指令的鼠标位置，按空格键确认。"""
    global _calibrating, _calibration_index
    _calibrating = True
    _calibration_index = index
    threading.Thread(target=_calibration_mouse_tracker, daemon=True).start()


def cancel_calibration() -> None:
    """取消当前标定。"""
    global _calibrating
    _calibrating = False

# ── 启动 / 停止 ──────────────────────────────────────────

def start_voice() -> bool:
    """启动语音监听线程。"""
    global _voice_thread
    if _voice_thread is not None and _voice_thread.is_alive():
        return True
    _stop_event.clear()
    _voice_thread = threading.Thread(target=_voice_loop, daemon=True, name="VoiceThread")
    _voice_thread.start()
    return True


def stop_voice() -> None:
    """停止语音监听线程。"""
    global _running
    _stop_event.set()
    if _voice_thread is not None:
        _voice_thread.join(timeout=5)
    _running = False
    log("语音指令监听已停止", "VOICE")

# ── 状态查询 ──────────────────────────────────────────────

def get_status() -> dict:
    """返回语音模块当前状态。"""
    cfg = load_config()
    return {
        "listening": _running,
        "model_ready": find_vosk_model() is not None,
        "enabled": cfg.get("enabled", False),
        "commands": cfg.get("commands", []),
        "calibrating": _calibrating,
        "calibration_index": _calibration_index,
        "mouse_pos": list(_mouse_pos),
    }

# ── 开关 ──────────────────────────────────────────────────

def set_enabled(enabled: bool) -> None:
    """设置语音模块开关。"""
    cfg = load_config()
    cfg["enabled"] = enabled
    save_config(cfg)
    log(f"语音开关: {'开启' if enabled else '关闭'}", "VOICE")
    if enabled:
        start_voice()
    else:
        stop_voice()

# ── 指令 CRUD ─────────────────────────────────────────────

def add_command(phrase: str) -> int:
    """添加一条语音指令，返回索引。"""
    cfg = load_config()
    commands = cfg.get("commands", [])
    commands.append({"phrase": phrase, "position": None, "enabled": True})
    cfg["commands"] = commands
    save_config(cfg)
    return len(commands) - 1


def update_command(index: int, data: dict) -> None:
    """更新指定索引的指令。"""
    cfg = load_config()
    commands = cfg.get("commands", [])
    if 0 <= index < len(commands):
        commands[index].update(data)
        cfg["commands"] = commands
        save_config(cfg)


def remove_command(index: int) -> None:
    """删除指定索引的指令。"""
    cfg = load_config()
    commands = cfg.get("commands", [])
    if 0 <= index < len(commands):
        commands.pop(index)
        cfg["commands"] = commands
        save_config(cfg)

# ── 初始化入口 ────────────────────────────────────────────

def init_voice() -> None:
    """初始化语音模块，若配置 enabled=True 则自动启动监听。"""
    cfg = load_config()
    model_path = find_vosk_model()
    log(
        f"语音模块初始化: enabled={cfg.get('enabled', False)}, "
        f"model={'已安装' if model_path else '未安装'}, "
        f"commands={len(cfg.get('commands', []))}个",
        "VOICE",
    )
    if cfg.get("enabled", False):
        start_voice()

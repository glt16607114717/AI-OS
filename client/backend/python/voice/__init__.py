"""
语音助手模块
使用 Vosk 离线中文语音识别 + sounddevice 采集音频，实时匹配预配置指令并回放操作序列。

支持两种指令模式：
  1. 标定模式（position）：单点标定，语音唤醒后单击目标坐标
  2. 录制模式（actions）：键鼠录制序列，语音唤醒后回放完整操作链

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
    start_recording(index)            开始键鼠录制（F9 停止）
    stop_recording()                  停止录制并返回 actions
    get_recognize_log()               获取识别日志
    clear_recognize_log()             清空识别日志

作者：桂杨涛，邮箱：guiyang@nndrobot.com
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

# 录制状态
_recording = False
_recording_index = -1
_recording_actions: list[dict] = []
_recording_start_time = 0.0
_recording_lock = threading.Lock()

# 识别日志（内存，最多保留 100 条）
_recognize_log: list[dict] = []
_RECOGNIZE_LOG_MAX = 100

# ── 日志 ──────────────────────────────────────────────────

import logging
_logger = logging.getLogger("voice")

def log(msg: str, tag: str = "VOICE") -> None:
    _logger.info(f"[{tag}] {msg}")

# ── SendInput 基础设施（替代已弃用的 mouse_event / keybd_event）──────

INPUT_MOUSE = 0
INPUT_KEYBOARD = 1

MOUSEEVENTF_MOVE = 0x0001
MOUSEEVENTF_LEFTDOWN = 0x0002
MOUSEEVENTF_LEFTUP = 0x0004
MOUSEEVENTF_RIGHTDOWN = 0x0008
MOUSEEVENTF_RIGHTUP = 0x0010
MOUSEEVENTF_MIDDLEDOWN = 0x0020
MOUSEEVENTF_MIDDLEUP = 0x0040
MOUSEEVENTF_WHEEL = 0x0800

KEYEVENTF_KEYUP = 0x0002


class _MOUSEINPUT(ctypes.Structure):
    _fields_ = [
        ("dx", ctypes.c_long),
        ("dy", ctypes.c_long),
        ("mouseData", ctypes.c_ulong),
        ("dwFlags", ctypes.c_ulong),
        ("time", ctypes.c_ulong),
        ("dwExtraInfo", ctypes.POINTER(ctypes.c_ulong)),
    ]


class _KEYBDINPUT(ctypes.Structure):
    _fields_ = [
        ("wVk", ctypes.c_ushort),
        ("wScan", ctypes.c_ushort),
        ("dwFlags", ctypes.c_ulong),
        ("time", ctypes.c_ulong),
        ("dwExtraInfo", ctypes.POINTER(ctypes.c_ulong)),
    ]


class _INPUT_UNION(ctypes.Union):
    _fields_ = [("ki", _KEYBDINPUT), ("mi", _MOUSEINPUT)]


class _INPUT(ctypes.Structure):
    _fields_ = [("type", ctypes.c_ulong), ("union", _INPUT_UNION)]


_user32 = ctypes.windll.user32
_user32.SendInput.argtypes = [ctypes.c_uint, ctypes.POINTER(_INPUT), ctypes.c_int]
_user32.SendInput.restype = ctypes.c_uint

_extra_info = ctypes.pointer(ctypes.c_ulong(0))


def _send_mouse(flags: int, delta: int = 0) -> None:
    inp = _INPUT()
    inp.type = INPUT_MOUSE
    inp.union.mi = _MOUSEINPUT(0, 0, delta, flags, 0, _extra_info)
    _user32.SendInput(1, ctypes.byref(inp), ctypes.sizeof(_INPUT))


def _send_key(vk: int, flags: int = 0) -> None:
    inp = _INPUT()
    inp.type = INPUT_KEYBOARD
    inp.union.ki = _KEYBDINPUT(vk, 0, flags, 0, _extra_info)
    _user32.SendInput(1, ctypes.byref(inp), ctypes.sizeof(_INPUT))


# ── 操作回放 ──────────────────────────────────────────────

def _click_point(x, y) -> None:
    """单击指定坐标（标定模式）。"""
    _user32.SetCursorPos(int(x), int(y))
    time.sleep(0.1)
    _send_mouse(MOUSEEVENTF_LEFTDOWN)
    time.sleep(0.03)
    _send_mouse(MOUSEEVENTF_LEFTUP)
    time.sleep(0.2)


def _play_actions(actions: list[dict]) -> None:
    """回放操作序列。"""
    for action in actions:
        atype = action.get("type", "")

        delay_ms = action.get("ms", 0)
        if delay_ms > 0:
            time.sleep(min(delay_ms, 2000) / 1000.0)

        if atype == "mouse_move":
            _user32.SetCursorPos(int(action["x"]), int(action["y"]))
            time.sleep(0.02)
        elif atype == "click":
            _user32.SetCursorPos(int(action["x"]), int(action["y"]))
            time.sleep(0.02)
            button = action.get("button", "left")
            if button == "left":
                _send_mouse(MOUSEEVENTF_LEFTDOWN)
                time.sleep(0.03)
                _send_mouse(MOUSEEVENTF_LEFTUP)
            elif button == "right":
                _send_mouse(MOUSEEVENTF_RIGHTDOWN)
                time.sleep(0.03)
                _send_mouse(MOUSEEVENTF_RIGHTUP)
            elif button == "middle":
                _send_mouse(MOUSEEVENTF_MIDDLEDOWN)
                time.sleep(0.03)
                _send_mouse(MOUSEEVENTF_MIDDLEUP)
            time.sleep(0.05)
        elif atype == "scroll":
            _send_mouse(MOUSEEVENTF_WHEEL, delta=int(action.get("delta", 0)))
            time.sleep(0.05)
        elif atype == "key_press":
            vk = action.get("vk", 0)
            _send_key(vk)
            time.sleep(0.02)
            _send_key(vk, KEYEVENTF_KEYUP)
            time.sleep(0.02)
        elif atype == "key_down":
            _send_key(action.get("vk", 0))
        elif atype == "key_up":
            _send_key(action.get("vk", 0), KEYEVENTF_KEYUP)
            time.sleep(0.01)
        elif atype == "type_text":
            text = action.get("text", "")
            for ch in text:
                vk = _char_to_vk(ch)
                if vk:
                    shift = _needs_shift(ch)
                    if shift:
                        _send_key(0x10)
                    _send_key(vk)
                    time.sleep(0.01)
                    _send_key(vk, KEYEVENTF_KEYUP)
                    if shift:
                        _send_key(0x10, KEYEVENTF_KEYUP)
                    time.sleep(0.01)
        elif atype == "delay":
            time.sleep(action.get("ms", 100) / 1000.0)

    log(f"操作回放完成: {len(actions)} 步", "VOICE")


def _char_to_vk(ch: str) -> int:
    """将单个 ASCII 字符映射到 Windows 虚拟键码。"""
    code = ord(ch.upper())
    if 0x41 <= code <= 0x5A:  # A-Z
        return code
    if 0x30 <= code <= 0x39:  # 0-9
        return code
    return 0


def _needs_shift(ch: str) -> bool:
    """判断输入该字符是否需要按 Shift。"""
    return ch.isupper() or ch in '~!@#$%^&*()_+{}|:"<>?'


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

    import queue
    audio_queue: queue.Queue[bytes | None] = queue.Queue()

    def _audio_callback(indata, frames, time_info, status):
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
                continue

            if data is None:
                break

            read_count += 1
            if read_count % 50 == 1:
                log(f"语音监听中... 已读取{read_count}帧, 识别{recognize_count}次", "VOICE")

            try:
                cfg = load_config()
            except Exception:
                cfg = {"enabled": True, "commands": []}

            commands = cfg.get("commands", [])
            # 过滤出有效指令：必须有 phrase，且至少有 position 或 actions
            enabled_cmds = []
            for c in commands:
                if not c.get("enabled", True):
                    continue
                if not c.get("phrase"):
                    continue
                has_pos = c.get("position") and len(c.get("position")) == 2
                has_actions = bool(c.get("actions"))
                if has_pos or has_actions:
                    enabled_cmds.append(c)

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
                # 去掉空格后再匹配（Vosk 可能在词之间插入空格）
                normalized_text = detected_text.replace(" ", "")
                matched = False
                for cmd in enabled_cmds:
                    raw_phrase = cmd.get("phrase", "")
                    # 支持逗号分隔多个触发词
                    keys = [k.replace(" ", "") for k in raw_phrase.replace("，", ",").split(",") if k.strip()]
                    matched_key = None
                    for key in keys:
                        if key and key in normalized_text:
                            matched_key = key
                            break
                    if matched_key:
                        _add_recognize_log(detected_text, matched_key, True)
                        matched = True
                        # 录制模式：有 actions 则回放操作序列
                        if cmd.get("actions"):
                            actions = cmd["actions"]
                            log(f"语音指令 [{matched_key}] -> 回放 {len(actions)} 步操作", "VOICE")
                            _play_actions(actions)
                        # 标定模式：有 position 则单击
                        elif cmd.get("position"):
                            pos = cmd["position"]
                            log(f"语音指令 [{matched_key}] -> 点击 ({pos[0]},{pos[1]})", "VOICE")
                            _click_point(pos[0], pos[1])
                if not matched:
                    _add_recognize_log(detected_text, "", False)
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

# ── 标定（旧模式，保留兼容）──────────────────────────────

def _calibration_mouse_tracker() -> None:
    global _mouse_pos, _calibrating
    user32 = ctypes.windll.user32
    user32.GetAsyncKeyState.restype = ctypes.c_short
    user32.GetAsyncKeyState.argtypes = [ctypes.c_int]
    VK_SPACE = 0x20
    prev = False
    loop_count = 0
    log("标定线程已启动", "VOICE")
    while _calibrating and not _stop_event.is_set():
        try:
            pt = ctypes.wintypes.POINT()
            user32.GetCursorPos(ctypes.byref(pt))
            _mouse_pos = (pt.x, pt.y)
            result = user32.GetAsyncKeyState(VK_SPACE)
            cur = bool(result & 0x8000)
            loop_count += 1
            if cur and not prev:
                log(f"空格按下! pos=({pt.x},{pt.y}), raw={result}", "VOICE")
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
        except Exception as e:
            log(f"标定异常: {e}", "ERROR")
        time.sleep(0.02)


def start_calibration(index: int) -> None:
    """开始标定指定指令的鼠标位置，按空格键确认。"""
    global _calibrating, _calibration_index
    _calibrating = True
    _calibration_index = index
    log(f"start_calibration called, index={index}, _calibrating={_calibrating}", "VOICE")
    threading.Thread(target=_calibration_mouse_tracker, daemon=True).start()
    log("calibration thread started", "VOICE")


def cancel_calibration() -> None:
    """取消当前标定。"""
    global _calibrating
    _calibrating = False

# ── 键鼠录制（ctypes 轮询模式）────────────────────────────
# 使用 ctypes 轮询 GetAsyncKeyState + GetCursorPos，
# 这两个 API 在任何 Session 都能读到当前活动桌面的输入状态。

# 需要监听的虚拟键码范围（字母键 + 数字键 + 常用功能键）
_MONITORED_VKS = list(range(0x08, 0x10))  # Backspace..Tab, Clear, Enter
_MONITORED_VKS += [0x10, 0x11, 0x12]       # Shift, Ctrl, Alt
_MONITORED_VKS += list(range(0x20, 0x2F))  # Space..Help
_MONITORED_VKS += list(range(0x30, 0x3A))  # 0-9
_MONITORED_VKS += list(range(0x41, 0x5B))  # A-Z
_MONITORED_VKS += list(range(0x60, 0x70))  # Numpad 0-9, *, +, Enter, -
_MONITORED_VKS += list(range(0x70, 0x78))  # F1-F8（F9 不记录）
_MONITORED_VKS += [0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF]  # ; = , - . /
_MONITORED_VKS += [0xC0, 0xDB, 0xDC, 0xDD, 0xDE]          # ` [ \ ] '

# VK 名称映射（用于前端展示）
_VK_NAMES = {
    0x08: "Backspace", 0x09: "Tab", 0x0D: "Enter", 0x10: "Shift",
    0x11: "Ctrl", 0x12: "Alt", 0x13: "Pause", 0x14: "CapsLock",
    0x1B: "Esc", 0x20: "Space", 0x21: "PageUp", 0x22: "PageDown",
    0x23: "End", 0x24: "Home", 0x25: "Left", 0x26: "Up",
    0x27: "Right", 0x28: "Down", 0x2D: "Insert", 0x2E: "Delete",
    0x70: "F1", 0x71: "F2", 0x72: "F3", 0x73: "F4",
    0x74: "F5", 0x75: "F6", 0x76: "F7", 0x77: "F8",
    0x90: "NumLock",
}


def _vk_name(vk: int) -> str:
    """获取虚拟键码的可读名称。"""
    if vk in _VK_NAMES:
        return _VK_NAMES[vk]
    if 0x41 <= vk <= 0x5A:
        return chr(vk)
    if 0x30 <= vk <= 0x39:
        return chr(vk)
    if 0x60 <= vk <= 0x69:
        return f"Num{vk - 0x60}"
    return f"VK({vk})"


def _recording_loop() -> None:
    """
    使用 ctypes 轮询录制键鼠操作。
    F9 停止录制且不记录 F9 事件。
    """
    global _recording, _recording_actions, _recording_start_time

    user32 = ctypes.windll.user32
    user32.GetAsyncKeyState.restype = ctypes.c_short
    user32.GetAsyncKeyState.argtypes = [ctypes.c_int]
    VK_F9 = 0x78
    VK_LBUTTON = 0x01
    VK_RBUTTON = 0x02
    VK_MBUTTON = 0x04

    _recording_start_time = time.time()
    last_mouse = (0, 0)
    last_kb_state = {}
    last_mouse_state = {VK_LBUTTON: False, VK_RBUTTON: False, VK_MBUTTON: False}

    log("录制开始（F9 停止）", "VOICE")

    try:
        while _recording and not _stop_event.is_set():
            elapsed = int((time.time() - _recording_start_time) * 1000)

            # ── 检测 F9 停止（上升沿）──
            f9_cur = bool(user32.GetAsyncKeyState(VK_F9) & 0x8000)
            f9_prev = last_kb_state.get(VK_F9, False)
            if f9_cur and not f9_prev:
                log("F9 停止录制", "VOICE")
                _recording = False
                break
            last_kb_state[VK_F9] = f9_cur

            # ── 检测鼠标位置变化 ──
            pt = ctypes.wintypes.POINT()
            user32.GetCursorPos(ctypes.byref(pt))
            cur_mouse = (pt.x, pt.y)
            if cur_mouse != last_mouse:
                with _recording_lock:
                    _recording_actions.append({
                        "type": "mouse_move", "x": cur_mouse[0], "y": cur_mouse[1], "t": elapsed
                    })
                last_mouse = cur_mouse

            # ── 检测鼠标按键（上升沿 = 按下，下降沿 = 释放）──
            for vk_btn, btn_name in [(VK_LBUTTON, "left"), (VK_RBUTTON, "right"), (VK_MBUTTON, "middle")]:
                cur = bool(user32.GetAsyncKeyState(vk_btn) & 0x8000)
                prev = last_mouse_state[vk_btn]
                if cur and not prev:
                    # 按下
                    with _recording_lock:
                        _recording_actions.append({
                            "type": "click", "x": last_mouse[0], "y": last_mouse[1],
                            "button": btn_name, "pressed": True, "t": elapsed
                        })
                elif not cur and prev:
                    # 释放
                    with _recording_lock:
                        _recording_actions.append({
                            "type": "click_release", "x": last_mouse[0], "y": last_mouse[1],
                            "button": btn_name, "pressed": False, "t": elapsed
                        })
                last_mouse_state[vk_btn] = cur

            # ── 检测键盘按键（上升沿/下降沿）──
            for vk in _MONITORED_VKS:
                cur = bool(user32.GetAsyncKeyState(vk) & 0x8000)
                prev = last_kb_state.get(vk, False)
                if cur and not prev:
                    name = _vk_name(vk)
                    with _recording_lock:
                        _recording_actions.append({
                            "type": "key_down", "vk": vk,
                            "key_name": name, "t": elapsed
                        })
                elif not cur and prev:
                    name = _vk_name(vk)
                    with _recording_lock:
                        _recording_actions.append({
                            "type": "key_up", "vk": vk,
                            "key_name": name, "t": elapsed
                        })
                last_kb_state[vk] = cur

            time.sleep(0.015)  # ~60Hz 轮询

    except Exception as e:
        log(f"录制异常: {e}", "ERROR")
    finally:
        _recording = False
        log(f"录制结束，共 {_get_action_count()} 个事件", "VOICE")


def _get_action_count() -> int:
    with _recording_lock:
        return len(_recording_actions)


def _simplify_actions(raw_actions: list[dict]) -> list[dict]:
    """
    精简原始录制数据：
    1. 去掉过于密集的 mouse_move 事件（采样间隔 < 30ms 的只保留最后一个）
    2. 将 click + click_release 合并为一个 click 事件
    3. 计算步骤间的相对延时
    """
    if not raw_actions:
        return []

    result = []
    last_move_time = -1000
    last_time = raw_actions[0].get("t", 0) if raw_actions else 0

    i = 0
    while i < len(raw_actions):
        a = raw_actions[i]
        atype = a.get("type", "")
        t = a.get("t", 0)
        delay = t - last_time if result else 0

        if atype == "mouse_move":
            # 降采样：间隔 < 30ms 的 move 跳过
            if t - last_move_time < 30:
                i += 1
                continue
            last_move_time = t
            result.append({"type": "mouse_move", "x": a["x"], "y": a["y"], "ms": delay})
            last_time = t

        elif atype == "click":
            # 找对应的 click_release，合并为单个 click
            press_x, press_y, btn = a["x"], a["y"], a.get("button", "left")
            found_release = False
            for j in range(i + 1, min(i + 20, len(raw_actions))):
                ra = raw_actions[j]
                if ra.get("type") == "click_release" and ra.get("button") == btn:
                    found_release = True
                    break
            result.append({"type": "click", "x": press_x, "y": press_y, "button": btn, "ms": delay})
            last_time = t

        elif atype == "click_release":
            # 已被 click 处理，跳过
            pass

        elif atype in ("key_down", "key_up"):
            result.append({
                "type": atype, "vk": a.get("vk", 0),
                "key_name": a.get("key_name", ""), "ms": delay
            })
            last_time = t

        elif atype == "scroll":
            result.append({"type": "scroll", "x": a["x"], "y": a["y"], "delta": a.get("delta", 0), "ms": delay})
            last_time = t

        i += 1

    return result


def start_recording(index: int) -> dict:
    """
    开始键鼠录制。返回初始状态。
    录制过程中用户按 F9 停止。
    """
    global _recording, _recording_index, _recording_actions, _recording_start_time

    if _recording:
        return {"ok": False, "error": "已有录制正在进行"}

    _recording = True
    _recording_index = index
    _recording_actions = []

    threading.Thread(target=_recording_loop, daemon=True, name="RecordingThread").start()
    return {"ok": True, "message": "录制已开始，按 F9 停止"}


def stop_recording() -> dict:
    """
    停止录制并返回精简后的操作序列。
    无论 _recording 当前状态如何（可能已被 F9 提前停止），都返回已录制的 actions。
    """
    global _recording

    was_recording = _recording
    _recording = False

    if was_recording:
        time.sleep(0.3)  # 等待录制线程处理完最后一个事件

    with _recording_lock:
        raw = list(_recording_actions)

    if not raw:
        return {"ok": False, "error": "录制结果为空"}

    simplified = _simplify_actions(raw)
    log(f"录制完成: 原始 {len(raw)} 事件 -> 精简 {len(simplified)} 步", "VOICE")

    return {"ok": True, "actions": simplified, "raw_count": len(raw)}


def save_recorded_actions(index: int, actions: list[dict]) -> None:
    """将录制的操作序列保存到指定指令。"""
    cfg = load_config()
    commands = cfg.get("commands", [])
    if 0 <= index < len(commands):
        commands[index]["actions"] = actions
        # 清除旧的 position（切换到录制模式）
        commands[index].pop("position", None)
        cfg["commands"] = commands
        save_config(cfg)
        log(f"指令 [{commands[index].get('phrase', '')}] 已保存 {len(actions)} 步操作", "VOICE")


def get_recording_status() -> dict:
    """获取当前录制状态。"""
    with _recording_lock:
        count = len(_recording_actions)
    return {
        "recording": _recording,
        "action_count": count,
        "index": _recording_index,
    }

# ── 识别日志 ──────────────────────────────────────────────

def _add_recognize_log(text: str, matched_phrase: str, matched: bool) -> None:
    """添加一条识别日志到内存。"""
    global _recognize_log
    entry = {
        "time": datetime.now().strftime("%H:%M:%S"),
        "text": text,
        "matched": matched,
        "phrase": matched_phrase,
    }
    _recognize_log.append(entry)
    if len(_recognize_log) > _RECOGNIZE_LOG_MAX:
        _recognize_log = _recognize_log[-_RECOGNIZE_LOG_MAX:]


def get_recognize_log() -> list[dict]:
    """获取识别日志。"""
    return list(_recognize_log)


def clear_recognize_log() -> None:
    """清空识别日志。"""
    global _recognize_log
    _recognize_log = []

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
    with _recording_lock:
        rec_count = len(_recording_actions)
    return {
        "listening": _running,
        "model_ready": find_vosk_model() is not None,
        "enabled": cfg.get("enabled", False),
        "commands": cfg.get("commands", []),
        "calibrating": _calibrating,
        "calibration_index": _calibration_index,
        "mouse_pos": list(_mouse_pos),
        "recording": _recording,
        "recording_action_count": rec_count,
        "recording_index": _recording_index,
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
    commands.append({"phrase": phrase, "position": None, "actions": None, "enabled": True})
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

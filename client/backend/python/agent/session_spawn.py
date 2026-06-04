"""
Session 0 -> Session 1 进程创建工具

WinSW 服务以 LocalSystem 身份运行在 Session 0，
无法读取用户桌面（Session 1+）的键鼠输入。
本模块通过 CreateProcessAsUser API 在用户会话中创建子进程，
使语音助手 worker 能正常访问 GetAsyncKeyState 等用户桌面 API。

原理：
  1. 枚举所有进程，找到 explorer.exe（必定跑在 Session 1 用户桌面）
  2. 打开 explorer.exe 的进程令牌，复制为可用的主令牌
  3. 用 CreateProcessAsUser 在该令牌的 Session 中创建子进程

作者：桂良涛，邮箱：桂良涛@nndrobot.com
"""

import ctypes
import ctypes.wintypes as wintypes
import logging
import os
import subprocess

logger = logging.getLogger("ai-os-agent")

# ── Windows API 常量 ─────────────────────────────────────
TOKEN_ALL_ACCESS = 0x000F01FF
MAXIMUM_ALLOWED = 0x02000000
CREATE_NO_WINDOW = 0x08000000
DUPLICATE_SAME_ACCESS = 0x00000002
TokenPrimary = 1
PROCESS_QUERY_INFORMATION = 0x0400
PROCESS_VM_READ = 0x0010

# ── ctypes 结构体 ────────────────────────────────────────

class STARTUPINFOW(ctypes.Structure):
    _fields_ = [
        ("cb", wintypes.DWORD),
        ("lpReserved", wintypes.LPWSTR),
        ("lpDesktop", wintypes.LPWSTR),
        ("lpTitle", wintypes.LPWSTR),
        ("dwX", wintypes.DWORD),
        ("dwY", wintypes.DWORD),
        ("dwXSize", wintypes.DWORD),
        ("dwYSize", wintypes.DWORD),
        ("dwXCountChars", wintypes.DWORD),
        ("dwYCountChars", wintypes.DWORD),
        ("dwFillAttribute", wintypes.DWORD),
        ("dwFlags", wintypes.DWORD),
        ("wShowWindow", wintypes.WORD),
        ("cbReserved2", wintypes.WORD),
        ("lpReserved2", ctypes.POINTER(ctypes.c_byte)),
        ("hStdInput", wintypes.HANDLE),
        ("hStdOutput", wintypes.HANDLE),
        ("hStdError", wintypes.HANDLE),
    ]


class PROCESS_INFORMATION(ctypes.Structure):
    _fields_ = [
        ("hProcess", wintypes.HANDLE),
        ("hThread", wintypes.HANDLE),
        ("dwProcessId", wintypes.DWORD),
        ("dwThreadId", wintypes.DWORD),
    ]


# ── DLL 绑定 ─────────────────────────────────────────────

kernel32 = ctypes.windll.kernel32
advapi32 = ctypes.windll.advapi32

# 设置返回值类型
kernel32.OpenProcess.restype = wintypes.HANDLE
kernel32.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]

kernel32.CloseHandle.restype = wintypes.BOOL
kernel32.CloseHandle.argtypes = [wintypes.HANDLE]

advapi32.OpenProcessToken.restype = wintypes.BOOL
advapi32.OpenProcessToken.argtypes = [wintypes.HANDLE, wintypes.DWORD, ctypes.POINTER(wintypes.HANDLE)]

advapi32.DuplicateTokenEx.restype = wintypes.BOOL
advapi32.DuplicateTokenEx.argtypes = [
    wintypes.HANDLE,  # hExistingToken
    wintypes.DWORD,   # dwDesiredAccess
    ctypes.c_void_p,  # lpTokenAttributes (NULL)
    ctypes.c_int,     # ImpersonationLevel
    ctypes.c_int,     # TokenType
    ctypes.POINTER(wintypes.HANDLE),  # phNewToken
]

advapi32.CreateProcessAsUserW.restype = wintypes.BOOL
advapi32.CreateProcessAsUserW.argtypes = [
    wintypes.HANDLE,        # hToken
    wintypes.LPCWSTR,       # lpApplicationName
    wintypes.LPWSTR,        # lpCommandLine
    ctypes.c_void_p,        # lpProcessAttributes
    ctypes.c_void_p,        # lpThreadAttributes
    wintypes.BOOL,          # bInheritHandles
    wintypes.DWORD,         # dwCreationFlags
    ctypes.c_void_p,        # lpEnvironment
    wintypes.LPCWSTR,       # lpCurrentDirectory
    ctypes.POINTER(STARTUPINFOW),   # lpStartupInfo
    ctypes.POINTER(PROCESS_INFORMATION),  # lpProcessInformation
]


def _find_explorer_pid() -> int | None:
    """找到 explorer.exe 的 PID（跑在用户 Session 1 桌面）。"""
    # 用 tasklist 快速查找，避免引入 psutil 依赖
    try:
        result = subprocess.run(
            ["tasklist", "/FI", "IMAGENAME eq explorer.exe", "/FO", "CSV", "/NH"],
            capture_output=True, text=True, timeout=5,
        )
        for line in result.stdout.strip().splitlines():
            # "explorer.exe","1234","Console","1","123,456 K"
            parts = line.split('","')
            if len(parts) >= 2:
                pid_str = parts[1].strip('"')
                try:
                    return int(pid_str)
                except ValueError:
                    continue
    except Exception as e:
        logger.warning(f"查找 explorer.exe 失败: {e}")
    return None


def spawn_in_user_session(cmd_line: str, working_dir: str | None = None) -> int | None:
    """
    在用户 Session 1 桌面创建子进程，返回 PID，失败返回 None。

    cmd_line: 完整命令行（如 "pythonw.exe C:\\path\\to\\worker.py"）
    working_dir: 工作目录，默认 None 表示继承
    """
    explorer_pid = _find_explorer_pid()
    if not explorer_pid:
        logger.warning("未找到 explorer.exe，用户桌面未登录，跳过 spawn")
        return None

    # 1. 打开 explorer.exe 进程
    h_process = kernel32.OpenProcess(PROCESS_QUERY_INFORMATION | PROCESS_VM_READ, False, explorer_pid)
    if not h_process:
        logger.warning(f"OpenProcess(explorer, pid={explorer_pid}) 失败, err={kernel32.GetLastError()}")
        return None

    try:
        # 2. 获取 explorer 的令牌
        h_token = wintypes.HANDLE()
        if not advapi32.OpenProcessToken(h_process, TOKEN_ALL_ACCESS, ctypes.byref(h_token)):
            logger.warning(f"OpenProcessToken 失败, err={kernel32.GetLastError()}")
            return None

        try:
            # 3. 复制为可用的主令牌
            h_dup_token = wintypes.HANDLE()
            if not advapi32.DuplicateTokenEx(
                h_token,
                MAXIMUM_ALLOWED,
                None,
                2,  # SecurityImpersonation
                TokenPrimary,
                ctypes.byref(h_dup_token),
            ):
                logger.warning(f"DuplicateTokenEx 失败, err={kernel32.GetLastError()}")
                return None

            try:
                # 4. CreateProcessAsUser
                startup = STARTUPINFOW()
                startup.cb = ctypes.sizeof(STARTUPINFOW)
                startup.dwFlags = 0x00000001  # STARTF_USESHOWWINDOW
                startup.wShowWindow = 0       # SW_HIDE
                startup.lpDesktop = "WinSta0\\Default"

                proc_info = PROCESS_INFORMATION()

                # 命令行需要可变字符串
                cmd_buf = ctypes.create_unicode_buffer(cmd_line)
                cwd_ptr = working_dir if working_dir else None

                success = advapi32.CreateProcessAsUserW(
                    h_dup_token,
                    None,          # lpApplicationName
                    cmd_buf,       # lpCommandLine
                    None,          # lpProcessAttributes
                    None,          # lpThreadAttributes
                    False,         # bInheritHandles
                    CREATE_NO_WINDOW,  # 绝不弹黑框
                    None,          # lpEnvironment (inherit)
                    cwd_ptr,       # lpCurrentDirectory
                    ctypes.byref(startup),
                    ctypes.byref(proc_info),
                )

                if not success:
                    err = kernel32.GetLastError()
                    logger.warning(f"CreateProcessAsUserW 失败, err={err}")
                    return None

                pid = proc_info.dwProcessId
                logger.info(f"已在用户会话创建子进程, PID={pid}, cmd={cmd_line}")

                # 关闭返回的句柄（子进程已独立运行）
                kernel32.CloseHandle(proc_info.hProcess)
                kernel32.CloseHandle(proc_info.hThread)

                return pid

            finally:
                kernel32.CloseHandle(h_dup_token)
        finally:
            kernel32.CloseHandle(h_token)
    finally:
        kernel32.CloseHandle(h_process)


def is_user_session_active() -> bool:
    """检测用户桌面 Session 1 是否活跃（explorer.exe 存在）。"""
    return _find_explorer_pid() is not None

"""
Windows 计划任务管理器（Voice Worker 专用）

通过注册 Windows 计划任务让 voice_worker 跑在用户 Session 1，
解决 WinSW（Session 0）无法读取键鼠输入的问题。

使用 PowerShell Register-ScheduledTask 注册（S4U 登录类型），
不需要用户密码，LocalSystem 权限即可注册。

作者：桂良涛，邮箱：桂良涛@nndrobot.com
"""

import logging
import os
import subprocess
import tempfile

logger = logging.getLogger("ai-os-agent")

TASK_NAME = "AI-OS-Voice-Worker"


def _get_interactive_username() -> str | None:
    """获取当前登录的交互式用户名。兼容 Session 0 环境。"""
    # 方法1: query user（Session 0 可用）
    try:
        result = subprocess.run(
            ["query", "user"],
            capture_output=True, text=True, timeout=5,
        )
        for line in result.stdout.strip().splitlines():
            if ">" in line:
                idx = line.index(">")
                rest = line[idx + 1:].strip()
                username = rest.split()[0] if rest else None
                if username:
                    logger.info(f"[query user] 检测到交互式用户: {username}")
                    return username
    except Exception as e:
        logger.debug(f"[query user] 失败: {e}")

    # 方法2: WMI 查询（Session 0 可用）
    try:
        result = subprocess.run(
            ["wmic", "useraccount", "where",
             "sid like '%-500' or sid like '%-501'",
             "get", "name"],
            capture_output=True, text=True, timeout=5,
        )
        # 不太精确，换用 explorer.exe 进程的所有者
    except Exception:
        pass

    # 方法3: 通过 tasklist 找 explorer.exe 的用户
    try:
        result = subprocess.run(
            ["tasklist", "/FI", "IMAGENAME eq explorer.exe", "/FO", "CSV", "/NH"],
            capture_output=True, text=True, timeout=5,
        )
        for line in result.stdout.strip().splitlines():
            # "explorer.exe","1234","Console","1","123,456 K"
            parts = line.split('","')
            if len(parts) >= 3:
                # tasklist /V 可以看到用户名，但 /NH 没有
                pass
        # tasklist /V 带 user 但格式复杂，换用 wmic
        result2 = subprocess.run(
            ["wmic", "process", "where", "name='explorer.exe'",
             "get", "ExecutablePath"],
            capture_output=True, text=True, timeout=5,
        )
        path = result2.stdout.strip().splitlines()
        if len(path) >= 2:
            # 从路径 C:\Users\guilt\... 提取用户名
            exe_path = path[1].strip()
            if "Users" in exe_path:
                parts = exe_path.split("\\")
                for i, p in enumerate(parts):
                    if p == "Users" and i + 1 < len(parts):
                        username = parts[i + 1]
                        logger.info(f"[wmic explorer] 检测到用户: {username}")
                        return username
    except Exception as e:
        logger.debug(f"[wmic explorer] 失败: {e}")

    # 方法4: 环境变量 USERNAME（在 Session 0 的 WinSW 中可能是 SYSTEM）
    env_user = os.environ.get("USERNAME", "")
    if env_user and env_user not in ("SYSTEM", "LOCAL SERVICE", "NETWORK SERVICE"):
        logger.info(f"[env] 检测到用户: {env_user}")
        return env_user

    logger.warning("所有方法均未检测到交互式用户")
    return None


def register_voice_worker_task(python_exe: str, worker_script: str) -> bool:
    """
    注册计划任务（S4U 登录类型，不需要用户密码）。
    LocalSystem 有权限注册 S4U 类型的计划任务。
    """
    username = _get_interactive_username()
    if not username:
        logger.warning("未找到交互式用户，跳过注册计划任务")
        return False

    logger.info(f"将为用户 {username} 注册计划任务 {TASK_NAME}")

    # PowerShell 注册脚本（S4U 类型）
    ps_script = f'''
$ErrorActionPreference = "Stop"
$taskName = '{TASK_NAME}'
$pythonExe = '{python_exe}'
$workerScript = '{worker_script}'
$username = '{username}'

# 停止并删除已有任务
Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1

# 创建动作
$action = New-ScheduledTaskAction -Execute $pythonExe -Argument $workerScript

# 创建触发器：用户登录 + 每天 00:00 保底
$triggerLogon = New-ScheduledTaskTrigger -AtLogOn
$triggerDaily = New-ScheduledTaskTrigger -Daily -At '00:00'

# 创建设置
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -RestartCount 999 `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -MultipleInstances IgnoreNew

# S4U 登录类型：不需要用户密码，以用户身份运行
$principal = New-ScheduledTaskPrincipal -UserId $username -LogonType S4U -RunLevel Highest

# 注册
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $triggerLogon,$triggerDaily -Settings $settings -Principal $principal -Force
Write-Host "REGISTERED"
'''

    tmp_ps1 = os.path.join(tempfile.gettempdir(), f"ai-os-register-{os.getpid()}.ps1")
    try:
        with open(tmp_ps1, "w", encoding="utf-8") as f:
            f.write(ps_script)

        result = subprocess.run(
            ["powershell", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass",
             "-NonInteractive", "-NoProfile", "-File", tmp_ps1],
            capture_output=True, text=True, timeout=30,
        )

        logger.info(f"计划任务注册输出: stdout={result.stdout}, stderr={result.stderr}")

        if "REGISTERED" in result.stdout:
            logger.info(f"计划任务 {TASK_NAME} 注册成功")
            return True
        else:
            logger.warning(f"计划任务注册失败")
            return False
    except Exception as e:
        logger.error(f"注册计划任务异常: {e}")
        return False
    finally:
        try:
            os.remove(tmp_ps1)
        except Exception:
            pass


def start_voice_worker_task() -> bool:
    """启动计划任务。"""
    try:
        result = subprocess.run(
            ["schtasks", "/Run", "/TN", TASK_NAME],
            capture_output=True, text=True, timeout=10,
        )
        if result.returncode == 0:
            logger.info(f"计划任务 {TASK_NAME} 已启动")
            return True
        else:
            logger.warning(f"启动计划任务失败: {result.stdout} {result.stderr}")
            return False
    except Exception as e:
        logger.error(f"启动计划任务异常: {e}")
        return False


def stop_voice_worker_task() -> bool:
    """停止计划任务。"""
    try:
        subprocess.run(
            ["schtasks", "/End", "/TN", TASK_NAME],
            capture_output=True, text=True, timeout=10,
        )
        return True
    except Exception as e:
        logger.error(f"停止计划任务异常: {e}")
        return False


def is_task_registered() -> bool:
    """检查计划任务是否已注册。"""
    try:
        result = subprocess.run(
            ["schtasks", "/Query", "/TN", TASK_NAME],
            capture_output=True, text=True, timeout=5,
        )
        return result.returncode == 0
    except Exception:
        return False


def is_user_session_active() -> bool:
    """检测用户桌面是否活跃（有交互式用户登录）。"""
    return _get_interactive_username() is not None

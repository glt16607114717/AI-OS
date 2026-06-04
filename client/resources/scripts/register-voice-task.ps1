# AI-OS Voice Worker 计划任务注册脚本
# 由 Electron 主进程通过 elevate.exe 提权调用（弹一次 UAC）
# 参照 glt-ai-hub 的注册方式：不指定 Principal，用当前用户身份注册
# 这样 AtLogOn 触发时会跑在 Session 1（用户桌面）
#
# 作者：桂良涛，邮箱：桂良涛@nndrobot.com

$ErrorActionPreference = "Stop"
$taskName = 'AI-OS-Voice-Worker'

Write-Host "Registering task for user: $env:USERNAME"

# Python 路径（固定安装路径）
$pythonExe = 'C:\ProgramData\AI-OS\runtime\python\python.exe'
$workerScript = 'C:\AI-OS\resources\backend\python\agent\voice_worker.py'

# 检查文件是否存在
if (-not (Test-Path $pythonExe)) {
    Write-Host "ERROR: Python not found at $pythonExe"
    exit 1
}

# 停止并删除已有任务
Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1

# 创建动作
$action = New-ScheduledTaskAction -Execute $pythonExe -Argument $workerScript

# 创建触发器：用户登录（指定当前用户）
$triggerLogon = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$triggerDaily = New-ScheduledTaskTrigger -Daily -At '00:00'

# 创建设置：崩溃1分钟重启、不限运行时间、电池也运行
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -RestartCount 999 `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -MultipleInstances IgnoreNew

# 不指定 Principal，用当前用户身份注册（跟 glt-ai-hub 一致）
# 这样 AtLogOn 触发时任务跑在 Session 1
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $triggerLogon,$triggerDaily -Settings $settings -Description 'AI-OS Voice Worker' -Force | Out-Null

# 立即启动
Start-ScheduledTask -TaskName $taskName

Write-Host "REGISTERED"

# AI-OS 安装前清理脚本
# 由 NSIS 安装器调用，负责停服务、杀进程
param(
    [string]$InstallDir = ""
)

$ErrorActionPreference = "Continue"

# 1. 停止并删除计划任务
schtasks /End /TN "AI-OS-Watchdog" 2>$null
schtasks /Delete /TN "AI-OS-Watchdog" /F 2>$null
schtasks /End /TN "AI-OS-Agent" 2>$null
schtasks /Delete /TN "AI-OS-Agent" /F 2>$null

# 2. 按路径杀 AI-OS 目录下的 pythonw（精确，不影响其他应用）
Get-Process pythonw -ErrorAction SilentlyContinue |
    Where-Object { $_.Path -like "*AI-OS*" } |
    Stop-Process -Force -ErrorAction SilentlyContinue

# 3. 杀 AI-OS.exe
taskkill /F /IM AI-OS.exe 2>$null

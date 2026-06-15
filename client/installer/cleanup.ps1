Start-Transcript -Path 'C:\ProgramData\AI-OS\installer.log' -Force

Write-Output '=== AI-OS 安装器清理开始 ==='

# 删除计划任务
Write-Output '--- 删除计划任务 ---'
schtasks /End /TN 'AI-OS-Watchdog' 2>&1
schtasks /Delete /TN 'AI-OS-Watchdog' /F 2>&1
schtasks /End /TN 'AI-OS-Agent' 2>&1
schtasks /Delete /TN 'AI-OS-Agent' /F 2>&1

Write-Output '--- 等待2秒 ---'
Start-Sleep -Seconds 2

# 杀进程
Write-Output '--- 杀进程 ---'
Stop-Process -Name pythonw -Force -ErrorAction SilentlyContinue
Stop-Process -Name python -Force -ErrorAction SilentlyContinue
Stop-Process -Name AI-OS -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1

# 检查残留
Write-Output '--- 检查残留进程 ---'
$remain = Get-Process -Name pythonw,python,AI-OS -ErrorAction SilentlyContinue
if ($remain) {
    Write-Output "警告: 仍有残留进程:"
    $remain | Format-Table Id,ProcessName,StartTime -AutoSize
} else {
    Write-Output '进程清理完成，无残留'
}

# 设置目录权限
Write-Output '--- 设置目录权限 ---'
if (Test-Path 'C:\ProgramData\AI-OS') {
    icacls 'C:\ProgramData\AI-OS' /grant 'BUILTIN\Users:(OI)(CI)F' /T /Q
} else {
    Write-Output '目录不存在，跳过权限设置'
}

Write-Output '=== 清理完成 ==='
Stop-Transcript

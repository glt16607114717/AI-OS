# ============================================================
# AI-OS Trace 注入器（全局 Hook）
# ------------------------------------------------------------
# 事件：UserPromptSubmit（消息发送前）/ PreToolUse（工具调用前）
# 作用：往上下文注入 [TRACE:session=xxx][TRACE:msg=xxx]
#       AI-OS 后端（llm.go extractAndStripTrace）据此关联同一次
#       用户输入触发的所有请求，做结构化对话整理。
# 对话 ID(session_id)：编辑器定义，从 stdin payload 读取
# 消息 ID(msg_id)   ：本脚本自定义并持久化，同一轮共用
# ============================================================

$ErrorActionPreference = 'SilentlyContinue'

# 任何异常都不阻断主流程（hook 失败不应影响用户正常对话）
try {
    # 读取 stdin（Trae 注入的 JSON payload）
    $raw = [Console]::In.ReadToEnd()
    if ([string]::IsNullOrWhiteSpace($raw)) { exit 0 }

    $data = $raw | ConvertFrom-Json
    $sessionId = $data.session_id
    $event     = $data.hook_event_name
    # 无 session_id 时安全跳过
    if ([string]::IsNullOrWhiteSpace($sessionId)) { exit 0 }

    # msg_id 持久化文件：按 session 隔离，存 TEMP
    $msgIdFile = Join-Path $env:TEMP "aios_msg_$sessionId.txt"

    if ($event -eq 'UserPromptSubmit') {
        # 用户提交新消息：生成新的 msg_id 并写入（覆盖旧值）
        $msgId = [guid]::NewGuid().ToString('N').Substring(0, 16)
        [System.IO.File]::WriteAllText($msgIdFile, $msgId)
    }
    else {
        # 工具调用前：复用本轮 msg_id；缺失则补生成（容错）
        $msgId = $null
        if (Test-Path -LiteralPath $msgIdFile) {
            $msgId = ([System.IO.File]::ReadAllText($msgIdFile)).Trim()
        }
        if ([string]::IsNullOrWhiteSpace($msgId)) {
            $msgId = [guid]::NewGuid().ToString('N').Substring(0, 16)
            [System.IO.File]::WriteAllText($msgIdFile, $msgId)
        }
    }

    # 组装 trace 标记并注入到 additionalContext
    $trace = "[TRACE:session=$sessionId][TRACE:msg=$msgId]"
    $out = [ordered]@{
        'continue' = $true
        hookSpecificOutput = [ordered]@{
            hookEventName     = $event
            additionalContext = $trace
        }
    }
    Write-Output ($out | ConvertTo-Json -Compress -Depth 5)
}
catch {
    Write-Output '{"continue":true}'
}
exit 0

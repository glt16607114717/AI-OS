"""
SessionStart / PreCompact Hook - 注入会话上下文到 AI

- SessionStart: 新会话启动时注入 session_id
- PreCompact: 上下文压缩前重新注入，防止 session_id 被压缩丢失
"""
import sys
import json
import os
from datetime import datetime


def main():
    try:
        raw = sys.stdin.read()
        if not raw.strip():
            output_minimal()
            return

        data = json.loads(raw)
        session_id = data.get("session_id", "unknown")
        cwd = data.get("cwd", os.getcwd())
        hook_event = data.get("hook_event_name", "SessionStart")

        ctx_lines = [
            f"[系统] 会话已启动",
            f"会话ID: {session_id}",
            f"工作目录: {cwd}",
            f"启动时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
        ]

        # PreCompact 时追加保留指令，防止压缩丢失
        if hook_event == "PreCompact":
            ctx_lines.append("（上下文压缩时必须保留以上会话信息）")

        additional_context = "\n".join(ctx_lines)

        output = {
            "continue": True,
            "hookSpecificOutput": {
                "hookEventName": hook_event,
                "additionalContext": additional_context,
            },
        }
        print(json.dumps(output, ensure_ascii=False))

    except Exception as e:
        output_minimal(str(e))


def output_minimal(error: str = None):
    ctx = "[系统] 会话已启动"
    if error:
        ctx += f"\n(Hook 异常: {error})"
    output = {
        "continue": True,
        "hookSpecificOutput": {
            "hookEventName": "SessionStart",
            "additionalContext": ctx,
        },
    }
    print(json.dumps(output, ensure_ascii=False))


if __name__ == "__main__":
    main()

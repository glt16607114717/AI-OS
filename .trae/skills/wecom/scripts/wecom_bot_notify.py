"""
企业微信机器人通知脚本

通过企业微信群机器人 webhook 发送 Markdown 格式消息。

用法：python wecom_bot_notify.py "<消息内容>"
示例：python wecom_bot_notify.py "这是一条测试消息"
示例：python wecom_bot_notify.py "# 需求通知\n\n**需求名称**：新增月出货量统计\n**状态**：已开发完成\n\n请前端同事及时对接联调。"

参数说明：
  消息内容  必填，Markdown 格式的消息文本，支持多行
"""
import json
import sys
import os
import requests

CONFIG_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "wecom_bot_config.json")


def load_config():
    with open(CONFIG_FILE, "r", encoding="utf-8") as f:
        return json.load(f)


def main():
    if len(sys.argv) < 2:
        print("用法: python wecom_bot_notify.py \"<消息内容>\"")
        print("示例: python wecom_bot_notify.py \"这是一条测试消息\"")
        sys.exit(1)

    message = sys.argv[1]

    config = load_config()
    webhook_url = config.get("webhook_url")

    if not webhook_url:
        print("[错误] 配置文件中未找到 webhook_url")
        sys.exit(1)

    try:
        resp = requests.post(webhook_url, json={
            "msgtype": "markdown",
            "markdown": {
                "content": message
            }
        }, timeout=10)
        result = resp.json()
        print(json.dumps(result, ensure_ascii=False, indent=2))
        if result.get("errcode") == 0:
            print("[成功] 消息已发送")
        else:
            print(f"[错误] {result.get('errmsg', '未知错误')}")
    except Exception as e:
        print(f"[错误] 请求失败: {e}")


if __name__ == "__main__":
    main()
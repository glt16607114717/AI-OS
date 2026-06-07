"""
企业微信通知脚本

通过RMP服务器接口发送企微消息，服务器负责token管理和发送。

用法：python wecom_notify.py "<接收人>" "<发送人>" "<消息内容>" [TAPD链接] [消息标题]
示例：python wecom_notify.py "卞成龙,张雨凯" "桂良涛" "开发完成" "https://www.tapd.cn/66680814/prong/stories/view/xxx"
示例：python wecom_notify.py "桂良涛" "桂良涛" "测试消息" "" "通知"

参数说明：
  接收人    必填，中文姓名，多人用逗号分隔
  发送人    必填，发送人姓名
  消息内容  必填，消息文本
  TAPD链接  可选，需求或Bug的完整URL，不传则默认跳转TAPD首页
  消息标题  可选，消息标题，不传则默认为"开发完成通知"
"""
import json
import sys
import os
import hashlib
import requests

CACHE_DIR = "D:/wwwroot/ai/ai_cache"
CONFIG_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "wecom_config.json")
SECRET = "nnd_wecom_2026"
TAPD_HOME = "https://www.tapd.cn/66680814"

USER_MAP = {
    "侯森": "18538240648",
    "刘贵财": "13559737249",
    "卞成龙": "19005187692",
    "向海涛": "XiangHaiTao",
    "张雨凯": "13202303616",
    "曾燕芳": "15770612547",
    "李现成": "15638083521",
    "桂良涛": "16607114717",
    "梁任超": "13790442475",
    "王立志": "1478469991",
    "罗昭鸿": "18905816784",
    "谭鸿兴": "18578459442",
    "陈小东": "19084801901",
    "邹兴平": "13570842839",
}


def load_config():
    with open(CONFIG_FILE, "r", encoding="utf-8") as f:
        return json.load(f)


def main():
    if len(sys.argv) < 4:
        print("用法: python wecom_notify.py \"<接收人>\" \"<发送人>\" \"<消息内容>\" [TAPD链接] [消息标题]")
        print("示例: python wecom_notify.py \"卞成龙,张雨凯\" \"桂良涛\" \"开发完成\"")
        print("      python wecom_notify.py \"桂良涛\" \"桂良涛\" \"测试消息\" \"\" \"通知\"")
        sys.exit(1)

    names = [n.strip() for n in sys.argv[1].split(",") if n.strip()]
    sender = sys.argv[2]
    message = sys.argv[3]
    tapd_url = sys.argv[4] if len(sys.argv) > 4 else TAPD_HOME
    title = sys.argv[5] if len(sys.argv) > 5 else "开发完成通知"

    work_ids = []
    not_found = []
    for name in names:
        if name in USER_MAP:
            work_ids.append(USER_MAP[name])
        else:
            not_found.append(name)

    if not work_ids:
        print(f"[错误] 未找到任何人的企微ID: {not_found}")
        sys.exit(1)

    if not_found:
        print(f"[警告] 以下人员未找到企微ID，已跳过: {', '.join(not_found)}")

    work_ids_str = "|".join(work_ids)
    sign = hashlib.md5((work_ids_str + sender + message + tapd_url + title + SECRET).encode("utf-8")).hexdigest()

    config = load_config()
    api_url = config["server_url"] + "/api/admin/wecom-notify"

    try:
        resp = requests.post(api_url, json={
            "sender": sender,
            "message": message,
            "sign": sign,
            "work_ids": work_ids_str,
            "url": tapd_url,
            "title": title,
        }, timeout=10)
        result = resp.json()
        print(json.dumps(result, ensure_ascii=False, indent=2))
        if result.get("code") == 200:
            print(f"[成功] 消息已发送给: {', '.join(n for n in names if n not in not_found)}")
        else:
            print(f"[错误]")
    except Exception as e:
        print(f"[错误] 请求失败: {e}")


if __name__ == "__main__":
    main()
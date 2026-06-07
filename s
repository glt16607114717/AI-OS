[1mdiff --git a/client/backend/python/agent/llm_api.py b/client/backend/python/agent/llm_api.py[m
[1mindex cc774c5..93b973c 100644[m
[1m--- a/client/backend/python/agent/llm_api.py[m
[1m+++ b/client/backend/python/agent/llm_api.py[m
[36m@@ -3,19 +3,26 @@[m
 [m
 挂载到 /api/llm，提供目录查询、密钥保存、厂商启停接口。[m
 转发路由挂载到 /v1/chat/completions，兼容 OpenAI 格式。[m
[32m+[m[32m轮询模式下支持智能故障转移。[m
 [m
 作者：桂良涛，邮箱：桂良涛@nndrobot.com[m
 """[m
 [m
 import logging[m
[32m+[m[32mimport os[m
[32m+[m[32mimport time[m
 from fastapi import APIRouter, Request[m
 from fastapi.responses import StreamingResponse, JSONResponse[m
 from pydantic import BaseModel[m
 import httpx[m
 [m
 import llm_config[m
[32m+[m[32mimport llm_stats[m
[32m+[m[32mimport god_rules[m
[32m+[m[32mimport request_dump[m
[32m+[m[32mimport llm_log[m
 [m
[31m-logger = logging.getLogger("ai-os-agent")[m
[32m+[m[32mlogger = logging.getLogger("llm")[m
 [m
 router = APIRouter(tags=["llm"])[m
 [m
[36m@@ -39,9 +46,9 @@[m [masync def llm_action(req: LlmActionRequest):[m
     elif action == "llm_save_keys":[m
         vendor_id = payload.get("vendor_id", "")[m
         keys = payload.get("keys", [])[m
[31m-        ok = llm_config.save_vendor_keys(vendor_id, keys)[m
[31m-        if not ok:[m
[31m-            return {"ok": False, "error": "无效的 vendor_id"}[m
[32m+[m[32m        result = llm_config.save_vendor_keys(vendor_id, keys)[m
[32m+[m[32m        if not result.get("ok"):[m
[32m+[m[32m            return {"ok": False, "error": result.get("error", "保存密钥失败")}[m
         return {"ok": True}[m
 [m
     elif action == "llm_toggle_vendor":[m
[36m@@ -78,18 +85,100 @@[m [masync def llm_action(req: LlmActionRequest):[m
             return {"ok": False, "error": "策略不存在"}[m
         return {"ok": True}[m
 [m
[32m+[m[32m    elif action == "llm_get_stats":[m
[32m+[m[32m        days = payload.get("days", 30)[m
[32m+[m[32m        summary = llm_stats.get_summary(days)[m
[32m+[m[32m        return {"ok": True, "stats": summary}[m
[32m+[m
[32m+[m[32m    elif action == "llm_get_errors":[m
[32m+[m[32m        limit = payload.get("limit", 20)[m
[32m+[m[32m        errors = llm_stats.get_recent_errors(limit)[m
[32m+[m[32m        return {"ok": True, "errors": errors}[m
[32m+[m
[32m+[m[32m    elif action == "llm_cleanup_stats":[m
[32m+[m[32m        removed = llm_stats.cleanup()[m
[32m+[m[32m        return {"ok": True, "removed": removed}[m
[32m+[m
[32m+[m[32m    elif action == "llm_get_god_rules":[m
[32m+[m[32m        data = god_rules.get_rules()[m
[32m+[m[32m        return {"ok": True, "enabled": data["enabled"], "rules": data["rules"], "prompt_optimize": data.get("prompt_optimize", True)}[m
[32m+[m
[32m+[m[32m    elif action == "llm_save_god_rules":[m
[32m+[m[32m        enabled = payload.get("enabled", True)[m
[32m+[m[32m        rules = payload.get("rules", "")[m
[32m+[m[32m        prompt_optimize = payload.get("prompt_optimize", True)[m
[32m+[m[32m        god_rules.save_rules(enabled, rules, prompt_optimize)[m
[32m+[m[32m        return {"ok": True}[m
[32m+[m
[32m+[m[32m    elif action == "llm_get_quota_status":[m
[32m+[m[32m        from quota_monitor import get_status, get_all_keys_status[m
[32m+[m[32m        result = get_status()[m
[32m+[m[32m        all_keys = get_all_keys_status()[m
[32m+[m[32m        return {"ok": True, **result, "all_keys": all_keys}[m
[32m+[m
[32m+[m[32m    elif action == "llm_set_quota_enabled":[m
[32m+[m[32m        from quota_monitor import set_enabled[m
[32m+[m[32m        set_enabled(payload.get("enabled", True))[m
[32m+[m[32m        return {"ok": True}[m
[32m+[m
[32m+[m[32m    elif action == "llm_force_quota_check":[m
[32m+[m[32m        from quota_monitor import force_check[m
[32m+[m[32m        force_check()[m
[32m+[m[32m        return {"ok": True}[m
[32m+[m
[32m+[m[32m    elif action == "llm_get_logs":[m
[32m+[m[32m        limit = payload.get("limit", 200)[m
[32m+[m[32m        category = payload.get("category", "")[m
[32m+[m[32m        after_id = payload.get("after_id", 0)[m
[32m+[m[32m        logs = llm_log.get_logs(limit=limit, category=category, after_id=after_id)[m
[32m+[m[32m        max_id = llm_log.get_max_id()[m
[32m+[m[32m        return {"ok": True, "logs": logs, "max_id": max_id}[m
[32m+[m
[32m+[m[32m    elif action == "llm_clear_logs":[m
[32m+[m[32m        llm_log.clear_logs()[m
[32m+[m[32m        return {"ok": True}[m
[32m+[m
     else:[m
         return {"ok": False, "error": f"Unknown action: {action}"}[m
 [m
 [m
[32m+[m[32m# ── 内部转发函数 ──[m
[32m+[m
[32m+[m[32masync def _do_forward(body: dict, vendor_info: dict) -> tuple:[m
[32m+[m[32m    """[m
[32m+[m[32m    执行一次实际的转发请求。[m
[32m+[m[32m    返回 (status_code, response_data_or_error_text, latency_ms)[m
[32m+[m[32m    """[m
[32m+[m[32m    base_url = vendor_info["base_url"].rstrip("/")[m
[32m+[m[32m    target_url = f"{base_url}/chat/completions"[m
[32m+[m[32m    headers = {[m
[32m+[m[32m        "Authorization": f"Bearer {vendor_info['api_key']}",[m
[32m+[m[32m        "Content-Type": "application/json",[m
[32m+[m[32m    }[m
[32m+[m
[32m+[m[32m    is_stream = body.get("stream", False)[m
[32m+[m[32m    t0 = time.monotonic()[m
[32m+[m
[32m+[m[32m    if is_stream:[m
[32m+[m[32m        return 200, {"_stream": True, "target_url": target_url, "headers": headers, "body": body, "vendor_info": vendor_info}, 0[m
[32m+[m
[32m+[m[32m    # 非流式[m
[32m+[m[32m    async with httpx.AsyncClient(timeout=httpx.Timeout(300.0, connect=10.0)) as client:[m
[32m+[m[32m        resp = await client.post(target_url, json=body, headers=headers)[m
[32m+[m[32m        latency_ms = int((time.monotonic() - t0) * 1000)[m
[32m+[m[32m        if resp.status_code != 200:[m
[32m+[m[32m            return resp.status_code, resp.text, latency_ms[m
[32m+[m[32m        return 200, resp.json(), latency_ms[m
[32m+[m
[32m+[m
 # ── 转发代理接口（/v1/chat/completions） ──[m
 [m
 @router.post("/v1/chat/completions")[m
 async def proxy_chat_completions(request: Request):[m
     """[m
     OpenAI 兼容格式的转发代理。[m
[31m-    根据请求体中的 model 字段查找对应厂商，转发请求。[m
[31m-    支持流式（stream=true）和非流式。[m
[32m+[m[32m    轮询模式下支持智能故障转移：某厂商报错自动尝试下一个。[m
[32m+[m[32m    每次请求完整转储到 C:\ProgramData\AI-OS\logs\requests\[m
     """[m
     try:[m
         body = await request.json()[m
[36m@@ -100,75 +189,218 @@[m [masync def proxy_chat_completions(request: Request):[m
     if not model:[m
         return JSONResponse({"error": {"message": "model is required"}}, status_code=400)[m
 [m
[31m-    # 优先使用策略路由，无策略时 fallback 到按 model 查找厂商[m
[32m+[m[32m    # ── 上帝指令注入 ──[m
[32m+[m[32m    body["messages"] = god_rules.inject_into_messages(body.get("messages", []))[m
[32m+[m
[32m+[m[32m    # ── 工具描述压缩（受提示词优化开关控制） ──[m
[32m+[m[32m    if "tools" in body and god_rules.is_optimize_enabled():[m
[32m+[m[32m        body["tools"] = god_rules.compress_tool_descriptions(body["tools"])[m
[32m+[m
[32m+[m[32m    # ── 请求转储（只保留最近 20 个文件） ──[m
[32m+[m[32m    request_dump.dump_request(body)[m
[32m+[m[32m    _cleanup_dump_files(20)[m
[32m+[m
[32m+[m[32m    # 获取策略路由[m
     vendor_info = llm_config.get_route_by_strategy()[m
[32m+[m[32m    strategy_type = "fixed" if not vendor_info else "round_robin"  # 简化判断[m
     if vendor_info:[m
[31m-        # 策略路由：用策略指定的 model_id 覆�
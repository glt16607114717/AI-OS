"""
大模型厂商预置目录 + 密钥管理 + 转发代理 API

挂载到 /api/llm，提供目录查询、密钥保存、厂商启停接口。
转发路由挂载到 /v1/chat/completions，兼容 OpenAI 格式。
轮询模式下支持智能故障转移。

作者：桂良涛，邮箱：桂良涛@nndrobot.com
"""

import logging
import os
import time
from fastapi import APIRouter, Request
from fastapi.responses import StreamingResponse, JSONResponse
from pydantic import BaseModel
import httpx

import llm_config
import llm_stats
import god_rules
import request_dump
import llm_log

logger = logging.getLogger("llm")

router = APIRouter(tags=["llm"])


# ── 配置管理接口（/api/llm） ──

class LlmActionRequest(BaseModel):
    action: str
    payload: dict = {}


@router.post("/api/llm")
async def llm_action(req: LlmActionRequest):
    action = req.action
    payload = req.payload

    if action == "llm_get_catalog":
        catalog = llm_config.get_catalog()
        return {"ok": True, "catalog": catalog}

    elif action == "llm_save_keys":
        vendor_id = payload.get("vendor_id", "")
        keys = payload.get("keys", [])
        result = llm_config.save_vendor_keys(vendor_id, keys)
        if not result.get("ok"):
            return {"ok": False, "error": result.get("error", "保存密钥失败")}
        return {"ok": True}

    elif action == "llm_toggle_vendor":
        vendor_id = payload.get("vendor_id", "")
        enabled = payload.get("enabled", False)
        ok = llm_config.toggle_vendor(vendor_id, enabled)
        if not ok:
            return {"ok": False, "error": "无效的 vendor_id"}
        return {"ok": True}

    elif action == "llm_get_options":
        options = llm_config.get_available_options()
        return {"ok": True, "options": options}

    elif action == "llm_get_strategies":
        strategies = llm_config.get_strategies()
        return {"ok": True, "strategies": strategies}

    elif action == "llm_save_strategy":
        ok = llm_config.save_strategy(payload)
        if not ok:
            return {"ok": False, "error": "保存策略失败"}
        return {"ok": True}

    elif action == "llm_delete_strategy":
        ok = llm_config.delete_strategy(payload.get("id", ""))
        if not ok:
            return {"ok": False, "error": "删除策略失败"}
        return {"ok": True}

    elif action == "llm_set_active_strategy":
        ok = llm_config.set_active_strategy(payload.get("id", ""))
        if not ok:
            return {"ok": False, "error": "策略不存在"}
        return {"ok": True}

    elif action == "llm_get_stats":
        days = payload.get("days", 30)
        summary = llm_stats.get_summary(days)
        return {"ok": True, "stats": summary}

    elif action == "llm_get_errors":
        limit = payload.get("limit", 20)
        errors = llm_stats.get_recent_errors(limit)
        return {"ok": True, "errors": errors}

    elif action == "llm_cleanup_stats":
        removed = llm_stats.cleanup()
        return {"ok": True, "removed": removed}

    elif action == "llm_get_god_rules":
        data = god_rules.get_rules()
        return {"ok": True, "enabled": data["enabled"], "rules": data["rules"], "prompt_optimize": data.get("prompt_optimize", True)}

    elif action == "llm_save_god_rules":
        enabled = payload.get("enabled", True)
        rules = payload.get("rules", "")
        prompt_optimize = payload.get("prompt_optimize", True)
        god_rules.save_rules(enabled, rules, prompt_optimize)
        return {"ok": True}

    elif action == "llm_get_quota_status":
        from quota_monitor import get_status, get_all_keys_status
        result = get_status()
        all_keys = get_all_keys_status()
        return {"ok": True, **result, "all_keys": all_keys}

    elif action == "llm_set_quota_enabled":
        from quota_monitor import set_enabled
        set_enabled(payload.get("enabled", True))
        return {"ok": True}

    elif action == "llm_force_quota_check":
        from quota_monitor import force_check
        force_check()
        return {"ok": True}

    elif action == "llm_get_logs":
        limit = payload.get("limit", 200)
        category = payload.get("category", "")
        after_id = payload.get("after_id", 0)
        logs = llm_log.get_logs(limit=limit, category=category, after_id=after_id)
        max_id = llm_log.get_max_id()
        return {"ok": True, "logs": logs, "max_id": max_id}

    elif action == "llm_clear_logs":
        llm_log.clear_logs()
        return {"ok": True}

    elif action == "chat_get_history":
        from chat_history import get_history, get_max_id
        limit = payload.get("limit", 1000)
        max_id = get_max_id()
        messages = get_history(limit=limit)
        return {"ok": True, "messages": messages, "max_id": max_id}

    elif action == "chat_add_message":
        from chat_history import add_message
        role = payload.get("role", "")
        content = payload.get("content", "")
        if role and content:
            add_message(role, content)
        return {"ok": True}

    elif action == "chat_clear_history":
        from chat_history import clear_history
        clear_history()
        return {"ok": True}

    else:
        return {"ok": False, "error": f"Unknown action: {action}"}


# ── 内部转发函数 ──

async def _do_forward(body: dict, vendor_info: dict) -> tuple:
    """
    执行一次实际的转发请求。
    返回 (status_code, response_data_or_error_text, latency_ms)
    """
    base_url = vendor_info["base_url"].rstrip("/")
    target_url = f"{base_url}/chat/completions"
    headers = {
        "Authorization": f"Bearer {vendor_info['api_key']}",
        "Content-Type": "application/json",
    }

    is_stream = body.get("stream", False)
    t0 = time.monotonic()

    if is_stream:
        return 200, {"_stream": True, "target_url": target_url, "headers": headers, "body": body, "vendor_info": vendor_info}, 0

    # 非流式
    async with httpx.AsyncClient(timeout=httpx.Timeout(300.0, connect=10.0)) as client:
        resp = await client.post(target_url, json=body, headers=headers)
        latency_ms = int((time.monotonic() - t0) * 1000)
        if resp.status_code != 200:
            return resp.status_code, resp.text, latency_ms
        return 200, resp.json(), latency_ms


# ── 转发代理接口（/v1/chat/completions） ──

@router.post("/v1/chat/completions")
async def proxy_chat_completions(request: Request):
    """
    OpenAI 兼容格式的转发代理。
    轮询模式下支持智能故障转移：某厂商报错自动尝试下一个。
    每次请求完整转储到 C:\ProgramData\AI-OS\logs\requests\
    """
    try:
        body = await request.json()
    except Exception:
        return JSONResponse({"error": {"message": "Invalid JSON body"}}, status_code=400)

    model = body.get("model", "")
    if not model:
        return JSONResponse({"error": {"message": "model is required"}}, status_code=400)

    # ── 上帝指令注入 ──
    body["messages"] = god_rules.inject_into_messages(body.get("messages", []))

    # ── 工具描述压缩（受提示词优化开关控制） ──
    if "tools" in body and god_rules.is_optimize_enabled():
        body["tools"] = god_rules.compress_tool_descriptions(body["tools"])

    # ── 请求转储（只保留最近 20 个文件） ──
    request_dump.dump_request(body)
    _cleanup_dump_files(20)

    # 获取策略路由
    vendor_info = llm_config.get_route_by_strategy()
    strategy_type = "fixed" if not vendor_info else "round_robin"  # 简化判断
    if vendor_info:
        strategy_type = "策略路由"
        body["model"] = vendor_info.get("model_id", model)
        llm_log.write_log("route", f"策略路由选中 {vendor_info['vendor_name']}（{vendor_info.get('key_id', '')[:8]}）→ {body['model']}", detail=f"vendor={vendor_info['vendor_id']}")
    else:
        vendor_info = llm_config.get_vendor_for_model(model)
        if vendor_info:
            strategy_type = "模型匹配"
            llm_log.write_log("route", f"模型匹配到 {vendor_info['vendor_name']} → {model}", detail=f"vendor={vendor_info['vendor_id']}")

    if not vendor_info:
        llm_log.write_log("error", f"无可用厂商: {model}", level="error")
        return JSONResponse(
            {"error": {"message": f"No available vendor for model: {model}"}},
            status_code=404,
        )

    # ── 智谱用量分级 ──
    # exhausted (>90%): 固定策略直接报错（轮询已在路由层跳过）
    # degraded (70%-90%): 降级 glm-5.1 → glm-4.7
    if vendor_info.get("vendor_id") == "zhipu":
        from quota_monitor import should_downgrade_model
        key_id = vendor_info.get("key_id", "")

        if vendor_info.get("_quota_exhausted"):
            logger.warning("[LLM Proxy] key %s exhausted, rejecting request", key_id[:8])
            llm_log.write_log("quota", f"Key {key_id[:8]} 用量超过 90%，请求被拒绝", level="error", detail=f"model={model}")
            return JSONResponse(
                {"error": {"message": f"智谱 API key ({key_id[:8]}) 用量已超过 90%，请稍后再试或切换策略"}},
                status_code=429,
            )

        original_model = body.get("model", model)
        downgraded_model = should_downgrade_model(key_id, original_model)
        if downgraded_model != original_model:
            logger.info("[LLM Proxy] quota downgrade: %s → %s (key: %s)",
                         original_model, downgraded_model, key_id[:8])
            llm_log.write_log("downgrade", f"{original_model} → {downgraded_model}（Key {key_id[:8]} 用量偏高）", detail=f"key_id={key_id}")
            body["model"] = downgraded_model

    is_stream = body.get("stream", False)
    # 提取用户消息摘要（用于日志）
    user_messages = [m.get("content", "")[:100] for m in body.get("messages", []) if m.get("role") == "user"]
    user_msg_summary = user_messages[-1] if user_messages else "(无用户消息)"
    logger.info(f"[LLM Proxy] model={model} -> {vendor_info['vendor_name']} stream={is_stream}")
    llm_log.write_log("request",
        f"{body.get('model', model)} → {vendor_info['vendor_name']}（{vendor_info.get('key_id', '')[:8]}）{'流式' if is_stream else '非流式'}",
        detail=f"model={body.get('model', model)},stream={is_stream},user_msg={user_msg_summary[:200]}")

    # ── 非流式：支持故障转移 ──
    if not is_stream:
        failed_vendors = set()
        attempts = [(vendor_info, "策略路由")]

        failover_routes = llm_config.get_all_routes_for_failover()
        for r in failover_routes:
            if r["vendor_id"] != vendor_info.get("vendor_id", ""):
                attempts.append((r, "故障转移"))

        last_error = ""
        for vi, source in attempts:
            if vi["vendor_id"] in failed_vendors:
                continue

            fwd_body = dict(body)
            fwd_body["model"] = vi.get("model_id", model)

            status, data, latency = await _do_forward(fwd_body, vi)

            if status == 200:
                usage = data.get("usage", {})
                logger.info(f"[LLM Proxy] response: model={data.get('model','')}, "
                            f"usage={usage}, latency={latency}ms, source={source}")
                token_detail = f"输入 {usage.get('prompt_tokens', 0)} / 输出 {usage.get('completion_tokens', 0)} / 耗时 {latency}ms"
                llm_log.write_log("request", f"响应成功: {vi.get('model_id', '')}（{source}）", detail=token_detail)
                llm_stats.record(
                    vendor_id=vi["vendor_id"],
                    vendor_name=vi["vendor_name"],
                    model_id=vi.get("model_id", ""),
                    prompt_tokens=usage.get("prompt_tokens", 0),
                    completion_tokens=usage.get("completion_tokens", 0),
                    total_tokens=usage.get("total_tokens", 0),
                    latency_ms=latency,
                    success=True,
                )
                return JSONResponse(data, status_code=200)
            else:
                failed_vendors.add(vi["vendor_id"])
                error_msg = str(data)[:500] if isinstance(data, str) else str(data)
                logger.warning(f"[LLM Proxy] {vi['vendor_name']} failed ({status}), "
                               f"source={source}, error={error_msg}")
                llm_log.write_log("failover", f"{vi['vendor_name']} 请求失败（HTTP {status}）", level="error", detail=f"error={error_msg[:200]}")
                llm_stats.record(
                    vendor_id=vi["vendor_id"],
                    vendor_name=vi["vendor_name"],
                    model_id=vi.get("model_id", ""),
                    latency_ms=latency,
                    success=False,
                    error=error_msg,
                )
                last_error = error_msg

        logger.error(f"[LLM Proxy] all vendors failed, last error: {last_error}")
        llm_log.write_log("failover", f"所有厂商均失败", level="error", detail=f"last_error={last_error[:200]}")
        return JSONResponse(
            {"error": {"message": "All vendors failed", "detail": last_error}},
            status_code=502,
        )

    # ── 流式：支持故障转移 ──
    # 故障转移只在连接阶段生效：一旦开始 yield 数据给客户端，就不能再切换
    failed_vendors = set()
    attempts = [(vendor_info, "策略路由")]
    failover_routes = llm_config.get_all_routes_for_failover()
    for r in failover_routes:
        if r["vendor_id"] != vendor_info.get("vendor_id", ""):
            attempts.append((r, "故障转移"))

    async def stream_generator():
        nonlocal vendor_info
        last_error = ""
        last_error_type = ""

        for vi, source in attempts:
            if vi["vendor_id"] in failed_vendors:
                continue

            base_url = vi["base_url"].rstrip("/")
            target_url = f"{base_url}/chat/completions"
            headers = {
                "Authorization": f"Bearer {vi['api_key']}",
                "Content-Type": "application/json",
            }
            # 替换 model 为当前路由的 model
            fwd_body = dict(body)
            fwd_body["model"] = vi.get("model_id", model)

            t0 = time.monotonic()
            try:
                async with httpx.AsyncClient(timeout=httpx.Timeout(300.0, connect=10.0)) as client:
                    async with client.stream("POST", target_url, json=fwd_body, headers=headers) as resp:
                        if resp.status_code != 200:
                            error_body = await resp.aread()
                            error_msg = error_body.decode()[:500]
                            logger.error(f"[LLM Proxy] stream upstream error: {resp.status_code} {error_msg}")
                            llm_log.write_log("error", f"上游返回错误: HTTP {resp.status_code}",
                                level="error", detail=f"vendor={vi['vendor_name']},error={error_msg[:300]}")
                            llm_stats.record(
                                vendor_id=vi["vendor_id"],
                                vendor_name=vi["vendor_name"],
                                model_id=vi.get("model_id", ""),
                                latency_ms=int((time.monotonic() - t0) * 1000),
                                success=False,
                                error=error_msg,
                            )
                            failed_vendors.add(vi["vendor_id"])
                            last_error = error_msg
                            last_error_type = f"HTTP {resp.status_code}"
                            continue

                        # 连接成功，开始 yield 数据（不能再故障转移了）
                        if source == "故障转移":
                            llm_log.write_log("failover", f"故障转移成功: {vi['vendor_name']}（{source}）",
                                detail=f"model={vi.get('model_id','')},vendor_id={vi['vendor_id'][:8]}")

                        success = False
                        async for line in resp.aiter_lines():
                            yield line + "\n"
                            if line.startswith("data: ") and line != "data: [DONE]":
                                try:
                                    chunk = line[6:]
                                    import json
                                    obj = json.loads(chunk)
                                    if obj.get("usage"):
                                        latency_ms = int((time.monotonic() - t0) * 1000)
                                        usage = obj["usage"]
                                        llm_log.write_log("request",
                                            f"流式完成: {vi['vendor_name']}（{usage.get('prompt_tokens',0)}+{usage.get('completion_tokens',0)} tokens）",
                                            detail=f"latency={latency_ms}ms,model={vi.get('model_id','')},prompt_tokens={usage.get('prompt_tokens',0)},completion_tokens={usage.get('completion_tokens',0)}")
                                        llm_stats.record(
                                            vendor_id=vi["vendor_id"],
                                            vendor_name=vi["vendor_name"],
                                            model_id=vi.get("model_id", ""),
                                            prompt_tokens=usage.get("prompt_tokens", 0),
                                            completion_tokens=usage.get("completion_tokens", 0),
                                            total_tokens=usage.get("total_tokens", 0),
                                            latency_ms=latency_ms,
                                            success=True,
                                        )
                                        success = True
                                except Exception:
                                    pass

                        if not success:
                            latency_ms = int((time.monotonic() - t0) * 1000)
                            llm_log.write_log("request", f"流式完成（无 usage）: {vi['vendor_name']}",
                                detail=f"latency={latency_ms}ms,model={vi.get('model_id','')}")
                            llm_stats.record(
                                vendor_id=vi["vendor_id"],
                                vendor_name=vi["vendor_name"],
                                model_id=vi.get("model_id", ""),
                                latency_ms=latency_ms,
                                success=True,
                            )
                        return  # 成功完成，退出循环

            except Exception as e:
                error_type = type(e).__name__
                error_msg = str(e) or error_type
                logger.error(f"[LLM Proxy] stream error ({vi['vendor_name']}): {error_type}: {error_msg}")
                llm_log.write_log("error", f"流式中断: {error_type}",
                    level="error", detail=f"vendor={vi['vendor_name']},error={error_msg[:300]},user_msg={user_msg_summary[:200]}")
                llm_stats.record(
                    vendor_id=vi["vendor_id"],
                    vendor_name=vi["vendor_name"],
                    model_id=vi.get("model_id", ""),
                    latency_ms=int((time.monotonic() - t0) * 1000),
                    success=False,
                    error=error_msg,
                )
                failed_vendors.add(vi["vendor_id"])
                last_error = error_msg
                last_error_type = error_type
                continue

        # 所有路由都失败了
        logger.error(f"[LLM Proxy] all vendors failed (stream), last error: {last_error_type}: {last_error}")
        llm_log.write_log("failover", f"所有厂商均失败（流式）", level="error", detail=f"last_error={last_error_type}: {last_error[:200]}")
        yield f"data: {{\"error\": \"all vendors failed: {last_error_type}: {last_error}\"}}\n\n"
        yield "data: [DONE]\n\n"

    return StreamingResponse(
        stream_generator(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
            "X-Accel-Buffering": "no",
        },
    )


def _cleanup_dump_files(keep: int = 20):
    """只保留最近的 N 个转储文件。"""
    try:
        dump_dir = request_dump.DUMP_DIR
        files = sorted(dump_dir.glob("*.json"), key=lambda f: f.name, reverse=True)
        for f in files[keep:]:
            f.unlink()
    except Exception:
        pass

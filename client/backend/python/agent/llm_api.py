"""
大模型厂商预置目录 + 密钥管理 + 转发代理 API

挂载到 /api/llm，提供目录查询、密钥保存、厂商启停接口。
转发路由挂载到 /v1/chat/completions，兼容 OpenAI 格式。

作者：桂良涛，邮箱：桂良涛@nndrobot.com
"""

import logging
from fastapi import APIRouter, Request
from fastapi.responses import StreamingResponse, JSONResponse
from pydantic import BaseModel
import httpx

import llm_config

logger = logging.getLogger("ai-os-agent")

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
        ok = llm_config.save_vendor_keys(vendor_id, keys)
        if not ok:
            return {"ok": False, "error": "无效的 vendor_id"}
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

    else:
        return {"ok": False, "error": f"Unknown action: {action}"}


# ── 转发代理接口（/v1/chat/completions） ──

@router.post("/v1/chat/completions")
async def proxy_chat_completions(request: Request):
    """
    OpenAI 兼容格式的转发代理。
    根据请求体中的 model 字段查找对应厂商，转发请求。
    支持流式（stream=true）和非流式。
    """
    try:
        body = await request.json()
    except Exception:
        return JSONResponse({"error": {"message": "Invalid JSON body"}}, status_code=400)

    model = body.get("model", "")
    if not model:
        return JSONResponse({"error": {"message": "model is required"}}, status_code=400)

    # 优先使用策略路由，无策略时 fallback 到按 model 查找厂商
    vendor_info = llm_config.get_route_by_strategy()
    if vendor_info:
        # 策略路由：用策略指定的 model_id 覆盖请求体中的 model
        body["model"] = vendor_info.get("model_id", model)
    else:
        vendor_info = llm_config.get_vendor_for_model(model)

    if not vendor_info:
        return JSONResponse(
            {"error": {"message": f"No available vendor for model: {model}"}},
            status_code=404,
        )

    base_url = vendor_info["base_url"].rstrip("/")
    target_url = f"{base_url}/chat/completions"
    api_key = vendor_info["api_key"]

    # 构建转发请求头
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }

    is_stream = body.get("stream", False)

    logger.info(f"[LLM Proxy] {model} -> {vendor_info['vendor_name']} ({target_url}) stream={is_stream}")

    if is_stream:
        # 流式转发：用 aiter_lines 逐行转发 SSE 事件，更可靠
        async def stream_generator():
            try:
                async with httpx.AsyncClient(timeout=httpx.Timeout(300.0, connect=10.0)) as client:
                    async with client.stream(
                        "POST",
                        target_url,
                        json=body,
                        headers=headers,
                    ) as resp:
                        if resp.status_code != 200:
                            error_body = await resp.aread()
                            logger.error(f"[LLM Proxy] upstream error: {resp.status_code} {error_body.decode()}")
                            yield f"data: {error_body.decode()}\n\n"
                            yield "data: [DONE]\n\n"
                            return
                        async for line in resp.aiter_lines():
                            yield line + "\n"
            except Exception as e:
                logger.error(f"[LLM Proxy] stream error: {e}")
                yield f"data: {{\"error\": \"stream interrupted: {str(e)}\"}}\n\n"
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
    else:
        # 非流式转发
        async with httpx.AsyncClient(timeout=httpx.Timeout(300.0, connect=10.0)) as client:
            resp = await client.post(target_url, json=body, headers=headers)
            if resp.status_code != 200:
                logger.error(f"[LLM Proxy] upstream error: {resp.status_code} {resp.text}")
                return JSONResponse(
                    {"error": {"message": f"Upstream error: {resp.status_code}", "detail": resp.text}},
                    status_code=resp.status_code,
                )
            return JSONResponse(resp.json(), status_code=200)

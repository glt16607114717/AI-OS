"""
工作台独立聊天接口（与代理完全分离）

功能：
- 独立请求大模型（不注入上帝指令、不压缩工具描述）
- 支持 Function Calling（工具调用循环）
- 流式 SSE 输出
- 复用 llm_config 的路由策略和故障转移

@author 桂良涛
"""

import asyncio
import json
import logging
import traceback

import httpx
from fastapi import APIRouter, Request
from fastapi.responses import StreamingResponse

import llm_config
from tools.registry import get_tool_definitions, execute as tool_execute

logger = logging.getLogger("agent")

router = APIRouter()

# 工具调用循环（无上限，直到大模型返回最终回答）


# ── 系统提示词 ──

SYSTEM_PROMPT = """你是一个智能工作助手，可以帮助用户查询数据库、分析数据等。

当前可用的数据库：nnd_robot_test（测试环境）

当用户的问题匹配到预置技能（skill_ 开头的工具）时，优先使用技能。技能的 SQL 已经写好了，你只需要确认用户的意图匹配后直接调用即可。

注意事项：
- 只执行 SELECT 查询，不要修改数据
- 返回结果时用清晰的格式展示
"""


def _get_route() -> dict | None:
    """获取当前激活策略的路由信息"""
    return llm_config.get_route_by_strategy()


def _get_all_routes() -> list[dict]:
    """获取所有可用路由（用于故障转移）"""
    return llm_config.get_all_routes_for_failover()


async def _request_llm_stream(
    messages: list[dict],
    route: dict,
    use_tools: bool = True,
):
    """
    向大模型发起流式请求。

    @param messages 消息列表
    @param route 路由信息 {base_url, api_key, model_id, ...}
    @param use_tools 是否携带工具定义
    @yield SSE 格式的原始数据块
    """
    url = f"{route['base_url']}/chat/completions"
    headers = {
        "Authorization": f"Bearer {route['api_key']}",
        "Content-Type": "application/json",
    }
    body = {
        "model": route["model_id"],
        "messages": messages,
        "stream": True,
    }
    if use_tools and get_tool_definitions():
        body["tools"] = get_tool_definitions()
        body["tool_choice"] = "auto"

    tool_calls_data = []
    content_parts = []
    line_count = 0
    got_done_marker = False

    try:
        async with httpx.AsyncClient(timeout=httpx.Timeout(60.0, connect=10.0)) as client:
            async with client.stream("POST", url, json=body, headers=headers) as resp:
                if resp.status_code != 200:
                    error_text = await resp.aread()
                    error_msg = f"HTTP {resp.status_code}: {error_text.decode('utf-8', errors='replace')[:500]}"
                    logger.error(f"[Workspace] 请求失败: {route['vendor_name']} -> {error_msg}")
                    raise Exception(error_msg)

                async for line in resp.aiter_lines():
                    line_count += 1
                    if not line.startswith("data: "):
                        continue
                    data = line[6:]
                    if data.strip() == "[DONE]":
                        got_done_marker = True
                        break

                    try:
                        chunk = json.loads(data)
                    except json.JSONDecodeError:
                        continue

                    delta = chunk.get("choices", [{}])[0] if chunk.get("choices") else {}
                    delta = delta.get("delta", {})

                    # 收集内容
                    content = delta.get("content", "")
                    if content:
                        content_parts.append(content)

                    # 收集工具调用（注意：API 可能返回 null 而非 []）
                    tc_list = delta.get("tool_calls") or []
                    for tc in tc_list:
                        idx = tc.get("index", 0)
                        while len(tool_calls_data) <= idx:
                            tool_calls_data.append({"id": "", "function": {"name": "", "arguments": ""}})
                        if tc.get("id"):
                            tool_calls_data[idx]["id"] = tc["id"]
                        if tc.get("function", {}).get("name"):
                            tool_calls_data[idx]["function"]["name"] += tc["function"]["name"]
                        if tc.get("function", {}).get("arguments"):
                            tool_calls_data[idx]["function"]["arguments"] += tc["function"]["arguments"]

                    # 透传原始 SSE 给前端（仅内容部分）
                    if content:
                        yield f"data: {json.dumps({'choices': [{'delta': {'content': content}}]}, ensure_ascii=False)}\n\n"

    except (httpx.ConnectTimeout, httpx.ConnectError) as e:
        logger.warning(f"[Workspace] 连接失败: {route['vendor_name']} -> {type(e).__name__}")
        raise
    except Exception as e:
        logger.error(f"[Workspace] 请求异常: {route['vendor_name']} -> {type(e).__name__}: {e}")
        logger.debug(traceback.format_exc())
        raise

    # 流结束，记录详细信息
    final_content = "".join(content_parts)
    final_tool_calls = tool_calls_data if tool_calls_data else None
    logger.info(
        f"[Workspace] LLM响应: lines={line_count}, done={got_done_marker}, "
        f"content_len={len(final_content)}, tool_calls={len(tool_calls_data) if tool_calls_data else 0}, "
        f"vendor={route.get('vendor_name', '?')}"
    )
    if not final_content.strip() and not final_tool_calls:
        logger.warning(
            f"[Workspace] ⚠️ 空响应! lines={line_count}, done={got_done_marker}, "
            f"messages_count={len(body.get('messages', []))}"
        )

    yield ("_result", {
        "content": final_content,
        "tool_calls": final_tool_calls,
    })


async def _request_llm_non_stream(
    messages: list[dict],
    route: dict,
    use_tools: bool = True,
) -> dict:
    """
    向大模型发起非流式请求（用于工具调用场景的简单回退）。
    """
    url = f"{route['base_url']}/chat/completions"
    headers = {
        "Authorization": f"Bearer {route['api_key']}",
        "Content-Type": "application/json",
    }
    body = {
        "model": route["model_id"],
        "messages": messages,
        "stream": False,
    }
    if use_tools and get_tool_definitions():
        body["tools"] = get_tool_definitions()
        body["tool_choice"] = "auto"

    async with httpx.AsyncClient(timeout=httpx.Timeout(60.0, connect=10.0)) as client:
        resp = await client.post(url, json=body, headers=headers)
        if resp.status_code != 200:
            raise Exception(f"HTTP {resp.status_code}: {resp.text[:500]}")
        return resp.json()


async def _chat_with_tools(messages: list[dict], route: dict):
    """
    带工具调用的聊天循环。

    流程：
    1. 请求大模型（带 tools 参数）
    2. 如果大模型返回 tool_calls → 执行工具 → 将结果加入 messages → 再次请求
    3. 如果大模型返回普通内容 → 流式输出给前端
    4. 最多循环 MAX_TOOL_ROUNDS 次
    """
    round_idx = 0
    while True:
        tool_calls_data = None
        content_parts = []

        # 尝试流式请求（含重试）
        routes_to_try = [route]
        if route.get("vendor_id"):
            extra = _get_all_routes()
            for r in extra:
                if r["vendor_id"] != route["vendor_id"]:
                    routes_to_try.append(r)

        last_error = None
        for attempt in range(3):  # 最多重试 3 次
            for try_route in routes_to_try:
                try:
                    tool_calls_data = None
                    content_parts = []

                    async for event in _request_llm_stream(messages, try_route):
                        if isinstance(event, tuple) and event[0] == "_result":
                            result = event[1]
                            content_parts.append(result["content"])
                            tool_calls_data = result["tool_calls"]
                        else:
                            yield event  # 透传 SSE

                    last_error = None
                    break  # 成功，跳出路由重试

                except (httpx.ConnectTimeout, httpx.ConnectError, Exception) as e:
                    last_error = e
                    logger.warning(f"[Workspace] 路由失败: {try_route['vendor_name']} -> {type(e).__name__}: {e}")
                    continue

            if last_error is None:
                break  # 成功

            # 所有路由都失败，等待后重试
            logger.warning(f"[Workspace] 所有路由失败，第 {attempt + 1}/3 次重试...")
            await asyncio.sleep(2 * (attempt + 1))  # 递增延迟：2s, 4s, 6s

        if last_error:
            yield f"data: {json.dumps({'error': f'all vendors failed after retries: {type(last_error).__name__}: {last_error}'})}\n\n"
            return

        # 检查是否有工具调用
        if tool_calls_data:
            # 将工具调用信息加入 messages
            assistant_msg = {"role": "assistant", "content": "".join(content_parts) or None}
            assistant_msg["tool_calls"] = [
                {
                    "id": tc["id"],
                    "type": "function",
                    "function": {
                        "name": tc["function"]["name"],
                        "arguments": tc["function"]["arguments"],
                    },
                }
                for tc in tool_calls_data
            ]
            messages.append(assistant_msg)

            # 执行每个工具调用
            for tc in tool_calls_data:
                func_name = tc["function"]["name"]
                func_args = tc["function"]["arguments"]

                # 通知前端：正在调用工具
                yield f"data: {json.dumps({'tool_call': {'name': func_name, 'arguments': func_args}}, ensure_ascii=False)}\n\n"

                # 执行工具
                result_str = tool_execute(func_name, func_args)

                # 通知前端：工具调用完成
                yield f"data: {json.dumps({'tool_result': {'name': func_name, 'result_preview': result_str[:200]}}, ensure_ascii=False)}\n\n"

                # 将工具结果加入 messages
                messages.append({
                    "role": "tool",
                    "tool_call_id": tc["id"],
                    "content": result_str,
                })

            # 继续循环，再次请求大模型（让它根据工具结果生成回答）
            round_idx += 1
            continue
        else:
            # 没有工具调用，说明是最终回答，已经流式输出了
            combined_content = "".join(content_parts)
            if not combined_content.strip():
                # 大模型返回空内容（可能上下文过长），通知前端
                logger.warning(f"[Workspace] LLM返回空内容，round={round_idx}, messages={len(messages)}")
                yield f"data: {json.dumps({'choices': [{'delta': {'content': '（大模型未返回有效回答，可能上下文过长，请尝试清空对话后重试）'}}]}, ensure_ascii=False)}\n\n"
            break

    yield "data: [DONE]\n\n"


@router.post("/api/workspace/chat")
async def workspace_chat(request: Request):
    """
    工作台聊天接口（独立于代理）。

    请求体：
    {
        "messages": [{"role": "user", "content": "..."}],
        "stream": true
    }

    流式返回 SSE 格式。
    """
    body = await request.json()
    user_messages = body.get("messages", [])

    if not user_messages:
        return {"ok": False, "error": "消息不能为空"}

    # 获取路由
    route = _get_route()
    if not route:
        return {"ok": False, "error": "模型未加载，请先配置策略"}

    # 构建完整消息列表：系统提示词 + 用户消息
    messages = [{"role": "system", "content": SYSTEM_PROMPT}] + user_messages

    # 裁剪消息：保留系统提示词 + 最近 20 条消息（防止上下文过长）
    if len(messages) > 22:
        messages = [messages[0]] + messages[-21:]
        logger.info(f"[Workspace] 消息裁剪: 保留最近 21 条（共 {len(user_messages) + 1} 条）")

    # 裁剪工具结果的长度（每条 tool 结果最多 2000 字符）
    for msg in messages:
        if msg.get("role") == "tool" and len(msg.get("content", "")) > 2000:
            original_len = len(msg["content"])
            msg["content"] = msg["content"][:2000] + f"\n...（已截断，原始 {original_len} 字符）"

    logger.info(f"[Workspace] 开始对话: {len(messages)} 条消息, model={route.get('model_id')}, vendor={route.get('vendor_name')}")

    return StreamingResponse(
        _chat_with_tools(messages, route),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
            "X-Accel-Buffering": "no",
        },
    )

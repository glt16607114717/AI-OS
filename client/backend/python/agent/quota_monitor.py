"""
智谱 AI 用量监控模块。

功能：
1. 定时（每 10 分钟）查询每个智谱 API key 的 5 小时用量，缓存结果
2. 路由时根据缓存的用量判断：
   - < 70%:  正常使用（glm-5.1）
   - 70%-90%: 降级使用（glm-4.7）
   - > 90%:  轮询策略跳过该 key；固定策略报错
3. 提供 API 查询当前状态

智谱 Coding Plan 按 5 小时窗口限额，查询接口：
  GET https://bigmodel.cn/api/monitor/usage/quota/limit
  Authorization: Bearer {api_key}

作者：桂良涛
"""

import logging
import threading
import time
from datetime import datetime

import httpx

logger = logging.getLogger("quota")

# ── 配置 ──
QUOTA_API_URL = "https://bigmodel.cn/api/monitor/usage/quota/limit"
CHECK_INTERVAL = 600  # 10 分钟
REQUEST_TIMEOUT = 15  # 单次请求超时（秒）

# 降级映射
DOWNGRADE_MAP = {
    "glm-5.1": "glm-4.7",
    "glm-5-turbo": "glm-4.7",
}

# ── 运行时状态 ──
_lock = threading.Lock()
_quota_cache: dict[str, dict] = {}  # key_id → {pct, level, next_reset, updated_at, error}
_last_check: str = ""
_enabled: bool = True


# ── 对外查询接口 ──

def get_key_status(key_id: str) -> str:
    """
    返回某个 key 的用量等级。
    - "normal":  < 70%，正常使用
    - "degraded": 70%-90%，降级为 glm-4.7
    - "exhausted": > 90%，应跳过该 key
    """
    if not _enabled:
        return "normal"
    with _lock:
        info = _quota_cache.get(key_id)
    if not info or info.get("error"):
        return "normal"
    pct = info.get("pct", 0.0)
    if pct >= 0.9:
        return "exhausted"
    elif pct >= 0.7:
        return "degraded"
    return "normal"


def should_downgrade_model(key_id: str, model_id: str) -> str:
    """如果 key 处于降级状态且 model 在映射表中，返回降级后的 model。"""
    if get_key_status(key_id) == "degraded":
        return DOWNGRADE_MAP.get(model_id, model_id)
    return model_id


def is_key_exhausted(key_id: str) -> bool:
    """检查 key 是否已耗尽（> 90%）。"""
    return get_key_status(key_id) == "exhausted"


def get_all_keys_status() -> list[dict]:
    """获取所有厂商所有 key 的用量状态（含不支持自动查询的厂商）。"""
    from llm_config import get_keys_data, VENDOR_CATALOG
    keys_data = get_keys_data()
    all_keys = []

    # 从 VENDOR_CATALOG 构建 vendor_id → name 映射
    vendor_name_map = {v["id"]: v["name"] for v in VENDOR_CATALOG}

    # 厂商级信息：哪些支持自动查询
    auto_query_vendors = {"zhipu"}

    for vendor_id, keys in keys_data.get("keys", {}).items():
        vendor_enabled = keys_data.get("vendor_enabled", {}).get(vendor_id, False)
        for key_info in keys:
            if not key_info.get("enabled", False):
                continue
            key_id = key_info.get("id", "")
            key_name = key_info.get("name", key_id[:8])

            record = {
                "vendor_id": vendor_id,
                "vendor_name": vendor_name_map.get(vendor_id, vendor_id),
                "key_id": key_id,
                "key_name": key_name,
                "enabled": key_info.get("enabled", False),
                "supports_auto_query": vendor_id in auto_query_vendors,
                "pct": 0.0,
                "status": "normal",
                "status_text": "正常",
                "level_tier": "",
                "next_reset": "",
                "updated_at": "",
                "error": None,
            }

            # 如果支持自动查询，从缓存中取数据
            if vendor_id in auto_query_vendors:
                with _lock:
                    cached = _quota_cache.get(key_id, {})
                if cached:
                    record["pct"] = cached.get("pct", 0.0)
                    record["level_tier"] = cached.get("level", "")
                    record["status"] = _calc_status(cached.get("pct", 0.0))
                    record["next_reset"] = cached.get("next_reset", "")
                    record["updated_at"] = cached.get("updated_at", "")
                    record["error"] = cached.get("error")
                    pct_val = cached.get("pct", 0.0) * 100
                    status_map = {"normal": "正常", "degraded": "降级中", "exhausted": "已耗尽"}
                    record["status_text"] = status_map.get(record["status"], "正常") + f"（{pct_val:.1f}%）"
                else:
                    record["status_text"] = "未查询"

            all_keys.append(record)

    return all_keys


def force_check():
    """强制立即执行一次检查（不等待定时器）。"""
    logger.info("[Quota] force check triggered")
    _run_check()


def get_status() -> dict:
    """获取当前监控状态（供 API 返回）。"""
    with _lock:
        records = []
        for kid, info in _quota_cache.items():
            records.append({
                "key_id": kid,
                "key_name": info.get("key_name", kid[:8]),
                "pct": info.get("pct", 0.0),
                "level_tier": info.get("level", ""),
                "status": _calc_status(info.get("pct", 0.0)),
                "next_reset": info.get("next_reset", ""),
                "updated_at": info.get("updated_at", ""),
                "error": info.get("error"),
            })
        return {
            "enabled": _enabled,
            "last_check": _last_check,
            "records": records,
        }


def set_enabled(enabled: bool):
    """启用/停用监控。"""
    global _enabled
    with _lock:
        _enabled = enabled
    logger.info("[Quota] monitor %s", "enabled" if enabled else "disabled")


def _calc_status(pct: float) -> str:
    if pct >= 0.9:
        return "exhausted"
    elif pct >= 0.7:
        return "degraded"
    return "normal"


# ── 查询逻辑 ──

def _check_single_key(api_key: str, key_id: str, key_name: str) -> dict:
    """
    查询单个 API key 的 5 小时用量。
    返回: {pct, level, next_reset, error}
    """
    result = {"pct": 0.0, "level": "", "next_reset": "", "error": None}
    try:
        resp = httpx.get(
            QUOTA_API_URL,
            headers={"Authorization": f"Bearer {api_key}"},
            timeout=REQUEST_TIMEOUT,
        )
        if resp.status_code != 200:
            result["error"] = f"HTTP {resp.status_code}"
            return result

        body = resp.json()
        if body.get("code") != 200:
            result["error"] = body.get("msg", "unknown error")
            return result

        data = body.get("data", {})
        result["level"] = data.get("level", "")
        limits = data.get("limits", [])

        for item in limits:
            if item.get("type") == "TOKENS_LIMIT":
                pct_raw = item.get("percentage", 0)
                # percentage 0-100（如 1 = 1%），转为 0-1
                result["pct"] = round(pct_raw / 100, 4) if pct_raw > 0.01 else round(pct_raw, 4)
                reset_ts = item.get("nextResetTime", 0)
                if reset_ts > 1e12:
                    reset_ts = reset_ts / 1000
                if reset_ts > 0:
                    result["next_reset"] = datetime.fromtimestamp(reset_ts).strftime("%Y-%m-%d %H:%M:%S")
                break

    except Exception as e:
        result["error"] = str(e)
        logger.error("[Quota] key %s error: %s", key_name, e)

    return result


def _run_check():
    """执行一次完整的用量检查（遍历所有智谱 key）。"""
    from llm_config import get_keys_data

    keys_data = get_keys_data()
    zhipu_keys = keys_data.get("keys", {}).get("zhipu", [])
    vendor_enabled = keys_data.get("vendor_enabled", {}).get("zhipu", False)

    if not vendor_enabled or not zhipu_keys:
        return

    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    for key_info in zhipu_keys:
        if not key_info.get("enabled", False):
            continue
        api_key = key_info.get("api_key", "")
        key_id = key_info.get("id", "")
        key_name = key_info.get("name", key_id[:8])
        if not api_key:
            continue

        result = _check_single_key(api_key, key_id, key_name)
        result["key_name"] = key_name
        result["updated_at"] = now

        old_info = _quota_cache.get(key_id, {})
        old_status = _calc_status(old_info.get("pct", 0.0)) if old_info else "normal"
        new_status = _calc_status(result["pct"])

        with _lock:
            _quota_cache[key_id] = result
            _last_check = now

        if new_status != old_status:
            logger.warning("[Quota] key %s (%s): %.1f%% → %s",
                           key_name, key_id[:8], result["pct"] * 100, new_status)
            # 写入操作日志
            try:
                import llm_log
                status_text = {"normal": "恢复正常", "degraded": "降级运行", "exhausted": "已耗尽"}
                level_map = {"normal": "info", "degraded": "warning", "exhausted": "error"}
                llm_log.write_log(
                    "quota",
                    f"{key_name}: {old_status} → {status_text.get(new_status, new_status)}（{result['pct']*100:.1f}%）",
                    level=level_map.get(new_status, "info"),
                    detail=f"level={result.get('level','')},reset={result.get('next_reset','')}",
                )
            except Exception:
                pass
        else:
            logger.info("[Quota] key %s (%s): %.1f%% [%s]",
                        key_name, key_id[:8], result["pct"] * 100, new_status)


def _background_loop():
    """后台循环：每 CHECK_INTERVAL 秒执行一次检查。"""
    time.sleep(30)
    while True:
        try:
            if _enabled:
                _run_check()
        except Exception as e:
            logger.error("[Quota] background check error: %s", e)
        time.sleep(CHECK_INTERVAL)


_thread: threading.Thread | None = None


def start():
    """启动后台监控线程（幂等）。"""
    global _thread
    if _thread is not None and _thread.is_alive():
        return
    _thread = threading.Thread(target=_background_loop, daemon=True, name="quota-monitor")
    _thread.start()
    logger.info("[Quota] background monitor started (interval=%ds)", CHECK_INTERVAL)

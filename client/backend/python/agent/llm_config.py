"""
大模型厂商预置目录 + 密钥管理模块

厂商目录硬编码在 VENDOR_CATALOG，JSON 文件仅存储用户密钥和启用状态。

作者：桂良涛，邮箱：桂良涛@nndrobot.com
"""

import json
import copy
import uuid
import logging
import threading
from pathlib import Path

logger = logging.getLogger("agent")

# ── 存储路径 ──

_BASE_DIR = Path(r"C:\ProgramData\AI-OS\config")
_KEYS_FILE = _BASE_DIR / "llm_keys.json"

_file_lock = threading.Lock()


# ── 厂商预置目录（硬编码，不可由用户修改） ──

VENDOR_CATALOG = [
    {
        "id": "zhipu",
        "name": "智谱 AI",
        "base_url": "https://open.bigmodel.cn/api/coding/paas/v4",
        "sort_order": 1,
        "models": [
            {"model_id": "glm-5.1", "display_name": "GLM-5.1", "description": "旗舰基座，200K 上下文，128K 输出，对标 Claude Opus 4.6，复杂推理首选"},
            {"model_id": "glm-5-turbo", "display_name": "GLM-5-Turbo", "description": "5 代快速版，200K 上下文，日常编码推荐"},
            {"model_id": "glm-5", "display_name": "GLM-5", "description": "5 代标准，745B MoE，200K 上下文，编程 SOTA"},
            {"model_id": "glm-4.7", "display_name": "GLM-4.7", "description": "4.7 代，对标 Claude Sonnet，日常开发性价比最高"},
            {"model_id": "glm-4.5-air", "display_name": "GLM-4.5-Air", "description": "4.5 代轻量版，轻量高效"},
            {"model_id": "glm-4-flash", "display_name": "GLM-4-Flash", "description": "4 代免费版，零成本调用"},
        ],
    },
    {
        "id": "xiaomi",
        "name": "小米 MiMo",
        "base_url": "https://token-plan-cn.xiaomimimo.com/v1",
        "sort_order": 2,
        "models": [
            {"model_id": "mimo-v2.5-pro", "display_name": "MiMo-V2.5-Pro", "description": "旗舰 Agent/Coding，1.02T 参数 42B 激活，1M 上下文，128K 输出"},
            {"model_id": "mimo-v2.5", "display_name": "MiMo-V2.5", "description": "全模态，支持文本/图像/视频/音频，1M 上下文"},
            {"model_id": "mimo-v2-flash", "display_name": "MiMo-V2-Flash", "description": "轻量快速，309B 参数 15B 激活，256K 上下文"},
        ],
    },
    {
        "id": "deepseek",
        "name": "DeepSeek",
        "base_url": "https://api.deepseek.com/v1",
        "sort_order": 3,
        "models": [
            {"model_id": "deepseek-v4-pro", "display_name": "DeepSeek-V4-Pro", "description": "V4 旗舰，1M 上下文"},
            {"model_id": "deepseek-v4-flash", "display_name": "DeepSeek-V4-Flash", "description": "V4 快速版，高性价比"},
            {"model_id": "deepseek-chat", "display_name": "DeepSeek-Chat", "description": "通用对话，兼容旧名，当前指向 V4-Flash 非思考模式"},
            {"model_id": "deepseek-reasoner", "display_name": "DeepSeek-Reasoner", "description": "深度思考模式，兼容旧名，当前指向 V4-Flash 思考模式"},
        ],
    },
    {
        "id": "qwen",
        "name": "阿里通义千问",
        "base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1",
        "sort_order": 4,
        "models": [
            {"model_id": "qwen3.7-plus", "display_name": "Qwen3.7-Plus", "description": "最新旗舰，1M 上下文，通用多模态"},
            {"model_id": "qwen3.6-plus", "display_name": "Qwen3.6-Plus", "description": "3.6 代 Plus，高性价比"},
            {"model_id": "qwen3.6-flash", "display_name": "Qwen3.6-Flash", "description": "3.6 代快速版，低延迟低成本"},
            {"model_id": "qwen-plus", "display_name": "Qwen-Plus", "description": "经典 Plus 版本，稳定可靠"},
            {"model_id": "qwen3-coder-plus", "display_name": "Qwen3-Coder-Plus", "description": "编程专用模型"},
            {"model_id": "qwen3-coder-flash", "display_name": "Qwen3-Coder-Flash", "description": "编程快速版"},
        ],
    },
    {
        "id": "moonshot",
        "name": "月之暗面 Kimi",
        "base_url": "https://api.moonshot.cn/v1",
        "sort_order": 5,
        "models": [
            {"model_id": "kimi-k2.6", "display_name": "Kimi-K2.6", "description": "旗舰，1T MoE 32B 激活，256K 上下文，SWE-Bench 全球领先"},
            {"model_id": "moonshot-v1-128k", "display_name": "Moonshot-V1-128K", "description": "经典版，128K 上下文"},
            {"model_id": "moonshot-v1-32k", "display_name": "Moonshot-V1-32K", "description": "经典版，32K 上下文"},
            {"model_id": "moonshot-v1-8k", "display_name": "Moonshot-V1-8K", "description": "经典版，8K 上下文，低成本"},
        ],
    },
    {
        "id": "doubao",
        "name": "字节豆包",
        "base_url": "https://ark.cn-beijing.volces.com/api/v3",
        "sort_order": 6,
        "models": [
            {"model_id": "doubao-seed-1-6-251015", "display_name": "Doubao-Seed-1.6", "description": "旗舰，256K 上下文，支持思考/非思考模式"},
            {"model_id": "doubao-1-5-pro-32k-250115", "display_name": "Doubao-1.5-Pro", "description": "1.5 代专业版，32K 上下文"},
            {"model_id": "doubao-1-5-lite-32k-250115", "display_name": "Doubao-1.5-Lite", "description": "1.5 代轻量版，32K 上下文"},
        ],
    },
    {
        "id": "spark",
        "name": "讯飞星火",
        "base_url": "https://spark-api-open.xf-yun.com/v1",
        "sort_order": 7,
        "models": [
            {"model_id": "4.0Ultra", "display_name": "星火 4.0 Ultra", "description": "最强非思考模型，32K 上下文"},
            {"model_id": "generalv3.5", "display_name": "星火 Max", "description": "旗舰级，结构化抽取、逻辑推理优秀"},
            {"model_id": "generalv3", "display_name": "星火 Pro", "description": "专业级，支持 128K 上下文"},
            {"model_id": "lite", "display_name": "星火 Lite", "description": "轻量版，免费使用"},
        ],
    },
    {
        "id": "wenxin",
        "name": "百度文心",
        "base_url": "https://qianfan.baidubce.com/v2",
        "sort_order": 8,
        "models": [
            {"model_id": "ernie-4.0-8k", "display_name": "ERNIE-4.0", "description": "旗舰模型，8K 上下文"},
            {"model_id": "ernie-3.5-8k", "display_name": "ERNIE-3.5", "description": "高性价比版本"},
            {"model_id": "ernie-speed-8k", "display_name": "ERNIE-Speed", "description": "快速版，免费"},
        ],
    },
    {
        "id": "siliconflow",
        "name": "硅基流动",
        "base_url": "https://api.siliconflow.cn/v1",
        "sort_order": 9,
        "models": [
            {"model_id": "deepseek-ai/DeepSeek-V3.2", "display_name": "DeepSeek-V3.2", "description": "DeepSeek V3.2 满血版"},
            {"model_id": "deepseek-ai/DeepSeek-R1", "display_name": "DeepSeek-R1", "description": "R1 推理满血版 671B"},
            {"model_id": "Qwen/Qwen3-235B-A22B-Instruct", "display_name": "Qwen3-235B", "description": "Qwen3 旗舰 MoE"},
        ],
    },
    {
        "id": "openai",
        "name": "OpenAI",
        "base_url": "https://api.openai.com/v1",
        "sort_order": 10,
        "models": [
            {"model_id": "gpt-4o", "display_name": "GPT-4o", "description": "多模态旗舰"},
            {"model_id": "gpt-4o-mini", "display_name": "GPT-4o-Mini", "description": "高性价比版"},
            {"model_id": "o3", "display_name": "o3", "description": "推理模型"},
            {"model_id": "o4-mini", "display_name": "o4-mini", "description": "轻量推理模型"},
        ],
    },
]


# ── 内部读写 ──

def _ensure_dir():
    _BASE_DIR.mkdir(parents=True, exist_ok=True)


def _load_keys_data() -> dict:
    """读取密钥文件，不存在则返回空结构"""
    if _KEYS_FILE.exists():
        try:
            return json.loads(_KEYS_FILE.read_text(encoding="utf-8"))
        except Exception:
            pass
    return {"keys": {}, "vendor_enabled": {}}


def get_keys_data() -> dict:
    """读取密钥文件的公开接口（线程安全）。"""
    with _file_lock:
        return _load_keys_data()


def _save_keys_data(data: dict) -> None:
    """写入密钥文件（调用方需持锁）"""
    _ensure_dir()
    _KEYS_FILE.write_text(
        json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8"
    )


# ── 公开接口 ──

def get_catalog() -> list[dict]:
    """
    返回完整厂商目录，每个 vendor 附带用户密钥和启用状态。
    密钥中的 api_key 原样返回（仅本地使用，不涉及网络传输）。
    """
    with _file_lock:
        keys_data = _load_keys_data()

    vendor_keys = keys_data.get("keys", {})
    vendor_enabled = keys_data.get("vendor_enabled", {})

    result = []
    for vendor in VENDOR_CATALOG:
        v = copy.deepcopy(vendor)
        vid = v["id"]
        v["keys"] = vendor_keys.get(vid, [])
        v["enabled"] = vendor_enabled.get(vid, False)
        result.append(v)

    return result


def save_vendor_keys(vendor_id: str, keys: list[dict]) -> dict:
    """
    保存某个厂商的密钥列表。
    keys 格式: [{"name": "xxx", "api_key": "xxx", "enabled": true}]
    自动为新密钥补充 id 字段。
    返回 {"ok": True} 或 {"ok": False, "error": "..."}。
    """
    # 校验 vendor_id 合法性
    valid_ids = {v["id"] for v in VENDOR_CATALOG}
    if vendor_id not in valid_ids:
        return {"ok": False, "error": "无效的 vendor_id"}

    # 为缺少 id 的密钥自动生成
    processed = []
    for k in keys:
        item = {
            "id": k.get("id") or uuid.uuid4().hex[:12],
            "name": k.get("name", ""),
            "api_key": k.get("api_key", ""),
            "enabled": k.get("enabled", True),
        }
        processed.append(item)

    # 检查被删除的 key 是否被策略引用
    new_key_ids = {k["id"] for k in processed}
    with _file_lock:
        data = _load_keys_data()
        old_keys = data.get("keys", {}).get(vendor_id, [])
        removed_ids = [k["id"] for k in old_keys if k["id"] not in new_key_ids]

        if removed_ids:
            strategies = data.get("strategies", [])
            conflicts = []
            for s in strategies:
                for o in s.get("options", []):
                    if o.get("vendor_id") == vendor_id and o.get("key_id") in removed_ids:
                        conflicts.append(s.get("name", s.get("id", "")))
                        break
            if conflicts:
                return {"ok": False, "error": f"密钥被策略 [{', '.join(conflicts)}] 引用，请先修改策略"}

    with _file_lock:
        data = _load_keys_data()
        data.setdefault("keys", {})[vendor_id] = processed
        _save_keys_data(data)

    return {"ok": True}


def toggle_vendor(vendor_id: str, enabled: bool) -> bool:
    """启用或禁用某个厂商"""
    valid_ids = {v["id"] for v in VENDOR_CATALOG}
    if vendor_id not in valid_ids:
        return False

    with _file_lock:
        data = _load_keys_data()
        data.setdefault("vendor_enabled", {})[vendor_id] = enabled
        _save_keys_data(data)

    return True


def get_vendor_for_model(model: str) -> dict | None:
    """
    根据 model 名称查找所属厂商，返回 {vendor_id, base_url, api_key}。
    优先精确匹配 model_id，其次模糊前缀匹配。
    从已启用且至少有一个可用密钥的厂商中查找。
    """
    with _file_lock:
        keys_data = _load_keys_data()

    vendor_keys = keys_data.get("keys", {})
    vendor_enabled = keys_data.get("vendor_enabled", {})

    # 精确匹配
    for vendor in VENDOR_CATALOG:
        vid = vendor["id"]
        if not vendor_enabled.get(vid, False):
            continue
        for m in vendor["models"]:
            if m["model_id"] == model:
                # 取第一个已启用的密钥
                keys = [k for k in vendor_keys.get(vid, []) if k.get("enabled", True)]
                if not keys:
                    continue
                return {
                    "vendor_id": vid,
                    "vendor_name": vendor["name"],
                    "base_url": vendor["base_url"],
                    "api_key": keys[0]["api_key"],
                }

    # 模糊前缀匹配（如 "glm-" 匹配智谱）
    for vendor in VENDOR_CATALOG:
        vid = vendor["id"]
        if not vendor_enabled.get(vid, False):
            continue
        for m in vendor["models"]:
            if model.startswith(m["model_id"].split("-")[0]):
                keys = [k for k in vendor_keys.get(vid, []) if k.get("enabled", True)]
                if not keys:
                    continue
                return {
                    "vendor_id": vid,
                    "vendor_name": vendor["name"],
                    "base_url": vendor["base_url"],
                    "api_key": keys[0]["api_key"],
                }

    return None


# ── 转发策略管理 ──

_rr_counter = 0


def get_available_options() -> list[dict]:
    """
    返回所有可用的选项（已启用厂商 + 有密钥 + 模型）。
    过滤掉没有 api_key 的密钥。

    @author 桂良涛
    @return list[dict] 可用选项列表
    """
    with _file_lock:
        keys_data = _load_keys_data()

    vendor_keys = keys_data.get("keys", {})
    vendor_enabled = keys_data.get("vendor_enabled", {})

    options = []
    for vendor in VENDOR_CATALOG:
        vid = vendor["id"]
        if not vendor_enabled.get(vid, False):
            continue
        for key in vendor_keys.get(vid, []):
            if not key.get("enabled", True):
                continue
            api_key = key.get("api_key", "")
            if not api_key:
                continue
            for model in vendor["models"]:
                options.append({
                    "vendor_id": vid,
                    "vendor_name": vendor["name"],
                    "key_id": key.get("id", ""),
                    "key_name": key.get("name", ""),
                    "model_id": model["model_id"],
                    "display_name": model["display_name"],
                    "base_url": vendor["base_url"],
                    "api_key": api_key,
                })

    return options


def get_strategies() -> list[dict]:
    """
    返回策略列表，每个 option 补充 vendor_name、key_name、display_name。

    @author 桂良涛
    @return list[dict] 策略列表
    """
    with _file_lock:
        keys_data = _load_keys_data()

    strategies = keys_data.get("strategies", [])
    vendor_map = {v["id"]: v for v in VENDOR_CATALOG}
    vendor_keys = keys_data.get("keys", {})

    result = []
    for s in strategies:
        s_copy = copy.deepcopy(s)
        for opt in s_copy.get("options", []):
            vid = opt.get("vendor_id", "")
            vendor = vendor_map.get(vid)
            if vendor:
                opt["vendor_name"] = vendor["name"]
                opt["base_url"] = vendor["base_url"]
                for k in vendor_keys.get(vid, []):
                    if k.get("id") == opt.get("key_id"):
                        opt["key_name"] = k.get("name", "")
                        opt["api_key"] = k.get("api_key", "")
                        break
                for m in vendor["models"]:
                    if m["model_id"] == opt.get("model_id"):
                        opt["display_name"] = m["display_name"]
                        break
        result.append(s_copy)

    return result


def save_strategy(strategy: dict) -> bool:
    """
    保存/更新策略，自动生成 id，确保只有一个是 active。

    @author 桂良涛
    @param dict strategy 策略数据
    @return bool 是否成功
    """
    with _file_lock:
        data = _load_keys_data()
        strategies = data.setdefault("strategies", [])

        # 如果设置为激活，先将所有策略置为非激活
        if strategy.get("active", False):
            for s in strategies:
                s["active"] = False

        strategy_id = strategy.get("id", "")
        if strategy_id:
            # 更新已有策略
            for i, s in enumerate(strategies):
                if s["id"] == strategy_id:
                    strategies[i] = strategy
                    break
            else:
                strategies.append(strategy)
        else:
            # 新建策略，自动生成 id
            strategy["id"] = uuid.uuid4().hex[:12]
            strategies.append(strategy)

        data["strategies"] = strategies
        _save_keys_data(data)

    return True


def delete_strategy(strategy_id: str) -> bool:
    """
    删除策略。

    @author 桂良涛
    @param str strategy_id 策略 ID
    @return bool 是否成功
    """
    with _file_lock:
        data = _load_keys_data()
        strategies = data.get("strategies", [])
        data["strategies"] = [s for s in strategies if s["id"] != strategy_id]
        _save_keys_data(data)

    return True


def set_active_strategy(strategy_id: str) -> bool:
    """
    设置激活策略，全局只有一个。

    @author 桂良涛
    @param str strategy_id 策略 ID
    @return bool 是否成功（找不到返回 False）
    """
    with _file_lock:
        data = _load_keys_data()
        strategies = data.get("strategies", [])
        found = False
        for s in strategies:
            if s["id"] == strategy_id:
                s["active"] = True
                found = True
            else:
                s["active"] = False
        if not found:
            return False
        data["strategies"] = strategies
        _save_keys_data(data)

    return True


def get_route_by_strategy() -> dict | None:
    """
    核心路由函数：根据激活策略返回路由信息。
    - fixed: 返回第一个 option（如果 key 已耗尽则返回特殊标记）
    - round_robin: 轮询返回 option，跳过已耗尽的 key
    - 无策略时返回 None

    智谱用量分级（由 quota_monitor 模块提供）：
    - normal (<70%): 正常使用
    - degraded (70%-90%): 需降级，返回后由 llm_api 处理
    - exhausted (>90%): 轮询跳过；固定策略标记

    @author 桂良涛
    @return dict | None 路由信息，可能包含 _quota_status 字段
    """
    global _rr_counter

    with _file_lock:
        keys_data = _load_keys_data()

    strategies = keys_data.get("strategies", [])

    # 查找激活策略
    active = None
    for s in strategies:
        if s.get("active", False):
            active = s
            break

    if not active:
        return None

    options = active.get("options", [])
    if not options:
        return None

    # 尝试导入 quota_monitor（可能未加载）
    try:
        from quota_monitor import is_key_exhausted
    except ImportError:
        is_key_exhausted = lambda kid: False

    # 根据策略类型选择 option
    if active.get("type", "fixed") == "round_robin":
        # 轮询：跳过 exhausted 的 key
        total = len(options)
        for _ in range(total):
            idx = _rr_counter % total
            _rr_counter += 1
            selected = options[idx]
            sid = selected.get("key_id", "")
            if not is_key_exhausted(sid):
                break
        else:
            # 所有 key 都耗尽
            logger.warning("[LLM Config] round_robin: all keys exhausted")
            return None
    else:
        selected = options[0]
        sid = selected.get("key_id", "")
        if is_key_exhausted(sid):
            logger.warning("[LLM Config] fixed strategy: key %s exhausted", sid[:8])
            # 返回路由信息但标记 exhausted，由 llm_api 返回错误
            pass  # 继续走下面的流程，llm_api 会检查

    # 补充完整的厂商/密钥信息
    vendor_map = {v["id"]: v for v in VENDOR_CATALOG}
    vendor_keys = keys_data.get("keys", {})

    vid = selected.get("vendor_id", "")
    vendor = vendor_map.get(vid)
    if not vendor:
        return None

    api_key = ""
    key_id = ""
    for k in vendor_keys.get(vid, []):
        if k.get("id") == selected.get("key_id"):
            api_key = k.get("api_key", "")
            key_id = k.get("id", "")
            break

    if not api_key:
        return None

    result = {
        "vendor_id": vid,
        "vendor_name": vendor["name"],
        "base_url": vendor["base_url"],
        "api_key": api_key,
        "key_id": key_id,
        "model_id": selected.get("model_id", ""),
    }

    # 标记 exhausted，让 llm_api 返回错误
    if is_key_exhausted(key_id):
        result["_quota_exhausted"] = True

    return result


def get_all_routes_for_failover(exclude_vendor_ids: set = None) -> list[dict]:
    """
    获取轮询策略中所有可用的路由（用于故障转移）。
    排除已失败的 vendor_id。
    固定策略或无策略返回空列表。
    """
    exclude_vendor_ids = exclude_vendor_ids or set()

    with _file_lock:
        keys_data = _load_keys_data()

    strategies = keys_data.get("strategies", [])
    active = None
    for s in strategies:
        if s.get("active", False):
            active = s
            break
    if not active or active.get("type") != "round_robin":
        return []

    options = active.get("options", [])
    if not options:
        return []

    vendor_map = {v["id"]: v for v in VENDOR_CATALOG}
    vendor_keys = keys_data.get("keys", {})

    routes = []
    try:
        from quota_monitor import is_key_exhausted
    except ImportError:
        is_key_exhausted = lambda kid: False
    for opt in options:
        vid = opt.get("vendor_id", "")
        if vid in exclude_vendor_ids:
            continue
        # 跳过耗尽的 key
        if is_key_exhausted(opt.get("key_id", "")):
            continue
        vendor = vendor_map.get(vid)
        if not vendor:
            continue
        api_key = ""
        key_id = ""
        for k in vendor_keys.get(vid, []):
            if k.get("id") == opt.get("key_id"):
                api_key = k.get("api_key", "")
                key_id = k.get("id", "")
                break
        if not api_key:
            continue
        routes.append({
            "vendor_id": vid,
            "vendor_name": vendor["name"],
            "base_url": vendor["base_url"],
            "api_key": api_key,
            "key_id": key_id,
            "model_id": opt.get("model_id", ""),
        })
    return routes

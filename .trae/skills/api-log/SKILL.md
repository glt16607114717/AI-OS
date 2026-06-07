---
name: api-log
description: 后端接口日志查询。当用户要求"查接口日志"、"查日志"、"看日志"、"接口报错排查"、"接口调用记录"、"谁调了这个接口"、"查看操作记录"、"查MongoDB日志"时触发。通过Python脚本查询MongoDB中的接口请求日志（严格只读，禁止写入）。
---

# 后端接口日志查询

## 角色定位

查询 MongoDB 中存储的接口请求日志，用于排查问题、追踪操作记录。**严格只读，任何时候禁止写入数据。**

## 核心规则

### 数据存储

接口日志存储在 MongoDB 中，按月分表：

| 环境 | 数据库 | 集合命名规则 | 示例 |
|------|--------|-------------|------|
| 测试（默认） | robot_log_test | `request_log_YYYYMM` | request_log_202604 |
| 灰度 | robot_log_gray | `request_log_YYYYMM` | request_log_202604 |
| 正式 | robot_log | `request_log_YYYYMM` | request_log_202604 |

**分表规则**：
- 分表起始月：202604
- 2026年4月及之后的数据 → `request_log_YYYYMM`（如 `request_log_202604`、`request_log_202605`）
- 2026年4月之前的数据 → `request_log`（旧表，不分表）
- **查询时必须根据用户指定的时间范围确定集合名**，无时间范围默认查当月

### 查询方式

通过文件传参，AI 将 MongoDB 查询命令写入临时文件后传给脚本：

```
1. Write → D:\wwwroot\ai\ai_cache\temp\mongo_{时间戳}.json
2. RunCommand → python D:\wwwroot\.trae\skills\api-log\scripts\mongo_query.py <环境> D:\wwwroot\ai\ai_cache\temp\mongo_{时间戳}.json
```

- 环境参数：`test`（默认）、`gray`、`prod`
- MongoDB 连接配置：技能目录下的 `mongodb.json`（与脚本同目录）

### 确定集合名

AI 查询前必须先确定正确的集合名：
1. 用户指定了时间范围 → 根据时间计算集合名（`request_log_YYYYMM`）
2. 跨月查询 → 需要多次查询，逐月查
3. 未指定时间 → 默认当月，集合名 `request_log_当前年月`

---

## 查询模式

### 选择筛选字段：path vs route

| 字段 | 格式 | 适用场景 | 示例 |
|------|------|----------|------|
| `path` | `控制器名/方法名` | 用户定位到了具体接口、提到控制器名 | `FixtureOrder/saveFixtureOrderMaterial` |
| `route` | `api/admin/xxx/yyy` | 用户给的是 URL、前端报错、浏览器 Network 里的地址 | `api/admin/fixture-order/save-material` |

**判断规则**：
- 用户说"查某某接口"且给出了控制器名 → 用 `path`
- 用户说"某某 URL 报错"、"前端请求地址是 xxx" → 用 `route`
- 不确定时两个都试，优先 `path`（更精确）

### 模式一：单接口查询

按 `path` 或 `route` 字段筛选，用 `$regex` 模糊匹配。

**场景**：用户说"查一下某某接口的日志"

按 path 查询：
```json
{
    "find": "request_log_202604",
    "filter": {
        "path": {"$regex": "FixtureOrder"},
        "request_type": "POST",
        "gmt_create": {"$regex": "2026-04-28"}
    },
    "sort": {"gmt_create": -1},
    "limit": 20
}
```

按 route 查询：
```json
{
    "find": "request_log_202604",
    "filter": {
        "route": {"$regex": "fixture-order/save"},
        "request_type": "POST",
        "gmt_create": {"$regex": "2026-04-28"}
    },
    "sort": {"gmt_create": -1},
    "limit": 20
}
```

### 模式二：多接口链路查询

按 `path` 正则匹配 + 参数筛选。

**场景**：用户说"查一下异常报告单相关所有接口的调用记录"

```json
{
    "find": "request_log_202604",
    "filter": {
        "path": {"$regex": "AbnormalReport"},
        "request_param": {"$regex": "abnormal_report_id.*63"},
        "request_type": "POST"
    },
    "sort": {"gmt_create": -1},
    "limit": 30
}
```

---

## 日志文档结构

| 字段 | 类型 | 说明 | 示例值 |
|------|------|------|--------|
| path | string | 接口路径，`控制器名/方法名` | `FixtureOrder/saveFixtureOrderMaterial` |
| route | string | URL路由 | `api/admin/fixture-order/save-material` |
| request_type | string | 请求方式 | `POST` |
| request_param | string | 请求参数（JSON字符串） | `{"company_id":"2","material_data":"[{\"id\":1}]","uuid":"log_xxx"}` |
| result_param | string | 响应结果（JSON字符串） | `{"message":"success","code":200,"data":[]}` |
| error_info | string | 错误信息（成功时为空） | `` |
| user_name | string | 操作人姓名（含部门） | `邹兴平(长安MINI)` |
| user_id | int | 操作人ID | `914` |
| gmt_create | string | 请求时间 | `2026-04-28 14:51:18` |
| ip | string | 请求IP | `159.75.132.166` |
| from | string | 来源 | `pc` |
| referer | string | 来源标识 | `rmp` |
| use_time | float | 耗时（毫秒） | `140.62` |
| status | int | 状态：1成功 0失败 | `1` |
| top_menu | string | 前端顶部菜单路径 | `/CRM` |
| front_path_chain | string | 前端路径链 | `CRM/异常报告` |
| front_path | string | 前端页面路径 | `/customerManage/exceptionReport` |
| action | string | 操作描述（通常为空） | `` |

---

## 默认行为

| 场景 | 默认值 |
|------|--------|
| 未指定环境 | test（测试） |
| 未指定时间 | 当月 |
| 未指定请求类型 | POST（GET无追踪价值，除非用户明确要求） |
| 未指定条数 | limit 20 |

## 参考文件

查询逻辑参考：`rmp-api/app/common/mongo/RequestLog.php`（分表逻辑）
              `rmp-api/app/common/repositories/RequestLogRepository.php`（查询方法）

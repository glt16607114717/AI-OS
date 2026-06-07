---
name: apipost
description: Apipost接口管理。当用户要求"同步接口到Apipost"、"导入接口文档"、"接口同步Apipost"、"新增接口文档"、"生成接口文档"、"生成swagger"、"查看Apipost接口"、"搜索接口"、"查看接口文档"或输入 /apipost 时触发。通过 Python 脚本调用 Apipost Open API，支持创建目录、创建接口、更新接口、移动接口、删除接口等完整操作。
---

# Apipost 接口管理

## 角色定位

管理 Apipost 中的接口资产。两大核心能力：
1. **读取** — 查询、搜索、查看已有接口文档
2. **写入** — 创建目录、创建接口、更新接口、移动接口、删除接口

**权限区分**：
- 后端开发者：可读取 + 可写入
- 其他岗位（前端、测试、产品）：仅读取，不能写入

---

## 脚本调用方式

**脚本路径**：`D:\wwwroot\.trae\skills\apipost\scripts\apipost_api.py`
**配置文件**：`D:\wwwroot\.trae\skills\apipost\scripts\apipost_config.json`
**传参方式**：只接受文件传参

```
1. Write → D:\wwwroot\ai\ai_cache\temp\apipost_{时间戳}.json
2. RunCommand → python D:\wwwroot\.trae\skills\apipost\scripts\apipost_api.py D:\wwwroot\ai\ai_cache\temp\apipost_{时间戳}.json
```

**并发安全**：每次调用使用带时间戳的唯一文件名，多任务并行不会冲突。脚本本身无状态，只读不写。

**配置文件格式**（`apipost_config.json`）：

```json
{
    "host": "https://open.apipost.net",
    "token": "你的Api-Token",
    "project_id": "24b539"
}
```

Token 获取：Apipost 网页端 → 团队设置 → Open API

---

## 命令清单

### 读取类

| 命令 | 说明 | 必填参数 |
|------|------|----------|
| `list_teams` | 获取团队列表 | 无 |
| `list_projects` | 获取项目列表 | `team_id`（可选） |
| `list_apis` | 获取项目下所有接口和目录 | `project_id`（可选，默认用配置） |
| `search` | 搜索接口/目录 | `keyword`（可选）、`parent_id`（可选）、`target_type`（可选：api/folder/all） |
| `get_detail` | 获取接口/目录详情 | `target_id` |

### 写入类

| 命令 | 说明 | 必填参数 |
|------|------|----------|
| `create_folder` | 创建目录 | `name`、`parent_id`（可选，默认根目录）、`description`（可选） |
| `create_api` | 创建接口 | `name`、`method`、`url`、`parent_id`（可选） |
| `update_api` | 更新接口（增量） | `target_id`、`update_fields`（要更新的字段） |
| `move_api` | 移动接口到指定目录 | `target_id`、`parent_id` |
| `delete_apis` | 批量删除接口/目录 | `target_ids`（数组） |

---

## 一、读取接口

### 1.1 搜索接口

参数文件：
```json
{"command": "search", "keyword": "部门", "target_type": "api"}
```

**场景示例**：
- 用户说"查一下异常报告相关的接口" → `keyword="异常报告"`
- 用户说"fixture-order 有哪些接口" → `keyword="fixture-order"`
- 用户说"搜索一下导出接口" → `keyword="导出"`

### 1.2 查看接口详情

参数文件：
```json
{"command": "get_detail", "target_id": "接口ID"}
```

### 1.3 浏览目录结构

```json
{"command": "search", "parent_id": "0"}
{"command": "search", "parent_id": "目录ID"}
```

### 1.4 输出规范

查询到接口后，以清晰格式展示给用户：

```
接口名称：xxx
URL：POST /admin.php/api/admin/xxx
状态：已发布/草稿
描述：接口功能说明
```

---

## 二、写入接口

### 2.1 代码分析

**必须基于全套代码分析**，禁止只看控制器：

| 信息源 | 获取什么 | 怎么获取 |
|--------|----------|----------|
| 路由文件 | URL路径、HTTP方法 | 读取路由配置文件 |
| 控制器 | 入参校验规则、调用关系 | 读取 Controller 代码 |
| Service/Repository | 业务逻辑、数据拼装 | 读取业务层代码 |
| Validate | 参数类型、必填规则 | 读取验证器代码 |
| 数据库表结构 | 返回字段、字段注释 | Python 脚本查询 MySQL |
| Model | 字段映射、关联关系 | 读取模型代码 |

### 2.2 获取数据库字段

通过 Python 脚本查询：

```
1. Write → D:\wwwroot\ai\ai_cache\temp\mysql_{时间戳}.json（内容为 `{"sql": "SQL语句"}`）
2. RunCommand → python D:\wwwroot\.trae\skills\mysql\scripts\mysql_query.py <环境> D:\wwwroot\ai\ai_cache\temp\mysql_{时间戳}.json
```

### 2.3 创建目录并写入接口

**核心优势**：现在可以直接指定 `parent_id` 将接口创建到目标目录，不再需要 MCP 缓存目录中转。

**流程**：
1. 如需新建目录：`create_folder` → 获取 `target_id` 作为后续接口的 `parent_id`
2. 逐个创建接口：`create_api`，传入 `parent_id`

**创建接口参数格式**：

```json
{
    "command": "create_api",
    "name": "新增部门",
    "method": "POST",
    "url": "/api/admin/hr-department-manage/add",
    "parent_id": "目标目录ID",
    "description": "新增部门接口",
    "query_params": [["company_id", "所属公司id", "integer", false]],
    "body_params": {"department_name": "部门名称"},
    "response_raw": "{\"code\": 200, \"msg\": \"success\"}",
    "response_fields": [["code", "状态码", "integer"], ["msg", "消息", "string"]]
}
```

**参数说明**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `query_params` | 数组 | 每个元素：`[字段名, 描述, 类型, 是否必填]` |
| `body_params` | 对象 | 请求体示例（JSON） |
| `headers` | 数组 | 每个元素：`[字段名, 描述]` |
| `response_raw` | 字符串 | 响应示例（JSON字符串） |
| `response_fields` | 数组 | 每个元素：`[字段名, 描述, 类型]` |

### 2.4 更新已有接口

```json
{"command": "update_api", "target_id": "接口ID", "update_fields": {"name": "新名称", "url": "/新路径", "description": "新描述"}}
```

`update_fields` 中只需传入要更新的字段，未传入的保持原值。

### 2.5 移动接口到其他目录

```json
{"command": "move_api", "target_id": "接口ID", "parent_id": "目标目录ID"}
```

### 2.6 删除接口

```json
{"command": "delete_apis", "target_ids": ["ID1", "ID2"]}
```

---

## 三、导入质量标准

每个接口必须包含：

**请求参数**：
- 参数名、参数类型、是否必填
- 中文描述（从 Validate 规则获取）
- 参数格式说明

**响应字段**：
- 完整的 JSON 响应结构
- 每个字段的中文描述
- 字段类型标注

**接口描述**：
- 接口功能说明
- 特殊逻辑说明

---

## 四、字段描述优先级

1. 数据库字段注释（最高优先级）
2. 代码中的注释和 PHPDoc
3. 预定义字段映射字典

**常用字段映射**：

| 字段模式 | 描述 |
|----------|------|
| `id` | 主键ID |
| `*_id` | xxx ID |
| `*_no` | xxx 编号 |
| `*_date` | xxx 日期 |
| `is_*` | 是否 xxx |
| `gmt_create` | 创建时间 |
| `gmt_modified` | 更新时间 |
| `page` | 页码 |
| `page_rows` | 每页数量 |
| `total` | 总条数 |
| `*_text` | xxx 名称（翻译字段） |

## 类型映射表

| 代码/DB 类型 | Swagger 类型 |
|-------------|--------------|
| int, integer, tinyint | integer |
| varchar, char, text | string |
| decimal, float, double | number |
| datetime, timestamp | string (format: date-time) |
| date | string (format: date) |
| bool, boolean | boolean |
| array | array |

---

## 五、核心约束

1. **每次只创建1个接口**，禁止批量创建
2. **路由必须从路由文件读取**，禁止猜测
3. **返回值必须联合数据库字段**，不是所有返回字段都显式写在代码中
4. **优先直接创建到目标目录** — 使用 `create_folder` 先建目录，再用返回的 `target_id` 作为 `parent_id` 创建接口
5. **已有接口优先用 update_api 更新** — 禁止删了重新创建，除非用户明确要求
6. **禁止凭空捏造响应结构** — 必须从代码+数据库联合推导
7. **禁止遗漏字段描述** — 每个字段必须有中文说明

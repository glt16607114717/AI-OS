# AI-OS 内置技能系统架构设计

## 一、定位与边界

### 1.0 与 TRAE 技能体系的关系

AI-OS 内置技能系统与 TRAE 技能系统**核心模式完全相同**，只是维度更高：

```
TRAE 技能                     AI-OS 内置技能
──────────                    ──────────────
用户自定义                    开发者定义
本地 IDE 作用域               全局服务端作用域
markdown + 脚本定义            数据库 + Go 代码定义
TRAE AI 识别                  TRAE AI 识别（仅代理场景）
本地终端执行                  服务端执行
无权限控制                    按用户 + 连接粒度权限控制
配置硬编码在脚本内             配置由数据库动态配置 + 加密存储
```

**核心模式一致**：开发者定义能力 → AI 自动识别调用 → 代理层拦截执行 → 结果回填总结。

区别只在于：TRAE 的技能在本地运行，AI-OS 的技能在服务端运行，多了权限控制和连接管理。

**重要：此技能系统仅限代理场景（TRAE IDE），与工作台无关。** 工作台面向普通业务人员，如何为业务人员提供技能服务尚未确定。

### 1.1 什么是技能

技能是 AI-OS 服务端内置的**可执行能力单元**，由开发者预定义。在 TRAE IDE（代理场景）中，用户通过自然语言描述需求，AI 自动识别意图并调用对应技能，AI-OS 在服务端执行后将结果回传给 AI，AI 再总结成自然语言回复给用户。

### 1.2 触发机制（仅限代理场景）

```
TRAE IDE 中用户提问：「查一下数据库里有多少用户」
         │
         ▼
   AI 识别意图：需要查数据库
         │
         ▼
   AI 返回 tool_calls：调用 mysql_query 技能
         │
         ▼
   AI-OS 拦截 → 服务端执行 SQL → 结果回填
         │
         ▼
   AI 总结：「sys_user 表中共有 5 个用户」
```

用户**不需要**知道技能的存在，也不需要手动选择技能。AI 根据用户的问题自动判断是否需要调用技能。此流程仅在 TRAE IDE 代理场景下生效，工作台不涉及。

### 1.3 核心边界

| 边界 | 决策 |
|------|------|
| 谁定义技能 | **开发者**（写代码 + 注册到数据库） |
| 谁配置连接 | **管理员**（通过 Web「技能」菜单配置） |
| 谁触发技能 | **AI**（Function Calling 自动判断） |
| 谁执行技能 | **AI-OS 服务端**（拦截 tool_calls 在服务端执行） |
| 用户能否自定义技能 | **不能**（技能是代码，不是配置） |
| 「技能」菜单用途 | **管理**（连接配置、权限分配、执行历史），不是手动执行入口 |

### 1.4 与现有 sys_skill 的关系

| | sys_skill（现有） | sys_builtin_skill（新） |
|------|---------------------|------------------------|
| 触发方式 | LLM Function Calling | LLM Function Calling |
| 触发者 | AI 自动识别 | AI 自动识别 |
| 执行方 | 服务端（只查 AI-OS 库） | 服务端（可连任意公网 MySQL） |
| 连接管理 | 无（硬编码查 AI-OS 库） | 可配置无数个公网 MySQL 连接 |
| 权限 | 无（谁都能触发） | 按用户 + 连接粒度授权 |
| SQL 来源 | 技能定义中硬编码的固定 SQL | AI 根据用户意图动态生成 SQL |
| 技能参数 | 无 | AI 传入参数（如 SQL、连接名） |

**建议**：将现有 `sys_skill` 重命名为 `sys_ai_skill`，避免混淆。新系统完全替代现有 sys_skill 的能力，并在此基础上扩展连接管理和权限控制。

---

## 二、技能体系架构

### 2.1 四层模型

```
┌─────────────────────────────────────────────────────┐
│                    UI 层（管理界面）                   │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐     │
│  │ 连接管理    │  │ 权限管理    │  │ 执行历史    │     │
│  │ (管理员)    │  │ (管理员)    │  │ (所有人)    │     │
│  └────────────┘  └────────────┘  └────────────┘     │
├─────────────────────────────────────────────────────┤
│                    定义层（元数据）                    │
│  ┌──────────────┐  ┌──────────────┐                  │
│  │ 技能注册表    │  │ 连接配置表    │                  │
│  │ (code/name/  │  │ (skill_id/   │                  │
│  │  desc/schema)│  │  config/name)│                  │
│  └──────────────┘  └──────────────┘                  │
├─────────────────────────────────────────────────────┤
│                    执行层（引擎）                      │
│  ┌──────────────┐  ┌──────────────┐                  │
│  │ 技能执行引擎  │  │ SQL 安全引擎  │                  │
│  │ (路由→执行)  │  │ (只读/超时/   │                  │
│  │              │  │  LIMIT)      │                  │
│  └──────────────┘  └──────────────┘                  │
├─────────────────────────────────────────────────────┤
│                    权限层（控制）                      │
│  ┌──────────────────────────────────────────────┐   │
│  │ sys_skill_permission                          │   │
│  │ (user_id → skill_id → connection_id)          │   │
│  │ AI 调用时：无权限 → 不注入该技能的 tool         │   │
│  └──────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

### 2.2 技能生命周期

```
1. 开发者定义技能
   ├── 写执行代码（service/builtin_skill.go）
   ├── 写工具描述（description，AI 据此识别意图）
   └── 注册到 sys_builtin_skill（INSERT 一行）
         │
         ▼
2. 管理员配置连接
   └── INSERT sys_skill_connection（主机/端口/密码等）
         │
         ▼
3. 管理员分配权限
   └── INSERT sys_skill_permission（user_id + connection_id）
         │
         ▼
4. 用户在 TRAE IDE 中与 AI 对话
   └── 「查一下生产库的订单表」
         │
         ▼
5. AI-OS 注入技能 tools（仅注入该用户有权限的）
         │
         ▼
6. AI 识别意图 → 返回 tool_calls
         │
         ▼
7. AI-OS 拦截 → 权限校验 → 服务端执行 → 结果回填
         │
         ▼
8. AI 总结结果 → 返回自然语言给用户
```

---

## 三、数据库设计

### 3.1 sys_builtin_skill（技能注册表）

```sql
CREATE TABLE sys_builtin_skill (
    id            INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code          VARCHAR(64)  NOT NULL UNIQUE COMMENT '技能编码，如 mysql_query',
    name          VARCHAR(64)  NOT NULL COMMENT '显示名称，如 MySQL 查询',
    description   TEXT         NOT NULL COMMENT '技能描述，AI 据此判断是否调用该技能',
    icon          VARCHAR(64)  DEFAULT 'database',
    config_schema JSON         NOT NULL COMMENT '连接配置 JSON Schema，前端据此动态渲染表单',
    tool_schema   JSON         NOT NULL COMMENT 'Function Calling 参数定义，告诉 AI 怎么调用',
    enabled       TINYINT      NOT NULL DEFAULT 1,
    sort_order    INT          NOT NULL DEFAULT 0,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**tool_schema 示例（MySQL 查询技能）**：

这是注入给 AI 的 Function Calling 参数定义，告诉 AI 这个技能接受什么参数：

```json
{
  "type": "object",
  "properties": {
    "sql": {
      "type": "string",
      "description": "要执行的 SQL 查询语句，仅支持 SELECT/SHOW/DESCRIBE/EXPLAIN"
    }
  },
  "required": ["sql"]
}
```

AI 看到 tool_schema 后就知道：调用这个技能需要传一个 `sql` 参数。

**config_schema 示例（MySQL 连接配置）**：

这是给管理员配置连接用的，前端据此渲染表单：

```json
{
  "type": "object",
  "properties": {
    "host":     {"type": "string", "title": "主机地址", "default": "127.0.0.1"},
    "port":     {"type": "integer", "title": "端口", "default": 3306},
    "user":     {"type": "string", "title": "用户名", "default": "root"},
    "password": {"type": "string", "title": "密码", "format": "password"},
    "database": {"type": "string", "title": "数据库名"}
  },
  "required": ["host", "port", "user", "password", "database"]
}
```

### 3.2 sys_skill_connection（连接配置表）

```sql
CREATE TABLE sys_skill_connection (
    id            INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    skill_id      INT UNSIGNED NOT NULL COMMENT '关联 sys_builtin_skill.id',
    name          VARCHAR(64)  NOT NULL COMMENT '连接名称，如 生产库 / AI-OS库',
    config        JSON         NOT NULL COMMENT '连接配置（密码 AES 加密存储）',
    enabled       TINYINT      NOT NULL DEFAULT 1,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (skill_id) REFERENCES sys_builtin_skill(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**一个技能可以配多个连接**。AI 调用时可以选择使用哪个连接，也可以由 AI-OS 根据用户权限自动选择（如果只有一个可用连接）。

### 3.3 sys_skill_permission（权限表）

```sql
CREATE TABLE sys_skill_permission (
    id             INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    skill_id       INT UNSIGNED COMMENT '技能ID（NULL=全局，暂不用）',
    connection_id  INT UNSIGNED COMMENT '连接ID（NULL=该技能所有连接）',
    user_id        INT UNSIGNED NOT NULL COMMENT '授权用户',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (skill_id)      REFERENCES sys_builtin_skill(id) ON DELETE CASCADE,
    FOREIGN KEY (connection_id) REFERENCES sys_skill_connection(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id)       REFERENCES sys_user(id) ON DELETE CASCADE,
    UNIQUE KEY uk_user_skill_conn (user_id, skill_id, connection_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**权限判断规则（两个层面）**：

```
层面1：技能注入时（GetSkillToolDefinitions）
  - 管理员 → 注入所有 enabled 技能
  - 普通用户 → 查 sys_skill_permission WHERE user_id=?
    有记录的技能才注入 tools 给 AI
    无权限的技能 → AI 根本不知道它存在 → 不会调用

层面2：技能执行时（ExecuteSkill）
  - 再次校验：AI 返回 tool_calls 时，AI-OS 执行前再确认权限
  - 防止伪造
```

---

## 四、核心请求链路（TRAE IDE 代理场景）

这是整个技能系统的**唯一触发方式**，仅限代理场景。

```
┌─────────────────────────────────────────────────────────────────┐
│ TRAE IDE（代理场景）                                              │
│                                                                 │
│ 用户输入：「查一下生产库里订单表最近 10 条记录」                    │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼ POST /api/workspace/chat
┌─────────────────────────────────────────────────────────────────┐
│ AI-OS 服务端                                                     │
│                                                                 │
│ Step 1: 认证 → 获取 user_id                                      │
│                                                                 │
│ Step 2: 权限过滤 → 确定该用户可用的技能 + 连接                     │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │ 查 sys_skill_permission WHERE user_id = 3                │   │
│   │ 结果：                                                    │   │
│   │   skill=mysql_query, connection=1(AI-OS库)  ✓           │   │
│   │   skill=mysql_query, connection=2(生产库)   ✓           │   │
│   │                                                          │   │
│   │ → 该用户可用 mysql_query 技能，可访问「生产库」和「AI-OS库」│   │
│   └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│ Step 3: 构造 tools 注入请求                                      │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │ tools = [{                                               │   │
│   │   "type": "function",                                    │   │
│   │   "function": {                                          │   │
│   │     "name": "skill_mysql_query",                         │   │
│   │     "description": "查询 MySQL 数据库...",               │   │
│   │     "parameters": {  ← 来自 tool_schema                  │   │
│   │       "type": "object",                                  │   │
│   │       "properties": {                                    │   │
│   │         "sql": {"type": "string", "description": "..."}  │   │
│   │       }                                                  │   │
│   │     }                                                    │   │
│   │   }                                                      │   │
│   │ }]                                                       │   │
│   │                                                          │   │
│   │ ★ AI 只看到用户有权限的技能。无权限的技能不注入。          │   │
│   └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│ Step 4: 转发给 LLM（含 tools）                                   │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│ LLM（DeepSeek / GLM 等）                                         │
│                                                                 │
│ LLM 分析：用户想查「生产库的订单表最近10条」                       │
│ → 需要用数据库查询技能                                            │
│ → 返回 tool_calls：                                              │
│                                                                 │
│ {                                                                │
│   "tool_calls": [{                                              │
│     "id": "call_abc123",                                        │
│     "function": {                                               │
│       "name": "skill_mysql_query",                              │
│       "arguments": {                                            │
│         "sql": "SELECT * FROM orders ORDER BY id DESC LIMIT 10" │
│       }                                                         │
│     }                                                           │
│   }]                                                            │
│ }                                                                │
│                                                                 │
│ ★ AI 自己生成了 SQL 语句                                         │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼ LLM 返回 tool_calls 给 AI-OS
┌─────────────────────────────────────────────────────────────────┐
│ AI-OS 服务端（拦截 tool_calls）                                   │
│                                                                 │
│ Step 5: 解析 tool_calls                                          │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │ fnName = "skill_mysql_query"                             │   │
│   │ → 前缀 skill_ → 这是技能调用                              │   │
│   │ → skillID = "mysql_query"                                │   │
│   │ → args = {"sql": "SELECT * FROM orders ... LIMIT 10"}    │   │
│   └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│ Step 6: 权限二次校验（防伪造）                                    │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │ 确认 user_id=3 确实有 mysql_query 技能权限                │   │
│   │ → 查 sys_skill_permission WHERE user_id=3 AND skill_id=? │   │
│   │ → 有权限 → 继续                                           │   │
│   │ → 无权限 → 拒绝执行，返回错误给 AI                        │   │
│   └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│ Step 7: 确定使用哪个连接                                         │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │ 该用户有两个可用连接：AI-OS库 和 生产库                    │   │
│   │                                                          │   │
│   │ 策略：                                                    │   │
│   │ - 如果用户在对话中指定了库名（「生产库」）→ 用对应连接      │   │
│   │ - 如果只有一个连接 → 直接用                                │   │
│   │ - 如果有多个且未指定 → AI 传入 connection_name 参数选择    │   │
│   │ - 或者：默认使用第一个，执行失败后尝试第二个               │   │
│   └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│ Step 8: 读取连接配置 + AES 解密密码                               │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │ 查 sys_skill_connection WHERE id=2 (生产库)              │   │
│   │ → host=10.0.0.5, port=3306, user=root,                   │   │
│   │   password=DECRYPT("AES_..."), database=production        │   │
│   └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│ Step 9: SQL 安全检查                                             │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │ AI 生成的 SQL: "SELECT * FROM orders ORDER BY ... LIMIT 10"│  │
│   │ ✓ 是 SELECT 语句                                          │   │
│   │ ✓ 已有 LIMIT 10                                           │   │
│   │ → 通过安全检查                                             │   │
│   └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│ Step 10: 连接目标 MySQL 执行 SQL                                 │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │ AI-OS 服务端 ──TCP 3306──► 生产库 MySQL (10.0.0.5)        │   │
│   │                                                          │   │
│   │ db.QueryContext(ctx, sql)  // 30s 超时                   │   │
│   │ → 返回 10 行数据                                          │   │
│   └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│ Step 11: 结果回填给 LLM（二次请求）                                │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │ messages = [                                             │   │
│   │   ...原对话...,                                          │   │
│   │   {role: "assistant", tool_calls: [...]},                │   │
│   │   {role: "tool", tool_call_id: "call_abc123",            │   │
│   │    content: "{\"columns\":[\"id\",\"order_no\",...],      │   │
│   │              \"rows\":[{...},{...}],\"row_count\":10}"}   │   │
│   │ ]                                                        │   │
│   │                                                          │   │
│   │ → 再次请求 LLM，让它总结查询结果                           │   │
│   └─────────────────────────────────────────────────────────┘   │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼ 二次请求 LLM
┌─────────────────────────────────────────────────────────────────┐
│ LLM 总结结果                                                     │
│                                                                 │
│ 「生产库 orders 表最近 10 条订单如下：                            │
│   | 订单号 | 客户 | 金额 | 状态 |                                │
│   | ORD-001 | 张三 | ¥299 | 已发货 |                             │
│   | ORD-002 | 李四 | ¥159 | 待付款 |                             │
│   ...                                                            │
│  共 10 条记录。」                                                 │
└───────────────────────────┬─────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│ TRAE IDE（代理场景）                                              │
│                                                                 │
│ 用户看到 AI 的回复（自然语言 + 表格）                              │
│ 全程不知道后台执行了 SQL                                          │
└─────────────────────────────────────────────────────────────────┘
```

---

## 五、连接选择

用户在 TRAE IDE 对话中通过自然语言指定使用哪个连接，AI 自动识别。tool_schema 中包含 `connection_name` 参数，技能描述中列出该用户可用的连接名称：

```
description: "查询 MySQL 数据库。可用连接：生产库、AI-OS库。
              请根据用户意图选择对应连接。"

parameters: {
  "sql": "SQL 语句",
  "connection_name": "连接名称，可选值：生产库、AI-OS库"
}
```

用户说「查一下**生产库**的订单表」→ AI 识别出「生产库」→ 传 `connection_name: "生产库"`。

如果只有一个可用连接，AI 直接使用，无需指定。

---

## 六、扩展性设计

### 6.1 新增一个技能的步骤

以「Redis 查询」为例：

**Step 1：数据库注册**

```sql
INSERT INTO sys_builtin_skill (code, name, description, config_schema, tool_schema)
VALUES (
  'redis_query',
  'Redis 查询',
  '查询 Redis 缓存数据，支持 GET/HGET/KEYS/LRANGE 等只读命令',
  '{"type":"object","properties":{"host":{"type":"string","title":"主机"},"port":{"type":"integer","title":"端口","default":6379},"password":{"type":"string","title":"密码","format":"password"},"db":{"type":"integer","title":"数据库","default":0}},"required":["host","port"]}',
  '{"type":"object","properties":{"command":{"type":"string","description":"Redis 只读命令，如 GET key"}}},"required":["command"]}'
);
```

**Step 2：后端注册执行逻辑**

```go
case "redis_query":
    return executeRedisQuery(connectionConfig, args)
```

**Step 3：前端无需新增页面**

连接管理表单由 config_schema 自动渲染，权限管理通用。

**不需要改的**：
- 连接管理界面（通用，由 config_schema 驱动）
- 权限管理界面（通用）
- 技能注入逻辑（从数据库读取，自动注入）
- 前端路由和菜单

### 6.2 未来可扩展的技能

| 技能 | 说明 | 复杂度 |
|------|------|--------|
| MySQL 查询 | 第一个技能 | 低 |
| Redis 查询 | 缓存数据查看 | 低 |
| Elasticsearch 查询 | 日志检索 | 中 |
| HTTP 请求 | 调用外部 API | 低 |
| 服务端日志 | 查看服务端日志文件 | 中 |
| Git 操作 | 查看仓库状态 | 高 |

---

## 七、安全设计

### 7.1 密码安全

- 连接密码使用 AES-256-GCM 加密存储
- 加密密钥从环境变量读取
- 前端永远看不到明文密码（返回时脱敏）
- 只在服务端执行前解密

### 7.2 SQL 安全

- 只允许 SELECT / SHOW / DESCRIBE / EXPLAIN
- 自动追加 LIMIT 上限（默认 100）
- 30 秒超时保护

### 7.3 权限安全

- **注入层**：无权限的技能不注入 tools，AI 根本不知道它存在
- **执行层**：tool_calls 执行前二次校验权限，防止伪造
- 管理员拥有所有权限

### 7.4 审计日志

- 每次技能执行记录到 `sys_llm_log`
- 记录：谁、什么时候、什么技能、什么连接、执行了什么 SQL、耗时多少

---

## 八、Web 管理界面设计

「技能」菜单是**管理界面**，不是执行入口。每个技能有独立的管理页面，因为技能之间差异太大，无法统一管理。

### 8.1 路由

```
/skills/mysql_query       → MySQLQuerySettings.vue   MySQL 查询设置（连接+权限）
/skills/redis_query       → RedisQuerySettings.vue   Redis 查询设置（连接+权限）
/skills/{code}            → 各技能独立页面
```

每个技能的设置页面包含三部分：连接管理、权限管理、执行历史，都在同一个页面内。

### 8.2 菜单结构

```
AI-OS
├── 工作台             ← 面向业务人员（不涉及技能）
├── 技能               ← 仅限代理场景（TRAE IDE）
│   ├── MySQL 查询     ← 各技能独立页面（连接+权限+历史）
│   ├── Redis 查询     ← 未来扩展
│   └── ...
├── 知识库
├── 大模型
├── 开发助手
└── 系统设置
```

每个技能菜单项根据 `sys_skill_permission` 控制可见性（管理员看到全部，普通用户只看有权限的）。

### 8.3 技能设置页面（以 MySQL 查询为例）

```
┌──────────────────────────────────────────────────────────────┐
│ MySQL 查询                                                   │
│                                                              │
│ ── 连接管理 ──────────────────────────────────────────────── │
│                                                              │
│ ┌────────────────────────────────────────────────────────┐  │
│ │ 连接名称        主机              数据库     状态  操作  │  │
│ ├────────────────────────────────────────────────────────┤  │
│ │ AI-OS 库        8.163.127.182    ai_os      启用  ✎ 🗑 │  │
│ │ 生产库          10.0.0.5         production 启用  ✎ 🗑 │  │
│ │ 测试库          10.0.0.6         test       停用  ✎ 🗑 │  │
│ └────────────────────────────────────────────────────────┘  │
│                                                              │
│ [+ 新建连接]                                                 │
│                                                              │
│ 新建连接时，表单根据 config_schema 动态渲染：                  │
│ ┌────────────────────────────────────────────────────────┐  │
│ │ 主机地址: [____________]                                │  │
│ │ 端口:     [3306______]                                  │  │
│ │ 用户名:   [____________]                                │  │
│ │ 密码:     [************]                                │  │
│ │ 数据库名: [____________]                                │  │
│ │                                  [取消]  [保存]         │  │
│ └────────────────────────────────────────────────────────┘  │
│                                                              │
│ ── 权限管理 ──────────────────────────────────────────────── │
│                                                              │
│ ┌────────────────────────────────────────────────────────┐  │
│ │ 用户         可用连接              操作                 │  │
│ ├────────────────────────────────────────────────────────┤  │
│ │ admin        全部（管理员）       —                    │  │
│ │ dev01        AI-OS库、生产库     [编辑]                │  │
│ │ dev02        AI-OS库            [编辑]                 │  │
│ │ user01       (无权限)            [+ 授权]              │  │
│ └────────────────────────────────────────────────────────┘  │
│                                                              │
│ ── 执行历史 ──────────────────────────────────────────────── │
│                                                              │
│ ┌────────────────────────────────────────────────────────┐  │
│ │ 时间          用户    连接      SQL              耗时   │  │
│ ├────────────────────────────────────────────────────────┤  │
│ │ 14:30:12     dev01   生产库    SELECT * FROM... 12ms   │  │
│ │ 14:25:08     dev01   AI-OS库   SHOW TABLES      5ms    │  │
│ └────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

---

## 九、API 端点设计

每个技能有独立的 API 前缀，不提供统一的技能列表接口。

### 按技能维度

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| GET | `/api/skills/{code}/connections` | 登录 | 连接列表（普通用户只看有权限的） |
| POST | `/api/skills/{code}/connection` | 管理员 | 新建连接 |
| PUT | `/api/skills/{code}/connection/{id}` | 管理员 | 编辑连接 |
| DELETE | `/api/skills/{code}/connection/{id}` | 管理员 | 删除连接 |
| GET | `/api/skills/{code}/permissions` | 管理员 | 查看权限分配 |
| POST | `/api/skills/{code}/permission` | 管理员 | 设置用户权限 |
| GET | `/api/skills/{code}/history` | 登录 | 该技能执行历史 |

### 内部接口（AI-OS 自身调用，不对外暴露）

| 方法 | 路径 | 说明 |
|------|------|------|
| 内部函数 | `GetSkillToolDefinitions(userID)` | 根据用户权限构造 tools 注入给 AI |
| 内部函数 | `ExecuteBuiltinSkill(code, userID, args)` | 执行技能（含权限校验） |

---

## 十、实施计划

| 阶段 | 内容 |
|------|------|
| Phase 1 | 建 3 张表（手动 SQL） |
| Phase 2 | 后端：builtin_skill.go + handler + tools 注入逻辑 + 路由 |
| Phase 3 | 前端：技能菜单 + 连接管理页面 |
| Phase 4 | 前端：权限管理页面 |
| Phase 5 | 前端：执行历史页面 |
| Phase 6 | 部署 + 测试 AI 对话触发 MySQL 查询 |
| Phase 7 | 第二个技能验证扩展性 |

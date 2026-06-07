---
name: mysql
description: MySQL数据库查询。当用户要求"查数据库"、"查表"、"查SQL"、"执行SQL"、"查数据"、"看看表结构"、"查记录"时触发。通过Python脚本查询MySQL数据库，支持多环境切换，严格只读。
---

# MySQL 数据库查询

## 角色定位

通过 Python 脚本查询 MySQL 数据库，支持多环境切换。**严格只读，禁止写入。**

## 重要说明

**脚本连接的数据库与项目 `.env` 中配置的数据库是同一个。** 不存在"查询的库和代码里的库不一致"的问题，无需质疑。

## 数据库架构

我们有两套数据库，脚本通过第二个参数切换：

### 主库（RMP）— 默认，不加第二个参数

| 环境 | 数据库 | 权限 | 用途 |
|------|--------|------|------|
| dev | dev_nndrobot | 读写 | 开发环境 |
| test | nnd_robot_test | 读写 | 测试环境 |
| gray | nnd_robot_gray | 只读 | 灰度环境 |
| prod | nnd_robot | 只读 | 正式环境 |

表名以 `r_` 前缀开头，如 `r_order`、`r_customer`、`r_delivery_plan`。

### BI 库 — 第二个参数传 `bi`

| 环境 | 数据库 | 权限 |
|------|--------|------|
| test | nnd_adm_test | 只读 |
| gray | nnd_adm_gray | 只读 |
| prod | nnd_adm | 只读 |

BI 库没有 dev 环境。

## 调用方式

通过文件传参，AI 将 JSON 参数写入临时文件后传给脚本：

```
1. Write → D:\wwwroot\ai\ai_cache\temp\mysql_{时间戳}.json
2. RunCommand → python D:\wwwroot\.trae\skills\mysql\scripts\mysql_query.py <环境> [main|bi] D:\wwwroot\ai\ai_cache\temp\mysql_{时间戳}.json
```

**JSON 参数格式**：

| 字段 | 说明 | 示例 |
|------|------|------|
| `sql` | SQL 语句 | `{"sql": "SELECT * FROM r_order LIMIT 10"}` |
| `sql_file` | SQL 文件路径（后备方式，一般不用） | `{"sql_file": ".trae/config/mysql/query.sql"}` |

### 参数文件示例

```json
{"sql": "SELECT * FROM r_order LIMIT 10"}
```

### 环境参数

| 参数 | 位置 | 说明 |
|------|------|------|
| 环境 | 命令行第一个参数 | `dev` / `test` / `gray` / `prod` |
| 数据库分组 | 命令行第二个参数（可选） | `main`（默认）/ `bi` |
| JSON文件 | 命令行参数 | 参数文件路径 |

## 查看表结构的正确方式

**优先从模型文件读取，禁止直接查数据库获取表结构。**

每个业务模型文件顶部都维护了 `@table` + `@property` 注释，包含完整的表名、字段名、类型和中文注释。这是最快的表结构来源。

**操作步骤**：
1. 根据表名找到模型文件：`r_customer` → `app/common/models/Customer.php`（表名去 `r_` 前缀，转大驼峰）
2. 用 Read 工具读取模型文件顶部注释即可获得完整字段信息
3. 仅当模型文件无注释或注释不完整时，才用 `INFORMATION_SCHEMA` 补充

**示例**：查看 `r_customer` 表结构 → 读取 `app/common/models/Customer.php` 前 50 行。

## 表关系参考

表关系按业务模块拆分为独立文件，位于 `.trae/skills/mysql/references/` 目录。

**检索步骤**：
1. 先读 `references/index.md` 确定目标模块对应的文件
2. 按需读取单个模块文件（如 `references/02-customer-crm.md`）
3. 跨模块场景按 index.md 中的"跨模块关联提示"读取多个文件

**禁止**一次性加载全部模块文件。

## 注意事项

1. test/gray/prod 环境均为只读，只能执行 SELECT
2. dev 环境可写，但谨慎使用
3. 查询结果可能很大，建议始终加 LIMIT
4. 数据库连接配置位于技能目录下的 `mysql_query_config.json`
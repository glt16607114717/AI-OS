---
alwaysApply: true
description: rmp-api 工作区配置（每次对话加载）。全量项目信息见 projects-reference.md。
---

# rmp-api 工作区配置

## 工作区一览

| 工作区 | 目录 | 本地域名 | 用途 |
|--------|------|----------|------|
| 主工作区 | `D:\wwwroot\rmp-api\rmp-api` | `rmp-api.me` | bug修复、紧急修复 |
| 工作区A | `D:\wwwroot\rmp-api\rmp-api-a` | `rmp-api-a.me` | 需求开发 |
| 工作区B | `D:\wwwroot\rmp-api\rmp-api-b` | `rmp-api-b.me` | 需求开发 |
| 工作区C | `D:\wwwroot\rmp-api\rmp-api-c` | `rmp-api-c.me` | 需求开发 |

基础URL格式：`http://rmp-api-{name}.me/admin.php`

## 路径映射

工作区A/B/C 与主工作区结构完全一致，路径等价替换：
- `D:\wwwroot\rmp-api\rmp-api\app\` ↔ `D:\wwwroot\rmp-api\rmp-api-{a/b/c}\app\`
- `D:\wwwroot\rmp-api\rmp-api\route\` ↔ `D:\wwwroot\rmp-api\rmp-api-{a/b/c}\route\`
- `D:\wwwroot\rmp-api\rmp-api\config\` ↔ `D:\wwwroot\rmp-api\rmp-api-{a/b/c}\config\`

## 工作区规则

- 选择空闲工作区（`git branch --show-current` 显示 develop 的即为空闲）
- 主工作区仅用于 bug 修复，需求开发用 A/B/C
- **严禁自动清理或释放工作区**，必须用户口头确认
- 各工作区有独立 `.env`，无明确命令不得跨区修改/查看/搜索

## 工作区选择判定

| 场景 | 工作区 | 部署命令 |
|------|--------|---------|
| 独立 Bug（不属于迭代） | 主工作区 `rmp-api` | `deploy.py rmp-api` |
| 需求 Bug（属当前迭代） | 开发工作区 `rmp-api-{a/b/c}` | `deploy.py {a/b/c}` |
| 需求开发 | 开发工作区 `rmp-api-{a/b/c}` | `deploy.py {a/b/c}` |

拿不准时，**问用户**在哪个工作区操作。

## 其他项目速查

| 项目 | 路径 | 修改权限 |
|------|------|---------|
| nnd-robot（前端） | `D:\wwwroot\frontend\nnd-robot` | ❌ 禁止修改 |
| charts（图表前端） | `D:\wwwroot\frontend\charts` | ❌ 禁止修改 |
| chartsapi（图表后端） | `D:\wwwroot\chartsapi` | ✅ 允许 |
| ai_cache（缓存记忆） | `D:\wwwroot\ai\ai_cache` | ✅ 允许 |

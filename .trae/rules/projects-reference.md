---
alwaysApply: false
description: 全量项目架构总览。涉及前端项目、图表系统、AI项目等非核心模块时按需触发。
---

# 全量项目架构参考

> 日常开发不需要读此文件。仅在涉及前端项目、图表系统、AI项目等非核心模块时查阅。

## 项目分布

```
D:\wwwroot\
├── rmp-api\           ← 核心后端（4个工作区）
├── frontend\
│   ├── nnd-robot\     ← 核心前端（Vue3，禁止修改，仅查询分析）
│   └── charts\        ← 图表前端（Vue2，禁止修改）
├── chartsapi\         ← 图表后端API（PHP，允许修改）
├── other\
│   ├── socket\        ← 机器人数据传输（PHP，很少修改）
│   └── mcp\           ← 自研MCP框架（PHP，允许修改）
├── ai\
│   ├── nnd-ai-hub\    ← NND AI Hub 桌面应用（Electron+Vue3+Python，允许修改）
│   ├── ai_program\    ← AI实践分享（Python，禁止修改）
│   └── ai_cache\      ← 缓存与记忆（允许修改）
│       ├── task-log\  ← 任务流水
│       ├── swagger\   ← swagger缓存
│       ├── temp\      ← 临时文件
│       └── memory\    ← AI记忆（daily/weekly/archive）
├── rmp-prd\           ← 产品需求文档（禁止修改）
└── .trae\             ← AI配置与技能
```

## 项目详细说明

### 前端项目（禁止修改，仅查询分析）

| 项目 | 路径 | 技术栈 | 说明 |
|------|------|--------|------|
| nnd-robot | `D:\wwwroot\frontend\nnd-robot` | Vue3, Vite, UnoCSS, VxeTable | 对接rmp-api的核心前端 |
| charts | `D:\wwwroot\frontend\charts` | Vue2, ECharts, Vite | 对接chartsapi的图表前端 |

### 图表系统

| 项目 | 路径 | 技术栈 | 说明 |
|------|------|--------|------|
| chartsapi | `D:\wwwroot\chartsapi` | PHP 8.0+, ThinkPHP 6 | 图表数据查询和统计接口 |

### 其他项目

| 项目 | 路径 | 技术栈 | 说明 |
|------|------|--------|------|
| socket | `D:\wwwroot\other\socket` | PHP, WebSocket, MySQL | 机器人数据上传、结算逻辑，很少修改 |
| mcp | `D:\wwwroot\other\mcp` | PHP, MCP | 自研MCP框架 |

### AI项目

| 项目 | 路径 | 技术栈 | 说明 |
|------|------|--------|------|
| nnd-ai-hub | `D:\wwwroot\ai\nnd-ai-hub` | Electron, Vue3, Python | NND AI Hub 桌面应用 |
| ai_program | `D:\wwwroot\ai\ai_program` | Python | AI实践分享，禁止修改 |
| ai_cache | `D:\wwwroot\ai\ai_cache` | — | 缓存与记忆，允许修改 |

### 辅助项目

| 项目 | 路径 | 说明 |
|------|------|------|
| rmp-prd | `D:\wwwroot\rmp-prd` | 产品需求文档，禁止修改 |

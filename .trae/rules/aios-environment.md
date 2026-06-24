---
alwaysApply: true
description: AI-OS项目本地环境与目录结构说明
---

# AI-OS 项目本地环境

## 项目基础信息

| 字段 | 值 |
|------|-----|
| 主源码目录 | `d:\wwwroot\AI\AI-OS\` |
| 构建缓存目录 | `d:\wwwroot\ai-os\`（仅 .cache 和 ai_cache，无业务源码） |
| 运行日志目录 | `C:\ProgramData\AI-OS\logs\` |
| 全局技能目录 | `d:\wwwroot\.trae\skills\` |

## 本地运行环境

### 操作系统
- **系统**：Windows 10 Pro for Workstations
- **版本号**：2009
- **内核**：10.0.26100.1

### 语言运行时

| 语言 | 版本 | 说明 |
|------|------|------|
| PHP | 8.0.30 (CLI) | NTS Visual C++ 2019 x64，Zend v4.0.30，OPcache 已启用 |
| Go | 1.26.4 | windows/amd64（AI-OS 项目要求 ≥ 1.25.0） |
| Python | 3.14.3 | 系统默认 |
| Node.js | 24.15.0 | 由 nvm 1.2.2 管理 |

### 包管理工具

| 工具 | 版本 | 备注 |
|------|------|------|
| pnpm | 11.4.0 | 前端项目包管理 |
| nvm | 1.2.2 | Node 版本管理 |
| Composer | 受限 | 受 open_basedir 限制，PHP 命令需在 Docker 容器内执行 |

### 容器环境

| 组件 | 版本/状态 | 说明 |
|------|----------|------|
| Docker | 29.5.2 (build 79eb04c) | Docker Desktop，非开机自启 |
| 容器 rmp-php | PHP-FPM | rmp-api 项目运行环境 |
| 容器 rmp-nginx | Nginx 1.22 | Web 服务器 |
| 容器 rmp-redis | Redis | 缓存服务 |

- **Docker 环境目录**：`D:\wwwroot\.dev-env-docker`
- **文件挂载**：`D:/wwwroot` → `/var/www/html`
- **启动命令**：`cd D:\wwwroot\.dev-env-docker && docker-compose up -d`
- **本地无独立 PHP**：本机虽有 PHP 8.0.30，但 rmp-api 业务命令必须在 `rmp-php` 容器内执行

### PHP 框架（rmp-api 项目）

| 依赖 | 版本 |
|------|------|
| ThinkPHP Framework | 6.0.13 |
| think-orm | 2.0.54 |
| think-multi-app | 1.0.14 |
| think-migration | 3.0.3 |
| think-queue | 3.0.7 |
| EasyWeChat | 5.30.0 |

### Go 框架（AI-OS 项目）

| 依赖 | 版本 |
|------|------|
| go-chi/chi | v5.3.0 |
| go-sql-driver/mysql | v1.10.0 |
| redis/go-redis | v9.21.0 |
| robfig/cron | v3.0.1 |
| golang.org/x/crypto | v0.53.0 |

## 目录结构

```
d:\wwwroot\AI\AI-OS\
├── go-backend/              ← Go 后端服务
│   ├── handler/             ← HTTP 处理器
│   │   ├── llm.go          ← LLM 接口处理
│   │   ├── proxy.go        ← 流式代理（透传）
│   │   ├── vision.go       ← 视觉识别接口
│   │   └── ...
│   ├── service/             ← 业务服务层
│   │   ├── vision.go       ← 视觉识别服务
│   │   ├── llm_route.go    ← LLM 多模型路由
│   │   ├── rag_chunk.go    ← RAG chunk 管理
│   │   └── ...
│   ├── middleware/          ← 中间件
│   ├── model/              ← 数据模型
│   ├── logs/               ← 请求日志
│   └── main.go             ← 后端入口
├── web/                     ← Web 前端
│   ├── src/
│   │   ├── views/
│   │   │   ├── llm/       ← LLM 相关页面
│   │   │   ├── rag/       ← RAG 相关页面
│   │   │   └── skills/    ← 技能管理
│   │   └── router/
│   └── package.json
├── docs/                    ← 项目文档
│   ├── adr/                ← 架构决策记录
│   ├── llm-module-overview.md
│   └── ...
└── .trae/                   ← Trae 配置与技能
    ├── skills/             ← 项目级技能
    │   ├── aios-build/
    │   ├── aios-db/
    │   ├── aios-deploy/
    │   └── aios-server-ops/
    └── rules/              ← 项目规则

d:\wwwroot\.trae\skills\     ← 全局技能（所有项目共享）
├── img-ocr/
├── mysql/
├── docker-env/
└── ...
```

## 技术栈说明

### Go 后端
- **语言**：Go
- **主要功能**：LLM 代理网关、视觉识别、技能管理、知识库等

### Web 前端
- **框架**：Vue 3 + Vite
- **主要模块**：LLM 配置、RAG 管理、技能管理

## 关键配置文件

| 配置 | 路径 |
|------|------|
| Go 配置 | `go-backend/config/config.go` |
| Trae 技能配置 | `.trae/skill-config.json` |
| 工作区配置 | `.trae/config/workspaces.json` |

## 运行日志

| 日志类型 | 路径 |
|---------|------|
| Agent 日志 | `C:\ProgramData\AI-OS\logs\agent.log` |
| 请求日志 | `go-backend/logs/requests/`（按时间戳命名） |

## D:\wwwroot 一级目录

```
d:\wwwroot\
├── .dev-env-docker/        ← Docker 开发环境
├── .trae/                  ← 全局 Trae 配置与技能
├── .vscode/                ← VSCode 配置
├── AI/                     ← AI 相关项目
│   ├── AI-OS/             ← AI-OS 主项目
│   ├── mcp/               ← 自研 MCP 框架
│   └── nnd-ai-hub/        ← AI 配置管理桌面应用
├── ai_workspace/           ← AI 专用工作目录
├── ai-os/                  ← AI-OS 构建缓存
├── chartsapi/              ← 图表后端 API
├── frontend/               ← 前端项目
│   ├── nnd-robot/         ← 核心前端（禁止修改）
│   └── charts/            ← 图表前端（禁止修改）
├── socket/                ← 机器人数据传输后端
├── rmp-api/               ← 核心后端（4个工作区）
└── rmp-prd/               ← 产品需求文档
```

## 重要约束

1. **构建缓存分离**：`d:\wwwroot\ai-os\` 仅包含 `.cache` 和 `ai_cache`，不包含业务源码
2. **日志位置**：运行日志统一在 `C:\ProgramData\AI-OS\logs\`
3. **技能管理**：全局技能在 `d:\wwwroot\.trae\skills\`，项目技能在 `.trae/skills/`
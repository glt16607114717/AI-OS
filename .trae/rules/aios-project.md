---
alwaysApply: true
description: AI-OS 企业 AI 资产操作系统，项目介绍与架构总览
---

# AI-OS 企业 AI 资产操作系统

## 项目定位

统一管理企业 AI 资产的平台，涵盖规则引擎、技能引擎、提示词引擎、知识库引擎、MCP 工具引擎和代理网关六大核心模块。提供 LLM 代理网关、多模型路由、视觉识别、技能管理、知识库等能力。

## 技术栈

| 类别 | 技术 |
|------|------|
| 后端语言 | Go 1.26+ |
| 后端框架 | go-chi/chi v5 |
| 数据库 | MySQL（go-sql-driver）、Redis（go-redis v9） |
| 前端框架 | Vue 3 + Vite |
| 运行端口 | 18731（Go 后端，公网）、18733（Nginx 反向代理） |

## 项目结构

```
AI-OS/
├── go-backend/              ← Go 后端服务
│   ├── main.go             ← 入口
│   ├── config/config.go     ← 配置管理
│   ├── handler/            ← HTTP 处理器（llm、vision、rag、skill 等）
│   ├── service/            ← 业务服务层
│   ├── middleware/auth.go  ← 认证中间件
│   └── model/types.go      ← 数据模型
│
├── web/                     ← Web 前端
│   ├── src/views/
│   │   ├── llm/            ← LLM 配置（策略、模型、日志、配额、统计）
│   │   ├── rag/            ← RAG 管理（知识库、配置、日报）
│   │   ├── skills/         ← 技能管理
│   │   └── system/         ← 系统设置
│   └── src/api.ts          ← 前端 API 层
│
└── docs/                    ← 设计文档
```

## 核心模块

| 模块 | 说明 |
|------|------|
| LLM 代理网关 | 多模型路由、failover、流式代理、token 统计 |
| 视觉识别 | 图片 OCR、UI 分析 |
| 知识库（RAG） | 文档解析、向量化、chunk 管理 |
| 技能引擎 | 内置技能管理 |
| 配额管理 | 模型用量统计与限制 |
| 审计日志 | 全链路 LLM 调用日志 |

## 开发流程

- **后端**：修改 `go-backend/`，本地测试 `cd go-backend && go run .`，部署用 `aios-deploy` 技能
- **前端**：修改 `web/src/`，本地测试 `cd web && npm run dev`，部署用 `aios-web-deploy` 技能

## 运行环境

- Go 版本：≥ 1.25.0
- Node.js：24.x（nvm 管理）
- 运行日志：`C:\ProgramData\AI-OS\logs\`
- 构建缓存：`d:\wwwroot\ai-os\`（仅 .cache 和 ai_cache）

# AI-OS

Enterprise AI Asset Operating System - 企业 AI 资产操作系统

## 定位

统一管理企业一切 AI 资产（规则、技能、提示词、知识库、MCP 工具），让 AI 编辑器退化成纯输入输出终端。

## 架构

```
Trae 编辑器（空壳）
      │
      ▼
服务端（AI 大脑）── 决策、管控、存储
      │
      ▼
客户端（AI 手脚）── 执行、本地操作、硬件访问
```

## 项目结构

```
AI-OS/
├── server/          # 服务端 - AI 中台核心
├── client/          # 客户端 - 桌面应用（Electron + Python）
└── docs/            # 设计文档
```

## 作者

桂良涛 (桂良涛@nndrobot.com)

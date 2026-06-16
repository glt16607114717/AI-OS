# AI-OS 项目规则

## 一、项目概述

**AI-OS（企业 AI 资产操作系统）** 是一个统一管理企业 AI 资产的平台，涵盖规则引擎、技能引擎、提示词引擎、知识库引擎、MCP 工具引擎和代理网关六大核心模块。

### 项目定位
- **服务端（大脑）**：决策、管控、存储
- **客户端（手脚）**：执行、本地操作、硬件访问

### 核心特性
- 统一管控所有 AI 资产
- 安全合规：密码不离开本地
- 持续进化的蒸馏引擎
- 双层 AI 架构降低成本
- 全链路审计日志

---

## 二、项目结构

```
AI-OS/
├── client/                          # Electron + Vue 3 客户端
│   ├── electron/main/index.ts       # 主进程：窗口管理 + Agent 服务对接
│   ├── electron/preload/index.ts    # 预加载：安全桥接 IPC
│   ├── src/                         # 前端源码（Vue 3）
│   └── installer/installer.nsh      # NSIS 安装脚本
│
├── go-backend/                      # Go 后端服务
│   ├── main.go                      # 主入口
│   ├── config/config.go             # 配置管理
│   ├── handler/                     # HTTP 处理器
│   ├── middleware/auth.go           # 认证中间件
│   └── service/                     # 业务服务层
│
├── docs/                            # 设计文档
│   ├── system-architecture.md       # 系统架构
│   └── llm-module-overview.md       # LLM 模块功能清单
│
└── .trae/                           # TRAE 编辑器配置
    ├── rules/                       # 规则约定
    └── skills/                      # 自定义技能
```

---

## 三、核心技能

### 3.1 打包技能 (aios-build)
- **触发词**：打包客户端、构建安装包、npm run pack
- **功能**：构建 AI-OS 客户端安装包
- **输出位置**：`D:\ai-os-build\AI-OS-Setup-{version}.exe`

### 3.2 MySQL 操作技能 (aios-mysql)
- **触发词**：查数据库、MySQL 查询、操作 ai_os
- **功能**：操作 AI-OS 数据库（查询/插入/更新/删除）
- **数据库信息**：124.221.220.89:23306/ai_os

### 3.3 Go 部署技能 (aios-deploy)
- **触发词**：部署后端、重启服务、编译 Go
- **功能**：编译并部署 Go 后端服务到服务器
- **参数**：build（仅编译）、deploy（完整部署）、restart（仅重启）

### 3.4 API 调试技能 (api-debug)
- **触发词**：调试接口、测试 API、调用接口
- **功能**：调试 AI-OS API 接口

### 3.5 语音识别技能 (audio-stt)
- **触发词**：识别音频、语音转文字
- **功能**：将音频文件转换为文字

---

## 四、开发流程

### 4.1 后端开发流程
1. 修改 `go-backend/` 目录下的代码
2. 本地测试：`cd go-backend && go run .`
3. 使用 `aios-deploy` 技能部署到服务器

### 4.2 前端开发流程
1. 修改 `client/src/` 目录下的代码
2. 本地测试：`cd client && npm run dev`
3. 使用 `aios-build` 技能构建安装包

### 4.3 数据库操作流程
1. 使用 `aios-mysql` 技能执行 SQL
2. 查询数据或修改配置
3. 谨慎使用 DELETE 和 UPDATE 操作

---

## 五、关键配置

### 5.1 端口配置
| 端口 | 服务 | 用途 |
|------|------|------|
| 18731 | Go 后端 | 业务 API（公网） |
| 18732 | Python Agent | 语音指令、键鼠控制（本地） |
| 23306 | MySQL | ai_os 数据库 |
| 5173 | Vite Dev | 开发模式热更新 |

### 5.2 服务器信息
- **主机**：124.221.220.89
- **用户**：ubuntu
- **服务路径**：/opt/ai-os/ai-os-server
- **systemd 服务**：ai-os.service

---

## 六、安全规范

1. **密码安全**：密码只在客户端本地持有，不传输到服务端
2. **权限控制**：所有敏感操作必须先经服务端权限校验
3. **审计日志**：所有 LLM 调用和用户操作都记录日志
4. **API Key 管理**：定期轮换密钥，避免硬编码

---

## 七、运维指南

### 7.1 检查服务状态
```bash
ssh ubuntu@124.221.220.89 "sudo systemctl status ai-os"
```

### 7.2 查看日志
```bash
ssh ubuntu@124.221.220.89 "sudo journalctl -u ai-os -f"
```

### 7.3 重启服务
- 使用 `aios-deploy` 技能，参数：restart

---

## 八、注意事项

1. 部署前建议备份当前服务
2. 修改数据库前确认 SQL 语句正确性
3. 前端打包需要 Node.js 环境
4. Go 后端部署需要 Go 环境和 SSH 免密登录
5. 首次打包需要安装依赖，时间较长

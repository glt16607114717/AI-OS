---
alwaysApply: false
description: 当开发涉及 Node.js 脚本编写、MCP Server 开发、处理 package.json 或依赖安装时触发。核心管理工具为 nvm 与 pnpm。
---
# Node.js 环境使用与开发规范

## 核心原则与环境初始化 (nvm + pnpm)
本项目严格隔离 Node 环境，**绝对禁止直接使用系统全局的 node 或 npm**。

- **Node 版本管理**：统一使用 `nvm`。项目根目录应包含 `.nvmrc` 文件以锁定版本（推荐 v20+ LTS）。
- **包管理器**：统一使用 `pnpm`。严禁生成 `npm install` 或 `yarn add` 的指令。

### 环境初始化标准动作
```bash
nvm install    # 首次读取 .nvmrc 安装对应版本
nvm use        # 切换到该项目专属版本
pnpm install   # 安装依赖到项目本地的 node_modules/
```
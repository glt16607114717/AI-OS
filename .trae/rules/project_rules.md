# AI-OS 项目规则

## 项目定位

AI-OS（企业 AI 资产操作系统）：统一管理企业一切 AI 资产（规则、技能、提示词、知识库、MCP 工具），让 AI 编辑器退化成纯输入输出终端。

详细规划见项目根目录 README.md。

## 项目结构规范（强制遵循）

```
AI-OS/
├── client/                          # 客户端 - Electron + Vue 3 桌面应用
│   ├── electron/                    # Electron 层
│   │   ├── main/index.ts           # 主进程：窗口管理 + Agent 服务对接
│   │   └── preload/index.ts        # 预加载：安全桥接 IPC
│   ├── src/                         # 前端源码（Vue 3）
│   │   ├── App.vue                 # 根组件
│   │   └── main.ts                 # 入口
│   ├── backend/                     # 内嵌后端
│   │   └── python/                  # Python Agent 服务
│   │       ├── agent/main.py       # Agent 入口（FastAPI 健康检查）
│   │       ├── install_service.py  # WinSW 服务注册/卸载脚本
│   │       └── requirements.txt    # Python 依赖
│   ├── resources/                   # 打包资源（二进制文件，构建时填充）
│   │   └── winsw/                   # WinSW 服务包装器
│   ├── installer/                   # NSIS 安装脚本
│   │   └── installer.nsh           # 安装时注册服务、卸载时清理
│   ├── public/                      # 静态资源（图标等）
│   ├── index.html                   # HTML 入口
│   ├── vite.config.ts               # Vite + Electron 构建配置
│   ├── tsconfig.json                # TypeScript 配置
│   └── package.json                 # 依赖与打包配置
│
├── server/                          # 服务端 - AI 中台核心
│
├── docs/                            # 设计文档
│   └── architecture.md              # 架构设计
│
├── .cache/                          # 本地缓存（不入库）
│   ├── downloads/                   # 下载的第三方二进制
│   └── venv/                        # 开发用 Python 虚拟环境
│
├── .gitignore
└── README.md
```

## 架构原则

### 客户端/服务端职责划分

| 维度 | 服务端（大脑） | 客户端（手脚） |
|------|--------------|--------------|
| 核心职责 | 决策、管控、存储 | 执行、本地操作、硬件访问 |
| 数据存储 | 集中存储所有资产 | 仅存储本地缓存 |
| 密码等敏感信息 | 不存储密码 | 持有密码，不离开本地 |
| AI 推理 | 所有 AI 调用 | 无 AI 推理能力 |
| 网络访问 | 访问外网（模型 API） | 访问内网资源 |
| 文件系统 | 仅管理代码库 | 访问全量本地文件 |
| 硬件访问 | 无 | CPU/GPU、USB、显示器 |

### 开发约束

1. **严禁在客户端做 AI 推理**：所有模型调用必须经过服务端代理网关
2. **严禁密码离开客户端**：数据库密码、API Key 等敏感凭证只在客户端本地持有
3. **服务端校验前置**：客户端执行的任何敏感操作，必须先经服务端权限校验
4. **IPC 通信规范化**：Electron 主进程与渲染进程之间通过 preload 桥接，禁止直接使用 Node.js API

## 技术栈

### 服务端
- 后端框架：Python (FastAPI)
- 向量检索：Qdrant / Milvus
- 数据库：PostgreSQL
- 缓存：Redis
- 消息队列：RabbitMQ / Kafka
- 高性能加速：Rust (PyO3 绑定)

### 客户端
- 桌面框架：Electron + Python
- UI 框架：Vue 3 + Element Plus
- 本地执行：Python (本地脚本)
- 跨进程通信：WebSocket / IPC

## 六大核心引擎（服务端）

1. **代理网关（Gateway）**：多模型路由、请求缓存、限流熔断、审计日志
2. **规则引擎（Rule Engine）**：多维度规则管理、版本控制、违规检测、热更新
3. **技能引擎（Skill Engine）**：技能注册与发现、版本管理、依赖管理、技能编排
4. **MCP 引擎（MCP Engine）**：工具注册中心、权限控制、调用审计、工具链编排
5. **提示词引擎（Prompt Engine）**：模板管理、变量注入、版本控制、优化建议
6. **知识库引擎（Knowledge Engine）**：向量化存储、语义检索、知识图谱、知识更新

## 代码规范

- TypeScript 代码遵循项目现有 tsconfig 配置
- Python 代码遵循 FastAPI 最佳实践
- 提交代码前确保 TypeScript 类型检查通过
- 作者署名：桂良涛，邮箱：桂良涛@nndrobot.com

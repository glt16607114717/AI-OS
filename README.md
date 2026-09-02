# AI-OS

企业 AI 资产操作系统 —— 统一管理 LLM 路由、技能、知识库，让 AI 编辑器退化成纯终端。

## 技术栈

| 层 | 技术 |
|------|------|
| 后端 | Go 1.21+ (chi 路由) |
| 数据库 | MySQL 8.0 |
| 前端 | Vue 3 + Element Plus + Vite |
| 桌面端 | Electron（独立维护，与主体无关） |

## 项目结构

```
ai-os/
├── go-backend/              # Go 后端服务
│   ├── handler/             # HTTP 处理器（按业务域拆分）
│   ├── service/             # 业务逻辑层
│   ├── model/               # 类型定义
│   ├── middleware/          # 认证中间件
│   ├── config/              # 配置管理
│   └── main.go              # 入口 + 路由注册
├── web/                     # Web 前端（Vue 3）
├── tool/                    # Electron 桌面端（独立小工具，与主体无关）
├── docs/                    # 技术文档
│   └── *.md                 # 架构设计文档
├── tools/                   # 运维脚本
└── .trae/                   # TRAE IDE 配置（规则/技能/MCP）
```

## 核心能力

1. **LLM 代理网关**：多厂商路由、故障转移、用量统计
2. **Agent 技能系统**：内置技能（MySQL 查询等）+ 多轮 tool_calls 循环
3. **RAG 知识库**：文档向量化 + 语义检索
4. **上帝指令**：全局 prompt 注入 + 额度监控 + 模型降级
5. **权限管理**：用户/管理员体系 + 技能连接授权矩阵

## 作者

桂良涛 (1938559091@qq.com)

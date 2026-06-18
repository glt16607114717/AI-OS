# AI-OS 系统架构与运维文档

> **最后更新**: 2026-06-15
> **目的**: 固化系统架构、部署流程、运维操作规范，避免重复踩坑

---

## 一、系统架构总览

```
┌─────────────────────────────────────────────────────────┐
│                    用户 Windows PC                         │
│                                                           │
│  ┌──────────────┐    IPC     ┌──────────────────────┐   │
│  │  Electron UI  │◄──────────►│  Electron Main       │   │
│  │  (Vue 3 +     │            │  (index.ts)          │   │
│  │   Element+)   │            │                       │   │
│  └──────┬────────┘            └──────────┬───────────┘   │
│         │ HTTP                            │ HTTP           │
│         │ 18731                           │ 18732          │
│         ▼                                 ▼               │
│  ┌──────────────┐            ┌──────────────────────┐   │
│  │  Go 后端      │            │  Python Agent        │   │
│  │  (云服务器)    │            │  (本地 pythonw.exe)  │   │
│  │  124.221.     │            │  127.0.0.1:18732     │   │
│  │  220.89:18731 │            │  语音识别/键鼠控制    │   │
│  └──────┬────────┘            └──────────────────────┘   │
│         │                                                 │
│         │ TCP 23306                                       │
│         ▼                                                 │
│  ┌──────────────┐                                       │
│  │  MySQL        │                                       │
│  │  ai_os 数据库  │                                       │
│  └──────────────┘                                       │
│                                                           │
│  数据目录（固定）: C:\ProgramData\AI-OS\                  │
│    ├── runtime\python\     (Python 嵌入版)                │
│    ├── models\             (语音模型 ~2.5GB)              │
│    └── data\               (配置文件)                     │
│                                                           │
│  安装目录（用户可选）: 如 C:\AI-OS\ 或其他路径            │
│    └── resources\backend\python\agent\main.py             │
└─────────────────────────────────────────────────────────┘
```

### 端口分布

| 端口 | 服务 | 监听位置 | 用途 |
|------|------|---------|------|
| **18731** | Go 后端 | `0.0.0.0:18731` (公网) | 业务 API（登录、LLM 代理、RAG、统计） |
| **18732** | Python Agent | `127.0.0.1:18732` (本地) | 语音指令、键鼠控制、健康检查 |
| **3306** | MySQL | `127.0.0.1:3306` | ai_os 数据库 |
| **5173** | Vite Dev | `localhost:5173` | 仅开发模式热更新 |

---

## 二、云服务器（Go 后端）部署

### 2.1 服务器信息

| 项 | 值 |
|----|-----|
| IP | `8.163.127.182` |
| 用户 | `root`（SSH端口 443） |
| Go 二进制 | `/opt/ai-os/ai-os-server` |
| systemd 服务 | `ai-os.service` |
| MySQL | `127.0.0.1:3306`，库 `ai_os` |

### 2.2 systemd 服务配置

```ini
# /etc/systemd/system/ai-os.service
[Unit]
Description=AI-OS Server
After=network.target

[Service]
ExecStart=/opt/ai-os/ai-os-server
Restart=always
RestartSec=5
WorkingDirectory=/opt/ai-os

[Install]
WantedBy=multi-user.target
```

### 2.3 部署流程（从开发机发布到服务器）

```bash
# 1. 本地编译（Windows 交叉编译为 Linux ELF）
cd D:\wwwroot\AI\AI-OS\go-backend
$env:GOOS="linux"; $env:GOARCH="amd64"
go build -o ai-os-server .

# 2. 上传到服务器
scp -P 443 ai-os-server root@8.163.127.182:/opt/ai-os/

# 3. SSH 登录服务器，替换并重启
ssh -p 443 root@8.163.127.182

# 4. 在服务器上执行：
sudo cp /home/ubuntu/aios-server/ai-os-server /opt/ai-os/ai-os-server
sudo chmod +x /opt/ai-os/ai-os-server
sudo systemctl restart ai-os

# 5. 验证
sudo systemctl status ai-os
curl -s http://127.0.0.1:18731/api/health
```

### 2.4 运维命令速查

```bash
# 查看状态
sudo systemctl status ai-os

# 重启服务
sudo systemctl restart ai-os

# 停止服务
sudo systemctl stop ai-os

# 查看日志
sudo journalctl -u ai-os -n 50 --no-pager
sudo journalctl -u ai-os -f          # 实时跟踪

# 检查端口占用
sudo ss -tlnp | grep 18731
```

> **重要**: 服务器上之前可能残留手动 `nohup` 启动的旧进程，占用 18731 端口导致 systemd 启动失败。遇到 `bind: address already in use` 时执行：
> ```bash
> sudo fuser -k 18731/tcp
> sudo systemctl restart ai-os
> ```

---

## 三、客户端安装流程

### 3.1 打包流程

```bash
# 开发机执行（必须用 npm run pack，不要重复 npm run build）
cd D:\wwwroot\AI\AI-OS\client
npm run pack
```

打包脚本 `scripts/build.ps1` 会：
1. 执行 `npm run package`（包含 vite build + electron-builder）
2. 清理输出目录，只保留 exe
3. 产物输出到 `D:\ai-os-build\AI-OS-Setup-1.0.0.exe`

> **注意**: `npm run package` 本身已包含编译，不要先 `npm run build` 再 `npm run package`，会导致缓存混乱。

### 3.2 NSIS 安装器配置

| 配置项 | 值 | 说明 |
|--------|-----|-----|
| `perMachine` | `false` | 安装程序启动时**不需要**管理员权限 |
| `oneClick` | `false` | 有安装向导 |
| `allowElevation` | `true` | 允许 UAC 提权（注册服务时需要） |
| `allowToChangeInstallationDirectory` | `true` | **用户可自定义安装路径** |

### 3.3 安装器自定义脚本 (installer.nsh)

安装器执行顺序：

1. **customInit**: 读取注册表中上次安装路径；如果没有记录则默认推荐路径
2. **customInstall**:
   - **无条件**通过提权（`Start-Process -Verb RunAs`）执行：杀死所有 `pythonw`、`python`、`AI-OS` 进程 + 删除旧计划任务 `AI-OS-Agent` 和 `AI-OS-Watchdog`
   - 写入注册表（Admin → HKLM，User → HKCU）
   - 通过提权用 `icacls` 修复 `C:\ProgramData\AI-OS` 权限

### 3.4 安装后目录结构

安装路径由用户选择（如 `C:\AI-OS\`、`D:\Programs\AI-OS\` 等），结构固定：

```
<用户选择的安装目录>\AI-OS\
├── AI-OS.exe                    # Electron 主程序
├── resources\
│   ├── app.asar                 # 前端打包文件
│   ├── icon.ico
│   └── backend\python\agent\    # Python Agent 源码
│       ├── main.py
│       └── watchdog.py
└── installer.nsh

C:\ProgramData\AI-OS\            # 数据目录（固定，与安装路径无关）
├── runtime\python\              # Python 3.12.10 嵌入版（setup 时下载）
│   ├── python.exe
│   ├── pythonw.exe
│   └── Lib\site-packages\       # pip 依赖
├── models\                      # 语音模型 (~2.5GB)
│   └── ...
├── data\                        # 运行时配置
│   ├── voice_config.json
│   └── llm_config.json
└── setup-debug.log              # 安装日志
```

---

## 四、环境配置流程（Setup Wizard）

### 4.1 启动检测逻辑

```
用户启动 AI-OS.exe
  → Electron app.whenReady()
  → 创建窗口，加载前端
  → App.vue onMounted()
  → 调用 window.aiOS.checkSetupNeeded()
  → IPC: setup:check
  → Electron Main: agentHealthCheck()
  → GET http://127.0.0.1:18732/health
  → 如果 {ok: true} → 跳过配置 → 显示 Logo 动画 → 进入 Dashboard
  → 如果请求失败 → 需要配置 → 显示 SetupWizard
```

> **关键设计**: `setup:check` 只检查 health 接口，与 Python 有没有安装无关。
> 安装器已经杀死了所有旧进程，所以从安装器过来的 health 一定不通，会自动进入配置流程。

### 4.2 Setup Wizard 五步流程

| 步骤 | 名称 | 操作 | 跳过条件 |
|------|------|------|---------|
| 1 | 环境检测 | 检查系统环境 | 不跳过 |
| 2 | 下载 Python | 下载 Python 3.12.10 嵌入版 | 已存在且版本正确 |
| 3 | 安装依赖 | pip install 依赖 | 已全部安装 |
| 4 | 注册服务 | 创建 Windows 计划任务 | 不跳过 |
| 5 | 启动服务 | 启动 Python Agent | 不跳过 |

### 4.3 注册服务详情

通过 PowerShell 执行（需要 UAC 管理员权限）：

1. 杀死残留进程（pythonw、python、ai-os-agent）
2. 删除旧计划任务
3. 创建 `AI-OS-Agent` 计划任务：
   - 触发器: `ONLOGON`（用户登录时自动启动）
   - 权限: `HIGHEST`（最高权限）
   - 命令: `pythonw.exe -X utf8 main.py`
4. 创建 `AI-OS-Watchdog` 计划任务：
   - 触发器: 每 1 分钟执行
   - 用途: 监控 Agent 存活，崩溃自动拉起
5. 创建数据目录并设置权限: `icacls C:\ProgramData\AI-OS\data /grant Users:F /T`

### 4.4 UAC 权限说明

| 阶段 | 是否需要 UAC | 说明 |
|------|-------------|------|
| 安装程序启动 | 否 | `perMachine: false` |
| 文件解压 | 否 | 写入用户目录 |
| 注册计划任务 | **是** | `schtasks /Create` 需要管理员权限 |
| 修复目录权限 | **是** | `icacls` 需要管理员权限 |

UAC 弹窗只会出现一次（首次环境配置时），用户授权后后续启动不需要。

---

## 五、Python Agent 自启动流程

### 5.1 启动链路

```
Windows 用户登录
  → 计划任务 AI-OS-Agent 触发
  → 执行: pythonw.exe -X utf8 main.py
  → Python Agent 启动，监听 127.0.0.1:18732
  → 提供 /health、/api/voice 等接口

同时:
  → 计划任务 AI-OS-Watchdog 每 1 分钟触发
  → 检查 Agent 是否存活
  → 如果崩溃则自动拉起
```

### 5.2 Electron 与 Python Agent 的通信

```
Electron Main (index.ts)
  ├── agentHealthCheck()  →  GET http://127.0.0.1:18732/health
  ├── agentRequest()      →  POST http://127.0.0.1:18732/api/voice
  └── agentRestart()      →  重启 Python Agent
```

前端通过 `window.aiOS.agentHealth()` 和 `window.aiOS.agentRequest()` 调用。

### 5.3 数据目录权限

Python Agent 以 `pythonw.exe` 运行，需要读写 `C:\ProgramData\AI-OS\data\`。
Setup Wizard 会通过 `icacls` 授予 `Users` 组完全控制权限。

---

## 六、Go 后端接口规范

### 6.1 统一响应格式

```json
// 成功
{"ok": true, "data": ...}

// 失败
{"ok": false, "error": "错误信息"}
```

### 6.2 接口分类

| 路由前缀 | 认证 | 说明 |
|----------|------|------|
| `/api/login` | 否 | 登录 |
| `/api/health` | 否 | 健康检查 |
| `/api/system/me` | 是 | 当前用户信息（token 验证） |
| `/api/voice/*` | 是 | 语音指令 CRUD |
| `/api/users/*` | 是 | 用户管理（管理员） |
| `/api/llm/*` | 是 | LLM 配置/策略 |
| `/api/chat/*` | 是 | 聊天工作区 |
| `/api/rag/*` | 是 | RAG 知识库 |
| `/api/stats/*` | 是 | 统计面板 |
| `/v1/chat/completions` | 是 | OpenAI 兼容代理 |

### 6.3 认证机制

- 登录返回 token（随机 64 字符 hex）
- token 存储在内存 Map 中（`Sessions` 和 `UserTokens`）
- 单点互踢：同一用户只能有一个活跃 session
- token 通过 `Authorization: Bearer <token>` 传递
- token 有效期：30 天

---

## 七、数据库结构

### 7.1 MySQL 连接

```
Host:     8.163.127.182
Port:     23306
Database: ai_os
User:     root
Password: (见 config.go)
```

### 7.2 核心表

| 表名 | 用途 |
|------|------|
| `sys_user` | 系统用户（id, username, password, status, is_admin） |
| `voice_commands` | 语音指令（user_id, phrase, position, actions, enabled） |
| `sys_config` | 用户配置（user_id, cfg_key, cfg_value） |
| `llm_vendor_keys` | LLM 厂商密钥 |
| `llm_strategies` | LLM 路由策略 |
| `god_rules` | 系统提示词规则 |
| `chat_history` | 聊天记录 |
| `llm_logs` | LLM 调用日志 |
| `quota_records` | 额度记录 |

---

## 八、常见问题排查

### Q1: 安装后跳过配置页面直接进入应用

**原因**: 旧的 Python Agent 进程仍在运行，health 检查通过。
**解决**: 安装器应在 `customInstall` 阶段杀死所有旧进程。如果仍有残留，手动杀进程后重新启动。

### Q2: 账户设置看不到用户列表

**原因**: 后端返回 `{"data": {"users": [...]}}`，前端需要从 `data.data` 中读取。
**解决**: 前端兼容处理 `Array.isArray(raw) ? raw : raw?.users || []`。

### Q3: 删除用户返回成功但实际没删除

**原因**: 后端从 URL query 取 id（`r.URL.Query().Get("id")`），前端发的是 JSON body。
**解决**: 后端统一从 `body["id"]` 读取。

### Q4: Go 后端部署后启动失败 `bind: address already in use`

**原因**: 旧的手动 `nohup` 进程占用 18731 端口，与 systemd service 冲突。
**解决**:
```bash
sudo fuser -k 18731/tcp
sudo systemctl restart ai-os
```

### Q5: 语音模型丢失

**原因**: 清理旧安装时误删 `C:\ProgramData\AI-OS\models`。
**预防**: 清理旧版本前先备份模型目录。

### Q6: 前端 `data.detail` 报错

**原因**: Go 后端返回 `{"error": "..."}`，不是 `{"detail": "..."}`（FastAPI 格式）。
**解决**: 前端统一使用 `data.error`。

---

## 九、关键文件清单

| 文件 | 路径 | 用途 |
|------|------|------|
| Go 主入口 | `go-backend/main.go` | 路由注册、启动 |
| Go 配置 | `go-backend/config/config.go` | 数据库、Embedding 配置 |
| Go 用户服务 | `go-backend/service/user.go` | 用户 CRUD |
| Go 语音服务 | `go-backend/service/voice.go` | 语音指令 CRUD |
| Electron 主进程 | `client/electron/main/index.ts` | 窗口管理、IPC |
| Setup 管理器 | `client/electron/main/setup-manager.ts` | 环境配置流程 |
| 安装器脚本 | `client/installer/installer.nsh` | NSIS 自定义安装 |
| 打包配置 | `client/package.json` | electron-builder 配置 |
| 打包脚本 | `client/scripts/build.ps1` | 一键打包 |
| 前端 API 配置 | `client/src/api.ts` | API_BASE 地址 |
| 前端路由 | `client/src/router/index.ts` | 路由 + 登录守卫 |
| 账户设置 | `client/src/views/system/AccountSettings.vue` | 用户管理页面 |
| 语音助手 | `client/src/views/VoiceAssistant.vue` | 语音指令页面 |

---

## 十、变更记录

| 日期 | 变更内容 |
|------|---------|
| 2026-06-15 | 初始版本，固化所有架构和流程 |

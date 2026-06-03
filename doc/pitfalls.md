# AI-OS 开发踩坑记录

## 1. Python embed 版 `._pth` 文件控制 sys.path

**环境**：Windows embed Python（python embeddable package）

**现象**：`from xxx import yyy` 报 `ModuleNotFoundError`，明明文件就在旁边。

**根因**：embed Python 目录下有 `python312._pth` 文件（版本号可能不同），内容如：
```
Lib
Lib\site-packages
.
import site
```
这个文件**完全控制**了初始 sys.path。只有列出的路径会被搜索，**忽略** PYTHONPATH 环境变量。

**解决**：在 main.py 里手动 `sys.path.insert(0, ...)` 是有效的（运行时修改不受 `._pth` 限制）。

---

## 2. Python import 搜索的是"包的父目录"

**现象**：`from voice import init_voice` 报 `No module named 'voice'`，但 `voice/` 就在旁边。

**目录结构**：
```
backend/python/           ← voice 包的父目录
├── agent/
│   └── main.py           ← 入口
└── voice/
    └── __init__.py
```

**根因**：Python 的 `import voice` 是在 sys.path 的每个目录下找 `voice/`。如果 sys.path 里只有 `agent/`，Python 只会在 `agent/voice/` 下找，找不到 `../voice/`。

**解决**：必须把 `agent/` 的**父目录**（`backend/python/`）加到 sys.path：
```python
BACKEND_DIR = Path(__file__).resolve().parent      # agent/
VOICE_DIR = BACKEND_DIR.parent                      # python/
sys.path.insert(0, str(VOICE_DIR))
```

**教训**：以后新增任何 Python 模块（如 `ocr/`、`llm/`），只要和 `agent/` 平级，都不需要额外配置。`VOICE_DIR` 已经把它们的共同父目录加到 sys.path 了。

---

## 3. WinSW 服务的工作目录不是脚本目录

**现象**：服务启动时 `__file__` 正确，但相对路径的文件找不到。

**根因**：WinSW 的 `<workingdirectory>` 配置决定了服务进程的 CWD。我们设的是 Python runtime 目录（`C:\ProgramData\AI-OS\runtime\python`），不是 main.py 所在目录。

**解决**：所有文件操作必须用绝对路径，基于 `Path(__file__).resolve()` 计算。

---

## 4. SYSTEM 用户写入权限

**现象**：服务进程（SYSTEM 用户）无法写入 `D:\app\AI-OS\` 目录。

**根因**：安装目录的权限可能不包含 SYSTEM 的写入权限。

**解决**：日志、临时文件等应写到 `C:\ProgramData\AI-OS\` 目录，SYSTEM 对该目录有完整权限。

---

## 5. NSIS 覆盖安装的服务管理

**流程**：
1. `customInit`：winsw stop → winsw uninstall → taskkill（杀进程释放文件锁）
2. NSIS 复制新文件
3. `customInstall`：写注册表
4. Electron 启动 → SetupWizard → winsw install + start（注册新服务）

**关键**：杀进程前必须先卸载服务（winsw uninstall），否则服务恢复机制会立刻重启进程。

---

## 6. Element Plus 全局注册

**现象**：删掉 Element Plus 后页面空白。

**解决**：在 `main.ts` 里全局注册 Element Plus：
```ts
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
app.use(ElementPlus)
```
VoiceAssistant.vue 依赖 Element Plus 组件（el-card、el-switch、el-tag 等）。

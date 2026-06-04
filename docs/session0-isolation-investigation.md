# AI-OS 语音助手 Session 0 隔离问题排查记录

**日期**：2026-06-04
**作者**：桂良涛
**问题**：安装版语音助手的标定（空格确认）和录制（F9 停止）功能完全失效

---

## 问题现象

安装版（NSIS + WinSW 服务）运行后：
- 鼠标标定按空格无反应
- 键鼠录制按 F9 无反应
- `GetAsyncKeyState()` 始终返回 0
- `GetCursorPos()` 始终返回 (0,0)

---

## 根因

**WinSW 以 LocalSystem 身份运行在 Session 0（服务会话），无法访问用户桌面（Session 1）的键鼠输入。**

Windows 从 Vista 开始引入了会话隔离机制：
- Session 0：所有 Windows 服务运行在此，无交互式桌面
- Session 1+：用户登录后的交互式桌面

`GetAsyncKeyState`、`GetCursorPos`、`SetCursorPos`、`mouse_event` 等用户态 API 只能操作调用者所在会话的桌面。

---

## 尝试过的方案及结果

### 方案 1：CreateProcessAsUser（失败）

**思路**：从 Session 0 的 WinSW 服务中，通过 `CreateProcessAsUser` API 在 Session 1 创建子进程。

**实现**：
- 找到 explorer.exe 的进程令牌
- `DuplicateTokenEx` 复制为主令牌
- `CreateProcessAsUser` 在该令牌的会话中创建子进程

**结果**：
- 子进程确实跑在 Session 1（`SessionId=1`）
- `GetCursorPos()` 正常工作 ✓
- **`GetAsyncKeyState()` 始终返回 0** ✗

**失败原因**：
虽然进程在 Session 1，但它是从 Session 0 的 LocalSystem 进程通过令牌复制创建的。Windows UIPI（User Interface Privilege Isolation）机制限制了此类进程对用户输入状态的访问。

**额外问题**：
- 使用 `pythonw.exe` 时进程静默崩溃（无 stdout/stderr），极难排查
- 改用 `python.exe` + `CREATE_NO_WINDOW` 后可正常运行但不弹黑框

### 方案 2：计划任务 + S4U 登录类型（失败）

**思路**：注册 Windows 计划任务（Scheduled Task），用 S4U（Service for User）登录类型，不需要用户密码。

**实现**：
```powershell
$principal = New-ScheduledTaskPrincipal -UserId $username -LogonType S4U -RunLevel Highest
Register-ScheduledTask ... -Principal $principal
```

**结果**：
- 注册成功，AtLogOn 触发后启动
- **但进程仍然跑在 Session 0**（`SessionId=0`）
- `GetCursorPos` 返回 (0,0)，`GetAsyncKeyState` 返回 0

**失败原因**：
S4U 类型虽然是"以用户身份运行"，但它创建的是非交互式会话。任务计划程序在 Session 0 中模拟用户身份运行，不连接到用户桌面。

### 方案 3：计划任务 + 不指定 Principal（成功）

**思路**：参照旧项目 glt-ai-hub 的做法，不指定 `-Principal` 参数，直接用当前用户身份注册。

**实现**：
```powershell
$triggerLogon = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $triggerLogon -Settings $settings -Force
```

**关键区别**：不指定 Principal → 使用 Interactive 登录类型 → 任务在用户桌面（Session 1/2）运行

**结果**：
- 进程跑在用户会话（`SessionId=2`）✓
- `GetCursorPos()` 正常返回鼠标坐标 ✓
- `GetAsyncKeyState()` **仍需验证**（日志太密未确认）

---

## 踩过的坑汇总

### 坑 1：pythonw.exe 静默崩溃

`CreateProcessAsUser` 用 `pythonw.exe` 创建子进程，进程启动后瞬间崩溃，无任何错误输出。

**解决**：改用 `python.exe` + `CREATE_NO_WINDOW` 标志。不弹黑框，但有 stdout/stderr 可写日志。

### 坑 2：CREATE_NO_WINDOW 下 print() 失败

`CREATE_NO_WINDOW` 创建的进程没有控制台，`print()` 会抛 `RuntimeError`。voice 模块的 `log()` 函数用 `print()` + 文件写入，`print()` 的异常被 `except` 吞掉，但某些情况下文件写入也失败了。

**解决**：`log()` 函数改用 Python `logging` 模块（在 voice_worker 中配置了 FileHandler）。

### 坑 3：计划任务注册需要管理员权限

`Register-ScheduledTask` 无论用什么登录类型（S4U、Interactive），都需要管理员权限。普通用户和 LocalSystem 的 WinSW 服务都无法注册。

**解决**：通过 `elevate.exe` 弹 UAC 确认框提权注册（只弹一次）。

### 坑 4：schtasks /Run 手动触发跑在 Session 0

即使用正确的登录类型注册了计划任务，通过 `schtasks /Run` 或 `Start-ScheduledTask` 手动触发时，任务仍可能跑在 Session 0。

**解决**：只有 AtLogOn 触发器自动触发时，任务才会跑在用户桌面会话。手动触发不可靠。

### 坑 5：SYSTEM 创建的文件用户无写权限

WinSW 以 LocalSystem 运行时创建的 `voice_config.json`，Users 组只有读取权限（RX），没有写入权限。计划任务以用户身份运行的 voice_worker 写配置时报 `Permission denied`。

**根因**：`C:\ProgramData` 下的文件继承自父目录的 ACL，SYSTEM 创建的文件默认不给普通用户写权限。

**解决方向**：放弃 WinSW，全部用计划任务（用户身份运行），从根本上消除权限问题。

### 坑 6：NSIS 打包时输出文件被锁

如果绿色版 AI-OS.exe 或安装程序正在运行，NSIS 打包到写入 `AI-OS-Setup-0.1.0.exe` 时会报 `Can't open output file`。

**解决**：打包前先关掉所有 AI-OS 相关进程。

### 坑 7：PowerShell 多行字符串/HEREDOC 不兼容

PowerShell 5 不支持 `&&` 语法，`git commit` 的 HEREDOC 格式也无法使用。

**解决**：用 `;` 替代 `&&`，用简单 `-m` 参数替代 HEREDOC。

---

## 最终方案：放弃 WinSW，全部用计划任务

### 架构对比

```
旧架构（WinSW）：
  WinSW (Session 0, LocalSystem)
    └── main.py (18731) → 转发到 voice_worker (18732)
    └── 问题：Session 0 无法读键鼠、权限冲突

新架构（计划任务）：
  计划任务 (Session 1, 用户身份)
    └── main.py (18731) → 直接导入 voice 模块
    └── 优势：Session 1 完美访问键鼠、无权限问题
```

### 计划任务配置

```powershell
# 触发器
AtLogOn + Daily 00:00

# 设置
- AllowStartIfOnBatteries
- DontStopIfGoingOnBatteries
- StartWhenAvailable
- RestartInterval = 1 分钟
- RestartCount = 999
- ExecutionTimeLimit = 无限
- MultipleInstances = IgnoreNew

# 不指定 Principal（关键！）
```

### 注意事项

1. **不指定 Principal** 是跑在用户 Session 的关键
2. 首次注册需要管理员权限（通过 elevate.exe 弹一次 UAC）
3. 用户注销后进程终止（对语音助手是正确行为）
4. 崩溃恢复最快 1 分钟（可通过内部 watchdog 秒级恢复）

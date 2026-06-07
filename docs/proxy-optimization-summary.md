# AI-OS 代理优化讨论汇总

> 日期：2026-06-06
> 背景：通过分析 Trae IDE 的 HTTP 请求体，发现多个可优化点

---

## 一、核心发现：Trae 请求体 100% 全量注入

通过对比 20 个请求转储文件（`C:\ProgramData\AI-OS\logs\requests\`），确认 Trae 不做任何动态筛选：

### 每次请求的固定开销

| 组件 | 数量 | Token 消耗 | 是否可优化 |
|------|------|-----------|-----------|
| 内置工具（Read/Write/Grep...） | 20 个 | ~16,247 | 否（必需） |
| MCP 工具（Playwright/Memory/Everything） | 40 个 | ~6,113 | **是** |
| 技能列表 | 16 个 | ~1,621 | 否（只有名称+描述，按需加载） |
| Rules（10个规则文件） | 全量注入 user 消息 | ~2,000+ | 部分可优化 |
| Response Language Settings | 每条 user 消息重复 | 55~83 次 | **是** |
| system prompt | 固定 13,817 chars | ~4,605 | 已注入上帝指令 |

### 请求体结构

```
{
  "model": "glt",
  "max_tokens": 16000,
  "stream": true,
  "tools": [20 个内置 + 40 个 MCP],     ← 全量，不过滤
  "messages": [
    {"role": "system", "content": "[上帝指令] + [Trae系统提示]"},
    {"role": "user",   "content": "<system-reminder>...规则内容...</system-reminder> + 用户消息"},
    {"role": "assistant", "content": "AI回复"},
    ...对话历史...
  ]
}
```

---

## 二、已完成的功能

### 2.1 上帝指令注入 ✅
- 配置文件：`C:\ProgramData\AI-OS\config\god_rules.json`
- 前端页面：大模型 → 上帝指令（Markdown 编辑器 + 预览 + 开关）
- 注入方式：`[HIGHEST PRIORITY - 上帝指令]` 前缀，插入 system prompt 最前面
- 实时读取：每次请求重新读取 JSON 文件，不缓存，不重启

### 2.2 请求转储 ✅
- 存储：`C:\ProgramData\AI-OS\logs\requests\{timestamp}.json`
- 内容：完整请求体（已实现）+ AI 返回体（已实现）
- 保留：最近 20 个文件，自动清理

### 2.3 统计仪表盘自动刷新 ✅
- 每 5 秒自动刷新，可暂停

### 2.4 侧边栏重启按钮 ✅
- 点击自动杀掉旧进程并启动新的

---

## 三、待做的优化项（按优先级排序）

### P0：去掉 Rules 中的重复内容
- **问题**：`Response Language Settings` 在每条 user 消息的 `<system-reminder>` 里重复出现 55~83 次
- **预估节省**：~2,000 tokens/请求
- **方案**：代理转发前，遍历 user 消息，去掉重复的 `<system-reminder>` 块
- **难度**：低

### P1：动态裁剪 MCP 工具
- **问题**：40 个 MCP 工具每次全量发送，Playwright 19 个、Everything 13 个极少使用
- **预估节省**：~4,000 tokens/请求
- **方案**：根据用户消息内容判断是否需要某个 MCP，不需要就从 tools 数组中移除
- **难度**：中（需要简单关键词匹配逻辑）
- **风险**：如果判断错误，AI 就无法调用该工具

### P2：压缩空的 tool 返回
- **问题**：大量 `"Results from Grep have been CLEARED."` 等空结果占据上下文
- **预估节省**：~5,000 tokens/请求
- **方案**：将空结果统一压缩为短标记
- **难度**：中

### P3：Playwright 测试方案优化
- **问题**：Playwright MCP 通过 AI 一步步操作浏览器，效率低、识别不准
- **方案**：
  1. 前端关键元素加 `data-testid` 属性（5-10个/页面）
  2. AI 读前端代码提取 testid，生成完整测试脚本
  3. 脚本一次性跑完，结果截图保存
  4. 用 Python 脚本 + 技能代替 MCP，省 ~3,000 tokens
- **难度**：中（需前端配合加 testid）

### P4：前置/后置处理
- **概念**：在请求发送前（前置）和响应返回后（后置）加处理层
- **前置**：敏感信息过滤（正则即可）、Prompt 增强
- **后置**：质量验收、格式规范化
- **建议**：前置/后置走用户自己的策略，用便宜的/免费的模型（如智谱 glm-4-flash）
- **难度**：高（需要额外 LLM 调用，增加延迟）

---

## 四、关键结论

### 4.1 Skills vs MCP vs Python 脚本

| 方案 | 上下文消耗 | 适用场景 |
|------|-----------|---------|
| **Skills + Python 脚本** | 0 额外 token（复用 RunCommand） | 数据操作、API 调用、文件处理、自动化测试 |
| **Skills + MCP** | ~6,000 tokens/请求 | 需要实时交互、独立进程持续运行的场景 |
| **纯 Rules** | 按内容长度 | 行为规范、约束条件 |

**结论：能用 Python 脚本实现的，不要用 MCP。** Skills 是"说明书"（教 AI 怎么用工具），底层用脚本还是 MCP 取决于场景。脚本省 token，MCP 省开发量。

### 4.2 Rules 的生效方式
- **10 个规则文件全部注入请求体**，通过 user 消息的 `<system-reminder>` 传递
- **不在 system prompt 里**（system 只有 Trae 自己的指令 + 我们的上帝指令）
- 如果想让规则优先级更高，可以考虑把关键规则也放进上帝指令

### 4.3 前置/后置策略
- 不固定走某个免费模型（不薅羊毛）
- 用户自己配置策略，用用户自己的 Key
- 前置/后置当作独立的策略路由，用户可选配

---

## 五、建议执行顺序

1. **P0 去重 Response Language Settings** — 最简单，立竿见影
2. **P3 Playwright 测试方案** — 你现在就能用，前端加 testid 后效果立竿见影
3. **P1 动态裁剪 MCP** — 中期优化
4. **P2 压缩空 tool 返回** — 中期优化
5. **P4 前置/后置处理** — 长期规划

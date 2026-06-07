---
name: nnd-trae-sync
description: 新佰人(NND) Trae 工作环境配置同步技能。确保在用户提到"同步脚手架"、"更新AI配置"、"贡献技能"、"回流配置"、"init-trae"、"contribute"、"配置中心"、"MCP服务管理"时使用此技能，仅在明确涉及 Trae 脚手架配置的同步更新或贡献回流时触发。普通 git 提交、推送、拉取操作不触发此技能。用户在 Trae IDE 内直接通过自然语言完成配置更新和贡献回流，无需切换到终端。
---

# NND Trae Sync - 工作环境配置同步

[系统级互斥锁] 严禁与其他重型技能（带流程制约的任务）并行执行。若侦测到当前上下文被要求同时运行其他技能，必须中止一切行为并要求用户降级为单线任务。

你是新佰人(NND) Trae AI 工作环境的配置同步助手。用户通过自然语言即可完成配置更新和贡献回流，无需离开 IDE 手动执行命令。

本技能的核心价值是消除"用户离开 IDE → 打开终端 → 手动拼命令"这一效率损耗，让配置同步成为对话中的一次自然交互。

## 核心工作流

本技能支持两种操作方向，通过用户意图自动判断：

### 方向一：配置更新（拉取）

用户说："同步配置"、"更新规则"、"更新 MCP"、"同步最新配置" 等。

**执行步骤**：

1. **读取上下文**：静默读取 `.trae/CONFIG_SUMMARY.md`，提取以下关键路径信息：
   - `ai-program 仓库` → ai-program 配置中心的本地绝对路径
   - `init-trae.py` → 装配脚本的本地绝对路径
   - `contribute.py` → 回流脚本的本地绝对路径

2. **路径安全检查**：确认 `init-trae.py` 路径指向的文件存在且可读。如果文件不存在，提示用户：
   > "未找到 init-trae.py，请确认 ai-program 仓库是否已克隆到本地。CONFIG_SUMMARY.md 中记录的路径为：{path}"

3. **更新 ai-program 仓库**：在执行 init-trae.py 之前，先确保 ai-program 仓库是最新版本。分步执行以下命令（兼容 PowerShell 5 / Bash / Zsh）：
   ```bash
   git -C {ai_program_path} checkout master
   git -C {ai_program_path} pull origin master
   ```
   使用 `git -C` 替代 `cd &&`，避免跨 Shell 兼容性问题。这确保使用最新的装配脚本和配置模板。

4. **执行装配命令**：使用 CONFIG_SUMMARY.md 中记录的 `init-trae.py` 绝对路径执行：
   ```bash
   python {init_trae_path} --name {name} --dept {dept} --role {role} --email {email}
   ```
   参数从 CONFIG_SUMMARY.md 中的基本信息区段提取（开发者、邮箱、部门、角色）。这里使用绝对路径而非相对路径，是因为 init-trae.py 位于 ai-program 仓库内，而当前工作目录是用户的项目目录。

   **注意**：init-trae.py 内置了本地 API Key 保护机制。同步时会自动保留用户本地已填写的敏感信息（Z_AI_API_KEY、Authorization 等），不会被脚手架模板的占位符覆盖。

5. **报告结果**：将命令输出中的关键信息（更新了哪些文件、分支同步状态、是否有保留的本地配置）整理后告知用户。

   注意：init-trae.py 内置了分支自动校验机制（`ensure_master_latest`）。如果用户上次执行贡献回流后仓库停留在 `contribute/xxx` 分支，装配时会自动切回 master 并拉取远程最新版本，贡献分支不会被删除。因此无需手动处理分支切换。

### 方向二：贡献回流（推送）

用户说："贡献技能"、"回流配置"、"分享我的优化"、"同步到配置中心" 等。

**执行步骤**：

1. **读取上下文**：同方向一，先读取 `.trae/CONFIG_SUMMARY.md`。

2. **路径安全检查**：确认 `.trae/contribute.py` 存在且可执行。

3. **确认远程最新**：contribute.py 会在 ai-program 仓库目录内执行 git 操作：
   - 内部自动执行 `git fetch origin + git reset --hard origin/master`
   - 确保贡献分支基于最新的远程 master 创建
   - 如需手动确认，可执行：`git -C {ai_program_path} fetch origin master`

4. **执行回流命令**：
   ```bash
   uv run .trae/contribute.py --type {type}
   ```
   其中 `{type}` 根据用户意图判断：
   - 用户只提技能 → `--type skills`
   - 用户只提规则 → `--type rules`
   - 用户只提 MCP → `--type mcp`
   - 用户未指定或明确说全部 → `--type all`

5. **智能差异感知（v4.0.0 新特性）**：
   contribute.py v4.0.0 采用「预扫描 + 变更清单 + 批量选择」模式：
   - **预扫描阶段**：静默检测所有配置项与脚手架的差异（通过内容 Hash 对比）
   - **变更清单展示**：分类展示 [新增] / [修改] / [无变化]
   - **批量选择**：用户一次性勾选要回流的配置项（支持 `all`/`none`/编号选择）
   - **无变化自动跳过**：内容相同的配置项自动标记为 [无变化]，不再逐个询问

   示例输出：
   ```
   ────────────────────────────────────────────────────────────
     📋 变更清单
   ────────────────────────────────────────────────────────────

   ✚ 新增 (2)
      [新增] 技能/my-new-skill (新技能目录)
      [新增] MCP/redis-cache (新 MCP 服务)

   ✎ 修改 (1)
      [修改] 规则/project-rules.md (+15/-3 行)

   ○ 无变化 (3)
      [无变化] 技能/existing-skill
      [无变化] 规则/env-python.md
      [无变化] MCP/mysql-query

   ────────────────────────────────────────────────────────────
   共检测到 3 项配置与脚手架存在差异

   请选择要回流的配置项（输入编号，多个用逗号分隔，输入 'all' 全选，'none' 全不选）：

     [1] ✚ 技能/my-new-skill (新技能目录)
     [2] ✚ MCP/redis-cache (新 MCP 服务)
     [3] ✎ 规则/project-rules.md (+15/-3 行)

     示例: 1,3,5 或 all 或 none

   ? 选择:
   ```

6. **GitLab Push Options 自动创建 MR（v4.0.0 新特性）**：
   回流完成后，脚本会自动利用 GitLab Push Options 创建 Merge Request：
   ```bash
   git push -o merge_request.create \
            -o merge_request.target=master \
            -o merge_request.title="feat(role): 回流 N 个优质配置" \
            -o merge_request.remove_source_branch \
            origin <branch_name>
   ```
   - **自动设置目标分支**：`master`
   - **自动设置 MR 标题**：与 commit message 一致
   - **自动删除源分支**：合并后自动清理
   - **降级策略**：若 Push Options 失败，自动降级为普通推送并提示手动创建 MR

7. **报告结果**：回流完成后，告知用户哪些配置已成功推送到配置中心，并展示 MR 链接。

## CONFIG_SUMMARY.md 格式参考

技能依赖此文件获取关键路径信息，这是装配脚本 init-trae.py 在初始化时自动生成的只读参考文件。其格式如下：

```markdown
# Trae 工作区配置摘要

## 基本信息
- **开发者**: 张三
- **邮箱**: zhangsan@nndrobot.com
- **部门**: ai_dept
- **角色**: backend
- **项目**: my-project
- **配置时间**: 2026-04-03 10:30:00

## 关键路径（由装配时自动探测生成）
- **ai-program 仓库**: `/Users/zhangsan/develop/ai-program`
- **init-trae.py**: `/Users/zhangsan/develop/ai-program/init-trae.py`
- **contribute.py**: `/Users/zhangsan/projects/my-project/.trae/contribute.py`

## 💬 IDE 内操作（推荐）
...（自然语言操作指引）

## ⌨️ 终端命令行（备选）
...（命令行操作指引）
```

## 安全边界

CONFIG_SUMMARY.md 中的路径信息由装配时自动探测生成，修改或删除会导致技能无法定位关键脚本，因此请保持原样。

在未确认路径存在的情况下执行命令可能引发不可预期的文件操作（如误删、误覆盖），所以务必先做路径校验。

执行命令前请向用户展示即将执行的完整命令，获得确认后再执行。用户需要知道即将运行的脚本和参数，以便在参数提取错误时有机会纠正。

如果 CONFIG_SUMMARY.md 不存在，说明当前项目尚未通过 init-trae.py 完成初始装配，提示用户先在终端手动执行一次初始化。

## 错误处理

| 场景 | 处理方式 |
|------|----------|
| CONFIG_SUMMARY.md 不存在 | 提示用户先手动执行 init-trae.py 初始装配 |
| init-trae.py 路径无效 | 提示用户 ai-program 仓库可能已移动，需重新克隆 |
| contribute.py 不存在 | 提示用户重新执行 init-trae.py 以生成分发 contribute.py |
| 命令执行失败 | 展示完整错误日志，建议用户检查环境（uv/nvm 是否已安装） |

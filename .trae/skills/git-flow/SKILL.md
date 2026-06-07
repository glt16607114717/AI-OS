---
name: common-git-flow
description: Git Flow 分支管理工具集。当用户要求"创建分支"、"切换分支"、"合并分支"、"部署"、"删除分支"、"清理分支"、"feature start"、"feature finish"、"hotfix start"、"hotfix finish"、"部署到开发/测试/灰度"、"提交"、"git提交"、"推送"、"commit"时触发。AI通过调用Python脚本完成所有Git操作，禁止直接执行git修改性命令（如checkout、merge、push等），只允许执行查看命令（如git log、git status、git branch）。
---

# Git Flow 技能文档

## 角色定位

Git Flow 自动化工具集，AI 通过调用统一脚本 `git_flow.py` 完成所有分支管理操作，包括代码提交与推送。

## ⛔ 铁律（违反任何一条都是严重事故）

### 铁律一：禁止 AI 直接执行 Git 修改性命令

必须通过脚本操作，不允许直接执行 `git checkout`、`git merge`、`git push`、`git commit`、`git add` 等。

允许的只读命令：`git log`、`git status`、`git branch -a`、`git diff`、`git show`

### 铁律二：脚本执行后必须等待结果，禁止并行操作

**脚本一旦启动，AI 必须停下来等，什么都不许做，直到脚本执行完毕。**

严禁以下行为：

- ❌ 脚本还在跑，AI 就去执行其他 git 命令
- ❌ 脚本还在等用户确认，AI 就去切换分支
- ❌ 重复执行同一个脚本
- ❌ 同时执行多个脚本
- ❌ 脚本返回后不看结果就继续下一步

**正确做法**：

1. 执行脚本 → 停下来 → 等用户确认 → 等脚本返回
2. 读取返回结果 → 分析结果 → 告知用户结果
3. 只有在用户明确指示后才执行下一步操作

### 铁律三：二次确认的脚本必须交给用户

需要二次确认的脚本（deploy、commit、feature_finish、hotfix_finish、branch_delete、branch_clean），
AI 不能用 `echo y | python` 自动输入确认，必须让用户在终端手动输入。

### 铁律四：部署/提交前必须执行记忆检查

每次调用 `git_flow.py deploy` 或 `git_flow.py commit` 之前，必须先暂停，回顾本次改动执行记忆检查：
1. 这次改动有没有踩坑？→ 有就调用 `memory-manage` 写入 daily
2. 有没有发现新的业务规则？→ 有就调用 `memory-manage` 写入 daily
3. 有没有绕弯路才找到正确做法？→ 有就调用 `memory-manage` 写入 daily

## 脚本目录

统一脚本：`.trae/skills/git-flow/scripts/git_flow.py`

## 调用方式

使用 `RunCommand` 工具执行，所有子命令的第一个参数是**项目完整路径**（如 `D:/wwwroot/rmp-api/rmp-api-a`），脚本会直接在该路径下执行 git 操作。

## 统一脚本用法

所有操作通过 `git_flow.py` 的子命令完成：

```bash
python git_flow.py <子命令> <项目路径> [参数...]
```

### 可用子命令

| 子命令 | 用途 | 参数 |
|--------|------|------|
| feature_start | 创建 feature 分支 | `<项目路径> <分支名>` |
| feature_finish | 完成 feature 分支 | `<项目路径> <分支名>` |
| hotfix_start | 创建 hotfix 分支 | `<项目路径> <分支名>` |
| hotfix_finish | 完成 hotfix 分支 | `<项目路径> <分支名>` |
| deploy | 部署到指定环境 | `<项目路径> <环境> [提交信息]` |
| commit | 提交并推送代码 | `<项目路径> <提交信息>` |
| branch_delete | 删除分支 | `<项目路径> <分支名>` |
| branch_clean | 清理过期分支 | `<项目路径> [过期天数]` |

## 详细用法

### 1. 创建 Feature 分支

**子命令**：`feature_start`

**用法**：
```powershell
python git_flow.py feature_start <项目路径> <分支名>
```

**示例**：
```powershell
python git_flow.py feature_start D:/wwwroot/rmp-api/rmp-api-a 1011255-report-export
```

**执行流程**：
1. 校验项目路径（必须是有效的 Git 仓库）
2. 检查未提交文件
3. 基于 develop 创建 `feature/<分支名>`
4. 推送新分支到远程

### 2. 完成 Feature 分支

**子命令**：`feature_finish`

**用法**：
```powershell
python git_flow.py feature_finish <项目路径> <分支名>
```

**示例**：
```powershell
python git_flow.py feature_finish D:/wwwroot/rmp-api/rmp-api-a 1011255-report-export
```

**执行流程**：
1. 校验参数和分支是否存在
2. 检查未提交文件
3. 二次确认（用户手动输入）
4. 切换到 develop 并拉取最新
5. 执行 `git flow feature finish -k <分支名>`（合并并保留原分支）
6. 推送 develop 到远程

### 3. 创建 Hotfix 分支

**子命令**：`hotfix_start`

**用法**：
```powershell
python git_flow.py hotfix_start <项目路径> <分支名>
```

**示例**：
```powershell
python git_flow.py hotfix_start D:/wwwroot/rmp-api/rmp-api-a 1009327-aborted-export
```

**执行流程**：
1. 校验项目路径
2. 检查未提交文件
3. 切换到 master 并拉取最新
4. 基于 master 创建 `hotfix/<分支名>`
5. 推送新分支到远程

### 4. 完成 Hotfix 分支

**子命令**：`hotfix_finish`

**用法**：
```powershell
python git_flow.py hotfix_finish <项目路径> <分支名>
```

**示例**：
```powershell
python git_flow.py hotfix_finish D:/wwwroot/rmp-api/rmp-api-a 1009327-aborted-export
```

**执行流程**：
1. 校验参数和分支是否存在
2. 检查未提交文件
3. 二次确认（用户手动输入）
4. 切换到 master 并拉取最新
5. 执行 `git flow hotfix finish <分支名>`（合并到 master）
6. 如有冲突需手动解决

### 5. 部署到指定环境

**子命令**：`deploy`

**用法**：
```powershell
python git_flow.py deploy <项目路径> <环境> [提交信息]
```

**示例**：
```powershell
python git_flow.py deploy D:/wwwroot/rmp-api/rmp-api-a test "--bug=1009333 --user=桂良涛  修复导出数据缺失"
python git_flow.py deploy D:/wwwroot/rmp-api/rmp-api-a dev
python git_flow.py deploy D:/wwwroot/rmp-api/rmp-api-a gray
```

**环境映射**：
- `dev` → `develop_deploy`
- `test` → `release_deploy`
- `gray` → `gray_deploy`

**分支类型约束**：
- `feature/*`、`hotfix/*` → 可部署到 dev、test、gray
- `release/*` → 只能部署到 gray

**执行流程**：
1. 校验项目路径和环境
2. 校验当前分支类型和目标环境的交叉约束
3. 检测未提交文件：有则二次确认后自动 add + commit + push
4. 二次确认部署操作（用户手动输入）
5. 切换到目标部署分支并拉取最新
6. 合并源分支到部署分支
7. 推送到远程
8. 切回原来的分支

### 6. 提交并推送代码

**子命令**：`commit`

**用法**：
```powershell
python git_flow.py commit <项目路径> <提交信息>
```

**示例**：
```powershell
python git_flow.py commit D:/wwwroot/rmp-api/rmp-api-a "--bug=1009333 --user=桂良涛  修复导出缺失"
python git_flow.py commit D:/wwwroot/rmp-api/rmp-api-a "--task=1011255 --user=桂良涛  报表导出功能新增"
```

**提交信息格式**：
- Bug 修复：`--bug=bug_id --user=桂良涛  bug名+原因+方案`
- 任务：`--task=task_id --user=桂良涛  任务名+修改内容`
- 需求：`--story=story_id --user=桂良涛  需求名+详情`

**执行流程**：
1. 校验项目路径
2. 校验当前分支类型（必须是 feature/*、hotfix/* 或 release/*）
3. 检查是否存在未提交文件
4. 展示变更文件列表
5. 二次确认（用户手动输入）
6. 执行 `git add -A`
7. 执行 `git commit -m <提交信息>`
8. 执行 `git push`

### 7. 删除分支

**子命令**：`branch_delete`

**用法**：
```powershell
python git_flow.py branch_delete <项目路径> <分支名>
```

**示例**：
```powershell
python git_flow.py branch_delete D:/wwwroot/rmp-api/rmp-api-a feature/1011255-report-export
```

**执行流程**：
1. 校验项目路径
2. 校验分支类型（只能删除 feature/* 或 hotfix/* 分支）
3. 检查未提交文件
4. 二次确认（用户手动输入）
5. 切换到 develop
6. 删除本地分支
7. 删除远程分支

### 8. 清理过期分支

**子命令**：`branch_clean`

**用法**：
```powershell
python git_flow.py branch_clean <项目路径> [过期天数]
```

**示例**：
```powershell
python git_flow.py branch_clean D:/wwwroot/rmp-api/rmp-api-a          # 默认60天
python git_flow.py branch_clean D:/wwwroot/rmp-api/rmp-api-a 30        # 30天无提交视为过期
```

**执行流程**：
1. 校验项目路径
2. 检查未提交文件
3. 获取所有本地分支及其最后提交时间
4. 筛选出超过指定天数无提交的过期分支
5. 展示过期分支列表
6. 二次确认（用户手动输入）
7. 逐个删除过期分支（本地 + 远程）

**保护分支**：master、main、develop、staging、production（永远不会参与清理）

**可清理前缀**：feature/、hotfix/

## 注意事项

1. **所有子命令的第一个参数必须是项目的完整绝对路径**（如 `D:/wwwroot/rmp-api/rmp-api-a`）
2. **分支名不含前缀**（如 `1011255-report-export`），脚本会自动拼接 `feature/` 或 `hotfix/` 前缀
3. **二次确认的脚本必须让用户在终端手动输入，AI 不得自动确认**

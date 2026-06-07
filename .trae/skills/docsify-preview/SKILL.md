---
name: docsify-preview
description: |
  Docsify 文档展示自动化（诊断→修复→启动→分享）。触发词：docsify、文档预览、文档分享、docsify打不开、侧边栏不显示、初始化docsify、搭建文档站。负责诊断修复、配置优化、导航生成、服务启动、分享链接生成。必须完整执行所有步骤。
---

# Docsify 文档展示自动化方案

你是 Docsify 文档站运维助手。将"诊断 → 修复 → 启动 → 分享"流程一体化，一次对话即可获得可直接访问和分享的文档站。通用技能，可在任意项目中使用。

**开始时宣布：** "我正在使用 Docsify 文档展示技能，将执行完整的诊断 → 修复 → 启动 → 分享流程。"

---

## 核心约束

1. **文档根目录固定为 `docs/`** — 所有 Docsify 配置、资源、内容都在 `docs/` 中
2. **资源本地化** — JS/CSS 依赖引用 `docs/assets/` 本地文件，禁止 CDN
3. **路径规范** — `relativePath: true` 下，`_sidebar.md`/`_navbar.md` 链接必须以 `/` 开头
4. **不触碰业务文档** — 只处理 Docsify 框架配置，不修改 `docs/` 下除 `README.md` 以外的业务 Markdown
5. **项目无关性** — 不硬编码项目特定内容，所有内容通过动态检测生成

---

## 参考文件（按需加载）

| 文件 | 内容 | 何时读取 |
|------|------|---------|
| `references/config-reference.md` | CDN 映射、Docsify 配置、资源下载清单 | Step 2 基础设施修复 |
| `references/naming-conventions.md` | 目录名→显示名推导规则、缩写词表 | Step 2 索引 + Step 3 导航 |
| `references/init-templates.md` | index.html 模板 + 站点名称解析规则 | Step 1 检测到 index.html 不存在 |
| `references/report-templates.md` | 所有步骤的输出报告模板 | 各步骤输出报告时 |
| `references/readme-rules.md` | README 生成规则、链接格式、URL编码、增量同步 | Step 2 索引生成 |
| `references/error-handling.md` | 异常场景与自动恢复策略 | 遇到错误时 |

---

## 强制工作流

严格按步骤顺序执行，禁止跳过。Step 1 构建全量快照后，后续步骤直接从快照读取，不再重复查找。

### Step 1: 全量快照采集

一次性并行采集所有后续步骤所需信息（index.html、assets、sidebar/navbar、目录结构、README 存在性、端口状态等），共 10 个采集项。
**站点名称解析**优先级：`docs/.nav-config.json` 的 `siteName` → `package.json` 的 `name`（去 scope）→ 目录名兜底，然后 kebab-case 转 Title Case（缩写词保持大写）。
若 `docs/index.html` 不存在，读取 `references/init-templates.md` 自动创建骨架。
**输出：** `<env_snapshot>` 快照报告（格式见 `references/report-templates.md`）。

---

### Step 2: 诊断与修复

基于快照，按优先级执行：
- **P1 基础设施**：assets 目录/文件补齐、CDN→本地替换、配置项补充、站点名称修正（读取 `references/config-reference.md`）
- **P2 导航修复**：断裂链接移除、孤立文档纳入
- **P3 索引生成**：递归扫描所有子目录，缺失 README.md 的按规则生成（读取 `references/readme-rules.md`）
已存在 README.md 的目录执行增量同步：新增条目追加、已删条目移除、格式修正，其他内容保持不变。
**输出：** `<diag_fix>` 诊断修复报告（格式见 `references/report-templates.md`）。

---

### Step 3: 导航动态生成与同步

基于快照目录结构，动态生成或增量同步 `_sidebar.md` 和 `_navbar.md`。

| 目录类型 | 判断条件 | 处理方式 |
|---------|---------|---------|
| 静态原型站 | 有 `index.html`，无 `README.md` 和 `_sidebar.md` | 外部链接（`<a>` + 绝对路径） |
| Docsify 文档目录 | 有 `README.md` 或 `_sidebar.md` 或 `.md` 文件 | 正常路由 |
| 静态资源目录 | 目录名匹配 assets/static/images 等 | 侧边栏忽略 |

显示名推导读取 `references/naming-conventions.md`。首次初始化：从零生成，一级目录为分组标题（`**加粗**`），二级目录为条目，按字母序。增量同步：新增追加、已删移除、已有保持用户排序。
**输出：** `<nav_sync>` 导航报告（格式见 `references/report-templates.md`）。

---

### Step 4: 启动服务与分享

基于快照端口状态：空闲→直接用 3000，占用→终止旧进程或递增端口。

```bash
cd <项目根目录> && npx docsify-cli serve docs --port 3000
```

非阻塞启动，等待 2-3 秒确认成功后获取局域网 IP。
**输出：** `<service_share>` 启动报告 + 汇总报告（格式见 `references/report-templates.md`）。

---

## 注意事项

- 修改范围仅限：`docs/index.html`、`_sidebar.md`、`_navbar.md`、`docs/assets/` 资源、子目录 `README.md`
- 服务启动后后台运行，关闭终端或手动停止时终止
- 导航内容由目录结构动态生成，自定义显示名用 `docs/.nav-config.json` 配置

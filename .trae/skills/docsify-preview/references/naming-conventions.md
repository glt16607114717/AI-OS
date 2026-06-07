# 目录命名规则与显示名推导

本文件定义从目录名推导显示名称的通用规则，用于 README 自动生成和导航栏/侧边栏的条目命名。

---

## 核心推导规则

目录名（kebab-case）→ 显示名的转换遵循以下步骤，按优先级从高到低：

### 优先级 1：项目级映射覆盖

如果 `docs/.nav-config.json` 存在，优先从中读取映射：

```json
{
  "siteName": "自定义站点名",
  "dirAliases": {
    "legacy-core": "传统业务线",
    "ai-innovation": "AI 业务线"
  }
}
```

`dirAliases` 中的键为 `docs/` 下的相对目录路径（不含 `docs/` 前缀），值为该目录的显示名。

### 优先级 2：缩写词识别

目录名中的片段如果匹配以下缩写词表，保持大写形式：

| 缩写 | 全称 | 示例 |
|------|------|------|
| ai | Artificial Intelligence | `ai-innovation` → `AI Innovation` |
| api | Application Programming Interface | `api-docs` → `API Docs` |
| cli | Command Line Interface | `cli-tools` → `CLI Tools` |
| crm | Customer Relationship Management | `crm-system` → `CRM System` |
| devops | Development Operations | `devops-pipeline` → `DevOps Pipeline` |
| erp | Enterprise Resource Planning | `erp-module` → `ERP Module` |
| faq | Frequently Asked Questions | `faq-list` → `FAQ List` |
| iot | Internet of Things | `iot-gateway` → `IoT Gateway` |
| pmo | Project Management Office | `pmo-workspace` → `PMO Workspace` |
| sdk | Software Development Kit | `sdk-reference` → `SDK Reference` |
| sre | Site Reliability Engineering | `sre-runbook` → `SRE Runbook` |
| ui | User Interface | `ui-components` → `UI Components` |
| ux | User Experience | `ux-research` → `UX Research` |

### 优先级 3：通用 Title Case 转换

不匹配上述规则的片段，执行标准 Title Case 转换：
- 中划线 `-` 替换为空格
- 每个单词首字母大写
- 示例：`cross-metrics` → `Cross Metrics`，`risk-assessments` → `Risk Assessments`

---

## 转换示例

| 目录名 | 推导结果 | 应用规则 |
|--------|---------|---------|
| `ai-innovation` | AI Innovation | 缩写词 |
| `legacy-core` | Legacy Core | Title Case |
| `cross-metrics` | Cross Metrics | Title Case |
| `cross-dept-meetings` | Cross Dept Meetings | Title Case |
| `pmo-workspace` | PMO Workspace | 缩写词 |
| `sdk-reference` | SDK Reference | 缩写词 |
| `weekly-progress` | Weekly Progress | Title Case |

# ADR 标准模板

本文档提供 ADR (Architecture Decision Record) 的标准模板结构。

---

## 模板结构

```markdown
# ADR-{编号}: {标题}

## 状态 (Status)

{Proposed | Accepted | Deprecated | Superseded by ADR-XXX}

## 背景上下文 (Context)

{客观描述面临的技术挑战或业务背景}

## 决策 (Decision)

{明确写出最终选择了什么方案,以及选择它的核心依据}

## 替代方案 (Alternatives Considered)

### 方案 A: {方案名称}
**描述**：{方案简介}
**优点**：{方案优势}
**缺点**：{方案劣势}
**结论**：{为什么未被采纳}

### 方案 B: {方案名称}
{同上结构}

## 后果 (Consequences)
### 正面收益
- {收益1}
- {收益2}

### 负面妥协
- {妥协1}
- {妥协2}

### 风险与缓解措施
| 风险 | 影响 | 缓解措施 |
|-----|------|---------|
| {风险1} | {高/中/低} | {缓解方案} |

## 参考 (References)
- {参考资料链接}

---
**决策人**：{{USER_NAME}}
**日期**：{YYYY-MM-DD}

---
affected_files:
  - README.md           # 如需更新项目文档
  - docs/adr/README.md  # 必须更新 ADR 索引
```

---

## 状态流转图
```
Proposed → Accepted → Deprecated
                ↓
            Superseded by ADR-XXX
```

## 状态说明
| 状态 | 说明 | 后续动作 |
|-----|------|---------|
| Proposed | 提议中 | 等待团队评审和讨论 |
| Accepted | 已接受 | 开始实施,更新相关文档 |
| Deprecated | 已废弃 | 停止新项目使用,逐步迁移 |
| Superseded | 被取代 | 注明替代的 ADR 编号,引导查阅新 ADR |

---

## 命名规范
- 格式: `YYYY-MM-DD-kebab-case-title.md`
- 示例: `2025-03-27-unified-uv-python-env.md`
- 示例: `2025-03-15-microservice-architecture.md`

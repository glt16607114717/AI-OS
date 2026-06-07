---
alwaysApply: true
---
# PHP后端需求开发流程

## 工作流程

```mermaid
flowchart TD
    A[开始] --> TA[技能:tapd<br/>获取任务详情]
    TA --> TC["技能:git-flow<br/>创建开发分支+任务流水"]
    TC --> C[方案论证]
    
    C --> D{用户确认<br/>方案?}
    D -->|否| C
    D -->|是| G[编写迁移文件+测试用例]
    
    G --> H[技能:dev-doc<br/>编写开发文档]
    H --> HB{用户确认<br/>开发文档?}
    HB -->|否| H
    HB -->|是| J["php think migrate:run<br/>AI自动执行"]
    
    J --> I[编码实现]
    I --> RV["技能:dev-reviewer<br/>/review 验收"]
    RV --> RA{验收通过?}
    RA -->|否| I
    RA -->|是| M[等待用户下达Git提交指令]
    
    M --> N["技能:git-flow<br/>提交推送 或 部署"]
    N --> O{目标环境功能<br/>是否生效?}
    O -->|是| PB["技能:apipost<br/>同步接口文档"]
    O -->|否| Q["技能:ssh<br/>SSH只读检查"]
    PB --> FIN[完成]
    Q --> FIN[完成]

    style A fill:#e1f5fe
    style D fill:#ff9800
    style HB fill:#ff9800
    style M fill:#ff9800
    style RV fill:#7c3aed,color:#fff
    style FIN fill:#059669,color:#fff
    style Q fill:#e74c3c,color:#fff
```

**门控规则**：橙色节点必须用户明确同意后才能继续。**严禁跳过门控节点自动推进。**

## 触发关键词

开始任务、做任务、开发任务、开始开发、任务开发、提供任务ID

## 任务流水

每个任务在 `ai_cache/task-log/` 下维护子目录，用于跨会话恢复上下文。命名与开发文档一致：`{YYYYMMDD}-{需求中文名}-{分支名}`

```
ai_cache/task-log/
└── 20260506-异常报告优化-1011489-abnormal-report-optimize/
    ├── flow.md              ← 项目流水
    └── test-cases.json      ← 接口测试用例
```

**flow\.md 格式**：

```markdown
# {需求名称}

## 基本信息

| 字段 | 值 |
|------|-----|
| 任务ID | TAPD任务ID |
| 任务名称 | 任务名称 |
| 需求ID | 关联需求ID |
| 需求名称 | 需求名称 |
| 开发分支 | feature/xxx |
| 工作区 | rmp-api-{a/b/c} |
| 本地域名 | rmp-api-{a/b/c}.me |
| 所属项目 | rmp-api |
| 开发文档 | docs/{命名}.md |
| 测试用例 | task-log/{命名}/test-cases.json |
| 状态 | 开发中 / 测试中 / 已完成 |

## 流水记录

### YYYY-MM-DD

| 时间 | 步骤 | 详情 |
|------|------|------|
| - | 创建分支 | feature/xxx |
| - | 方案论证 | 改动概述 |
| - | 编写开发文档 | 涉及的Story |
| - | 编码完成 | 涉及文件清单 |
| - | 验收 | 通过/失败数 |
| - | 部署到dev | 合并结果 |
```

- **创建时机**：获取任务详情后、创建分支时同步创建目录和 flow\.md
- **更新时机**：每个关键节点完成时在 flow\.md 追加流水记录
- **恢复上下文**：新会话说"继续做 1011489" → 读取对应 flow\.md

## 补充约束

- **任务开发**：先创建分支再进入公共流程，分支名从TAPD任务ID中提取7位短ID：`feature/{短taskID}-{描述}`
- **方案论证**：必须先查看前端页面精准定位接口地址（先 `git pull`），再分析后端代码和数据库
- **调试思维**：多打断点看代码执行是否符合预期，不要单纯靠推理

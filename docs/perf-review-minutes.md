# AI-OS 绩效考核功能 - 讨论纪要

> 日期：2026-06-21
> 状态：方案设计阶段，待决策

---

## 一、核心理念

**事件驱动计分 + 月度汇总**

- 不是月底一次性喂大量数据给 AI，而是**每次事件发生时实时计分**
- 月底只是简单求和，不涉及 LLM 处理大数据量
- 全员使用 AI 办公 → AI 日志 = 全量工作记录

---

## 二、计分模型（100 分基准制）

### 2.1 等级划分

| 等级 | 分数 | 含义 |
|------|------|------|
| S | ≥115 | 卓越 |
| A | 105-114 | 优秀 |
| B | 95-104 | 良好（达标） |
| C | 85-94 | 需改进 |
| D | <85 | 不及格 |

### 2.2 计分维度

| 维度 | 权重 | 说明 |
|------|------|------|
| Bug 解决时效 | 30% | 按时/提前解决加分，打回扣分 |
| 任务完成时效 | 30% | 提前完成加分，超期扣分 |
| Bug 数量控制 | 20% | 当月 Bug 数量过多扣分 |
| Token 使用度 | 20% | 低于及格线扣分，不奖励刷量 |

### 2.3 具体规则

**Bug 解决时效**
- P0：24h 内解决 +3，>72h -3/天
- P1：3 天内解决 +2，>7 天 -1/天
- 打回（reopened）：-5/次

**任务完成时效**
- 提前完成：+1~+5
- 超期 1-3 天：-1~-3
- 超期 >3 天：-5
- 任务被打回：-3/次

**Token 使用度**
- Token = 0：-20
- Token < 及格线：-10
- 及格线 ≤ Token < 上限：0（正常）
- Token > 上限：0（不奖励，防刷量）

---

## 三、TAPD 接口验证结果

**结论：✅ 技术上全部可行**

| 数据需求 | 接口 | 验证 |
|---------|------|------|
| Bug 列表（含时效字段） | `GET /bugs` | ✅ |
| Bug 打回次数 | `GET /bug_changes` | ✅ |
| 任务列表（含预估工时） | `GET /stories` | ✅ |
| 需求变更历史 | `GET /story_changes` | ✅ |
| 项目成员 | `GET /workspaces/users` | ✅ |

**项目信息**
- 项目名：RMP管理平台
- workspace_id：66680814
- Bug 总数：6988 条

---

## 四、数据表设计

```sql
-- 计分事件记录（事件驱动写入）
CREATE TABLE perf_score_event (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  user_id INT NOT NULL,
  event_type ENUM('bug_resolved','bug_reopened','task_done','task_rejected','no_bug_month','low_token'),
  event_date DATE NOT NULL,
  related_id VARCHAR(50),
  related_title VARCHAR(500),
  score_delta INT NOT NULL,
  reason VARCHAR(500),
  raw_data JSON,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 月度绩效汇总（每月 1 号生成）
CREATE TABLE perf_monthly (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  user_id INT NOT NULL,
  month DATE NOT NULL,
  total_score DECIMAL(5,1) NOT NULL,
  grade CHAR(2),
  bug_score INT DEFAULT 0,
  task_score INT DEFAULT 0,
  quantity_score INT DEFAULT 0,
  token_score INT DEFAULT 0,
  status ENUM('draft','published') DEFAULT 'draft',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_user_month (user_id, month)
);

-- 员工映射（AI-OS 用户 ↔ TAPD 用户名）
CREATE TABLE perf_user_map (
  id INT AUTO_INCREMENT PRIMARY KEY,
  sys_user_id INT NOT NULL,
  tapd_username VARCHAR(100) NOT NULL,
 岗位_type ENUM('dev','product','admin') DEFAULT 'dev',
  token_quota INT DEFAULT 500000
);
```

---

## 五、计分时机

| 类型 | 触发 | 数据来源 |
|------|------|---------|
| Bug 关闭 | 事件驱动 | TAPD |
| Bug 打回 | 事件驱动 | TAPD |
| 任务关闭 | 事件驱动 | TAPD |
| Bug 数量统计 | 每天凌晨 | 聚合 |
| Token 用量检查 | 每天凌晨 | llm_stats |
| 月度结算 | 每月 1 号 | 求和 |

---

## 六、风险与对策

| 风险 | 对策 |
|------|------|
| begin/due 未填写 | 改用 effort 完成趋势判断 |
| 跨多 TAPD 项目 | 初期只统计 RMP 一个项目 |
| API 限流 | 控制调用频率，每日增量同步 |
| 员工刷 Token 刷分 | Token 上限封顶，不加分 |
| 工期故意高估 | 靠 lead 填报工时合理性约束 |

---

## 七、待决策事项

1. **岗位类型划分**：哪些人是开发岗、产品岗、行政岗？
2. **Token 及格线**：开发岗 50 万/月 是否合理？行政岗设多少？
3. **Bug 优先级对应**：用 `priority` 还是 `severity` 区分 P0/P1/P2？
4. **实施节奏**：影子模式跑 1 个月，还是直接上线？

---

## 八、实施计划

```
Phase 1（1-2 周）：数据层
  → 建表 + TAPD 数据拉取脚本 + 基础聚合 API

Phase 2（1 周）：影子模式
  → 内部跑 1 个月，看分数分布，调整权重

Phase 3（1 周）：前端
  → 管理后台 + 员工视图 + 月度报告导出

Phase 4（1 个月）：试运行
  → 部分团队开放，收集反馈，优化标准
```

---

## 九、文档位置

详细方案：[perf-review-design.md](perf-review-design.md)

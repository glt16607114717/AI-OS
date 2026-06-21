# AI-OS 绩效考核功能方案

> 版本：v0.1 | 日期：2026-06-21
> 状态：**技术验证完成，方案待确认**

---

## 一、TAPD 接口可行性验证

### 验证结论：✅ 全部通过

| 数据需求 | 接口 | 验证结果 | 备注 |
|---------|------|---------|------|
| Bug 列表 + 时效字段 | `GET /bugs` | ✅ | `created/resolved/closed/current_owner` 均可获取 |
| Bug 变更历史 | `GET /bug_changes` | ✅ | 完整状态流转记录，含 author + created |
| 任务/需求列表 | `GET /stories` | ✅ | `effort/begin/due/owner` 均可获取 |
| 需求变更历史 | `GET /story_changes` | ✅ | 可追踪状态回退 |
| reopen 次数 | `GET /bug_changes` | ✅ | 统计 `new_value=reopened` 次数即可 |
| 当前项目 | RMP管理平台（66680814） | ✅ | 共 6988 条 Bug |

### 关键发现

**1. Bug 解决时效**
```
created: "2022-09-19 11:41:09"
resolved: "2022-09-19 17:13:14"  ← 直接可用
closed: "2022-09-20 08:53:53"
```
- `created → resolved` 即为解决时效，无需额外计算

**2. Bug 打回检测**
```
变更记录：
  old_value: resolved → new_value: reopened  ← 打回事件
  author: 徐洁
  created: 2026-06-12 18:15:39
```
- 统计 `new_value=reopened` 次数即可
- 该 Bug 被打回 2 次

**3. 任务预估工时**
```
effort: "10"           ← 预估工时（人天？）
effort_completed: "3"  ← 已完成工时
remain: "7"             ← 剩余工时
begin: null             ← ⚠️ 实际未填写
due: null               ← ⚠️ 实际未填写
```
**问题**：本项目中 `begin/due` 基本为 null，但 `effort` 有值。说明项目用的是"故事点"而非"日期"来管理工期。

**4. 需求变更历史**
```
field: "status"
old_value: "developing"
new_value: "planning"  ← 被退回
```
- 可追踪需求被打回/重开的完整路径

---

## 二、计分模型

### 2.1 总体设计

**100 分基准制**，封顶 130 分，扣分无下限（但实际最低约 40-50 分）。

| 等级 | 分数 | 含义 |
|------|------|------|
| S | ≥115 | 卓越 |
| A | 105-114 | 优秀 |
| B | 95-104 | 良好（达标） |
| C | 85-94 | 需改进 |
| D | <85 | 不及格 |

### 2.2 计分维度

#### 维度 1：Bug 解决时效（权重 30%）

| 场景 | 计分 | 说明 |
|------|------|------|
| P0 Bug：24h 内解决 | +3/条 | 紧急响应 |
| P0 Bug：24-72h 解决 | 0 | 正常 |
| P0 Bug：>72h | -3/天 | 严重超时 |
| P1 Bug：3 天内解决 | +2/条 | 及时响应 |
| P1 Bug：3-7 天解决 | 0 | 正常 |
| P1 Bug：>7 天 | -1/天 | 超期 |
| Bug 被打回（reopened） | -5/次 | 质量不达标 |

**优先级系数**：P0×1.5、P1×1.2、P2×1.0、P3×0.5

#### 维度 2：任务完成时效（权重 30%）

| 场景 | 计分 | 说明 |
|------|------|------|
| 提前完成（按 effort 完成度） | +1~+5 | 视提前比例 |
| 按时完成 | 0 | 基准 |
| 超期 1-3 天 | -1~-3 | 每超 1 天 -1 |
| 超期 >3 天 | -5 | 断崖扣分 |
| 任务被打回/重开 | -3/次 | 需重新开发 |

**注**：若 `begin/due` 为 null，改用 `effort_completed/remain` 趋势判断。

#### 维度 3：Bug 数量控制（权重 20%）

| 场景 | 计分 | 说明 |
|------|------|------|
| 当月无新 Bug | +5 | 代码质量好 |
| Bug 数量 ≤ 3 | 0 | 正常范围 |
| Bug 数量 4-8 | -2 | 偏多 |
| Bug 数量 > 8 | -5 | 质量堪忧 |

#### 维度 4：Token 使用度（权重 20%）

| 场景 | 计分 | 说明 |
|------|------|------|
| Token = 0 | -20 | 完全没用 AI |
| Token < 及格线 | -10 | 未跟上时代 |
| 及格线 ≤ Token < 上限 | 0 | 正常 |
| Token > 上限 | 0 | 不加分，防刷量 |

**及格线设定**：
- 开发岗：50 万 Token/月
- 产品岗：20 万 Token/月
- 行政岗：5 万 Token/月
- 初期取全员中位数 30% 作为动态及格线

---

## 三、计分时机

| 类型 | 触发 | 数据来源 |
|------|------|---------|
| **事件驱动** | Bug 关闭瞬间 | TAPD `bug_changes` |
| **事件驱动** | 需求/任务关闭瞬间 | TAPD `story_changes` |
| **日统计** | 每天凌晨 2:00 | 聚合昨日 Bug 数量、Token 量 |
| **月度结算** | 每月 1 号凌晨 | 汇总所有加减分 |

**月底不喂数据给 AI**，只做简单的加权求和。

---

## 四、数据表设计

```sql
-- 1. 计分事件记录（每次事件驱动时写入）
CREATE TABLE perf_score_event (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  user_id INT NOT NULL,
  event_type ENUM('bug_resolved', 'bug_reopened', 'task_done', 'task_rejected', 'no_bug_month', 'low_token') NOT NULL,
  event_date DATE NOT NULL,
  related_id VARCHAR(50),           -- TAPD Bug/Story ID
  related_title VARCHAR(500),       -- Bug/Story 标题
  score_delta INT NOT NULL,          -- 加减分值
  reason VARCHAR(500),               -- 扣/加分原因
  raw_data JSON,                     -- TAPD 原始数据快照
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_month (user_id, event_date)
);

-- 2. 月度绩效汇总（每月 1 号生成）
CREATE TABLE perf_monthly (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  user_id INT NOT NULL,
  month DATE NOT NULL,               -- 月份，如 2026-06
  total_score DECIMAL(5,1) NOT NULL,
  grade CHAR(2),                     -- S/A/B/C/D
  bug_score INT DEFAULT 0,
  task_score INT DEFAULT 0,
  quantity_score INT DEFAULT 0,
  token_score INT DEFAULT 0,
  status ENUM('draft', 'published') DEFAULT 'draft',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  published_at DATETIME,
  INDEX idx_user_month (user_id, month),
  UNIQUE KEY uk_user_month (user_id, month)
);

-- 3. 员工 TAPD 映射（AI-OS 用户 ↔ TAPD 用户名）
CREATE TABLE perf_user_map (
  id INT AUTO_INCREMENT PRIMARY KEY,
  sys_user_id INT NOT NULL,
  tapd_username VARCHAR(100) NOT NULL,
 岗位_type ENUM('dev', 'product', 'admin') DEFAULT 'dev',
  token_quota INT DEFAULT 500000,    -- 月度 Token 及格线
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_sys_user (sys_user_id),
  UNIQUE KEY uk_tapd_user (tapd_username)
);
```

---

## 五、TAPD 数据拉取策略

### 5.1 每日增量同步（事件驱动）

```python
# 伪代码
def sync_daily():
    yesterday = date.today() - timedelta(days=1)

    # 1. 拉取昨日关闭的 Bug（含 resolved 时间）
    bugs = tapd.get_bugs({
        'workspace_id': 66680814,
        'status': 'closed|resolved',
        'closed': f'>{yesterday}',
        'limit': 200
    })

    for bug in bugs:
        # 计算 resolved - created 时效
        elapsed = bug.resolved - bug.created
        if elapsed < threshold:
            score = +3  # 及时解决
        else:
            score = calc_penalty(elapsed)

        write_event(user=bug.current_owner, event_type='bug_resolved',
                    score=score, related_id=bug.id)

    # 2. 拉取昨日 reopen 的 Bug
    changes = tapd.get_bug_changes({
        'workspace_id': 66680814,
        'created': f'>{yesterday}',
        'field': 'status'
    })
    for change in changes:
        if change.new_value == 'reopened':
            write_event(user=change.author, event_type='bug_reopened',
                        score=-5, related_id=change.bug_id)
```

### 5.2 月度 Token 统计

```sql
-- 每月 1 号从 llm_stats 表聚合
SELECT user_id, SUM(input_tokens + output_tokens) as total_tokens
FROM llm_stats
WHERE DATE(created_at) BETWEEN '{month_start}' AND '{month_end}'
GROUP BY user_id
```

---

## 六、待决策事项

### 6.1 待确认

1. **岗位类型划分**：哪些人是开发岗、哪些是产品岗？
   → 需要在 `perf_user_map` 中手动配置

2. **Token 及格线**：开发岗 50 万/月 是否合理？
   → 建议首月用中位数 30% 作为动态值，之后根据数据调整

3. **Bug 优先级**：当前项目中 P0/P1/P2/P3 的严重程度字段是 `severity` 还是 `priority_label`？
   → 从实测数据看：`priority: "低"`, `severity: "normal"`
   → 需要和团队确认优先级系数对应关系

4. **预估工时（effort）单位**：人天？还是故事点？
   → 当前项目数据 `effort: "10"`、`effort_completed: "3"`
   → 需要确认是 10 人天还是 10 点

### 6.2 风险

1. **begin/due 未填写**：大部分需求没有设置截止日期
   → 改用 effort 完成趋势判断，或要求 lead 填写 begin/due

2. **变更历史查询量大**：6988 条 Bug × 每次查变更历史 = API 调用量大
   → 只查活跃用户的最近 N 条，限定时间范围

3. **跨项目**：如果员工参与多个 TAPD 项目？
   → 初期只统计 RMP管理平台 项目，后续扩展

---

## 七、实施计划

### Phase 1：数据层（1-2 周）
- [ ] 创建 `perf_score_event`、`perf_monthly`、`perf_user_map` 三张表
- [ ] 写 TAPD 数据拉取脚本（每日 cron）
- [ ] 对接 AI-OS llm_stats 表获取 Token 统计
- [ ] 基础聚合查询 API

### Phase 2：影子模式（1 周）
- [ ] 内部跑 1 个月，看分数分布是否合理
- [ ] 调整维度权重
- [ ] 确认岗位类型划分

### Phase 3：前端展示（1 周）
- [ ] 管理后台：员工绩效列表 + 明细
- [ ] 员工视图：自己的得分 + 明细
- [ ] 月度报告导出

### Phase 4：试运行（1 个月）
- [ ] 对部分团队开放
- [ ] 收集反馈
- [ ] 优化评分标准

---

## 八、优先级建议

1. **先把 Bug 时效和 Token 用量跑通**——数据来源稳定，逻辑简单
2. **任务时效暂缓**——因为 begin/due 未填写，需要先推动团队填写
3. **Token 及格线动态化**——首月用影子模式跑，看实际分布再定

---

## 附录：TAPD 接口实测数据

### Bug 列表返回字段
```json
{
  "id": "1166680814001000042",
  "title": "机器人信息显示异常",
  "status": "closed",
  "priority": "低",
  "severity": "normal",
  "current_owner": "翟雨瑞;",
  "reporter": "翟雨瑞",
  "created": "2022-09-19 11:41:09",
  "resolved": "2022-09-19 17:13:14",
  "closed": "2022-09-20 08:53:53",
  "resolution": "fixed"
}
```

### Bug 变更历史返回字段
```json
{
  "field": "status",
  "old_value": "resolved",
  "new_value": "reopened",
  "author": "徐洁",
  "created": "2026-06-12 18:15:39"
}
```

### 项目信息
- 项目名：RMP管理平台
- workspace_id：66680814
- Bug 总数：6988 条

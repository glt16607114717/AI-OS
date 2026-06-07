---
name: api-test
description: 接口测试用例管理和执行编排。当用户要求"写测试用例"、"接口测试用例"、"生成测试用例"、"跑测试用例"、"自测接口"时触发。负责测试用例的编写、存储、执行编排和结果汇总，通过调用 api-debug 技能完成具体的接口调用和验证。
---

# 接口测试用例管理和执行编排

## 角色定位

测试用例管理器和执行编排者，负责：
1. **测试用例编写**：根据接口设计编写完整的测试用例（正向、反向、边界、异常）
2. **用例存储**：将用例保存为结构化 JSON 文件
3. **执行编排**：逐条调用 `api-debug` 技能执行测试
4. **结果汇总**：对比预期与实际结果，输出测试报告

**必须全部通过才算开发完成。**

## 适用范围

所有开发工作都需要编写测试用例：需求开发、任务开发、Bug 修复。

## 存储位置

测试用例文件保存在 `D:\wwwroot\ai\ai_cache\task-log\{统一命名}\` 目录下的 `test-cases.json`：

```
D:\wwwroot\ai\ai_cache\task-log\
└── 20260506-异常报告优化-1011489-abnormal-report-optimize\
    ├── flow.md              ← 项目流水
    └── test-cases.json      ← 接口测试用例
```

**目录命名规则**：`{YYYYMMDD}-{需求中文名}-{分支名}`，与开发文档、项目流水保持统一命名，可追溯。

## 用例文件格式

```json
{
  "branch": "1011255-report-export",
  "created_at": "2026-04-29",
  "test_cases": [
    {
      "id": 1,
      "name": "创建报表导出任务 — 正向场景",
      "url": "/api/admin/report/export",
      "method": "POST",
      "params": {
        "report_type": "monthly",
        "company_id": 2,
        "month": "2026-04"
      },
      "expected": {
        "status": 200,
        "response": "data.task_id 不为空",
        "database": "r_report_task 表新增一条记录，status=0"
      }
    },
    {
      "id": 2,
      "name": "创建报表导出任务 — 参数校验",
      "url": "/api/admin/report/export",
      "method": "POST",
      "params": {
        "report_type": "",
        "company_id": 2
      },
      "expected": {
        "status": "error",
        "response": "返回参数校验错误"
      }
    }
  ]
}
```

### 字段说明

| 字段 | 说明 |
|------|------|
| branch | 分支名 |
| created_at | 创建日期 |
| test_cases | 测试用例数组 |
| test_cases[].id | 用例序号 |
| test_cases[].name | 用例名称（接口名 + 场景） |
| test_cases[].url | 接口路径 |
| test_cases[].method | 请求方式 |
| test_cases[].params | 请求参数 |
| test_cases[].expected | 预期结果 |
| test_cases[].expected.status | 预期HTTP状态或业务状态 |
| test_cases[].expected.response | 预期响应内容描述 |
| test_cases[].expected.database | 预期数据库变化（可选） |

## 用例编写要求

### 覆盖范围

每个接口至少覆盖：

| 场景类型 | 说明 | 必须覆盖 |
|----------|------|----------|
| 正向场景 | 正常参数，预期成功 | ✅ |
| 参数校验 | 缺少必填、类型错误、格式错误 | ✅ |
| 状态流转 | 正常流转 + 非法状态流转 | ✅（涉及状态变更的接口） |
| 边界值 | 空值、最大值、最小值 | ✅（涉及数据修改的接口） |
| 重复提交 | 重复提交相同请求 | ✅（涉及写入的接口） |

### 每条用例必须明确

1. **请求参数**：完整写出每个字段的值
2. **预期接口返回**：状态码 + 关键响应字段
3. **预期数据库变化**：哪个表、新增还是修改、关键字段值

### 参数名校验（铁律）

**编写测试用例时，必须先查阅目标接口的 Controller 源码，确认实际参数名。**

参数名以 Controller 中 `$this->apiParams([...])` 声明的为准，禁止凭猜测命名。例如：
- Controller 声明 `abnormal_report_id`，测试用例必须写 `abnormal_report_id`，不能写 `id`
- Controller 声明 `company_id`，测试用例必须写 `company_id`，不能写 `companyId`

**用例 ID 规则**：每个用例的 `id` 必须唯一，禁止重复。

## 执行流程

### 编写用例（开发文档确认后）

1. 开发文档审核确认后，根据开发设计中的接口清单编写测试用例
2. 将用例写入 `D:\wwwroot\ai\ai_cache\task-log\{统一命名}\test-cases.json`
3. 确保每个接口的用例覆盖全面

### 执行用例（开发完成后）

1. 读取用例文件
2. 逐条调用 `api-debug` 技能执行
3. 对比实际结果与预期结果
4. 全部通过 → 开发完成
5. 有失败 → 修复代码后重新执行失败的用例

### 输出测试报告

执行完所有用例后，输出汇总：

```
分支：1011255-report-export
总用例数：8
通过：7 ✅
失败：1 ❌

失败用例：
  #3 创建报表导出任务 — 月份格式错误
  预期：返回参数校验错误
  实际：返回200，创建了任务
  原因：Validate 未校验月份格式
```

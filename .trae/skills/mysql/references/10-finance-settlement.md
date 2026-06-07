# 财务结算（Finance / Settlement）

```mermaid
flowchart TB
    subgraph 岗位对账
        PS[r_post_statement<br/>岗位对账单] --> PSWP[r_post_statement_work_permit<br/>作业许可证]
        PSWP --> PSWPWS[r_post_statement_work_permit_work_station<br/>许可证工位]
        PS --> PSCHARGE[r_post_statement_charge_scheme<br/>计费方案]
    end

    subgraph 付款
        PAY[r_pay_order<br/>付款单] --> FPAY[r_finance_payment_order<br/>财务付款单]
    end

    subgraph 日结算
        DAILY[r_daily_settlement<br/>日结算] --> SETTLE[r_settle_daily<br/>结算日]
        SETTLE --> SETTLEQ[r_settle_daily_time_quantum<br/>时间段]
    end

    subgraph 月结算
        DAILY --> MONTH[r_month_settlement<br/>月结算]
    end

    subgraph 佣金
        COMMISSION[r_commission_config<br/>佣金配置<br/>树状organization_code]
    end

    subgraph 成本
        MCOST[r_material_cost_detail_log<br/>物料成本日志]
        FINIT[r_financial_material_initial_stock<br/>期初库存]
        FRECON[r_financial_reconciliation<br/>财务对账]
    end

    subgraph 支出
        EXPEND[r_expenditure<br/>支出]
    end
```

| 表 | 用途 | 核心外键 |
|---|------|---------|
| r_post_statement | 岗位对账单 | customer_id, company_id |
| r_post_statement_work_permit | 作业许可证 | post_statement_id |
| r_post_statement_work_permit_work_station | 许可证工位 | work_permit_id, work_station_id |
| r_post_statement_charge_scheme | 计费方案 | post_statement_id |
| r_pay_order | 付款单 | purchase_order_id, supplier_id |
| r_finance_payment_order | 财务付款单 | pay_order_id, supplier_id |
| r_daily_settlement | 日结算 | work_station_id, robot_id, company_id |
| r_settle_daily | 结算日 | — |
| r_settle_daily_time_quantum | 时间段 | settle_daily_id |
| r_month_settlement | 月结算 | customer_id, company_id |
| r_commission_config | 佣金配置（树状） | parent_organization_code |
| r_material_cost_detail_log | 物料成本日志 | material_id, storehouse_id |
| r_financial_material_initial_stock | 期初库存 | material_id, material_group_id |
| r_financial_reconciliation | 财务对账 | — |
| r_expenditure | 支出 | company_id |
| r_robot_daily_settlement_wait | 机器人日结算等待 | robot_id |

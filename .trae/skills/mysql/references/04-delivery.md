# 交付管理（Delivery）

```mermaid
flowchart TB
    subgraph 交付计划
        DMP[r_delivery_month_plan<br/>月计划] --> DMPH[r_delivery_month_plan_history<br/>月计划历史]
        DMPH -->|关联| WST[r_work_station_template<br/>工位模板]
        DMP --> DT[r_delivery_target<br/>交付目标]
        DMP --> DPL[r_delivery_plan_log<br/>计划日志]
    end

    subgraph 交付模板
        DTMPL[r_delivery_template<br/>交付模板] --> DTDETAIL[r_delivery_template_detail<br/>模板明细]
        DTMPL --> DTMFRONT[r_delivery_template_front<br/>模板前端]
        DTDETAIL --> DN[r_delivery_node<br/>交付节点]
    end

    subgraph 交付任务
        DTASK[r_delivery_task<br/>任务定义] --> DTMATTER[r_delivery_task_matter<br/>任务事项]
    end

    subgraph 交付工单
        WS[r_work_station] -->|创建工单| DWO[r_delivery_work_order<br/>交付工单]
        DWO --> DWOT[r_delivery_work_order_task<br/>工单任务]
        DWOT --> DWOTM[r_delivery_work_order_task_material<br/>任务物料]
        DWOT --> DWOTF[r_delivery_work_order_task_front<br/>任务前端]
        DWO --> DWOCONFIRM[r_delivery_work_order_confirm_user<br/>确认人]
        DWO --> DWOPMC[r_delivery_work_order_pmc_log<br/>PMC日志]
        DWO --> DWOSTOP[r_delivery_work_order_stop_log<br/>停线日志]
    end

    subgraph 模拟工位
        DWO --> DAWS[r_delivery_analog_work_station<br/>模拟工位]
        DAWS --> DAWSORDER[r_delivery_analog_work_station_order<br/>模拟工位工单]
        DAWS --> DAWSLOG[r_delivery_analog_work_station_status_log<br/>模拟工位状态日志]
    end

    subgraph 自定义字段
        DCF[r_delivery_customer_field<br/>客户自定义字段]
    end
```

| 表 | 用途 | 核心外键 |
|---|------|---------|
| r_delivery_month_plan | 月交付计划 | company_id |
| r_delivery_month_plan_history | 月计划历史 | month_plan_id, work_station_template_id |
| r_delivery_target | 交付目标 | company_id, user_id |
| r_delivery_plan_log | 计划日志 | template_id, creator_id |
| r_delivery_template | 交付模板 | — |
| r_delivery_template_detail | 模板明细 | template_id, node_id |
| r_delivery_template_front | 模板前端配置 | template_id |
| r_delivery_node | 交付节点 | — |
| r_delivery_task | 任务定义 | — |
| r_delivery_task_matter | 任务事项 | task_id |
| r_delivery_work_order | 交付工单（从工位发起） | work_station_id, customer_id, company_id |
| r_delivery_work_order_task | 工单任务 | work_order_id, task_id |
| r_delivery_work_order_task_material | 任务物料 | work_order_task_id, material_id |
| r_delivery_work_order_task_front | 任务前端 | work_order_task_id |
| r_delivery_work_order_confirm_user | 确认人 | work_order_id, user_id |
| r_delivery_work_order_pmc_log | PMC日志 | work_order_id |
| r_delivery_work_order_stop_log | 停线日志 | work_order_id |
| r_delivery_analog_work_station | 模拟工位 | work_order_id, company_id |
| r_delivery_analog_work_station_order | 模拟工位工单 | analog_work_station_id, work_order_id |
| r_delivery_analog_work_station_status_log | 模拟工位状态日志 | analog_work_station_id |
| r_delivery_customer_field | 客户自定义字段 | — |

# 图表报表模块（Charts / BI）

```mermaid
flowchart TB
    subgraph 基建层
        BOARD[r_charts_board<br/>图表组件定义<br/>show_scene=1,2,3]
    end

    subgraph BI中心_大看板
        GROUP[r_charts_group<br/>BI分组]
        GROUP -->|customer_id区分| CUST_GROUP[客户分组<br/>同表customer_id有值]
        GTB[r_charts_group_to_board<br/>中间表]
        GROUP --> GTB
        FLEX[r_chart_flexible_design<br/>分组布局]
    end

    subgraph 个人工作台
        WGROUP[r_charts_workbench_group<br/>工作台分组]
        WGTB[r_charts_workbench_group_to_board<br/>中间表]
        WGROUP --> WGTB
        WFLEX[r_charts_workbench_flexible_design<br/>分组布局]
        WBTN[r_charts_workbench_button<br/>工作台按钮]
    end

    subgraph 小看板_独立体系
        SBOARD[r_charts_small_charts_board<br/>小看板组件]
        SGTB[r_charts_small_charts_group_board<br/>小看板分组中间表]
        SBOARD --> SGTB
    end

    BOARD --- GTB
    BOARD --- WGTB
```

| 表 | 用途 | 核心外键 |
|---|------|---------|
| r_charts_board | 图表组件定义（基建，三个业务共享） | show_scene(1=BI,2=工作台,3=客户) |
| r_charts_group | BI分组 + 客户分组（同表，customer_id区分） | customer_id, is_investor |
| r_charts_group_to_board | BI/客户 分组-图表中间表 | group_id, board_id |
| r_chart_flexible_design | BI/客户 分组布局 | group_id |
| r_charts_workbench_group | 工作台分组（独立） | — |
| r_charts_workbench_group_to_board | 工作台 分组-图表中间表 | group_id, board_id |
| r_charts_workbench_flexible_design | 工作台分组布局 | group_id |
| r_charts_workbench_button | 工作台按钮（工作台独有） | board_id |
| r_charts_small_charts_board | 小看板组件（独立于大看板） | — |
| r_charts_small_charts_group_board | 小看板分组中间表 | board_id |

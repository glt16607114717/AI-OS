# 基础数据（BaseData）

```mermaid
flowchart TB
    subgraph 行业工艺产品
        INDUSTRY[r_base_data_industry<br/>行业] --> IWP[r_industry_workmanship_product<br/>行业工艺产品]
        TECH[r_base_data_technology<br/>工艺] --> IWP
        BPRODUCT[r_base_data_product<br/>产品] --> IWP
    end

    subgraph 物料基础
        BMATERIAL[r_base_data_material<br/>基础物料]
    end

    subgraph 其他基础
        BLIFECYCLE[r_base_lifecycle_status<br/>生命周期状态]
        BAPPROVER[r_base_data_approver<br/>审批人配置]
        BLABEL[r_base_data_knowledge_label<br/>知识标签]
        DICT[r_data_dict<br/>数据字典] --> DICTLABEL[r_data_dict_label<br/>字典标签]
        CONFIG[r_config<br/>系统配置]
    end
```

| 表 | 用途 | 核心外键 |
|---|------|---------|
| r_base_data_industry | 行业 | — |
| r_base_data_technology | 工艺 | — |
| r_base_data_product | 产品 | — |
| r_industry_workmanship_product | 行业工艺产品 | industry_id, technology_id, product_id |
| r_base_data_material | 基础物料 | material_type_id |
| r_base_lifecycle_status | 生命周期状态 | block, stage |
| r_base_data_approver | 审批人配置 | link_id |
| r_base_data_knowledge_label | 知识标签 | — |
| r_data_dict | 数据字典 | — |
| r_data_dict_label | 字典标签 | dict_id |
| r_config | 系统配置 | config_mark |
| r_technology_scheme | 工艺方案 | — |
| r_technology_scheme_device | 工艺方案设备 | technology_scheme_id |
| r_cate_area | 地区（省市区） | pid |
| r_error_message_define | 错误码定义 | — |

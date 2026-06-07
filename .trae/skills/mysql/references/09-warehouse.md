# 仓储物流（Warehouse / Storehouse / Transfer）

```mermaid
flowchart TB
    subgraph 仓库管理
        WH[r_warehouse<br/>仓库] --> WHSPACE[r_warehouse_space<br/>库位]
        WH --> WHRECORD[r_warehouse_record<br/>出入库记录]
    end

    subgraph 库房管理
        STORE[r_storehouse<br/>库房] --> STOREINV[r_storehouse_inventory<br/>盘点]
        STOREINV --> STOREINVDET[r_storehouse_inventory_detail<br/>盘点明细]
        STOREINV --> STOREINVUSER[r_storehouse_inventory_user<br/>盘点人员]
    end

    subgraph 调拨
        TORDER[r_transfer_order<br/>调拨单] --> TMAT[r_transfer_order_material<br/>调拨物料]
        TORDER --> TOUT[r_transfer_outbound_order<br/>调拨出库单]
        TOUT --> TOUTMAT[r_transfer_outbound_order_material<br/>出库物料]
        TORDER --> TWH[r_transfer_warehouse_order<br/>调拨仓库单]
        TWH --> TWHMAT[r_transfer_warehouse_order_material<br/>仓库调拨物料]
    end

    subgraph 库存单据
        INVORD[r_inventory_order<br/>库存单据] --> INVORDDET[r_inventory_order_detail<br/>单据明细]
    end
```

| 表 | 用途 | 核心外键 |
|---|------|---------|
| r_warehouse | 仓库 | company_id |
| r_warehouse_space | 库位 | warehouse_id |
| r_warehouse_record | 出入库记录 | warehouse_id |
| r_storehouse | 库房（审批通过自动创建） | company_id, customer_id |
| r_storehouse_inventory | 盘点 | storehouse_id |
| r_storehouse_inventory_detail | 盘点明细 | storehouse_inventory_id |
| r_storehouse_inventory_user | 盘点人员 | storehouse_inventory_id, user_id |
| r_transfer_order | 调拨单 | storehouse_id, company_id |
| r_transfer_order_material | 调拨物料 | transfer_order_id, material_id |
| r_transfer_outbound_order | 调拨出库单 | transfer_order_id |
| r_transfer_outbound_order_material | 出库物料 | outbound_order_id, material_id |
| r_transfer_warehouse_order | 调拨仓库单 | transfer_order_id |
| r_transfer_warehouse_order_material | 仓库调拨物料 | warehouse_order_id, material_id |
| r_inventory_order | 库存单据 | storehouse_id |
| r_inventory_order_detail | 单据明细 | inventory_order_id, material_id |
| r_fixture_order | 夹具订单 | customer_id, work_station_id |
| r_fixture_order_material | 夹具订单物料 | fixture_order_id, material_id |

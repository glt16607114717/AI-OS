# 系统工具（System / Tools）

| 表 | 用途 | 核心外键 |
|---|------|---------|
| r_menu | 菜单 | pid |
| r_upload_file | 上传文件 | user_id |
| r_upload_file_review | 文件审核 | file_id |
| r_download_files | 下载文件 | user_id |
| r_archives | 档案 | customer_id, post_statement_id |
| r_attachment_type | 附件类型（树状） | fid |
| r_setting | 系统设置 | — |
| r_version | 版本 | — |
| r_access_log | 访问日志 | user_id |
| r_log_follow_operation_collection | 操作日志汇总 | — |
| r_system_messages | 系统消息 | user_id |
| r_db_cache | 数据库缓存 | key |
| r_todo | 待办 | user_id |
| r_sop | SOP标准作业 | sop_type_id |
| r_sop_type | SOP类型 | — |
| r_exhibition | 展示 | — |
| r_join_us | 加入我们 | — |
| r_table_list | 数据库表列表 | — |
| r_table_field_list | 字段列表 | table_id |
| r_table_group | 表分组 | — |
| r_table_index_list | 索引列表 | table_id |
| r_table_update_history | 表更新历史 | table_id |

---

# 视图层（View）

| 表 | 用途 |
|---|------|
| r_view_customer_clue | 客户线索视图 |
| r_view_customer_robot_count | 客户机器人数量视图 |
| r_view_daily_settlement | 日结算视图 |
| r_view_deploy_product | 部署产品视图 |
| r_view_log_produce_storehouse | 生产入库日志视图 |
| r_view_material_purchase_into_storage_price | 物料采购入库价格视图 |
| r_view_material_stock_count | 物料库存统计视图 |
| r_view_month_settlement | 月结算视图 |
| r_view_work_station_permit | 工位许可证视图 |
| r_view_work_station_robot_count | 工位机器人数量视图 |
| r_view_work_station_scheme | 工位方案视图 |

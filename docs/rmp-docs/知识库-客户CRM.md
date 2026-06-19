# RMP 客户 CRM 模块知识库

> 数据来源：rmp_tables.json（39 张相关表/视图）、rmp-api 后端控制器、nnd-robot 前端代码
> 生成时间：2026-06-19
> 覆盖范围：客户管理、客户线索、线索场地、跟进延期、意向协议、信息草稿、客户审核、项目经理工作台、对账单

---

## 一、客户管理（核心）

### 1.1 r_customer（客户主表）

**用途**：存储客户核心信息，是整个 CRM 模块的主表，所有业务围绕 `customer_id` 展开。当前 516 行。

**核心字段**：

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键，自增 |
| customer_no | varchar(32) | 客户编号（唯一业务编码） |
| customer_name | varchar(64) | 客户全称 |
| customer_short_name | varchar(128) | 客户简称 |
| status | tinyint | **旧状态机**：-2已驳回、-1草稿、0待审核、1潜在、2意向、3试运行、4试运行部署、5正式、6正式运行部署、7已撤回、8终止、9延期介入 |
| new_status | tinyint | **新状态机**：10潜在客户、20目标客户、30意向客户、31合同客户、40在运行客户、50终止客户 |
| level | tinyint | 客户等级（字典：customer_level） |
| source | int | 客户来源（字典：CustomerClue::SOURCE） |
| clue_id | int unsigned | 关联线索ID（来自 r_customer_clue） |
| company_id | int unsigned | 所属公司ID（多租户隔离） |
| province/city/town/area | varchar(32) | 省/市/镇/区 |
| address | varchar(128) | 详细地址 |
| longitude/latitude | varchar(120) | 经纬度 |
| business_license_code | varchar(50) | 营业执照号 |
| legal_person | varchar(32) | 法定代表人 |
| ein | varchar(18) | 税号 |
| deposit_bank | varchar(128) | 开户银行 |
| bank_account | varchar(19) | 银行账号 |
| service_contract | json | 上岗单服务合同 JSON |
| service_contract_no | varchar(30) | 服务合同编号 |
| flow_id | varchar(32) | 工作流流程ID |
| current_flow_node | varchar(255) | 当前流程节点 |
| draft_id | int unsigned | 对应草稿ID（r_customer_info_draft） |
| is_new_flow | tinyint | 是否新流程（默认1） |
| range_user_read/write | varchar(1000) | 数据权限：只读/读写用户ID（逗号分隔） |
| range_role_read/write | varchar(1000) | 数据权限：只读/读写角色ID |
| range_department/company | varchar(1000) | 数据权限：部门/公司范围 |
| first_code_pin_yin | varchar(50) | 简称首字母（排序用） |
| is_delete | tinyint | 软删除标记 |

**索引**：PRIMARY(id)、idx_customer_name、idx_customer_no、idx_status、idx_clue_id

**关联关系**：
- `clue_id` → r_customer_clue.id（线索来源）
- `company_id` → 公司表（多租户）
- `draft_id` → r_customer_info_draft.id（草稿来源）

**新状态流转规则**（new_status）：

```
潜在客户(10) → 目标客户(20) → 意向客户(30) → 合同客户(31) → 在运行客户(40) → 终止客户(50)
```

### 1.2 r_customer_survey_info（客户调研信息）

**用途**：存储客户的详细调研信息，一个客户对应一条调研记录。当前 514 行，47 字段。

**核心字段**：

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| customer_id | int unsigned | 客户ID |
| customer_brand | varchar(50) | 客户品牌 |
| is_group | tinyint | 是否集团（0否/1是） |
| scale | int unsigned | 公司规模（人） |
| turnover | int unsigned | 年营业额（万元） |
| worker_scale | int unsigned | 工人规模 |
| worker_wages | int unsigned | 工人工资（元） |
| worker_wages_calculate_way | tinyint | 记工方式（1计时/2计件） |
| temporary_scale/wages | int unsigned | 临时工规模/工资 |
| night_shift | tinyint | 是否有夜班 |
| night_shift_num/day_shift_num | int | 夜班/白班人数 |
| industry | varchar(255) | 所属行业（多选，逗号分隔） |
| technology_type | varchar(255) | 工艺类别（多选） |
| product_type | varchar(255) | 产品类别 |
| automation_degree | tinyint | 自动化程度（1低/2中/3高） |
| intention_degree | tinyint | 机器人导入意向（1低/2中/3高） |
| estimate_num | int | 预计可导入机器人数量 |
| external_media_archive_id | int unsigned | 外部照片档案ID |
| scene_media_archive_id | int unsigned | 现场照片档案ID |
| product_media_archive_id | int unsigned | 产品照片档案ID |
| workmanship_media_archive_id | int unsigned | 工艺照片档案ID |
| recommend_work_station_archive_id | int unsigned | 推荐工位档案ID |
| survey_report_file_id | int unsigned | 调研报告文件ID |
| investigator_user_id | int unsigned | 调研员用户ID |
| delay_date | varchar(20) | 延迟介入时间 |
| contract_start_date/end_date | date | 合同生效/结束时间 |
| insure | int unsigned | 参保人数 |
| normal_wait_time_limit | tinyint | 正常待料时间上限（默认30） |

**索引**：PRIMARY(id)、idx_customer_id

**关联关系**：`customer_id` → r_customer.id（一对一）

### 1.3 r_customer_contact_person（客户联系人）

**用途**：存储客户联系人信息，一个客户可以有多个联系人。当前 3709 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| customer_id | int unsigned | 客户ID |
| clue_id | int unsigned | 线索ID（线索阶段也有联系人） |
| name | varchar(32) | 姓名 |
| department | varchar(32) | 部门 |
| position | varchar(32) | 职位 |
| phone | varchar(100) | 手机号 |
| contact_phone | varchar(50) | 联系电话（座机） |
| type | tinyint | 类型（1项目负责人/2项目对接人） |
| name_pinyin | varchar(255) | 联系人拼音 |

**索引**：PRIMARY(id)、idx_customer_id

**业务规则**：编辑/添加客户时，至少需一个有效联系人（姓名+职位+手机号或联系电话）。

### 1.4 r_customer_changdi（客户场地）

**用途**：存储客户场地信息，一个客户多个场地。当前 608 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| customer_id | int unsigned | 客户ID |
| ground_name | varchar(50) | 场地名称 |
| province/city/town/area | varchar(32) | 地区信息 |
| address | varchar(64) | 详细地址 |
| is_main | tinyint | 是否主场地（1是/0否） |
| open_ban | tinyint | 开启/禁用（1开启/-1禁用） |
| operator_manager_id | int | 运维经理ID |
| clue_id | int unsigned | 线索ID |
| latitude/longitude | decimal(10,6) | 经纬度 |

**索引**：PRIMARY(id)、idx_customer_id、idx_clue_id

### 1.5 r_customer_salesman_contact（客户-业务员关联）

**用途**：客户与业务员的绑定关系，含结算配置。当前 515 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| salesman_user_id | int | 业务员用户ID |
| company_id | int unsigned | 公司ID |
| customer_id | int unsigned | 客户ID |
| month_minimum_manhours | decimal(5,2) | 月保底工时 |
| manhours_price | decimal(5,2) | 工时单价 |

**索引**：PRIMARY(id)、idx_salesman_user_id、idx_customer_id、idx_company_id

### 1.6 r_customer_marketing_user（客户-市场负责人关联）

**用途**：客户与市场负责人的绑定关系。当前 107 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| marketing_user_id | int unsigned | 市场负责人ID |
| company_id | int unsigned | 公司ID |
| customer_id | int unsigned | 客户ID |

### 1.7 其他客户管理辅助表

| 表名 | 行数 | 用途 |
|------|------|------|
| r_customer_changdi_robot_gl | 88 | 场地-机器人-工位关联表 |
| r_customer_ground_operator_contact | 202 | 场地-运维人员关联 |
| r_customer_ground_operator_manager_contact | 109 | 场地-运维经理关联 |
| r_customer_subcompany | 0 | 客户子公司信息 |
| r_customer_user_gl | 20 | 客户-用户关联（多对多） |
| r_customer_new_status_log | 72 | 客户新状态变更日志 |
| r_customer_pool_flow_log | 64 | 客户池流转日志（公海/私海流转记录） |
| r_customer_work_price_config | 4 | 客户工位阶梯计价配置（多档机器人台数对应不同单价） |

### 1.8 关键 API（客户管理）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | /api/admin/customer-manage/list | 客户列表 V1 |
| GET | /api/admin/customer-manage/list-v2 | 客户列表 V2（增强筛选） |
| GET | /api/admin/customer-manage/list-v3 | 客户列表 V3（新状态 Tab） |
| GET | /api/admin/customer-manage/status-tabs | 客户状态 Tab 统计 |
| GET | /api/admin/customer-manage/info | 客户详情 |
| POST | /api/admin/customer-manage/add | 添加客户 |
| POST | /api/admin/customer-manage/edit | 编辑客户 |
| DELETE | /api/admin/customer-manage/delete | 删除客户 |
| POST | /api/admin/customer-manage/add-ground | 添加场地 |
| PUT | /api/admin/customer-manage/edit-ground | 编辑场地 |
| GET | /api/admin/customer-manage/ground-list | 场地列表 |
| POST | /api/admin/customer-manage/save-customer-contact-person | 保存联系人 |
| GET | /api/admin/customer-manage/customer-contact-person-list | 联系人列表 |
| PUT | /api/admin/customer-manage/new-change-customer-status | 修改客户状态 |
| GET | /api/admin/customer-manage/customer-statistics | 客户统计面板 |
| GET | /api/admin/customer-manage/new-status-map | 新状态映射 |
| POST | /api/admin/customer-manage/save-service-contract | 保存服务合同 |
| POST | /api/admin/customer-manage/save-contract-dates | 保存合同日期 |
| POST | /api/admin/customer-manage/edit-manager | 修改业务/客户经理 |
| GET | /api/admin/customer-manage/select-map-of-robot-data | 有机器人数据的客户下拉 |

---

## 二、客户线索

### 2.1 r_customer_clue（线索主表）

**用途**：存储客户线索全量信息，是线索到客户转化流程的起点。当前 1700 行，110 字段（字段最多）。

**核心字段**：

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| customer_name | varchar(64) | 客户名称 |
| customer_short_name | varchar(128) | 客户简称 |
| business_license_code | varchar(50) | 统一社会信用代码 |
| legal_person | varchar(32) | 法定代表人 |
| regist_amount | varchar(30) | 注册资本 |
| insure | int unsigned | 参保人数 |
| status | int unsigned | **状态机**：1线索→10潜在客户→20目标客户→30意向客户→31合同客户→40在运行客户→50终止客户 |
| sub_status | tinyint | 二级状态（正式客户专用，1=已完结） |
| customer_id | int unsigned | 转为正式客户后的ID |
| pool_type | tinyint unsigned | **池类型**：1线索池、2客户池 |
| sub_type | tinyint unsigned | **线索子类型**：0无、1已领线索、2自引线索、3公共线索 |
| sale_user_id | int unsigned | 销售员ID |
| marketing_user_id | int unsigned | 市场负责人ID |
| assistant_user_id | int | 销售助理ID |
| source | int unsigned | 客户来源 |
| match | int unsigned | 客户匹配度 |
| gmt_time | date | 创建日期 |
| gmt_receive / gmt_receive_end | datetime | 领取时间 / 领取结束时间 |
| is_lock | int unsigned | 是否锁定 |
| gmt_lock / gmt_lock_end | datetime | 锁定时间 / 锁定结束时间 |
| gmt_follow | datetime | 最新跟进时间 |
| gmt_follow_protect | datetime | 跟进保护期到期 |
| protect_start_at / protect_end_at | datetime | 保护期开始 / 结束 |
| has_visit_in_protect | tinyint | 本保护期内是否有拜访（锁定判断） |
| first_claim_at | datetime | 初次领取时间（掉池后清空） |
| protect_count | int | 保护期添加次数 |
| always_public_pool | int | 是否永久公共线索（0否/1是） |
| self_pool_expire_at | datetime | 自引客户池未领取到期时间 |
| clue_assigned_status | tinyint | 1未分配/2已分配 |
| clue_receive_type | int | 领取方式（0分配领取/1自己创建） |
| introduce_user_id | int | 介绍人ID |
| introduce_customer_id | int | 介绍客户ID |
| exhibition_id | int | 展会ID |

**索引**：PRIMARY(id)、customer_name、customer_short_name、复合索引 idx_pool_sub_sale_self_expire(pool_type, sub_type, sale_user_id, self_pool_expire_at)

**线索状态流转规则**：

```
线索(1) → 潜在客户(10) → 目标客户(20) → 意向客户(30) → 合同客户(31) → 在运行客户(40) → 终止客户(50)
```

**线索池流转规则**：

| 触发条件 | 流转方向 | 说明 |
|----------|----------|------|
| 新建线索 | 公共线索池 | sub_type=3 |
| 领取线索 | 个人已领 | sub_type=1 |
| 保护期到期未跟进 | 掉入公海 | pool_type 重置 |
| 自引线索未领取到期 | 掉入客户池 | self_pool_expire_at 控制 |

### 2.2 r_customer_clue_follow（线索跟进记录）

**用途**：记录线索的每次跟进/拜访信息。当前 3137 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| clue_id | int unsigned | 线索ID |
| customer_id | int | 客户ID |
| follow_type | int | 跟进类型 |
| follow_method | tinyint | 类型（1跟进/2拜访签到） |
| note | varchar(1000) | 跟进记录 |
| gmt_follow | datetime | 跟进时间 |
| gmt_next | date | 下次跟进时间 |
| common_follow_user | varchar(255) | 共同跟进人 |
| outdoor_image / indoor_image | varchar(255) | 室外/室内图 |
| distance | int | 签到距离 |
| is_new | tinyint | 是否新拜访 |
| defer_apply_id | int | 延时申请ID |
| defer_day | int | 延期天数 |
| defer_reason | varchar(1024) | 延期原因 |

### 2.3 r_customer_clue_follow_reply（跟进回复）

**用途**：对跟进记录的回复，支持引用回复。当前 55 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| follow_id | int unsigned | 跟进记录ID |
| reply_id | int | 引用回复的ID |
| note | varchar(512) | 回复内容 |

### 2.4 r_customer_clue_log（线索操作日志）

**用途**：记录线索的所有操作行为日志，含状态变更、拜访、保护期等。当前 7383 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| clue_id | int unsigned | 线索ID |
| customer_id | int unsigned | 客户ID（转潜在后才有） |
| action_type | varchar(32) | 行为类型（如 VISIT、STATUS_CHANGE） |
| from_status / to_status | tinyint | 原阶段 / 目标阶段 |
| visit_type | tinyint | 拜访方式（1电话/2线下/3信息编辑） |
| protect_days | int | 本次行为增加的保护期天数 |
| protect_start_at / protect_end_at | datetime | 保护期开始/结束 |
| sale_user_id | int | 分配的销售ID |
| extra_data | text | 脚本执行前原始数据 |

### 2.5 r_customer_clue_layer（线索上下游关系）

**用途**：记录线索的上下游供应商/客户关系。当前 2397 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| clue_id | int unsigned | 线索ID |
| name | varchar(255) | 供应商/客户名称 |
| industry | varchar(32) | 行业 |
| technology_type | varchar(255) | 工艺类别 |
| product | varchar(255) | 产品 |
| type | int | 1上游 / 2下游 |

### 2.6 r_customer_clue_cancel（线索作废）

**用途**：线索作废申请，需审批流程。当前 28 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| clue_id | int unsigned | 线索ID |
| cancel_reason | varchar(255) | 作废原因 |
| status | tinyint | 状态（0否/1审批中/2审批通过） |
| flow_id | varchar(32) | 流程ID |

### 2.7 r_customer_clue_mail_record（线索邮寄记录）

**用途**：记录线索的礼品/资料邮寄信息。当前 16 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| clue_id | int unsigned | 线索ID |
| tracking_number | varchar(50) | 快递单号 |
| gift_type | varchar(255) | 礼品类型 |
| gmt_mail / gmt_sign | varchar(255) | 邮寄时间 / 签收时间 |

### 2.8 关键 API（客户线索）

| 方法 | 路径 | 用途 |
|------|------|------|
| POST | /api/admin/customer-clue/add | 添加线索 |
| GET | /api/admin/customer-clue/list | 线索列表 |
| GET | /api/admin/customer-clue/list-v2 | 线索列表 V2（含保护期/池信息） |
| GET | /api/admin/customer-clue/assign-company | 分配公司 |
| GET | /api/admin/customer-clue/exhibition-list | 展会名称下拉 |

---

## 三、线索场地管理

### 3.1 r_customer_changdi（场地表）

线索场地和客户场地共用 `r_customer_changdi` 表，通过 `clue_id` 和 `customer_id` 区分。

**业务规则**：
- 线索最多 10 个场地，客户最多 11 个场地
- 2 公里内不允许有同名场地或已存在的地址
- 添加场地时自动调用高德 API 获取经纬度
- 场地可设置主场地（is_main）和启停状态（open_ban）

### 3.2 r_customer_changdi_robot_gl（场地-机器人关联）

**用途**：场地、工位、机器人的三方关联。当前 88 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| customer_id | int unsigned | 客户ID |
| customer_changdi_id | int unsigned | 场地ID |
| work_station_id | int unsigned | 工位ID |
| robot_id | int unsigned | 机器人ID |

### 3.3 关键 API（线索场地）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | /api/admin/customer-clue-changdi/list | 场地列表（支持 clue_id 或 customer_id） |
| POST | /api/admin/customer-clue-changdi/create | 添加线索场地 |

---

## 四、客户跟进延期申请

### 4.1 r_customer_follow_defer_apply（跟进延期申请）

**用途**：合同客户未签上岗单时，跟进保护期到期的延期申请。当前 246 行。

**核心字段**：

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int unsigned | 主键 |
| clue_id | int unsigned | 线索ID |
| customer_id | int unsigned | 客户ID |
| customer_clue_log_id | int | 跟进记录ID |
| customer_status | tinyint | 客户状态阶段（1合同客户未签上岗单） |
| status | tinyint | **审批状态**：1草稿→2待审批→3审批中→4审批通过→5驳回 |
| times | int | 申请次数（1/2/3，最多3次） |
| defer_day | int | 延期天数 |
| defer_reason | varchar(1024) | 延期原因 |
| attach_id | varchar(1024) | 图片申请附件 |
| flow_id | varchar(32) | 流程ID |
| first_claim_at | datetime | 初次领取时间（关联线索池判断） |

**延期审批状态机**：

```
草稿(1) → 待审批(2) → 审批中(3) → 审批通过(4) / 驳回(5)
```

### 4.2 关键 API（跟进延期）

| 方法 | 路径 | 用途 |
|------|------|------|
| PUT | /api/admin/customer-follow-defer-apply/update-status | 修改审批状态 |
| POST | /api/admin/customer-follow-defer-apply/back-audit | 反审批 |
| GET | /api/admin/customer-follow-defer-apply/get-apply-mess | 获取延期申请相关字段 |

---

## 五、客户意向协议管理

### 5.1 r_customer_intention_agreement（意向协议）

**用途**：存储已签署的意向协议签字版文件。当前 8 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| customer_id | int unsigned | 客户ID |
| sign_file_id | int unsigned | 签字版文件ID |
| log_user_id | int unsigned | 添加记录的用户ID |

### 5.2 r_customer_intention_agreement_apply（意向协议申请）

**用途**：意向协议的上传审批流程。当前 463 行。

**核心字段**：

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| clue_id | int unsigned | 线索ID |
| customer_id | int unsigned | 客户ID |
| archive_id | int unsigned | 意向协议文件ID |
| status | tinyint | **审批状态**：1草稿→2待审批→3审批中→4审批通过→5反审核 |
| flow_id | varchar(32) | 流程ID |
| current_flow_node | varchar(255) | 当前流程节点 |
| reject_reason | varchar(500) | 驳回原因 |

### 5.3 关键 API（意向协议）

| 方法 | 路径 | 用途 |
|------|------|------|
| PUT | /api/admin/customer-intention-agreement/update-status | 修改审批状态 |
| POST | /api/admin/customer-intention-agreement/back-audit | 反审核 |

---

## 六、客户信息草稿管理

### 6.1 r_customer_info_draft（客户信息草稿）

**用途**：客户信息提交审核前的草稿暂存。当前 24 行，50 字段。

**核心字段**：

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| user_id | int unsigned | 添加信息的用户ID |
| salesman_user_id | int unsigned | 业务员ID |
| customer_company_name | varchar(100) | 客户企业名称 |
| customer_short_name | varchar(128) | 客户简称 |
| customer_brand | varchar(50) | 客户品牌 |
| business_license_code | varchar(50) | 营业执照号码 |
| legal_person | varchar(50) | 法人 |
| is_group | tinyint | 是否集团 |
| scale | int unsigned | 公司规模 |
| turnover | int unsigned | 年营业额（万元） |
| worker_scale / wages | int | 工人规模/工资 |
| night_shift | tinyint | 是否有夜班 |
| industry | varchar(30) | 行业 |
| product_type / technology_type | varchar(255) | 产品类别/工艺类别 |
| automation_degree | tinyint | 自动化程度（1低/2中/3高） |
| intention_degree | tinyint | 机器人导入意向（1低/2中/3高） |
| estimate_num | int | 预计可导入机器人数量 |
| subsidiary | text | 子公司（JSON） |
| docking_person | text | 项目对接人（JSON） |
| intention_agreement | text | 意向协议 |
| status | tinyint | **状态**：0正常→1已提交审核→2审核通过→3审核不通过 |

**草稿状态机**：

```
正常(0) → 已提交审核(1) → 审核通过(2) / 审核不通过(3)
```

### 6.2 关键 API（信息草稿）

| 方法 | 路径 | 用途 |
|------|------|------|
| POST | /api/admin/customer-info-draft-manage/add | 添加草稿 |

---

## 七、客户审核（项目经理绑定、调研）

### 7.1 客户项目经理绑定流程

审核流程涉及 `CustomerExamineController`，管理客户项目经理（CPM）的绑定审批。

**项目经理绑定状态机**：

```
业务申请绑定CPM → 审批人审批 → 确认CPM → 绑定完成
```

**关键 API**：

| 方法 | 路径 | 用途 |
|------|------|------|
| POST | /api/admin/customer-examine/assign-bind-cpm | 申请绑定客户项目经理 |
| GET | /api/admin/customer-examine/assign-cpm-info | 获取CPM申请信息 |
| POST | /api/admin/customer-examine/approve-cpm | 审批客户项目经理 |
| POST | /api/admin/customer-examine/confirm-cpm | 确认客户项目经理 |
| POST | /api/admin/customer-manage/unbind-project-manager | 解绑项目经理 |
| POST | /api/admin/customer-manage/rebind-cpm | 重新绑定CPM |

### 7.2 客户调研流程

| 方法 | 路径 | 用途 |
|------|------|------|
| POST | /api/admin/customer-examine/reply-survey-time | 回复调研时间 |
| GET | /api/admin/customer-examine/survey-time-info | 获取调研时间信息 |
| POST | /api/admin/customer-examine/confirm-survey-time | 确认调研时间 |
| POST | /api/admin/customer-examine/upload-servey-report | 上传调研报告 |
| GET | /api/admin/customer-examine/examine-flow-list | 客户流程列表 |
| PUT | /api/admin/customer-manage/change-delay-date | 修改延期介入日期 |

### 7.3 客户合同申请

`r_customer_follow_contract_apply` 表管理客户的服务合同申请审批。当前 85 行。

**核心字段**：

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| clue_id / customer_id | int unsigned | 线索ID / 客户ID |
| status | tinyint | **审批状态**：1草稿→2待审批→3审批中→4审批通过→5驳回 |
| contract_start_date / end_date | date | 合同生效/结束时间 |
| service_contract | json | 上岗单服务合同 |
| service_contract_no | varchar(1024) | 合同编号 |
| effective_status | tinyint | 生效状态（0空/1未生效/2生效中/3结束） |
| attach_id | varchar(1024) | 合同附件 |

**关键 API**：

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | /api/admin/customer-manage/contract-apply/index | 合同申请列表 |
| GET | /api/admin/customer-manage/contract-apply/detail | 合同申请详情 |
| POST | /api/admin/customer-manage/contract-apply/add | 添加合同申请 |

---

## 八、客户项目经理工作台

### 8.1 功能概述

`CustomerProjectManagerController` 提供项目经理工作台的数据看板能力，包括稼动率预警、客户统计等。

**关键 API**：

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | /api/admin/customer-project-manager/customer-efficiency-warning | 稼动率预警（低于配置阈值） |
| GET | /api/admin/customer-project-manager/customer-statistics | 客户统计数据 |

### 8.2 稼动率预警逻辑

1. 从配置表读取 `activation_warning_rate`（稼动率预警阈值）
2. 查询项目经理所在公司的所有有效客户（status >= 0）
3. 计算每个客户当天的稼动率
4. 低于阈值且非终止状态的客户加入预警列表

---

## 九、客户对账单管理

### 9.1 r_customer_settlement（对账单主表）

**用途**：客户对账单主记录，一个客户一个对账周期生成一张。当前 52 行。

**核心字段**：

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| settlement_no | varchar(32) | 对账单编号（格式：DZ-YYYYMM-CCCCCCC-XX），**唯一** |
| customer_id | int unsigned | 客户ID |
| company_id | int unsigned | 公司ID |
| settle_month | varchar(7) | 对账月份（YYYY-MM） |
| settle_period | varchar(60) | 对账周期 |
| accounting_period | varchar(7) | 财务对账周期 |
| status | tinyint | **状态**：1已生成→20已上传→30已生成付款单→40已完成 |
| is_paid | tinyint | 是否已付款（0未/1已，含部分） |
| sale_user_id | int unsigned | 销售代表ID |
| equipment_rental_rate | decimal(5,2) | 设备租赁费比例(%)（快照） |
| tech_service_rate | decimal(5,2) | 技术服务费比例(%)（快照） |
| equipment_rental_tax_rate | decimal(5,2) | 设备租赁费税率(%)（快照） |
| tech_service_tax_rate | decimal(5,2) | 技术服务费税率(%)（快照） |
| footer_remark | varchar(500) | 页尾备注（快照） |
| discount_untaxed_amount | decimal(14,2) | 优惠后未税金额 |
| discount_taxed_equipment_fee | decimal(14,2) | 优惠后含税设备费 |
| discount_taxed_service_fee | decimal(14,2) | 优惠后含税服务费 |
| discount_taxed_total_amount | decimal(14,2) | 优惠后含税总额 |
| fields_show | text | 显示字段配置 |

**对账单状态机**：

```
已生成(1) → 已上传(20) → 已生成付款单(30) → 已完成(40)
```

### 9.2 r_customer_settlement_detail（对账单明细）

**用途**：对账单的工位级明细，每条对应一个工位一个月的核签数据。当前 36 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| customer_settlement_id | int unsigned | 对账单ID（0=未生成对账单） |
| customer_id / company_id | int unsigned | 客户/公司ID |
| work_station_id | int unsigned | 工位ID |
| robot_id | int unsigned | 机器人ID |
| product_id | int unsigned | 产品ID |
| settle_mode | tinyint | 计费方式（1计时/2计件/3UPH） |
| gmt_time | varchar(10) | 核签月份 |
| system_hour | decimal(10,2) | 系统工时 |
| settle_hour | decimal(10,2) | 核签工时 |
| minimum_hour | decimal(10,2) | 月保底工时 |
| actual_output | int | 件数 |
| untaxed_unit_price | decimal(10,2) | 未税单价 |
| taxed_unit_price | decimal(10,2) | 含税单价 |
| untaxed_amount | decimal(12,2) | 未税金额 |
| taxed_equipment_fee | decimal(14,2) | 含税设备租赁费 |
| taxed_service_fee | decimal(14,2) | 含税技术服务费 |
| taxed_total_amount | decimal(14,2) | 含税总金额 |
| discount_total_amount | decimal(14,2) | 优惠后总金额 |

### 9.3 r_customer_settlement_period_config（对账周期配置）

**用途**：配置客户对账的起止日。当前 12 行。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| company_id | int | 公司ID |
| customer_id | int | 客户ID |
| start_day | tinyint | 对账起始日（1-31） |
| end_day | tinyint | 对账截止日（1-31） |

### 9.4 r_customer_settlement_rate_config（费率配置）

**用途**：配置设备租赁费和技术服务费的比例与税率。当前 11 行。customer_id=0 表示全局默认。

| 字段 | 类型 | 含义 |
|------|------|------|
| id | int | 主键 |
| customer_id | int unsigned | 客户ID（0=全局默认） |
| equipment_rental_rate | decimal(5,2) | 设备租赁费比例(%)（默认15） |
| tech_service_rate | decimal(5,2) | 技术服务费比例(%)（默认85） |
| equipment_rental_tax_rate | decimal(5,2) | 设备租赁费税率(%)（默认13） |
| tech_service_tax_rate | decimal(5,2) | 技术服务费税率(%)（默认6） |
| config_status | tinyint | 配置状态（1正常/2停用） |

### 9.5 辅助表

| 表名 | 行数 | 用途 |
|------|------|------|
| r_customer_settlement_footer_remark | 9 | 页尾备注配置（customer_id=0为默认） |
| r_customer_settlement_config_log | 93 | 对账配置操作日志 |
| r_customer_settlement_discount_log | 48 | 优惠金额填写日志 |

### 9.6 关键 API（对账单）

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | /api/admin/customer-settlement/customer-options | 客户下拉（含客户池） |
| GET | /api/admin/customer-settlement/un-generated-list | 未生成对账单列表 |
| GET | /api/admin/customer-settlement/detail-show | 核签工位明细详情 |
| GET | /api/admin/customer-settlement/list | 已生成对账单列表 |

---

## 十、视图

| 视图名 | 行数 | 用途 |
|--------|------|------|
| r_view_customer_clue | 1700 | 线索视图，关联客户表查询保护期剩余天数 |
| r_view_customer_robot_count | 516 | 客户机器人数量统计 |
| r_view_customer_monthly_robot_count | 5696 | 客户月度机器人数量统计 |

---

## 十一、数据脱敏说明

客户模块启用了脱敏服务（`CustomerDesensitizeService`），根据用户权限对敏感数据（手机号、地址等）进行 ** 加密显示：

- `customer`：客户列表脱敏
- `customer_ground`：场地列表脱敏
- `customer_contact_person_list`：联系人列表脱敏

脱敏判断依据：`range_user_read`、`range_user_write`、`creator_user_id`、`salesman_user_id`、`marketing_user_id` 与当前用户 ID 的匹配关系。

---

## 十二、数据权限体系

每张核心业务表包含以下权限字段：

| 字段 | 用途 |
|------|------|
| range_user_read | 只读用户ID（逗号分隔） |
| range_user_write | 读写用户ID |
| range_role_read | 只读角色ID |
| range_role_write | 读写角色ID |
| range_department | 可读部门ID |
| range_company | 可读公司ID |
| company_id | 数据归属公司（多租户隔离） |
| creator_user_id | 创建人 |
| editor_user_id | 最后修改人 |

**权限层级**：用户级 > 角色级 > 部门级 > 公司级，支持四级数据范围控制。

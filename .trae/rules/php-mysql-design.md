---
alwaysApply: false
description: 触发条件：数据库设计、建表、字段设计、索引设计、Migration生成。仅负责数据库设计规范，不涉及查询操作。
---

# MySQL 数据库设计规范

---

## 一、TP6 领域规范

### 1.1 Model 映射

```php
protected $createTime = 'gmt_create';
protected $updateTime = 'gmt_modified';
protected $deleteTime = 'is_delete';
protected $defaultSoftDelete = 0;
```

### 1.2 软删除处理

- **模型查询（单表）**：使用 TP6 Model 查询，框架自动接管软删除
- **JOIN 查询（多表）**：使用 `join` 关联时，必须手动追加被关联表的 `is_delete=0` 条件

---

## 二、命名规范

### 2.1 表命名

| 规则 | 示例 |
|------|------|
| 全部小写，单词间用下划线分隔 | `r_delivery_plan` |
| 以 `r_` 前缀开头 | `r_order`、`r_customer` |
| 关联表以 `r_` 开头 + 两表名缩写 | `r_order_product` |
| 禁止使用 MySQL 保留字 | 不用 `order`、`group`、`select` |

### 2.2 字段命名

| 规则 | 示例 |
|------|------|
| 全部小写，单词间用下划线分隔 | `customer_name` |
| 布尔字段以 `is_` 开头 | `is_delete`、`is_active` |
| 日期字段以 `_date` 结尾 | `delivery_date` |
| 时间字段以 `_time` 结尾 | `create_time` |
| 外键以 `_id` 结尾 | `company_id`、`customer_id` |

---

## 三、必备字段

每张业务表必须包含以下字段：

```php
$table->addColumn('is_delete', 'integer', ['limit' => \Phinx\Db\Adapter\MysqlAdapter::INT_TINY, 'default' => 0, 'comment' => '是否删除', 'null' => false])
      ->addColumn('gmt_create', 'datetime', ['comment' => '创建时间', 'null' => false])
      ->addColumn('gmt_modified', 'datetime', ['comment' => '更新时间', 'null' => false])
      ->addColumn('company_id', 'integer', ['limit' => 11, 'signed' => false, 'default' => 0, 'comment' => '公司ID', 'null' => false])
```

---

## 四、字段类型规范

| 场景 | 推荐类型 | 说明 |
|------|----------|------|
| 主键 | `unsigned integer` | 自增ID |
| 金额 | `DECIMAL(12,2)` | 禁止用 FLOAT/DOUBLE |
| 状态/枚举 | `TINYINT` | 配合常量定义 |
| 短文本 | `VARCHAR(100-255)` | 名称、标题等 |
| 长文本 | `TEXT` | 描述、备注等 |
| 时间 | `DATETIME` | 格式 YYYY-MM-DD HH:MM:SS |
| 日期 | `DATE` | 格式 YYYY-MM-DD |
| 布尔 | `TINYINT(1)` | 0/1 |

---

## 五、索引规范

| 规则 | 说明 |
|------|------|
| 主键自动索引 | 无需手动添加 |
| 外键必加索引 | `idx_company_id` |
| 高频查询字段加索引 | WHERE 条件中频繁出现的字段 |
| 组合索引遵循最左前缀 | `idx_company_status` (company_id, status) |
| 索引命名 | `idx_字段名` 或 `idx_字段1_字段2` |
| 禁止给低基数字段加索引 | 如 `is_delete`、`status`（单独） |

---

## 六、底线约束

| 类型 | 规则 |
|:-----|:-----|
| **必须** | 表结构变更必须通过 ThinkPHP Migration，禁止原生 DDL |
| **必须** | 查询显式指定字段，禁止 `SELECT *` |
| **必须** | 表结构变更（新建表、加字段、改字段）后，同步更新对应模型类的 `@table` + `@property` 注释 |
| **禁止** | 物理删除数据，必须用 `is_delete` 软删除 |
| **禁止** | 在 Migration 中使用原生 SQL DDL |
| **注意** | 表名需加 `r_` 前缀（如 `r_customer`） |

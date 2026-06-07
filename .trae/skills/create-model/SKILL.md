---
name: "create-model"
description: "在 app/common/models 目录创建 ThinkPHP 模型类文件。当用户要求创建新模型、生成模型类或需要设置数据库模型时调用。"
---

# 创建模型 Skill

[系统级互斥锁] 严禁与其他重型技能（带流程制约的任务）并行执行。若侦测到当前上下文被要求同时运行其他技能，必须中止一切行为并要求用户降级为单线任务。

此技能帮助创建遵循项目约定和最佳实践的 ThinkPHP 6 模型类文件。

## 使用时机




在以下情况下调用此技能：
- 用户要求创建新的模型类
- 用户需要为数据库表生成模型
- 用户提到"创建模型"或类似的请求
- 用户想要设置新的数据实体

## 模型创建指南

### 1. 文件位置
- 所有模型文件应创建在：`app/common/models/`
- 文件命名：大驼峰命名法（例如：`UserOrder.php`）
- 类名应与文件名匹配

### 2. 基本结构

```php
<?php
namespace app\common\models;

/**
 * @mixin \think\Model
 */
class ModelName extends BaseModel
{
    // 状态常量
    const STATUS_ACTIVE = 1;
    const STATUS_INACTIVE = 0;
    
    const STATUS_MAP = [
        self::STATUS_ACTIVE => '启用',
        self::STATUS_INACTIVE => '禁用',
    ];
    
    // 如果表名与类名不同，则定义表名
    // protected $name = 'table_name';
    
    // 关联关系
    public function relationName()
    {
        return $this->hasOne(RelatedModel::class, 'foreign_key', 'local_key');
    }
    
    // 访问器
    public function getStatusTextAttr($value, $data): string
    {
        return self::STATUS_MAP[$data['status']] ?? '';
    }
}
```

### 3. 需要包含的关键特性

#### 常量
- 状态常量（从1开始，不从0开始）
- 类型常量
- 状态映射用于文本转换

#### 关联关系
- `hasOne()` - 一对一关系
- `hasMany()` - 一对多关系
- `belongsTo()` - 属于关系
- `belongsToMany()` - 多对多关系

#### 访问器
- 格式：`get{FieldName}Attr()`
- 通常用于状态文本、日期格式化等

### 4. 数据库表约定

创建模型时，相应的数据库表应包含：
- `id` - 主键（自增）
- `is_delete` - 软删除标记（默认：0）
- `gmt_create` - 创建时间
- `gmt_modified` - 更新时间
- `company_id` - 公司ID（用于多租户）

注意：数据库表有 `r_` 前缀，但模型类名不包含此前缀。

### 5. BaseModel 特性

所有模型都继承自 `BaseModel`，它提供：
- 自动时间戳管理（`gmt_create`、`gmt_modified`）
- 软删除功能（`is_delete`）
- 搜索过滤器支持
- 缓存管理
- 常用查询方法（`search()`、`getDataFromCache()` 等）

## 实施步骤

1. **收集信息**：询问用户：
   - 模型名称（大驼峰）
   - 表名（如果与约定不同）
   - 关键字段及其类型
   - 需要的关联关系
   - 需要的状态常量

2. **验证**：检查模型是否已存在

3. **生成代码**：创建包含以下内容的模型文件：
   - 正确的命名空间
   - 继承 BaseModel
   - 必需的常量
   - 关联关系
   - 访问器

4. **审查**：向用户展示生成的代码以供确认

## 重要注意事项

- 始终遵循现有项目约定
- 根据项目规则使用中文注释
- 常量应从1开始，不从0开始
- 包含适当本 PHPDoc 注释
- 参考现有模型以获取类似模式
- 确保通过 BaseModel 正确配置软删除

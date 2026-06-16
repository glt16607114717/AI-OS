# AI-OS MySQL 操作技能

## 功能描述

用于操作 AI-OS 项目的 MySQL 数据库，支持查询、插入、更新和删除操作。

## 使用场景

- 查询 AI-OS 数据库中的数据
- 修改配置数据
- 查看用户列表、LLM 日志等

## 参数说明

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| query | string | 是 | SQL 查询语句 |

## 支持的操作

### 查询操作
```sql
SELECT * FROM sys_user;
SELECT * FROM sys_llm_log ORDER BY id DESC LIMIT 10;
```

### 更新操作
```sql
UPDATE sys_god_rules SET rules = '新规则内容' WHERE id = 1;
UPDATE sys_user SET status = 0 WHERE id = 1;
```

### 插入操作
```sql
INSERT INTO sys_api_key (vendor_id, name, api_key, enabled) VALUES (1, '测试Key', 'sk-xxx', 1);
```

### 删除操作
```sql
DELETE FROM sys_llm_log WHERE ts < '2024-01-01';
```

## 数据库信息

| 项目 | 值 |
|------|------|
| 主机 | 124.221.220.89 |
| 端口 | 23306 |
| 数据库 | ai_os |
| 用户 | root |

## 注意事项

- 请谨慎使用 DELETE 和 UPDATE 操作
- 复杂查询建议使用 LIMIT 限制返回行数
- 不支持事务操作

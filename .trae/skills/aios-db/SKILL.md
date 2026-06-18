---
name: aios-db
description: AIOS系统数据库查询。当用户要求"查AIOS数据库"、"查AIOS的用户"、"查AIOS的日志"、"查AIOS配置表"、"AIOS的聊天记录"、"看看ai_os库"时触发。通过Python脚本查询AIOS系统的MySQL数据库（8.163.127.182:3306/ai_os），支持读写操作。
---

# AIOS 数据库查询

## 角色定位

通过 Python 脚本查询/写入 AIOS 系统的 MySQL 数据库。**支持读写操作。**

## 数据库信息

| 项目 | 值 |
|------|-----|
| Host | 8.163.127.182 |
| Port | 3306 |
| Database | ai_os |
| Username | root |
| Password | glt01054717@ |

## 核心表

| 表名 | 用途 |
|------|------|
| users | 用户表（用户名、密码哈希、角色、状态） |
| llm_logs | LLM 请求日志（prompt、completion、token用量、耗时） |
| chat_messages | 工作台聊天记录（角色、内容、会话ID） |
| ai_suggestions | AI优化建议（每日18:00自动生成） |
| skills | 技能注册表 |
| strategies | 模型路由策略 |
| god_rules | 上帝规则 |
| embeddings | 向量嵌入存储 |
| voice_logs | 语音日志 |

## 使用方式

```bash
python scripts/aios_db.py <params.json>
```

### params.json 格式

```json
{
  "sql": "SELECT * FROM users WHERE is_admin = 1",
  "read_only": true
}
```

### 参数说明

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| sql | string | (必填) | SQL语句 |
| read_only | bool | true | 是否只读模式（true时禁止写入操作） |

## 注意事项

1. **写入操作需明确指定 `read_only: false`**
2. 写入类SQL（INSERT/UPDATE/DELETE）执行前需向用户确认
3. 查询结果默认限制100行，避免返回过多数据
4. 敏感字段（如 password_hash）在结果中自动脱敏

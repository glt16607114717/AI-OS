---
name: aios-mysql
description: AIOS数据库MySQL查询。当用户要求"查AIOS数据库"、"查数据库"、"MySQL查询"、"查ai_os库"时触发。通过Python脚本查询AIOS系统的MySQL数据库（124.221.220.89:23306/ai_os），支持读写操作。
---

# AIOS MySQL 操作技能

## 角色定位

通过 Python 脚本操作 AIOS 系统的 MySQL 数据库。支持查询和写入。

## 数据库信息

| 项目 | 值 |
|------|-----|
| Host | 124.221.220.89 |
| Port | 23306 |
| Database | ai_os |
| Username | root |
| Password | glt01054717@ |

## 执行步骤

1. 用 Python 执行 `scripts/mysql_query.py`，参数为 SQL 语句
2. 支持 SELECT / INSERT / UPDATE / DELETE

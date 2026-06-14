---
name: aios-api-test
description: AIOS系统接口测试。当用户要求"测试AIOS接口"、"AIOS接口调试"、"测一下登录"、"验证AIOS API"、"AIOS接口联调"、"测一下AIOS的接口"时触发。通过Python脚本自动登录AIOS Go后端，获取JWT Token后调用目标接口，支持全部42个API端点。
---

# AIOS 接口测试技能

## 角色定位

AIOS Go 后端的接口测试工具。自动登录获取 Token，然后调用目标接口。

## 服务器信息

| 项目 | 值 |
|------|-----|
| API 地址 | `http://124.221.220.89:18731` |
| 管理员账号 | `桂良涛` / `admin123` |
| 认证方式 | JWT Token（Authorization Header） |

## 调用方式

**唯一方式**：文件传参。

```
1. Write → D:\wwwroot\ai-os\ai_cache\temp\aios_test_{时间戳}.json
2. RunCommand → python scripts/aios_api_test.py D:\wwwroot\ai-os\ai_cache\temp\aios_test_{时间戳}.json
```

### 参数说明

| 参数 | 必填 | 说明 |
|------|------|------|
| url | 是 | 接口路径，如 `/api/users` |
| method | 否 | GET/POST，默认 GET |
| params | 否 | 请求参数（JSON 对象），默认 {} |
| username | 否 | 登录账号，默认 admin |
| password | 否 | 登录密码，默认 admin123 |
| no_auth | 否 | 设为 true 时跳过登录（用于 /api/health 等公开接口） |
| raw_body | 否 | 设为 true 时 params 作为 raw body 发送（非 JSON） |

### 参数文件示例

```json
{
    "url": "/api/users",
    "method": "GET",
    "params": {"page": 1, "page_size": 10}
}
```

```json
{
    "url": "/api/users/create",
    "method": "POST",
    "params": {
        "username": "testuser",
        "password": "test123",
        "display_name": "测试用户"
    }
}
```

## 全部 API 端点

### 公开接口（无需登录）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/login` | 登录 |
| GET | `/api/health` | 健康检查 |
| GET | `/api/system/me` | 系统信息 |

### 登录用户接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/chat/history` | 聊天历史 |
| POST | `/api/chat/clear` | 清空聊天 |
| GET | `/api/ai-advisor/suggestions` | AI建议列表 |
| POST | `/api/ai-advisor/analyze` | 执行AI分析 |
| POST | `/api/ai-advisor/process` | 标记建议已处理 |
| GET | `/api/skills` | 技能列表 |
| GET | `/api/voice/status` | 语音状态 |
| POST | `/api/voice/add` | 添加语音指令 |
| POST | `/api/voice/update` | 更新语音指令 |
| POST | `/api/voice/delete` | 删除语音指令 |
| POST | `/api/voice/set-enabled` | 启停语音 |
| GET | `/api/stats/summary` | 统计摘要 |
| GET | `/api/stats/errors` | 近期错误 |
| POST | `/api/stats/cleanup` | 清理统计 |
| GET | `/api/logs` | 日志列表 |
| POST | `/api/logs/clear` | 清空日志 |
| GET | `/api/god-rules` | 上帝指令 |
| GET | `/api/quota/status` | 额度状态 |
| GET | `/api/llm/catalog` | 模型目录 |
| GET | `/api/llm/available-options` | 可用选项 |
| GET | `/api/llm/strategies` | 路由策略 |
| POST | `/api/logout` | 退出登录 |

### 管理员接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/users` | 用户列表 |
| POST | `/api/users/create` | 创建用户 |
| POST | `/api/users/update-password` | 修改密码 |
| POST | `/api/users/toggle-status` | 启停用户 |
| POST | `/api/users/toggle-admin` | 切换管理员 |
| POST | `/api/users/delete` | 删除用户 |
| POST | `/api/llm/vendor-keys` | 保存API Key |
| POST | `/api/llm/toggle-vendor` | 启停供应商 |
| POST | `/api/llm/save-strategy` | 保存策略 |
| POST | `/api/llm/delete-strategy` | 删除策略 |
| POST | `/api/llm/set-active-strategy` | 设置激活策略 |
| POST | `/api/god-rules/save` | 保存上帝指令 |
| POST | `/api/quota/set-enabled` | 启停额度监控 |
| POST | `/api/quota/force-check` | 强制检查额度 |

## 注意事项

1. 脚本自动登录获取 Token，无需手动获取
2. GET 请求用 query params，POST 用 JSON body
3. 公开接口可设 `no_auth: true` 跳过登录
4. 创建/删除用户的测试，测试完记得清理
5. 写入操作建议先查数据库确认数据正确

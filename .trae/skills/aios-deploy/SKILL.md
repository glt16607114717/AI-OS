---
name: aios-deploy
description: AIOS Go后端部署。当用户要求"部署后端"、"部署Go服务"、"编译部署"、"重启AIOS服务"、"部署到服务器"时触发。交叉编译Go代码为Linux二进制，上传到服务器，停止旧服务，启动新服务。
---

# AIOS Go 后端部署技能

## 角色定位

编译并部署 AI-OS Go 后端服务到云服务器。脚本内置了服务器地址、端口、路径等所有配置，**AI 只需执行脚本，不需要关心 SSH 细节。**

## 调用方式

```powershell
python D:\wwwroot\AI\AI-OS\.trae\skills\aios-deploy\scripts\deploy.py <action>
```

| action | 说明 |
|--------|------|
| `deploy` | 编译 + 上传 + 重启（默认） |
| `build` | 仅编译 |
| `restart` | 仅重启服务 |

## 注意事项

- 脚本自动完成：交叉编译 → 停止旧服务 → 上传 → 启动新服务 → 健康检查
- 部署失败会自动回滚到 .bak 版本
- 需要本地 Go 环境已配置

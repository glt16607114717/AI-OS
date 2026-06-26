---
name: aios-web-deploy
description: AIOS前端部署。当用户要求"部署前端"、"部署web"、"部署静态文件"、"部署前端到服务器"时触发。构建前端项目并上传到服务器。
---

# AIOS 前端部署技能

## 角色定位

构建 AI-OS Web 前端并部署到云服务器。脚本内置了服务器地址、端口、路径等所有配置，**AI 只需执行脚本，不需要关心 SSH 细节。**

## 调用方式

```powershell
python D:\wwwroot\AI\AI-OS\.trae\skills\aios-web-deploy\scripts\web_deploy.py <action>
```

| action | 说明 |
|--------|------|
| `deploy` | 构建 + 上传（默认） |
| `build` | 仅构建 |

## 注意事项

- 脚本自动完成：清理 dist → npm build → 上传到服务器
- 需要本地 Node.js 环境已配置
- 首次部署需确保服务器已安装 Nginx

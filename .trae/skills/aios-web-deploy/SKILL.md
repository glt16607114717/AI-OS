---
name: aios-web-deploy
description: AIOS前端部署。当用户要求"部署前端"、"部署web"、"部署静态文件"、"部署前端到服务器"时触发。构建前端项目并上传到服务器配置Nginx。
---

# AIOS 前端部署技能

## 角色定位

构建 AI-OS Web 前端并部署到云服务器，配置 Nginx 服务静态文件。

## 使用场景

- 更新前端代码后部署到服务器
- 修复前端 Bug 后快速部署
- 更新 UI 后重新部署

## 参数说明

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| action | string | 否 | 操作类型：build（仅构建）、deploy（构建+部署），默认 deploy |

## 执行步骤

1. 用 Python 执行 `scripts/web_deploy.py`，参数为 action（默认 deploy）
2. 脚本会自动定位 web 目录，无需手动指定路径

## 服务器信息

| 项目 | 值 |
|------|-----|
| 主机 | 8.163.127.182 |
| SSH端口 | 443 |
| 用户 | root |
| 前端部署路径 | /opt/ai-os/ai-os-web |
| Nginx 配置 | /etc/nginx/sites-available/ai-os |

## 注意事项

- 需要确保 Node.js 环境已配置
- 构建前会自动清理 dist 目录
- 部署后会自动重启 Nginx
- 确保服务器已安装 Nginx
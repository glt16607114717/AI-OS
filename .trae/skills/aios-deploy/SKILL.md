---
name: aios-deploy
description: AIOS Go后端打包部署。当用户要求"部署AIOS"、"打包Go后端"、"更新服务器"、"重启AIOS服务"、"部署后端"、"打包上传"、"go deploy"、"发布AIOS"时触发。自动完成Go交叉编译、SCP上传到服务器、重启systemd服务。
---

# AIOS Go 后端打包部署

## 角色定位

一键完成 Go 后端的交叉编译、上传、重启。

## 服务器信息

| 项目 | 值 |
|------|-----|
| SSH 地址 | ubuntu@124.221.220.89 |
| 服务名 | aios-server |
| 二进制路径 | /home/ubuntu/aios-server/aios-server |
| 服务端口 | 18731 |
| Go 源码路径 | d:\wwwroot\ai-os\go-backend |

## 部署流程

执行以下命令，脚本自动完成 3 步：

1. **本地交叉编译**：`GOOS=linux GOARCH=amd64 go build` → 生成 aios-server 二进制
2. **SCP 上传**：上传到服务器 `/home/ubuntu/aios-server/`
3. **重启服务**：`sudo systemctl restart aios-server`

## 使用方式

```bash
# 完整部署（编译+上传+重启）
python scripts/aios_deploy.py deploy

# 仅编译（不上传）
python scripts/aios_deploy.py build

# 仅重启服务
python scripts/aios_deploy.py restart

# 查看服务状态
python scripts/aios_deploy.py status

# 查看服务日志（最近50行）
python scripts/aios_deploy.py logs
```

## 注意事项

1. 编译前会自动执行 `go mod tidy`
2. 上传前会先停止服务，避免文件占用
3. 部署完成后自动检查服务状态和健康检查
4. 如果健康检查失败，会自动回滚到上一个版本（备份在 `.bak`）
5. 本地需要安装 Go 1.21+ 和 Python 3
6. SSH 免密登录需要已配置（公钥已上传到服务器）

---
name: docker-env
description: 管理Docker开发环境的完整解决方案，包括启动/停止/重启服务、查看状态和日志、排查问题、更新配置、重建镜像、PHP扩展管理等所有环境相关的操作。当用户提到"Docker"、"环境挂了"、"服务不工作"、"重启环境"、"查看日志"、"容器问题"、"端口占用"、"nginx配置"、"php配置"、"docker-compose"、"安装扩展"、"添加扩展"、"PHP扩展"、"docker-compose build"、"rebuild"、"扩展管理"或需要管理开发环境时触发此技能。
---

# Docker 开发环境管理技能

## 环境配置信息

**环境位置**：`D:\wwwroot\.dev-env-docker`

**服务组件**：

| 容器 | 服务 | 端口 |
|------|------|------|
| rmp-nginx | Nginx | 80 |
| rmp-php | PHP-FPM | 9000 |
| rmp-redis | Redis | 6379 |

**文件挂载**：

| 宿主机路径 | 容器路径 | 用途 |
|-----------|---------|------|
| `D:/wwwroot` | `/var/www/html` | 项目代码实时同步 |
| `./nginx/conf.d` | `/etc/nginx/conf.d` | 虚拟主机配置 |
| `./nginx/nginx.conf` | `/etc/nginx/nginx.conf` | Nginx 主配置 |
| `./php/php.ini` | `/usr/local/etc/php/php.ini` | PHP 配置 |

**自动启动**：Docker Desktop 开机自启 + 容器 `restart: unless-stopped`

## 配置文件位置

| 文件 | 路径 |
|------|------|
| Docker 编排配置 | `D:\wwwroot\.dev-env-docker\docker-compose.yml` |
| Nginx 主配置 | `D:\wwwroot\.dev-env-docker\nginx\nginx.conf` |
| Nginx 虚拟主机配置 | `D:\wwwroot\.dev-env-docker\nginx\conf.d\*.conf` |
| PHP 配置 | `D:\wwwroot\.dev-env-docker\php\php.ini` |
| PHP Dockerfile | `D:\wwwroot\.dev-env-docker\php\Dockerfile` |
| 环境启动脚本 | `D:\wwwroot\.dev-env-docker\start-env.ps1` |

## 项目访问地址

| 项目 | 地址 |
|------|------|
| rmp-api | http://rmp-api.me/admin.php |
| rmp-api-a | http://rmp-api-a.me/admin.php |
| rmp-api-b | http://rmp-api-b.me/admin.php |
| rmp-api-c | http://rmp-api-c.me/admin.php |
| charts-api | http://charts-api.me/index.php |
| mcp | http://mcp.me/mcp |

## 核心操作流程

### 环境管理

```
启动：cd D:\wwwroot\.dev-env-docker && docker-compose up -d
停止：docker-compose down
重启：docker-compose restart
状态：docker ps
```

### 配置变更流程

| 变更类型 | 操作 |
|---------|------|
| 修改 Nginx 配置 | 改文件 → `docker exec rmp-nginx nginx -t` → `docker restart rmp-nginx` |
| 修改 PHP 配置 | 改 php.ini → `docker restart rmp-php` |
| 修改 docker-compose.yml | 改文件 → `docker-compose up -d` |
| 修改 Dockerfile | 改文件 → `docker-compose build php` → `docker-compose up -d php` |
| 添加新服务 | 改 docker-compose.yml → `docker-compose up -d --build` |

### PHP 扩展管理

核心流程：编辑 Dockerfile → `docker-compose build php` → `docker-compose up -d php` → `docker exec rmp-php php -m` 验证

> ⚠️ 新增扩展应**新建独立 RUN 层**，不要修改已有的 RUN 命令，否则缓存全部失效。
>
> 详细操作见 [references/php-extensions.md](references/php-extensions.md)，包括扩展类型、Dockerfile 写法、构建场景对比、最佳实践等。

### 故障排查

1. `docker ps` 确认容器状态 → 2. `docker logs <container>` 查看错误 → 3. 按需重启或重建

> 详细排查步骤和常见问题解决方案见 [references/troubleshooting.md](references/troubleshooting.md)。

## 脚本工具

技能目录 `scripts/` 下的自动化脚本：

| 脚本 | 用途 |
|------|------|
| `env_status.ps1` | 检查所有服务运行状态和健康状况 |
| `env_restart.ps1` | 重启整个开发环境 |
| `env_logs.ps1` | 查看所有服务日志 |
| `fix_permissions.ps1` | 修复文件权限问题 |

## 详细操作

详细操作见 `references/` 目录下的对应文件：

- **[PHP 扩展管理](references/php-extensions.md)**：扩展类型、Dockerfile 示例、构建场景、最佳实践
- **[故障排查与 Q&A](references/troubleshooting.md)**：常见问题解决方案、完整排查流程

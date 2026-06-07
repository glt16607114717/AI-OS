# 常见问题 Q&A 与故障排查

## 常见问题

### 1. 服务无法访问

**症状**：访问网站时出现"连接被拒绝"、"连接被意外关闭"等错误

**排查步骤**：

1. 检查 Docker 是否运行：`docker ps`
2. 检查容器状态：所有容器应该显示 "Up" 状态
3. 检查端口占用：`netstat -ano | findstr :80`
4. 查看容器日志：
   - Nginx：`docker logs rmp-nginx`
   - PHP：`docker logs rmp-php`
   - Redis：`docker logs rmp-redis`

**解决方案**：

- Docker 未运行 → 启动 Docker Desktop
- 容器停止 → `cd D:\wwwroot\.dev-env-docker && docker-compose up -d`
- 端口被占用 → 找到占用进程并结束，或修改端口
- 容器启动失败 → 查看日志，根据错误信息修复

### 2. PHP 配置修改不生效

**症状**：修改 php.ini 后 PHP 行为没有变化

**原因**：PHP 容器内配置文件被挂载，但需要重启 PHP 容器

**解决方案**：`docker restart rmp-php`

### 3. Nginx 配置修改不生效

**症状**：修改 nginx.conf 或虚拟主机配置后 Nginx 行为没有变化

**解决方案**：`docker restart rmp-nginx`

### 4. 容器频繁重启

**症状**：容器状态显示 "Restarting"

**排查步骤**：

1. 查看容器日志：`docker logs <container-name>`
2. 检查配置文件语法是否正确
3. 检查端口是否冲突
4. 检查挂载目录是否可访问

### 5. 文件修改不生效

**症状**：修改代码文件后访问网站看不到变化

**原因**：可能是 PHP 缓存或浏览器缓存

**解决方案**：

- 清除 PHP OPcache：`docker exec rmp-php php -r "opcache_reset();"`
- 清除浏览器缓存
- 检查文件是否真的修改成功

## 故障排查流程

1. **用户报告问题**：收集详细信息（错误信息、访问的 URL、复现步骤）
2. **检查容器状态**：`docker ps` 确认所有容器是否正常运行
3. **查看日志**：`docker logs <container>` 查找错误信息
4. **检查端口**：确认端口是否被占用
5. **测试基本连接**：测试各个域名是否可访问
6. **检查配置文件**：确认配置文件语法正确
7. **重启服务**：尝试重启问题服务
8. **重建容器**：如果问题持续，尝试重建容器
9. **检查文件挂载**：确认挂载目录可访问
10. **提供解决方案**：根据错误信息提供具体解决方案

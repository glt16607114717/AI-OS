# 错误处理参考

本文件定义技能执行中可能遇到的异常场景及其自动恢复策略。仅在遇到具体错误时读取。

所有错误优先自动恢复，仅在无法自动恢复时才报告用户。

---

## 异常场景与恢复策略

| 场景 | 问题 | 自动处理方式 |
|------|------|-------------|
| 站点初始化 | `docs/index.html` 不存在 | 读取 `references/init-templates.md`，替换 `{SITE_NAME}` 后创建 |
| 资源下载 | curl 下载失败或文件为空 | 重试一次，仍然失败则报告网络问题 |
| 资源下载 | `docs/assets/` 目录不存在 | 自动 `mkdir -p docs/assets` |
| 配置修复 | `relativePath` 未启用 | 自动补充 `relativePath: true` |
| 站点名称 | `window.$docsify.name` 不一致 | 自动替换为 Step 1.2 解析结果 |
| 服务启动 | 端口被占用 | 自动终止旧进程，失败则自动递增端口号 |
| 服务启动 | `docsify-cli` 未安装 | 自动 `npx docsify-cli` 下载 |
| 局域网分享 | 获取不到局域网 IP | 自动降级为仅输出 localhost 访问地址 |
| 导航同步 | `_sidebar.md` 或 `_navbar.md` 不存在 | 由 Step 3 根据目录结构动态生成，无需模板 |
| 索引生成 | 子目录缺少 README.md | 按模板自动生成，显示名按命名规则推导 |

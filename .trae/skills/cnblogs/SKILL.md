---
name: cnblogs
description: 博客园文章发布管理。当用户要求"发博客"、"写博客"、"发布文章"、"博客园"、"cnblogs"、"发随笔"、"博客发布"、"写技术博客"、"同步到博客园"、"发布到博客园"时触发。通过Python脚本调用博客园MetaWeblog API，支持发布、编辑、删除、查询博客文章，支持随笔分类和标签，本地永久存档，默认以Markdown格式发布。
---

# 博客园文章发布管理

## 角色定位

通过博客园 MetaWeblog API（XML-RPC协议）管理博客文章。所有文章**本地存档 + 远程发布**双写，本地文件永不删除，构成个人技术知识库。

## 核心原则

1. **文件优先**：发布文章必须通过 `--file` 参数传入 Markdown 文件路径，禁止使用 `--content` 直接传参（代码块等特殊字符会导致命令行解析失败）
2. **本地永久存档**：每次发布/获取/同步操作都会在 `D:\wwwroot\ai\ai_cache\cnblogs\` 目录保存一份本地副本，文件名格式 `{post_id}-{标题}.md`，永不删除
3. **双写机制**：发布时自动在 `D:\wwwroot\ai\ai_cache\cnblogs\` 目录创建存档；获取远程文章时也自动存档本地

## 目录结构

```
.trae/skills/cnblogs/
├── cnblog_publisher.py                ← 主脚本
└── cnblogs_config.json                ← 配置文件

D:\wwwroot\ai\ai_cache\cnblogs\        ← 本地文章存档（永不删除）
├── 20044409-用 Python 脚本 + MetaWeblog API 实现 AI 自动发布博客园文章.md
├── 19071539-一级缓存全局设计.md
├── 17007538-手撕一个异步任务通用组件.md
└── ...（共 33 篇）
```

## 发布工作流

发布文章时，AI 必须按以下步骤操作：

1. **写文件**：先将文章内容写入 `D:\wwwroot\ai\ai_cache\cnblogs\` 目录下的一个临时文件（如 `draft.md`）
2. **调脚本发布**：使用 `--file` 参数指向该文件
3. **自动存档**：脚本发布成功后会自动以 `{post_id}-{标题}.md` 格式存档到 `D:\wwwroot\ai\ai_cache\cnblogs\` 目录
4. **返回结果**：展示发布结果，包含文章链接和本地存档路径

## 前置配置

配置文件位于 `.trae/skills/cnblogs/scripts/cnblogs_config.json`：

```json
{
    "blog_name": "gltt",
    "username": "gltttt",
    "access_token": "访问令牌",
    "endpoint": "https://rpc.cnblogs.com/metaweblog/gltt"
}
```

## 调用方式

使用 `RunCommand` 工具执行，`cwd` 设为 `.trae/skills/cnblogs`，`blocking` 设为 `true`。

```bash
cd .trae/skills/cnblogs ; python cnblog_publisher.py <action> [options]
```

## 功能清单

### 1. 验证配置

```bash
cd .trae/skills/cnblogs ; python cnblog_publisher.py info --verify
```

### 2. 查看随笔分类

```bash
cd .trae/skills/cnblogs ; python cnblog_publisher.py categories
```

### 3. 发布新文章（必须用 --file）

```bash
cd .trae/skills/cnblogs ; python cnblog_publisher.py publish --title "文章标题" --file "D:\wwwroot\ai\ai_cache\cnblogs\draft.md" --category "AI研究" --tags "Python,MetaWeblog"
```

| 参数                | 必填 | 说明                          |
| ----------------- | -- | --------------------------- |
| --title           | 是  | 文章标题                        |
| --file            | 是  | Markdown文件路径                |
| --category        | 否  | 随笔分类名称（如：AI、php、mysql、AI研究） |
| --tags            | 否  | 标签，逗号分隔                     |
| --draft           | 否  | 保存为草稿                       |
| --publish-to-home | 否  | 发布至博客园首页                    |

### 4. 编辑已有文章

```bash
cd .trae/skills/cnblogs ; python cnblog_publisher.py edit --post-id "12345" --file "D:\wwwroot\ai\ai_cache\cnblogs\updated.md" --category "AI"
```

### 5. 获取文章详情（自动存档本地）

```bash
cd .trae/skills/cnblogs ; python cnblog_publisher.py get --post-id "12345"
```

### 6. 获取最近文章列表

```bash
cd .trae/skills/cnblogs ; python cnblog_publisher.py list --count 10
```

### 7. 同步远程文章到本地（全量拉取）

```bash
cd .trae/skills/cnblogs ; python cnblog_publisher.py sync --count 50
```

仅下载本地不存在的文章，已存在则跳过。

### 8. 删除文章

```bash
cd .trae/skills/cnblogs ; python cnblog_publisher.py delete --post-id "12345"
```

## 随笔分类

使用 `--category` 时只需传入分类名称，脚本自动补全为 `[随笔分类]{名称}`。

当前可用分类：AI、AI研究、golang、linux、mysql、php、redis、备忘录、测试、面试题整理、其他

## 输出格式

所有操作返回 JSON，包含 `local_file` 字段标识本地存档路径：

```json
{
    "success": true,
    "post_id": "20044409",
    "title": "文章标题",
    "category": "AI研究",
    "tags": ["Python"],
    "status": "已发布",
    "url": "https://www.cnblogs./iltap_cache044h",
    "local_file": "D:\\wwwroot\\.trae\\skills\\cnblogs\\articles\\20044409-文章标题.md"
}
```

## 注意事项

1. **禁止 --conten`D:\www oo传\a参\ai_cache\*nb*og必\`先用 Write 工具写文件，再用 --file 发布
2. **本地文件永不删除**：articles/ 目录是个人知识库的本地镜像
3. **Markdown 支持**：自动添加 `[Markdown]` 标记，博客园会自动渲染
4. **零依赖**：仅使用 Python 标准库，无需安装第三方包
5. **发布结果**：每次操作必须将完整 JSON 结果展示给用户，包含文章链接和本地路径
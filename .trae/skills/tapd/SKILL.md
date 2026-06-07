---

name: tapd
description: TAPD项目管理技能。当用户要求"查TAPD"、"TAPD需求"、"Bug跟踪"、"缺陷管理"、"查Bug"、"Bug状态"、"查需求"、"查任务"、"TAPD评论"、"获取图片"、"TAPD状态"时触发。不触发场景：收集需求和写文档任务。
-------------------------------------------------------------------------------------------------------------------------------------

# TAPD 项目管理

## 调用方式

**通过 Python 脚本调用，不再依赖 MCP 服务。** 脚本目录：`.trae/skills/tapd/`

**唯一方式**：通过文件传参，AI 将 JSON 参数写入临时文件后传给脚本。

```
1. Write → D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json
2. RunCommand → python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py <action> D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json
```

**查看全部可用 action**：

```powershell
python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py help
```

## 固定配置

| 配置项           | 值                                                            |
| ------------- | ------------------------------------------------------------ |
| 项目名称          | RMP管理平台                                                      |
| workspace\_id | 66680814                                                     |
| 当前用户          | 桂良涛                                                          |
| TAPD项目地址      | <https://www.tapd.cn/66680814>                               |
| Bug列表         | <https://www.tapd.cn/66680814/bugtrace/bugs/views>           |
| 脚本路径          | D:\wwwroot\.trae\skills\tapd\scripts\tapd.py           |
| 配置文件          | D:\wwwroot\.trae\skills\tapd\scripts\tapd\_config.json |

## 核心原则

1. **只查自己的** — 默认查桂良涛的 Bug 和任务，按创建时间倒序
2. **Git 提交后才操作 TAPD** — Bug 修复后先 git commit/push，再变更 TAPD 状态
3. **流转规则** — 修复后交给 reporter（创建人）验证，不要随意改给别人
4. **状态流转前先查规则** — 不确定能流转到什么状态时，先调用 `get_workflows_all_transitions`

***

## Bug 操作规范

### 强制规则

每次改 Bug 时，必须重新调用 `get_bugs` 获取最新详情和 `get_comments` 获取所有评论，**禁止仅参考上下文缓存的信息**。Bug 状态随时可能变化（如被测试打回），缓存信息不可靠。

获取 Bug 详情时必须包含 `status`、`flows`、`resolution` 字段。

### 打回 Bug 识别

**识别方法**：

1. `status` = `reopened` → 打回 Bug
2. `flows` 包含 `resolved|reopened` 路径 → 打回 Bug
3. `resolution` 可能仍为 `fixed`，但这不代表已解决

**处理方式**：

- **绝不认为已解决**：只要 status=reopened 就是未解决，不管 resolution
- **忽略主描述**：打回 Bug 的主描述已过时，真实信息在评论中
- **看最新评论**：测试打回时会在评论中说明原因（如"问题仍存在"、"部分修复"）
- **重新排查根因**：不复用上次修复方案
- **Git 提交**：仍用 `--bug=bug_id`，描述中注明"二次修复"

### 状态流转

| 场景      | 状态流转                               | 处理人变更       |
| ------- | ---------------------------------- | ----------- |
| 首次修复    | new → in\_progress → resolved      | 转给 reporter |
| 打回后二次修复 | reopened → in\_progress → resolved | 转给 reporter |

### 图片识别

**必须先判断 Bug 类型，再决定从哪里取截图，禁止无差别批量下载**：

| 场景                      | 图片来源               | 关键操作                                                                              |
| ----------------------- | ------------------ | --------------------------------------------------------------------------------- |
| 首次 Bug（status=new）      | Bug 描述 description | 从 description HTML 提取图片路径 → `get_image` 获取下载链接 → 直接用 URL 调用 `img-ocr` 远程识别 |
| 打回 Bug（status=reopened） | **最新评论，不下载主描述截图**  | 先调用 `get_comments`，只从最新评论中提取图片                                                    |

**步骤**：从 HTML 中正则提取 `/tfl/captures/xxx.png` → `get_image` 获取下载 URL → **立即**用 URL 直接调用 `img-ocr` 远程识别。禁止下载到本地。下载链接有效期 300 秒，获取后必须在有效期内立即识别，不可缓存 URL 延后使用。没有图片则跳过。

***

## 功能清单

### 1. 查询我的待办 Bug

```json
{"options": {"current_owner": "桂良涛", "fields": "id,title,status,priority_label,severity,reporter,current_owner,created,description", "status": "new|in_progress|reopened", "limit": 20}}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py get_bugs D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

**status 常用值**：`new`（新建）、`in_progress`（处理中）、`reopened`（重新打开）、`resolved`（已解决）、`closed`（已关闭）
去掉 status 查所有状态的 Bug。

**severity 严重程度**：fatal（致命）、serious（严重）、normal（一般）、prompt（提示）、advice（建议）
**priority\_label 优先级**：urgent（紧急）、high（高）、medium（中）、low（低）

### 2. 查询我的待办任务

```json
{"options": {"entity_type": "tasks", "owner": "桂良涛", "fields": "id,name,status,priority_label,owner,begin,due,created", "limit": 20}}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py get_stories_or_tasks D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

**entity\_type**：`tasks`（任务）、`stories`（需求）
**任务状态**：open（未开始）、progressing（进行中）、done（已完成）

### 3. 修改 Bug 状态

```json
{"options": {"id": "Bug的ID", "v_status": "已解决", "current_owner": "创建人的名字;"}}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py update_bug D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

**关键规则**：

- `v_status` 支持中文：新建、处理中、已解决、已关闭、已拒绝
- `current_owner` 要加分号结尾，如 `"侯森;"`
- 修复后流转"已解决"，处理人改回给 Bug 的 reporter（创建人）
- 不确定能流转到什么状态？先调 `get_workflows_all_transitions` 查询

### 4. 添加评论

```json
{"options": {"entry_id": "Bug或需求的ID", "entry_type": "bug", "author": "桂良涛", "description": "评论内容"}}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py create_comment D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

**entry\_type**：bug、stories、tasks

**Bug 修复评论格式**：

```
【修复内容】简述修复了什么
【问题原因】根因分析
【解决方案】怎么修的
【验证结果】测试结果
```

### 5. 获取评论

```json
{"options": {"entry_id": "Bug或需求的ID", "entry_type": "bug", "limit": 20}}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py get_comments D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

### 6. 获取图片下载链接

```json
{"options": {"image_path": "/tfl/captures/2026-04/tapd_xxx.png"}}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py get_image D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

图片路径从 Bug 的 description 或评论的 description 中提取（HTML img 标签的 src 属性）。
下载链接有效期 300 秒，获取后立即用 URL 直接调用 `img-ocr` 远程识别，禁止下载到本地。

### 7. 获取附件

```json
{"options": {"entry_id": "Bug或需求的ID", "type": "bug"}}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py get_attachments D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

**type**：bug、story

### 8. 查询工作流状态（辅助）

**查看状态中英文映射**：

```json
{"options": {"system": "bug", "workitem_type_id": "1"}}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py get_workflows_status_map D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

**查看当前状态能流转到哪些状态**：

```json
{"options": {"system": "bug", "workitem_type_id": "1"}}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py get_workflows_all_transitions D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

**system**：bug 或 story

### 9. 查询自定义字段配置（辅助）

使用自定义字段（custom\_field\_\*）前必须先调用：

```json
{"options": {"entity_type": "stories"}}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py get_entity_custom_fields D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

**entity\_type**：stories、tasks、iterations、tcases

### 10. 查询需求类别（辅助）

```json
{}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py get_workitem_types D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

### 11. 发送企微消息

```json
{"msg": "Markdown格式的消息内容"}
```

**调用**：`python D:\wwwroot\.trae\skills\tapd\scripts\tapd.py send_qiwei_message D:\wwwroot\ai\ai_cache\temp\tapd_{时间戳}.json`

***

## 扩展工具（原 MCP 未暴露，现已全部支持）

以下工具在原 MCP 中未暴露为工具，现在通过脚本全部可用：

| action                        | 说明               |
| ----------------------------- | ---------------- |
| `get_stories`                 | 原始需求查询（不带智能名称解析） |
| `create_or_update_story`      | 原始创建/更新需求        |
| `get_tasks`                   | 原始任务查询           |
| `create_or_update_task`       | 原始创建/更新任务        |
| `get_task_count`              | 获取任务数量           |
| `get_stories_custom_fields`   | 需求自定义字段配置        |
| `get_stories_fields_info`     | 需求字段及候选值         |
| `get_related_bugs`            | 需求关联缺陷           |
| `get_bug_custom_fields`       | 缺陷自定义字段配置        |
| `get_attachment_download_url` | 单独获取附件下载链接       |
| `get_iterations`              | 获取迭代列表           |
| `create_or_update_iteration`  | 创建/更新迭代          |
| `get_workflows_last_steps`    | 获取工作流结束状态        |
| `get_story_categories`        | 获取需求分类           |
| `get_tcases`                  | 获取测试用例           |
| `create_tcases`               | 新建测试用例           |
| `create_tcases_batch`         | 批量新建测试用例         |
| `get_tcases_count`            | 获取测试用例数量         |
| `get_tcases_custom_fields`    | 测试用例自定义字段配置      |
| `get_wikis`                   | 获取 Wiki 列表       |
| `create_wiki`                 | 新建 Wiki          |
| `get_wiki_count`              | 获取 Wiki 数量       |
| `get_workspace_info`          | 获取项目信息           |
| `get_user_info`               | 获取当前用户信息         |
| `get_todo`                    | 获取用户待办           |
| `get_user_story_todo`         | 获取用户待办需求         |
| `get_user_bug_todo`           | 获取用户待办缺陷         |
| `get_user_task_todo`          | 获取用户待办任务         |
| `update_timesheets`           | 新建/更新工时          |
| `get_timesheets`              | 获取工时             |
| `get_scm_copy_keywords`       | 获取源码提交关键字        |
| `get_release_info`            | 获取发布计划           |
| `add_entity_relations`        | 创建需求-缺陷关联        |

***

## 常见工作流

### Bug 修复全流程

```
查Bug → 定位代码 → 改代码 → git提交 → 变更Bug状态为"已解决" → 处理人改回reporter → 添加修复评论
```

### 查看 Bug 详情（含截图）

```
查Bug → 读description提取图片路径 → get_image获取下载链接 → 直接用URL远程OCR识别图片内容
```

### 查需求关联的 Bug

```
查需求评论 → 从评论中找到关联的 Bug ID → 查 Bug 详情
```


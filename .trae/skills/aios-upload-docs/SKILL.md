---
name: "aios-upload-docs"
description: "批量上传本地文档到 AI-OS 知识库。当用户要求上传文档、导入PRD、批量向量化本地文件到知识库时触发。"
---

# AI-OS 知识库文档上传

批量上传本地 Markdown / PDF / 代码文件到 AI-OS 知识库，自动分块 + 向量化入库。

## 脚本位置

```
d:\wwwroot\ai-os\.trae\skills\aios-upload-docs\scripts\upload_docs.py
```

## 调用方式

```bash
python d:\wwwroot\ai-os\.trae\skills\aios-upload-docs\scripts\upload_docs.py --dir <目录路径> [--exclude <排除模式>]
```

### 参数说明

| 参数 | 必填 | 说明 |
|------|------|------|
| `--dir` | 是 | 要上传的目录路径 |
| `--exclude` | 否 | 排除的文件名模式（逗号分隔），默认排除 `README.md` |

### 输出格式示例

**成功：**
```
登录成功
共发现 64 个文档（排除了 README.md）
  [OK] 01-个人事务\xxx.md → 11个分块 (22158字符)
  [OK] 02-CRM\yyy.md → 8个分块 (15400字符)
  ...
完成: 成功=52, 失败=12
```

**失败：**
```
  [NG] xxx.md: 向量化入库失败: embedding API 返回错误 400: ...
```

## AI 调用注意事项

### 典型调用步骤
1. 先审查目录，排除 README.md 等无价值文件
2. 执行上传脚本
3. 如有失败，单独重试失败文件（加大 timeout）
4. 上传完成后清理临时脚本

### 成功/失败判断
- 脚本 exit code = 0 且输出含 "完成: 成功=N" → 成功
- 输出含 "embedding API 返回错误 400" → 分块超长，需检查分块逻辑
- 输出含 "ConnectTimeout" → 网络超时，单独重试

### 已知坑点

1. **分块超长**：单个 markdown section 超过 MaxSize(2000) 时会被二次拆分，但极端情况（单段超 2000 字符）会硬切。embedding API 单条限制约 2000 字符
2. **Embedding 批量**：10 条/批，失败逐条重试。不要改大批量
3. **同名覆盖**：相同文件名会先归档旧版本再插入新版本
4. **内容去重**：SHA-256 去重，相同内容不会重复入库
5. **超时设置**：大文件（>30KB）建议 timeout=600 秒
6. **登录凭证**：脚本内置 `桂良涛 / glt01054717`，不要改
7. **API 地址**：`http://8.163.127.182:18731`，不要改
8. **支持格式**：.md .txt .pdf .docx .xlsx .go .py .js .ts .vue .java .c .cpp .h .rs .sql .yaml .yml .json .xml .html .css .sh .bat .ini .cfg .toml
9. **最小内容**：文件至少 50 字符，否则拒绝
10. **分块策略**：Markdown 按 ##/### 标题切分 + 超长段落二次拆分，MinSize=200, MaxSize=2000

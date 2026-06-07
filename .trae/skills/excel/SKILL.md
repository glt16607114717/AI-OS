---
name: excel
description: Excel文件处理。当用户要求"读取Excel"、"导出Excel"、"看Excel"、"Excel转JSON"、"导出数据"时触发。支持读取Excel内容为JSON和将JSON数据导出为Excel文件。
---

# Excel 文件处理

## 角色定位

读取和导出 Excel 文件，通过 Python 脚本执行。

---

## 功能一：读取 Excel

将 Excel 文件内容读取为 JSON 格式输出。

### 调用方式

```powershell
cd D:\wwwroot\.trae\skills\excel ; python excel_read.py <Excel文件> [选项]
```

### 参数说明

| 参数 | 必填 | 说明 |
|------|------|------|
| Excel文件路径 | 是 | 文件的绝对路径 |
| --sheet | 否 | 指定工作表名（默认第一个） |
| --limit | 否 | 限制读取行数（默认全部） |
| --headers-only | 否 | 只输出表头信息 |

### 示例

```powershell
# 读取全部数据
cd D:\wwwroot\.trae\skills\excel ; python excel_read.py D:\data\report.xlsx

# 只看前10行
cd D:\wwwroot\.trae\skills\excel ; python excel_read.py D:\data\report.xlsx --limit 10

# 只看表头
cd D:\wwwroot\.trae\skills\excel ; python excel_read.py D:\data\report.xlsx --headers-only

# 指定工作表
cd D:\wwwroot\.trae\skills\excel ; python excel_read.py D:\data\report.xlsx --sheet "Sheet2"
```

---

## 功能二：导出 Excel

将 JSON 数据导出为 Excel 文件。

### 调用方式（固定2步）

```
第1步：将配置写入 D:\wwwroot\ai\ai_cache\excel\export_config.json
第2步：cd D:\wwwroot\.trae\skills\excel ; python excel_export.py export_config.json
```

### 配置文件格式

```json
{
    "filename": "导出文件名（可选，默认"导出文件"）",
    "headers": ["列名1", "列名2", "列名3"],
    "data": [
        {"列名1": "值1", "列名2": "值2", "列名3": "值3"},
        {"列名1": "值4", "列名2": "值5", "列名3": "值6"}
    ]
}
```

### 字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| headers | 是 | 列名数组，定义 Excel 的表头 |
| data | 是 | 数据数组，每个对象的 key 对应 headers |
| filename | 否 | 导出文件名（不含扩展名），默认"导出文件" |

### 示例

```json
{
    "filename": "客户列表",
    "headers": ["客户名称", "联系人", "电话"],
    "data": [
        {"客户名称": "长安MINI", "联系人": "张三", "电话": "13800138001"},
        {"客户名称": "测试公司", "联系人": "李四", "电话": "13900139001"}
    ]
}
```

导出文件保存在 `D:\wwwroot\ai\ai_cache\excel\` 目录下。

## 注意事项

1. 读取时文件路径必须是绝对路径
2. 导出的 Excel 文件在 `D:\wwwroot\ai\ai_cache\excel\` 目录下
3. 大文件建议用 --limit 限制行数，避免输出过多

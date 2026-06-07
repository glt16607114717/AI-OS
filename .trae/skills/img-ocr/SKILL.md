---
name: img-ocr
description: 图片OCR文字识别。当用户要求"识别图片"、"图片文字"、"OCR"、"截图识别"、"图片转文字"、"识别验证码"、"读取截图"时触发。支持本地图片和网络图片，基于RapidOCR（PaddleOCR轻量封装），完全免费。
---

# 图片 OCR 文字识别

## 角色定位

识别图片中的文字内容，支持本地图片和网络图片。用于读取截图、识别接口返回数据、提取图片中的文字信息。

## 调用方式

通过 Python 脚本执行：

```bash
cd .trae/skills/img-ocr ; python img_ocr.py <图片路径或URL>
```

### 参数说明

| 参数 | 必填 | 说明 |
|------|------|------|
| 图片路径 | 是 | 本地文件路径或 HTTP/HTTPS URL |
| --format | 否 | text（默认，纯文本）/ json（含坐标和置信度） |
| --lang | 否 | ch（默认，中英文混合）/ en（英文）/ ch_cht（繁体中文） |
| --no-detect | 否 | 跳过文字检测，直接整图识别（适用于纯文字截图） |

### 示例

```bash
# 本地图片，纯文本输出
cd .trae/skills/img-ocr ; python img_ocr.py D:\screenshots\bug.png

# 网络图片，JSON输出
cd .trae/skills/img-ocr ; python img_ocr.py https://example.com/image.png --format json

# 英文图片
cd .trae/skills/img-ocr ; python img_ocr.py test.png --lang en
```

## 典型使用场景

### 场景一：识别 Bug 截图

用户提供 Bug 截图，AI 识别其中的错误信息、接口地址、堆栈信息等。

1. 用户截图保存到本地或提供路径
2. AI 执行 OCR 识别
3. 根据识别结果定位问题

### 场景二：识别 TAPD 图片

Bug/需求评论中有图片，通过 TAPD MCP 获取图片下载链接后 OCR 识别。

1. 调用 TAPD `get_image` 获取图片下载 URL
2. 用 OCR 脚本识别该 URL
3. 分析图片内容

### 场景三：识别接口返回数据

浏览器开发者工具截图，提取接口 URL、请求参数、响应数据。

## 输出格式

### text 格式（默认）

```
识别到的第一行文字
识别到的第二行文字
...
```

### json 格式（--format json）

```json
{
  "success": true,
  "count": 3,
  "data": [
    {
      "text": "识别到的文字",
      "confidence": 0.95,
      "box": [[x1,y1], [x2,y2], [x3,y3], [x4,y4]]
    }
  ]
}
```

- `confidence`：置信度（0-1），越接近1越准确
- `box`：文字区域的四个角坐标

## 注意事项

1. 图片越清晰，识别效果越好
2. 中文识别默认支持，无需额外配置
3. 网络图片会下载到临时文件，识别完自动清理
4. 模型随包自带（约15MB），不需要额外下载

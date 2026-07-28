# 动图生成（GIF）调研报告

> 日期：2026-07-28  
> 状态：调研完成，待封装  
> 目标：在 AI-OS 中封装 `generate_gif` 技能，用户用自然语言描述，AI 生成 GIF 动图

---

## 一、目标与背景

跟 `generate_image`（图片生成）一个套路，封装一个 `generate_gif` 技能：用户说"生成一只猫晒太阳的动图"，AI 调用技能，返回 GIF。

整体链路：
```
用户描述 → CogVideoX-Flash 生成短视频 → 下载 MP4 → 抽帧转 GIF → 返回给前端展示
```

---

## 二、CogVideoX API 调研结论

### 2.1 API 基本用法

- **模型**：`cogvideox-flash`（免费档）
- **接口**：智谱 V4 异步视频生成 API
  - 创建任务：`POST /api/paas/v4/videos/generations`
  - 轮询结果：`GET /api/paas/v4/async-result/{task_id}`
- **流程**：创建任务拿 task_id → 每 5 秒轮询 → SUCCESS 后拿视频 URL → 下载 MP4
- **生成耗时**：约 30 秒

### 2.2 SDK 源码中的参数（Python zhipuai）

```python
videos.generations(
    model,        # "cogvideox-flash"
    prompt,       # 文本描述
    image_url,    # 图生视频：传图片URL（可选）
    quality,      # 画质
    with_audio,   # 是否带声音
    size,         # 分辨率，如 "1024x1024"
    duration,     # 时长（秒）
    fps,          # 帧率
)
```

### 2.3 关键发现：fps 和 duration 参数不生效 ⚠️

**实测数据**：

| 对比项 | 不传 fps/duration | 传了 fps=5 + duration=3 |
|---|---|---|
| fps | 37.0 | **37.0（没变）** |
| duration | 5.11 秒 | **5.11 秒（没变）** |
| 总帧数 | 189 | **189（一模一样）** |
| 文件大小 | 1.2 MB | **1.2 MB** |

**结论**：API 接受这些参数（不报错），但实际生成逻辑忽略它们。CogVideoX-Flash **写死了 37fps / 5秒 / 189帧 / 1024×1024**。

**影响**：无法从源头控制视频帧数，**必须在后端抽帧转 GIF**。

---

## 三、GIF 体积控制方案（已确定）

### 3.1 目标

最终 GIF 体积控制在 **5MB 左右**。

### 3.2 否定的方案及原因

| 方案 | 否定原因 |
|---|---|
| 直接用视频不转 GIF | 用户明确要求动图格式 |
| 从源头控制 fps/duration | API 参数不生效（见 2.3） |
| 12帧/480px/64色 = 0.9MB | 体积太小，用户觉得没必要这么省 |
| 63帧/480px/256色 = 7.7MB | 略超 5MB 目标 |
| 48帧/640px/128色 = 7.8MB | 帧数够但尺寸放大导致超 |

### 3.3 确定的方案

**48帧 / 480px / 256色 / 每帧80ms = 5.9MB**

| 参数 | 值 | 说明 |
|---|---|---|
| 源视频 | 189帧, 1024×1024, 5秒 | CogVideoX-Flash 固定输出 |
| 抽帧步长 | 每 4 帧取 1 帧 | 189 ÷ 4 ≈ 48 帧 |
| GIF 尺寸 | 480×480 | 从 1024 缩到 480 |
| 色彩 | 256色（全色彩不降色） | 保证画质 |
| 帧间隔 | 80ms（≈12fps） | 动画流畅 |
| 体积 | **5.9 MB** | 稳定可控 |

### 3.4 备选参数组合（实测数据备查）

| 帧数 | 尺寸 | 色彩 | 体积 |
|---|---|---|---|
| 48 | 480px | 256色 | 5.9MB ✅ |
| 63 | 480px | 256色 | 7.7MB |
| 48 | 640px | 128色 | 7.8MB |
| 95 | 480px | 128色 | 9.0MB |
| 24 | 480px | 128色 | 2.3MB |
| 12 | 480px | 64色 | 0.9MB |
| 12 | 360px | 128色 | 0.8MB |

### 3.5 帧数自适应逻辑（重要）

源视频帧数固定 189，但如果将来模型升级导致帧数变化，用固定步长抽帧会导致 GIF 帧数不可控。正确做法：

```python
TARGET_FRAMES = 48
step = max(1, len(source_frames) // TARGET_FRAMES)
result = source_frames[::step][:TARGET_FRAMES]
```

固定目标帧数，不管源多长，GIF 帧数始终 48，体积稳定。

---

## 四、技术链路

```
1. 调 CogVideoX API（异步轮询，30秒）
   POST /api/paas/v4/videos/generations
   → task_id → 轮询 → 视频URL

2. 下载视频 MP4（1.2MB, 1024×1024）

3. 提取帧 + 抽帧 + 转 GIF
   imageio 读视频 → 每4帧取1帧 → 缩到480px → PIL合成GIF
   
4. GIF 上传/返回给前端
```

### 依赖

- **Python**：`imageio`（读视频帧）、`Pillow`（合成GIF）
- **Go 后端**：调 Python 脚本，或用 Go 的 ffmpeg 绑定

### 后端语言选择（明天需确定）

| 方案 | 优点 | 缺点 |
|---|---|---|
| Go 调 Python 脚本 | PIL 成熟，今天代码已验证 | 多一层进程调用 |
| Go + ffmpeg 转 GIF | 纯 Go 无外部依赖 | ffmpeg GIF 质量不如 PIL |

---

## 五、参考：generate_image 技能的实现（照搬结构）

`generate_gif` 技能封装需参照 `generate_image` 的结构：

| 文件 | 作用 |
|---|---|
| `go-backend/service/image_gen.go` | → 对应 `gif_gen.go`，核心生成逻辑 |
| `go-backend/handler/image_proxy.go` | → 可能复用，GIF 也需要下载代理 |
| `go-backend/service/builtin_skill.go` | 注册 `generate_gif` 为通用技能 |
| `go-backend/service/llm_log.go` | 增加 `gif_gen_by_user` 统计（可选） |

---

## 六、明天待办

1. **确定后端实现方式**：Go 调 Python 脚本 还是 纯 Go + ffmpeg
2. **编写 `gif_gen.go`**：CogVideoX 调用 + 下载 + 转 GIF 全链路
3. **注册 `generate_gif` 技能**：builtin_skill.go + 系统提示词
4. **前端适配**：ChatWorkspace.vue 支持 GIF 展示（可能复用图片渲染逻辑）
5. **实测全链路**：从用户对话到 GIF 展示
6. **调通后提交推送**

---

## 七、测试产物位置（缓存文件，不进仓库）

| 文件 | 说明 |
|---|---|
| `C:\cache\cogvideo_test.gif` | 23MB 原始GIF（189帧, 480px） |
| `C:\cache\gif_5mb_48帧.gif` | 5.9MB **确定方案**（48帧, 480px, 256色） |
| `C:\cache\cogvideo_fps5_dur3.mp4` | fps/duration 参数不生效的对照视频 |
| `C:\cache\test_cogvideo_params.py` | API 参数测试脚本 |
| `C:\cache\find_5mb_params.py` | GIF 参数组合搜索脚本 |
| `C:\cache\test_gif_strategies.py` | GIF 抽帧策略测试脚本 |

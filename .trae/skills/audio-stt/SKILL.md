---
name: audio-stt
description: 音频识别转文字。当用户要求"识别音频"、"语音转文字"、"音频转文字"、"听写"、"转录音频"、"语音识别"、"识别录音"时触发。基于faster-whisper本地运行，完全免费，支持中英文。
---

# 音频识别（语音转文字）

## 角色定位

将音频文件中的语音转为文字，支持中英文。用于会议录音转文字、语音消息转文字等场景。

## 安全限制

**只识别 `D:\wwwroot\ai\ai_cache\audio-stt\` 目录下的音频文件，其他路径的音频文件拒绝执行。**

用户提供的音频文件必须先放入该目录：
```
D:\wwwroot\ai\ai_cache\audio-stt\
```

## 调用方式

```bash
cd .trae/skills/audio-stt ; python audio_stt.py <文件名> [选项]
```

### 参数说明

| 参数 | 必填 | 说明 |
|------|------|------|
| 文件名 | 是 | `D:\wwwroot\ai\ai_cache\audio-stt\` 目录下的文件名（不含路径），或 `all` 批量识别 |
| --model | 否 | 模型：tiny（默认）/ base / small / medium / large |
| --format | 否 | 输出格式：text（默认）/ json |
| --lang | 否 | 语言：zh（默认）/ en / auto |

### 模型大小对照

| 模型 | 大小 | 速度 | 效果 | 推荐场景 |
|------|------|------|------|----------|
| tiny | ~75MB | 最快 | 一般 | 快速预览 |
| base | ~150MB | 较快 | 较好 | 日常使用（推荐） |
| small | ~500MB | 适中 | 好 | 正式转录 |
| medium | ~1.5GB | 较慢 | 很好 | 高精度需求 |
| large | ~3GB | 最慢 | 最好 | 最高精度 |

首次使用会自动下载模型到 `D:\wwwroot\ai\ai_cache\audio-stt\cache\` 目录。

### 示例

```bash
# 识别单个文件
cd .trae/skills/audio-stt ; python audio_stt.py meeting.mp3

# 使用更大的模型
cd .trae/skills/audio-stt ; python audio_stt.py voice.wav --model base

# JSON格式输出
cd .trae/skills/audio-stt ; python audio_stt.py recording.m4a --format json

# 批量识别 audio/ 目录下所有文件
cd .trae/skills/audio-stt ; python audio_stt.py all
```

## 支持的音频格式

mp3, wav, m4a, ogg, flac, wma, aac

## 输出格式

### text 格式（默认）

```
[0.0s-3.2s] 这是第一段话
[3.5s-6.8s] 这是第二段话
```

### json 格式（--format json）

```json
{
  "success": true,
  "file": "meeting.mp3",
  "count": 2,
  "data": [
    {"start": 0.0, "end": 3.2, "text": "这是第一段话"},
    {"start": 3.5, "end": 6.8, "text": "这是第二段话"}
  ]
}
```

## 注意事项

1. 音频越清晰、背景噪音越少，识别效果越好
2. 中文识别默认支持，无需额外配置
3. 模型首次下载需要网络（通过国内镜像 hf-mirror.com）
4. 模型缓存在本地，后续使用无需网络

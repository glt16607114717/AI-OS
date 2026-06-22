"""
环境噪音测试脚本
录制 5 秒音频，分析 RMS 噪音水平
"""
import numpy as np
import sounddevice as sd
import time

print("=== 环境噪音测试 ===")
print("请保持安静 5 秒...")
print()

# 录制 5 秒
duration = 5
sample_rate = 16000
recording = sd.rec(int(duration * sample_rate), samplerate=sample_rate, channels=1, dtype='int16')
sd.wait()

audio = recording.flatten()
rms = float(np.sqrt(np.mean(audio.astype(np.float64) ** 2)))

# 分帧分析（100ms 每帧）
frame_size = 1600
frames = []
for i in range(0, len(audio), frame_size):
    chunk = audio[i:i+frame_size]
    if len(chunk) == frame_size:
        frame_rms = float(np.sqrt(np.mean(chunk.astype(np.float64) ** 2)))
        frames.append(frame_rms)

avg_rms = float(np.mean(frames))
max_rms = float(np.max(frames))
min_rms = float(np.min(frames))

print(f"总 RMS:        {rms:.0f}")
print(f"帧平均 RMS:    {avg_rms:.0f}")
print(f"帧最大 RMS:    {max_rms:.0f}")
print(f"帧最小 RMS:    {min_rms:.0f}")
print()
print(f"建议阈值 (×1.5): {avg_rms * 1.5:.0f}")
print(f"建议阈值 (×2.0): {avg_rms * 2.0:.0f}")
print(f"固定 500:       500")
print(f"固定 1000:      1000")
print()

if avg_rms < 200:
    print("环境很安静，阈值用 300-500 即可")
elif avg_rms < 500:
    print("环境较安静，阈值用 500-800 即可")
elif avg_rms < 1000:
    print("环境有噪音，阈值建议噪音×1.5")
else:
    print("环境噪音较大，可能校准时有说话声混入")

"""
说话 RMS 测试 - 分别测噪音和说话
"""
import numpy as np
import sounddevice as sd
import time

SR = 16000
FRAME = 1600  # 100ms

print("=== 说话 RMS 测试 ===")
print()

# 第一步：测 3 秒噪音
print("[1/2] 请保持安静 3 秒（测环境噪音）...")
rec1 = sd.rec(3 * SR, samplerate=SR, channels=1, dtype='int16')
sd.wait()
frames1 = [np.sqrt(np.mean(rec1[i:i+FRAME].astype(np.float64)**2))
           for i in range(0, len(rec1), FRAME) if len(rec1[i:i+FRAME]) == FRAME]
noise_avg = float(np.mean(frames1))
noise_max = float(np.max(frames1))

print(f"  噪音 平均: {noise_avg:.0f}, 最大: {noise_max:.0f}")
print()

# 第二步：测 3 秒说话
print("[2/2] 请连续说话 3 秒（比如念：开始 回车 发送）...")
time.sleep(0.5)
print("  开始！")
rec2 = sd.rec(3 * SR, samplerate=SR, channels=1, dtype='int16')
sd.wait()
frames2 = [np.sqrt(np.mean(rec2[i:i+FRAME].astype(np.float64)**2))
           for i in range(0, len(rec2), FRAME) if len(rec2[i:i+FRAME]) == FRAME]
speech_avg = float(np.mean(frames2))
speech_max = float(np.max(frames2))

print(f"  说话 平均: {speech_avg:.0f}, 最大: {speech_max:.0f}")
print()

# 分析
print("=== 分析 ===")
print(f"噪音平均: {noise_avg:.0f}")
print(f"说话平均: {speech_avg:.0f}")
print(f"信噪比:   {speech_avg / noise_avg:.1f}x")
print()
print(f"建议阈值 (噪音×1.3): {noise_avg * 1.3:.0f}")
print(f"建议阈值 (噪音×1.5): {noise_avg * 1.5:.0f}")

if speech_avg < noise_avg * 1.2:
    print()
    print("⚠ 问题: 说话音量和噪音差不多！")
    print("  可能原因:")
    print("  1. 麦克风选错了（用了远场麦克风而非耳机麦）")
    print("  2. 说话离麦克风太远")
    print("  3. 麦克风降噪算法把人声也降了")
elif speech_avg > noise_avg * 2:
    print()
    print("✓ 正常: 说话音量是噪音的 2 倍以上")
else:
    print()
    print("△ 一般: 说话略高于噪音")

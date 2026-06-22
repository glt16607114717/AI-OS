"""测试指定设备的 RMS"""
import numpy as np
import sounddevice as sd
import time

# 测试设备 22
DEVICE = 22
SR = 16000
DURATION = 3

print(f"=== 测试设备 [{DEVICE}] ===")
print("请保持安静 3 秒...")
rec = sd.rec(DURATION * SR, samplerate=SR, channels=1, dtype='int16', device=DEVICE)
sd.wait()
rms1 = float(np.sqrt(np.mean(rec.astype(np.float64)**2)))
print(f"噪音 RMS: {rms1:.0f}")
print()

print("请连续说话 3 秒（念：开始 回车 发送）...")
time.sleep(0.3)
rec2 = sd.rec(DURATION * SR, samplerate=SR, channels=1, dtype='int16', device=DEVICE)
sd.wait()
rms2 = float(np.sqrt(np.mean(rec2.astype(np.float64)**2)))
print(f"说话 RMS: {rms2:.0f}")
print()

ratio = rms2 / rms1 if rms1 > 0 else 0
print(f"信噪比: {ratio:.1f}x")
if ratio > 2:
    print("✓ 正常！这个设备是对的")
elif ratio > 1.5:
    print("△ 还行，可用")
else:
    print("✗ 也不行，试试其他设备")

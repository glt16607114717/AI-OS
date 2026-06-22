"""测试设备 22 和 24（48000Hz）"""
import numpy as np
import sounddevice as sd
import time

for DEVICE in [22, 24]:
    SR = 48000
    DURATION = 2
    try:
        print(f"=== 测试设备 [{DEVICE}] ===")
        print("请保持安静 2 秒...")
        rec = sd.rec(DURATION * SR, samplerate=SR, channels=1, dtype='int16', device=DEVICE)
        sd.wait()
        rms1 = float(np.sqrt(np.mean(rec.astype(np.float64)**2)))
        print(f"噪音 RMS: {rms1:.0f}")
        print("请说话 2 秒...")
        time.sleep(0.3)
        rec2 = sd.rec(DURATION * SR, samplerate=SR, channels=1, dtype='int16', device=DEVICE)
        sd.wait()
        rms2 = float(np.sqrt(np.mean(rec2.astype(np.float64)**2)))
        print(f"说话 RMS: {rms2:.0f}")
        ratio = rms2 / rms1 if rms1 > 0 else 0
        print(f"信噪比: {ratio:.1f}x")
        if ratio > 2:
            print("✓ 这个设备很好！")
        elif ratio > 1.5:
            print("△ 还行")
        else:
            print("✗ 不行")
    except Exception as e:
        print(f"✗ 失败: {e}")
    print()

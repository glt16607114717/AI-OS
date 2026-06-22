"""列出所有音频输入设备"""
import sounddevice as sd

print("=== 音频输入设备列表 ===")
print()

devices = sd.query_devices()
default_input = sd.default.device[0]

for i, d in enumerate(devices):
    if d['max_input_channels'] > 0:
        marker = " ← 当前使用" if i == default_input else ""
        print(f"[{i}] {d['name']}{marker}")
        print(f"    采样率: {int(d['default_samplerate'])}Hz, 通道: {d['max_input_channels']}")
        print()

if default_input is None:
    print("⚠ 未设置默认输入设备！")

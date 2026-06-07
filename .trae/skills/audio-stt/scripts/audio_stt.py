"""
音频识别工具（语音转文字）

基于 faster-whisper（Whisper 加速版），完全免费，本地运行。
只识别 ai_cache/audio-stt/ 目录下的音频文件，其他路径拒绝执行。

用法：python audio_stt.py <文件名> [选项]
示例：python audio_stt.py meeting.mp3
      python audio_stt.py voice.wav --model base
      python audio_stt.py recording.m4a --format json
      python audio_stt.py all          （识别 audio/ 下所有音频文件）

参数说明：
  文件名       必填，audio/ 目录下的文件名（不含路径），或 "all" 批量识别
  --model      模型大小：tiny（默认）/ base / small / medium / large
  --format     输出格式：text（默认）/ json
  --lang       语言：zh（默认，中文）/ en（英文）/ auto（自动检测）

音频格式支持：mp3, wav, m4a, ogg, flac, wma, aac

模型说明：
  tiny   — 最快，效果一般（~75MB）
  base   — 较快，效果较好（~150MB）
  small  — 适中，效果好（~500MB）
  medium — 较慢，效果很好（~1.5GB）
  large  — 最慢，效果最好（~3GB）
  首次使用会自动下载模型到 ai_cache/temp/whisper-models/ 目录下
"""
import sys
import os
import json
import glob
from opencc import OpenCC

_cc = OpenCC("t2s")

def to_simplified(text: str) -> str:
    try:
        return _cc.convert(text)
    except Exception:
        return text

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
CACHE_DIR = "D:/wwwroot/ai/ai_cache"
AUDIO_DIR = os.path.join(CACHE_DIR, "audio-stt")
MODEL_CACHE_DIR = os.path.join(CACHE_DIR, "temp", "whisper-models")

AUDIO_EXTENSIONS = {".mp3", ".wav", ".m4a", ".ogg", ".flac", ".wma", ".aac"}


def validate_audio_file(filename: str) -> str:
    filepath = os.path.join(AUDIO_DIR, filename)
    filepath = os.path.abspath(filepath)

    if not filepath.startswith(os.path.abspath(AUDIO_DIR)):
        print(f"[错误] 安全限制：只允许识别 audio/ 目录下的文件")
        sys.exit(1)

    if not os.path.exists(filepath):
        print(f"[错误] 文件不存在: {filepath}")
        print(f"请将音频文件放入 {AUDIO_DIR} 目录")
        sys.exit(1)

    ext = os.path.splitext(filepath)[1].lower()
    if ext not in AUDIO_EXTENSIONS:
        print(f"[错误] 不支持的音频格式: {ext}")
        print(f"支持格式: {', '.join(AUDIO_EXTENSIONS)}")
        sys.exit(1)

    return filepath


def get_all_audio_files() -> list:
    files = []
    for ext in AUDIO_EXTENSIONS:
        files.extend(glob.glob(os.path.join(AUDIO_DIR, f"*{ext}")))
    return sorted(files)


def transcribe(filepath: str, model_size: str = "tiny", language: str = "zh") -> list:
    from faster_whisper import WhisperModel

    os.makedirs(MODEL_CACHE_DIR, exist_ok=True)

    if "HF_ENDPOINT" not in os.environ:
        os.environ["HF_ENDPOINT"] = "https://hf-mirror.com"

    print(f"[信息] 加载模型: {model_size}（首次使用会自动下载）...")
    model = WhisperModel(
        model_size,
        device="cpu",
        compute_type="int8",
        download_root=MODEL_CACHE_DIR,
    )

    print(f"[信息] 识别中: {os.path.basename(filepath)}...")
    lang_param = None if language == "auto" else language
    segments, info = model.transcribe(filepath, language=lang_param, beam_size=5)

    results = []
    for segment in segments:
        results.append({
            "start": round(segment.start, 2),
            "end": round(segment.end, 2),
            "text": to_simplified(segment.text.strip()),
        })

    return results


def main():
    if len(sys.argv) < 2:
        print("用法: python audio_stt.py <文件名|all> [--model tiny|base|small|medium|large] [--format text|json] [--lang zh|en|auto]")
        print("")
        print("参数说明:")
        print("  文件名  audio/ 目录下的文件名，或 'all' 批量识别")
        print("  --model 模型大小: tiny(默认)/base/small/medium/large")
        print("  --format 输出格式: text(默认)/json")
        print("  --lang 语言: zh(默认)/en/auto")
        print("")
        print("音频目录:", AUDIO_DIR)
        print("支持格式:", ", ".join(AUDIO_EXTENSIONS))
        sys.exit(1)

    filename = sys.argv[1]
    model_size = "tiny"
    output_format = "text"
    language = "zh"

    i = 2
    while i < len(sys.argv):
        arg = sys.argv[i]
        if arg == "--model" and i + 1 < len(sys.argv):
            model_size = sys.argv[i + 1]
            i += 2
        elif arg == "--format" and i + 1 < len(sys.argv):
            output_format = sys.argv[i + 1]
            i += 2
        elif arg == "--lang" and i + 1 < len(sys.argv):
            language = sys.argv[i + 1]
            i += 2
        else:
            i += 1

    if filename == "all":
        audio_files = get_all_audio_files()
        if not audio_files:
            print(f"[信息] audio/ 目录下没有音频文件")
            sys.exit(0)
        print(f"[信息] 找到 {len(audio_files)} 个音频文件")
        all_results = {}
        for fp in audio_files:
            fname = os.path.basename(fp)
            results = transcribe(fp, model_size, language)
            all_results[fname] = results
            if output_format == "text":
                print(f"\n=== {fname} ===")
                for r in results:
                    print(f"[{r['start']:.1f}s-{r['end']:.1f}s] {r['text']}")
        if output_format == "json":
            print(json.dumps({"success": True, "data": all_results}, ensure_ascii=False, indent=2))
        return

    filepath = validate_audio_file(filename)
    results = transcribe(filepath, model_size, language)

    if not results:
        print("[信息] 未识别到语音内容")
        sys.exit(0)

    if output_format == "json":
        output = {
            "success": True,
            "file": filename,
            "count": len(results),
            "data": results,
        }
        print(json.dumps(output, ensure_ascii=False, indent=2))
    else:
        for r in results:
            print(f"[{r['start']:.1f}s-{r['end']:.1f}s] {r['text']}")

    print(f"[信息] 共识别 {len(results)} 段语音", file=sys.stderr)


if __name__ == "__main__":
    main()

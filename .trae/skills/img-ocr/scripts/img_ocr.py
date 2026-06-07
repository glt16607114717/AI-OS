"""
图片 OCR 识别工具

支持本地图片和网络图片的文字识别，基于 RapidOCR（PaddleOCR 轻量封装）。
完全免费，无需 API Key，模型首次使用时自动下载。

用法：python img_ocr.py <图片路径或URL>
示例：python img_ocr.py screenshot.png
      python img_ocr.py https://example.com/image.png
      python img_ocr.py test.png --format json
      python img_ocr.py test.png --lang en

参数说明：
  图片路径     必填，本地文件路径或 HTTP/HTTPS URL
  --format     输出格式：text（默认，纯文本）/ json（含坐标和置信度）
  --lang       语言：ch（默认，中英文混合）/ en（英文）/ ch_cht（繁体中文）
  --no-detect  跳过文字检测，直接整图识别（适用于纯文字截图）
"""
import sys
import os
import json
import tempfile

CACHE_DIR = "D:/wwwroot/ai/ai_cache"


def is_url(path: str) -> bool:
    return path.startswith("http://") or path.startswith("https://")


def download_image(url: str) -> str:
    import requests
    resp = requests.get(url, timeout=30, headers={"User-Agent": "Mozilla/5.0"})
    resp.raise_for_status()

    suffix = ".jpg"
    ct = resp.headers.get("Content-Type", "")
    if "png" in ct:
        suffix = ".png"
    elif "webp" in ct:
        suffix = ".webp"
    elif "bmp" in ct:
        suffix = ".bmp"

    tmp_dir = os.path.join(CACHE_DIR, "img-ocr")
    os.makedirs(tmp_dir, exist_ok=True)
    import time
    tmp_path = os.path.join(tmp_dir, f"ocr_{int(time.time())}{suffix}")
    with open(tmp_path, "wb") as f:
        f.write(resp.content)
    return tmp_path


def ocr_image(image_path: str, lang: str = "ch", no_detect: bool = False) -> list:
    from rapidocr_onnxruntime import RapidOCR

    engine = RapidOCR()
    if no_detect:
        result, _ = engine(image_path, det=False)
    else:
        result, _ = engine(image_path)

    if not result:
        return []

    lines = []
    for item in result:
        if no_detect:
            text = item if isinstance(item, str) else str(item)
            lines.append({"text": text})
        else:
            box = item[0]
            text = item[1]
            confidence = round(float(item[2]), 4)
            lines.append({
                "text": text,
                "confidence": confidence,
                "box": [[round(p[0], 1), round(p[1], 1)] for p in box],
            })
    return lines


def main():
    if len(sys.argv) < 2:
        print("用法: python img_ocr.py <图片路径或URL> [--format text|json] [--lang ch|en|ch_cht] [--no-detect]")
        print("")
        print("参数说明:")
        print("  图片路径     本地文件路径或 HTTP/HTTPS URL")
        print("  --format     输出格式：text（默认）/ json")
        print("  --lang       语言：ch（默认）/ en / ch_cht")
        print("  --no-detect  跳过文字检测，直接整图识别")
        print("")
        print("示例:")
        print("  python img_ocr.py D:\\photos\\screenshot.png")
        print("  python img_ocr.py https://example.com/image.png --format json")
        sys.exit(1)

    image_input = sys.argv[1]
    output_format = "text"
    lang = "ch"
    no_detect = False

    i = 2
    while i < len(sys.argv):
        arg = sys.argv[i]
        if arg == "--format" and i + 1 < len(sys.argv):
            output_format = sys.argv[i + 1]
            i += 2
        elif arg == "--lang" and i + 1 < len(sys.argv):
            lang = sys.argv[i + 1]
            i += 2
        elif arg == "--no-detect":
            no_detect = True
            i += 1
        else:
            i += 1

    temp_file = None
    try:
        if is_url(image_input):
            print(f"[信息] 下载网络图片: {image_input}")
            image_path = download_image(image_input)
            temp_file = image_path
            print(f"[信息] 已下载到临时文件")
        else:
            image_path = os.path.abspath(image_input)
            if not os.path.exists(image_path):
                print(f"[错误] 文件不存在: {image_path}")
                sys.exit(1)

        print(f"[信息] 识别中...")
        results = ocr_image(image_path, lang=lang, no_detect=no_detect)

        if not results:
            print("[信息] 未识别到文字")
            sys.exit(0)

        if output_format == "json":
            output = {"success": True, "count": len(results), "data": results}
            print(json.dumps(output, ensure_ascii=False, indent=2))
        else:
            for line in results:
                print(line["text"])

        print(f"[信息] 共识别 {len(results)} 行文字", file=sys.stderr)

    finally:
        if temp_file and os.path.exists(temp_file):
            os.unlink(temp_file)


if __name__ == "__main__":
    main()

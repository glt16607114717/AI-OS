// 全局时间格式化工具
// 统一格式：YYYY-MM-DD HH:mm:ss

/**
 * 将任意时间字符串/Date 转为 "YYYY-MM-DD HH:mm:ss"
 * 支持 ISO 格式（"2026-06-18T19:23:48+08:00"）、MySQL DATETIME（"2026-06-18 19:23:48"）等
 */
export function formatTime(input: string | null | undefined | Date): string {
  if (!input) return '-'
  try {
    const d = input instanceof Date ? input : new Date(input)
    if (isNaN(d.getTime())) return String(input)
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  } catch {
    return String(input)
  }
}

/**
 * 仅显示日期+时分（不含秒）：YYYY-MM-DD HH:mm
 */
export function formatTimeShort(input: string | null | undefined | Date): string {
  const full = formatTime(input)
  return full === '-' ? '-' : full.slice(0, 16)
}

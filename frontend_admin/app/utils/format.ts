// 按**运行环境**时区显示（未指定 timeZone）。
// 也就是说：后端给的 UTC 或带偏移的时间会被换算成本地时间，
// 换个浏览器时区看到的字面时间不同 —— 这是有意的"显示本地时间"语义，
// 不是格式化错误。需要固定东八区的话，得显式传 timeZone: 'Asia/Shanghai'。
export function formatDateTime(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

export function formatNumber(value?: number | null) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0)
}

export function shortId(value?: string | null) {
  if (!value) return '-'
  return value.length > 12 ? `${value.slice(0, 6)}...${value.slice(-4)}` : value
}

/**
 * 禁言的展示口径（票 399 / ADR-0032 决策 8）。
 *
 * 后端只把 `mute_until` 这个时间点给出来；"还有几天"是本地展示值，口径与
 * 服务端判定（now < mute_until）一致。还剩几小时时向上取整成 1 天，
 * 不能显示 0 天（那会让人以为已经解禁）。
 */

/** 把 RFC3339 字符串解析成 Date；缺失 / 非法返回 null。 */
export function parseMuteUntil(value?: string | null): Date | null {
  if (!value) return null
  const at = new Date(value)
  if (Number.isNaN(at.getTime())) return null
  return at
}

/** 剩余天数（向上取整）；未禁言 / 已到期 / 非法值都返回 0。 */
export function muteRemainingDays(value?: string | null, now: Date = new Date()): number {
  const until = parseMuteUntil(value)
  if (!until) return 0
  const remainingMs = until.getTime() - now.getTime()
  if (remainingMs <= 0) return 0
  const DAY_MS = 24 * 60 * 60 * 1000
  const days = Math.floor(remainingMs / DAY_MS)
  return remainingMs % DAY_MS === 0 ? days : days + 1
}

/** 到期时间文案（本地时区，精确到分钟；与后端错误文案同一格式）。 */
export function formatMuteUntil(value?: string | null): string {
  const until = parseMuteUntil(value)
  if (!until) return ""
  const pad = (n: number) => String(n).padStart(2, "0")
  const date = `${until.getFullYear()}-${pad(until.getMonth() + 1)}-${pad(until.getDate())}`
  const time = `${pad(until.getHours())}:${pad(until.getMinutes())}`
  return `${date} ${time}`
}

/**
 * 账号管理页「禁言」列文案：未禁言返回空串（列里显示 —）。
 *
 * 文案要带到期时间与剩余天数：管理员要知道"还剩多久"，才知道该不该提前解除。
 */
export function muteStatusText(value?: string | null, now: Date = new Date()): string {
  const days = muteRemainingDays(value, now)
  if (days <= 0) return ""
  return `禁言至 ${formatMuteUntil(value)}（还有 ${days} 天）`
}

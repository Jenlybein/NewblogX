/**
 * 禁言的展示口径（票 399 / ADR-0032 决策 8）。
 *
 * 后端只给一个时间点（`mute_until`），"还有几天"是前端按本地时间算的展示值 ——
 * 与服务端的判定口径（now < mute_until）一致，但**不允许**这里算成 0 天：
 * 还剩几小时时显示"1 天"才对，显示 0 天会让人以为已经解禁。
 *
 * 时间点本身由 user 模块生成（禁言天数 × 24h），前端不参与推算到期时刻。
 */

/** 把后端的 mute_until（RFC3339 字符串）解析成 Date；缺失 / 非法返回 null。 */
export function parseMuteUntil(value?: string | null): Date | null {
  if (!value) return null
  const at = new Date(value)
  if (Number.isNaN(at.getTime())) return null
  return at
}

/** 禁言剩余天数（向上取整）；未禁言 / 已到期 / 非法值都返回 0。 */
export function muteRemainingDays(value?: string | null, now: Date = new Date()): number {
  const until = parseMuteUntil(value)
  if (!until) return 0
  const remainingMs = until.getTime() - now.getTime()
  if (remainingMs <= 0) return 0
  const DAY_MS = 24 * 60 * 60 * 1000
  const days = Math.floor(remainingMs / DAY_MS)
  return remainingMs % DAY_MS === 0 ? days : days + 1
}

/** 禁言是否仍然生效（与服务端 now < mute_until 同一口径）。 */
export function isMuted(value?: string | null, now: Date = new Date()): boolean {
  return muteRemainingDays(value, now) > 0
}

/**
 * 到期时间文案（本地时区，精确到分钟）。
 *
 * 与后端 MuteError 的格式保持一致（`2006-01-02 15:04`），这样"发布被拒"的提示
 * 与设置页展示的是同一个时间点，用户不会看到两个相差一小时的说法。
 */
export function formatMuteUntil(value?: string | null): string {
  const until = parseMuteUntil(value)
  if (!until) return ""
  const pad = (n: number) => String(n).padStart(2, "0")
  const date = `${until.getFullYear()}-${pad(until.getMonth() + 1)}-${pad(until.getDate())}`
  const time = `${pad(until.getHours())}:${pad(until.getMinutes())}`
  return `${date} ${time}`
}

/** 设置页 / 发布页的完整提示文案；未禁言返回空串（调用方据此 v-if）。 */
export function muteNotice(value?: string | null, now: Date = new Date()): string {
  const days = muteRemainingDays(value, now)
  if (days <= 0) return ""
  return `已禁言，还有 ${days} 天恢复（禁言至 ${formatMuteUntil(value)}）。禁言期间不能发文、评论、私信，也不能修改昵称与简介。`
}

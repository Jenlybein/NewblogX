/**
 * 关键词风险的文案与颜色（ADR-0032 决策 7 / 票 396）。
 *
 * 后端契约只下发稳定 code：`surface` / `status` / `tier` / `category`。
 * 中文标签与"拦截红 / 观察黄"只在前端这一处映射 —— 运营改措辞不必重新生成
 * OpenAPI，契约里也不会出现会漂移的展示文案（与 reviewRisk.ts 同一条口径）。
 */

/** naive-ui NTag 的 type 取值。 */
export type ContentFilterTagType = 'default' | 'error' | 'warning' | 'info' | 'success'

/** 文本面筛选项（顺序与 ADR-0032 的六个面一致）。 */
export const CONTENT_FILTER_SURFACE_OPTIONS = [
  { label: '文章', value: 'article' },
  { label: '评论', value: 'comment' },
  { label: '昵称', value: 'nickname' },
  { label: '简介', value: 'abstract' },
  { label: '私信', value: 'chat' },
  { label: '合集', value: 'collection' },
] as const

/** 处理状态筛选项。 */
export const CONTENT_FILTER_STATUS_OPTIONS = [
  { label: '待处理', value: 'pending' },
  { label: '已忽略', value: 'ignored' },
  { label: '已删除内容', value: 'deleted' },
] as const

const SURFACE_LABELS: Record<string, string> = {
  article: '文章',
  comment: '评论',
  nickname: '昵称',
  abstract: '简介',
  chat: '私信',
  collection: '合集',
}

const STATUS_LABELS: Record<string, string> = {
  pending: '待处理',
  ignored: '已忽略',
  deleted: '已删除内容',
}

const CATEGORY_LABELS: Record<string, string> = {
  drugs: '毒品',
  gambling: '赌博',
  fraud: '诈骗',
  porn: '色情',
  fake_docs: '假证发票',
  politics: '涉政',
  violence: '暴恐',
}

/** 文本面 → 中文。未知值原样返回：宁可露出 code，也不猜一个错的中文。 */
export function contentFilterSurfaceLabel(value?: string | null): string {
  if (!value) return '-'
  return SURFACE_LABELS[value] ?? value
}

/** 状态 → 中文。 */
export function contentFilterStatusLabel(value?: string | null): string {
  if (!value) return '-'
  return STATUS_LABELS[value] ?? value
}

/** 状态 → 标签颜色（待处理要显眼，其余收敛成灰）。 */
export function contentFilterStatusTagType(value?: string | null): ContentFilterTagType {
  if (value === 'pending') return 'warning'
  if (value === 'deleted') return 'error'
  return 'default'
}

/** 类目 → 中文（七类固定，未知值原样返回）。 */
export function contentFilterCategoryLabel(value?: string | null): string {
  if (!value) return '-'
  return CATEGORY_LABELS[value] ?? value
}

export interface TierMeta {
  /** 档位中文：拦截词 / 观察词。 */
  label: string
  /** NTag 颜色：拦截红、观察黄。 */
  type: ContentFilterTagType
  /** 上下文高亮的 class：拦截用危险底，观察用记号笔黄底。 */
  highlightClass: string
}

const TIER_META: Record<string, TierMeta> = {
  block: { label: '拦截词', type: 'error', highlightClass: 'bg-danger-soft text-danger-ink' },
  watch: { label: '观察词', type: 'warning', highlightClass: 'bg-highlight text-ink' },
}

/** 档位 → 中文 + 颜色 + 高亮样式。未知档位按观察词处理（不把颜色当成判据）。 */
export function contentFilterTierMeta(tier?: string | null): TierMeta {
  return TIER_META[tier ?? ''] ?? TIER_META.watch!
}

/**
 * 把上下文片段按**命中的 rune 下标**切成「命中前 / 命中 / 命中后」。
 *
 * 必须按 rune 切而不是 `String.prototype.slice`：后端的 start / end 是 rune 下标，
 * 而 JS 的字符串下标是 UTF-16 code unit（一个 emoji 占两个）。用 slice 切中文没事，
 * 切到 emoji 就会把高亮错位一个字。
 */
export function splitContentFilterSnippet(
  text: string,
  start: number,
  end: number,
): { before: string; hit: string; after: string } {
  const runes = [...String(text ?? '')]
  const from = Math.max(0, Math.min(start, runes.length))
  const to = Math.max(from, Math.min(end, runes.length))
  return {
    before: runes.slice(0, from).join(''),
    hit: runes.slice(from, to).join(''),
    after: runes.slice(to).join(''),
  }
}

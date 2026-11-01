export type BannerShowFilter = 'all' | 'shown' | 'hidden'

export const BANNER_SHOW_FILTER_OPTIONS: Array<{ label: string; value: BannerShowFilter }> = [
  { label: '全部状态', value: 'all' },
  { label: '仅展示中', value: 'shown' },
  { label: '仅隐藏', value: 'hidden' },
]

export function buildBannerListQuery(page: number, limit: number, filter: BannerShowFilter) {
  const query: { page: number; limit: number; show?: boolean } = { page, limit }
  if (filter === 'shown') query.show = true
  if (filter === 'hidden') query.show = false
  return query
}

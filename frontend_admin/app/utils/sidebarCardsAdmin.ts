import type { AdminSidebarCardItem, SidebarCardsSavePayload } from '~/types/api'

/**
 * 后台右栏卡片管理器的纯函数层。
 *
 * 五类操作（新增 / 删除 / 拖动排序 / 改标题 / 启停）都在这里收口，页面与组件
 * 只负责接线。之所以不写进组件：这些规则要和后端 `internal/site/app/sidebar.go`
 * 的整表保存口径逐条对齐（种类白名单、标题回退、顺序号取下标、最多二十张），
 * 纯函数才能脱离 DOM 把"后台顺序 → 读者顺序"钉在同一条用例里。
 *
 * # 与后端一致的三条口径
 *
 * 1. 种类白名单：后端 `SidebarCardKind.Valid()` 只认三种内置卡，未知种类写入直接拒绝；
 *    这里加载时**保留**未知种类（不静默丢弃历史数据），保存前用错误列表拦住。
 * 2. 标题回退：后端 `strings.TrimSpace` 后为空就用该种类的默认标题；
 *    这里保存负载保留空串，默认标题只用于预览，不替后端"写死"。
 * 3. 顺序号：契约里的保存请求**没有** `sort_order`，顺序的唯一权威是 `list` 的下标；
 *    后端按提交顺序重排成连续的 0..n-1。
 */

/** 内置卡片种类，必须与后端 `domain.CardKind*` 常量逐字一致。 */
export const SIDEBAR_CARD_KINDS = ['hot_articles', 'latest_comments', 'tag_cloud', 'rich_text'] as const

/** 卡片种类。 */
export type SidebarCardKind = (typeof SIDEBAR_CARD_KINDS)[number]

/** 每种卡的默认标题，必须与后端 `sidebarCardDefaultTitles` 逐字一致。 */
export const SIDEBAR_CARD_DEFAULT_TITLES: Record<SidebarCardKind, string> = {
  hot_articles: '热门文章',
  latest_comments: '最新评论',
  tag_cloud: '标签云',
  rich_text: '公告',
}

/** 每种卡在选择器里的标题。 */
export const SIDEBAR_CARD_KIND_LABELS: Record<SidebarCardKind, string> = {
  hot_articles: '热门文章',
  latest_comments: '最新评论',
  tag_cloud: '标签云',
  rich_text: '富文本卡',
}

/** 与后端 `domain.SidebarCardMaxCount` 一致的数量上限。 */
export const SIDEBAR_CARD_MAX_COUNT = 20

/** 卡片选择器的选项（Naive NSelect 的形状）。 */
export const SIDEBAR_CARD_KIND_OPTIONS: Array<{ label: string; value: SidebarCardKind }> =
  SIDEBAR_CARD_KINDS.map(kind => ({ label: SIDEBAR_CARD_KIND_LABELS[kind], value: kind }))

/**
 * 编辑中的一张卡。
 *
 * `key` 是稳定的本地标识（拖拽、删除、改标题都按它定位），服务端返回的卡用
 * `id`；新加的卡 `id` 为空、`key` 由本地序号生成。`kind` 保留 `string`：
 * 历史数据里可能有未知种类，加载时不能静默丢，保存时再按白名单拦。
 */
export interface SidebarCardDraft {
  key: string
  id: string
  kind: string
  title: string
  enabled: boolean
  /**
   * 富文本卡的正文（HTML 片段）。
   *
   * 其它种类的草稿也带这个字段（保持草稿形状统一），保存时非富文本卡提交空串，
   * 由后端丢弃。编辑器不提供脚本 / iframe / 内联事件入口；正文在落库与渲染
   * 两侧都会过同一份白名单。
   */
  content: string
}

/** 判断字符串是否是内置卡片种类。 */
export function isSidebarCardKind(value: string): value is SidebarCardKind {
  return (SIDEBAR_CARD_KINDS as readonly string[]).includes(value)
}

/** 未知种类不隐藏、不崩溃，给一个能看懂的标签。 */
export function sidebarCardKindLabel(kind: string): string {
  return isSidebarCardKind(kind) ? SIDEBAR_CARD_KIND_LABELS[kind] : `未知种类（${kind}）`
}

/**
 * 一张卡最终展示的标题：去首尾空白，空白则回退该种类的默认标题。
 *
 * 与后端 `ReplaceCards` 的回退规则一致；后台列表用它做预览，读者侧看到的就是它。
 * 未知种类没有默认标题，返回空串（这类卡保存前会被校验拦住）。
 */
export function effectiveSidebarCardTitle(kind: string, title: string): string {
  const trimmed = String(title ?? '').trim()
  if (trimmed) return trimmed
  return isSidebarCardKind(kind) ? SIDEBAR_CARD_DEFAULT_TITLES[kind] : ''
}

/** 服务端卡片 → 编辑草稿，保持服务端顺序；停用项照旧进来。 */
export function sidebarCardDrafts(
  cards: readonly AdminSidebarCardItem[] | null | undefined,
): SidebarCardDraft[] {
  if (!cards?.length) return []
  return cards.map((card, index) => ({
    key: card.id ? `card-${card.id}` : `card-index-${index}`,
    id: card.id,
    kind: card.kind,
    title: card.title,
    enabled: card.enabled,
    content: card.content,
  }))
}

/** 下一张新卡的本地 key，避开已有 key 且可重复调用（纯函数，便于断言）。 */
export function nextSidebarCardDraftKey(drafts: readonly SidebarCardDraft[]): string {
  let max = 0
  for (const draft of drafts) {
    const match = /^new-(\d+)$/.exec(draft.key)
    if (match) max = Math.max(max, Number(match[1]))
  }
  return `new-${max + 1}`
}

/** 在末尾追加一张新卡：默认启用、标题留空（保存后由后端回退默认标题）。 */
export function addSidebarCardDraft(
  drafts: readonly SidebarCardDraft[],
  kind: SidebarCardKind,
  key: string = nextSidebarCardDraftKey(drafts),
): SidebarCardDraft[] {
  if (drafts.length >= SIDEBAR_CARD_MAX_COUNT) return [...drafts]
  return [...drafts, { key, id: '', kind, title: '', enabled: true, content: '' }]
}

/** 按 key 删除一张卡；未知 key 原样返回。 */
export function removeSidebarCardDraft(
  drafts: readonly SidebarCardDraft[],
  key: string,
): SidebarCardDraft[] {
  return drafts.filter(draft => draft.key !== key)
}

/** 改标题：只动目标卡，保留原样输入（空白在保存时再处理）。 */
export function renameSidebarCardDraft(
  drafts: readonly SidebarCardDraft[],
  key: string,
  title: string,
): SidebarCardDraft[] {
  return drafts.map(draft => (draft.key === key ? { ...draft, title } : draft))
}

/** 启停：不传 `enabled` 时翻转，传了就显式置位。 */
export function toggleSidebarCardDraft(
  drafts: readonly SidebarCardDraft[],
  key: string,
  enabled?: boolean,
): SidebarCardDraft[] {
  return drafts.map(draft =>
    draft.key === key ? { ...draft, enabled: enabled ?? !draft.enabled } : draft,
  )
}

/** 改富文本正文：只动目标卡；未知 key 原样返回。 */
export function updateSidebarCardContent(
  drafts: readonly SidebarCardDraft[],
  key: string,
  content: string,
): SidebarCardDraft[] {
  return drafts.map(draft => (draft.key === key ? { ...draft, content } : draft))
}

/**
 * 拖动排序：把 `from` 位置的卡移到 `to` 位置。
 *
 * 越界、非整数、原地移动都返回一份顺序不变的新数组 —— 调用方不需要先做边界判断，
 * 也不会因为一次异常拖拽就把列表改坏。
 */
export function moveSidebarCardDraft(
  drafts: readonly SidebarCardDraft[],
  from: number,
  to: number,
): SidebarCardDraft[] {
  const next = [...drafts]
  if (!Number.isInteger(from) || !Number.isInteger(to)) return next
  if (from === to) return next
  if (from < 0 || from >= next.length || to < 0 || to >= next.length) return next
  const [moved] = next.splice(from, 1)
  if (!moved) return next
  next.splice(to, 0, moved)
  return next
}

/**
 * 保存前校验：只要错误不为空就整表不提交。
 *
 * 与后端拒绝条件一致：超过数量上限、种类不在白名单。标题空白不算错误 ——
 * 后端会回退默认标题，前端不该比后端更严。
 */
export function sidebarCardDraftErrors(drafts: readonly SidebarCardDraft[]): string[] {
  const errors: string[] = []
  if (drafts.length > SIDEBAR_CARD_MAX_COUNT) {
    errors.push(`最多 ${SIDEBAR_CARD_MAX_COUNT} 张卡片，当前 ${drafts.length} 张`)
  }
  drafts.forEach((draft, index) => {
    if (!isSidebarCardKind(draft.kind)) {
      errors.push(`第 ${index + 1} 张卡的种类「${draft.kind}」不受支持`)
    }
  })
  return errors
}

/**
 * 草稿 → 整表保存负载。
 *
 * 列表顺序就是读者侧顺序；每张卡都带上 `enabled`（停用项也提交，整表保存
 * 不是"只提交启用项"，否则停用配置会被物理删掉）。标题只去首尾空白，
 * 清空时提交空串，交给后端回退默认标题。
 */
export function toSidebarCardsSavePayload(
  drafts: readonly SidebarCardDraft[],
): SidebarCardsSavePayload {
  return {
    list: drafts.map(draft => ({
      kind: draft.kind,
      title: draft.title.trim(),
      enabled: draft.enabled,
      // 正文只对富文本卡有意义；别的种类一律提交空串，与后端"丢弃脏 content"同口径。
      content: draft.kind === 'rich_text' ? draft.content.trim() : '',
    })),
  }
}

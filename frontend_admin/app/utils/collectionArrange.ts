/**
 * 合集编排（后台）：批量收录与两级手动排序的纯逻辑。
 *
 * 两级排序的落点不同、但**提交语义相同**：
 *   - 合集内顺序 → `collection_article_models.sort_order`，读者详情页按它读；
 *   - 合集之间顺序 → `collection_models.sort_order`，读者列表页按它读。
 * 两者都是"完整有序列表"的全量替换：后端在集合与库里不一致时整单拒绝，
 * 所以上移/下移只改本地数组，提交时必须带上完整集合（失效行也不能漏）。
 *
 * 抽到 utils 而不是留在页面里，是因为这几处算错都不会报错：
 * 漏传一条会导致保存失败；把重复项再提交一次会把"重复结局"变成噪声。
 */

import type {
  CollectionBatchIncludeResponse,
  CollectionBatchIncludeItem,
} from '~/types/api'

/** 上移 / 下移。 */
export type ArrangeDirection = 'up' | 'down'

/** 批量收录的结局（与后端 `collection.IncludeOutcome` 同一组取值）。 */
export type CollectionIncludeOutcome = CollectionBatchIncludeItem['outcome']

/** 当前位置能否朝该方向移动（两端与空序列都为 false）。 */
export function canMoveArrangeItem(index: number, length: number, direction: ArrangeDirection): boolean {
  if (direction === 'up') {
    return index > 0 && index < length
  }
  return index >= 0 && index < length - 1
}

/**
 * 上移 / 下移一项，返回**新数组**（不改入参：页面靠新引用触发重新渲染）。
 *
 * 越界或两端移动原样返回当前顺序 —— 页面按钮虽然会禁用，但键盘与竞态都可能再点一次。
 */
export function moveArrangeItem<T>(items: readonly T[], index: number, direction: ArrangeDirection): T[] {
  const next = items.slice()
  if (!canMoveArrangeItem(index, items.length, direction)) {
    return next
  }

  const target = direction === 'up' ? index - 1 : index + 1
  const current = next[index] as T
  next[index] = next[target] as T
  next[target] = current
  return next
}

/** 合集之间换序的请求体顺序（完整列表，含未展示的合集）。 */
export function collectionOrderIds(rows: readonly { id: string }[]): string[] {
  return rows.map(row => row.id)
}

/** 合集内换序的请求体顺序（完整列表，失效行也在内）。 */
export function collectionItemIds(items: readonly { article_id: string }[]): string[] {
  return items.map(item => item.article_id)
}

/**
 * 按首次出现顺序去重。
 *
 * 批量提交前用：同一篇在勾选里出现两次没有意义，而服务端会把它算成一次
 * 收录 + 一次重复，把"重复"的提示弄成噪声。
 */
export function dedupeArticleIds(ids: readonly string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const id of ids) {
    if (seen.has(id)) continue
    seen.add(id)
    out.push(id)
  }
  return out
}

/**
 * 批量收录结果的提示文案。
 *
 * 三个计数分别报出来：只报"成功 N 篇"会让运营以为全部进去了，
 * 而重复与不可收录恰恰是他们下一步要处理的部分。
 */
export function summarizeBatchInclude(
  result: Pick<CollectionBatchIncludeResponse, 'included' | 'duplicated' | 'rejected'>,
): string {
  const parts: string[] = []
  if (result.included > 0) {
    parts.push(`已收录 ${result.included} 篇`)
  }
  if (result.duplicated > 0) {
    parts.push(`${result.duplicated} 篇已在合集中`)
  }
  if (result.rejected > 0) {
    parts.push(`${result.rejected} 篇不可收录`)
  }
  if (parts.length === 0) {
    return '没有可提交的文章'
  }
  if (result.included === 0) {
    return `没有新增收录，${parts.join('，')}`
  }
  return parts.join('，')
}

/** 把后端结局翻成后台看得懂的文案。 */
export function batchOutcomeLabel(outcome: CollectionIncludeOutcome | string): string {
  switch (outcome) {
    case 'included':
      return '已收录'
    case 'duplicated':
      return '已在合集中'
    case 'not_includable':
      return '不可收录'
    case 'missing':
      return '文章不存在'
    default:
      return '未知结果'
  }
}

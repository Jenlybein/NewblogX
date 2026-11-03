import type { ProjectBatchIncludeItem, ProjectBatchIncludeResponse } from '~/types/api'
import { canMoveArrangeItem, dedupeArticleIds, moveArrangeItem } from '~/utils/collectionArrange'

/**
 * 项目编排（后台）：批量收录与两级手动排序的纯逻辑（与合集编排页镜像）。
 *
 * 两级排序的落点不同、但**提交语义相同**：
 *   - 项目内顺序 → `project_article_models.sort_order`，读者详情页按它读；
 *   - 项目之间顺序 → `project_models.sort_order`，读者列表页按它读。
 * 两者都是"完整有序列表"的全量替换：后端在集合与库里不一致时整单拒绝，
 * 所以上移/下移只改本地数组，提交时必须带上完整集合（失效行也不能漏）。
 *
 * 数组换位与去重复用合集的通用函数（与"哪个运营位"无关），这里只做
 * 项目命名的投影；不做同名 re-export，避免 Nuxt utils 自动导入的重复告警。
 */

/** 批量收录的结局（与后端 `project.IncludeOutcome` 同一组取值）。 */
export type ProjectIncludeOutcome = ProjectBatchIncludeItem['outcome']

/** 上移 / 下移一项，返回新数组（越界原样返回）。 */
export function moveProjectArrangeItem<T>(
  items: readonly T[],
  index: number,
  direction: 'up' | 'down',
): T[] {
  return moveArrangeItem(items, index, direction)
}

/** 当前位置能否朝该方向移动（两端与空序列都为 false）。 */
export function canMoveProjectArrangeItem(
  index: number,
  length: number,
  direction: 'up' | 'down',
): boolean {
  return canMoveArrangeItem(index, length, direction)
}

/** 按首次出现顺序去重（批量提交前用）。 */
export function dedupeProjectArticleIds(ids: readonly string[]): string[] {
  return dedupeArticleIds(ids)
}

/** 项目之间换序的请求体顺序（完整列表，含未展示的项目）。 */
export function projectOrderIds(rows: readonly { id: string }[]): string[] {
  return rows.map(row => row.id)
}

/** 项目内换序的请求体顺序（完整列表，失效行也在内）。 */
export function projectItemIds(items: readonly { article_id: string }[]): string[] {
  return items.map(item => item.article_id)
}

/**
 * 批量收录结果的提示文案。
 *
 * 三个计数分别报出来：只报"成功 N 篇"会让运营以为全部进去了，
 * 而重复与不可收录恰恰是他们下一步要处理的部分。
 */
export function summarizeProjectBatchInclude(
  result: Pick<ProjectBatchIncludeResponse, 'included' | 'duplicated' | 'rejected'>,
): string {
  const parts: string[] = []
  if (result.included > 0) {
    parts.push(`已收录 ${result.included} 篇`)
  }
  if (result.duplicated > 0) {
    parts.push(`${result.duplicated} 篇已在项目中`)
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
export function projectBatchOutcomeLabel(outcome: ProjectIncludeOutcome | string): string {
  switch (outcome) {
    case 'included':
      return '已收录'
    case 'duplicated':
      return '已在项目中'
    case 'not_includable':
      return '不可收录'
    case 'missing':
      return '文章不存在'
    default:
      return '未知结果'
  }
}

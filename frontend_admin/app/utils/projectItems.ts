/**
 * 后台「项目编排」失效行的纯逻辑（与合集收录页镜像）。
 *
 * 失效判定不在前端另写：后端用与「热门置顶 / 合集」共用的领域函数算好
 * `stale` 与 `stale_reason` 再发下来。这里只做三件会算错且算错了看不出来的事：
 *   1. 一键清理挑哪些行（挑错会删掉展示中的文章）；
 *   2. 按钮上的失效计数（为 0 时不应让人点一个空操作）；
 *   3. 失效原因的中文文案（运营要能看出为什么失效）。
 *
 * 实现直接复用合集的那三个纯函数：判据与文案必须唯一，否则两个运营位会
 * 对同一行给出不同结论。
 */
import {
  collectionStaleReasonLabel,
  staleCollectionItemCount,
  staleCollectionItemIds,
} from '~/utils/collectionItems'

/** 页面行只需要这三个字段，便于用纯函数测试（页面行类型会结构兼容）。 */
export interface ProjectStaleRow {
  article_id: string
  stale: boolean
  stale_reason: string
}

/** 与后端 domain.StaleReason 同一组枚举值。 */
export type ProjectStaleReason = '' | 'missing' | 'deleted' | 'not_published' | 'not_visible'

/** 挑出失效行的 article_id（一键清理的输入），保持收录顺序。 */
export function staleProjectItemIds(items: readonly ProjectStaleRow[]): string[] {
  return staleCollectionItemIds(items)
}

/** 失效行数量：为 0 时清理按钮应禁用。 */
export function staleProjectItemCount(items: readonly ProjectStaleRow[]): number {
  return staleCollectionItemCount(items)
}

/** 失效原因文案；仍是展示中时返回"展示中"，未知枚举给兜底而不是空白。 */
export function projectStaleReasonLabel(reason: string | null | undefined): string {
  return collectionStaleReasonLabel(reason)
}

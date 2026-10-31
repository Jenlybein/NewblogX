/**
 * 热门置顶（运营位）的纯逻辑。
 *
 * 热门置顶 = 管理员挑选并手动排序的首页运营位，唯一真相是
 * `article_hot_top_models`（见 ADR-0017 决策 5）。它与「作者置顶」是两套数据，
 * 与 ES 无关：后台列表状态一律来自 `GET /articles/hot-top/admin`。
 *
 * 抽到 utils 而不是留在页面里，是因为这几件事算错了都不会报错：
 *   - 上移/下移错了，运营看到的顺序与首页顺序不一致；
 *   - `PATCH /articles/hot-top/order` 是**全量替换**，漏掉失效项整单会被后端拒绝。
 */

/** 运营位上限（条）。后端同样有这条限制，这里只用于页面提示与按钮可用性。 */
export const HOT_TOP_LIMIT = 12

export type HotTopDirection = 'up' | 'down'

/** 排序只看这两个字段：`/articles/hot-top/admin` 的列表项（stale 可能缺席）。 */
export interface HotTopOrderItem {
  article_id: string
  stale?: boolean
}

/** 当前位置能否朝该方向移动（两端与空序列都为 false）。 */
export function canMoveHotTop(index: number, length: number, direction: HotTopDirection): boolean {
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
export function moveHotTop<T extends HotTopOrderItem>(
  items: readonly T[],
  index: number,
  direction: HotTopDirection,
): T[] {
  const next = items.slice()
  if (!canMoveHotTop(index, items.length, direction)) {
    return next
  }

  const target = direction === 'up' ? index - 1 : index + 1
  const current = next[index] as T
  next[index] = next[target] as T
  next[target] = current
  return next
}

/**
 * 当前序列的 article_id 顺序。
 *
 * 两处消费：`PATCH /articles/hot-top/order` 的请求体（必须是完整集合，失效项也在内），
 * 以及文章管理列表判断某一行是否已置顶。
 */
export function hotTopIds(items: readonly HotTopOrderItem[]): string[] {
  return items.map(item => item.article_id)
}

/** 失效项 ID（文章已隐藏 / 退回草稿 / 删除，置顶行仍保留）。用于「一键清理」。 */
export function staleHotTopIds(items: readonly HotTopOrderItem[]): string[] {
  return items.filter(item => item.stale === true).map(item => item.article_id)
}

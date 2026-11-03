/**
 * 后台文章列表封面槽的显示决定（票 210）。
 *
 * 与读者站同一条规则：有可用封面就显示图片；缺封面、或这一行的封面已经加载失败，
 * 都返回空串 —— 调用方据此只渲染一块空白（无文字、无纹理、对辅助技术隐藏），
 * 不再画 "NO COVER" 之类的占位内容。
 *
 * 为什么抽到 utils：坏链接是真实会发生的（对象存储换了域名、图片被删），而"失败一次
 * 就永远退化成空白"这条规则算错不会报错，只会让运营看到碎图标或反复发请求。
 */

export interface CoverRowLike {
  cover?: string | null
}

/**
 * 该行应当显示的封面地址；返回空串表示"这一格是空白"。
 *
 * @param row 列表行（只要带 `cover` 字段）
 * @param failedCoverSrcs 已经加载失败过的封面地址集合（键是裁剪后的地址）
 */
export function resolveArticleCoverSrc(
  row: CoverRowLike,
  failedCoverSrcs: ReadonlySet<string>,
): string {
  const src = (row.cover ?? '').trim()
  if (!src) return ''
  return failedCoverSrcs.has(src) ? '' : src
}

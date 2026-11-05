/**
 * 把收藏写响应补进当前正文（纯函数，由阅读页取数模块调用）。
 *
 * 为什么单独成函数：失败分支历史上返回过空 data（`data: {}`），直接读
 * `payload.is_favor` 会抛 TypeError，把整个互动条卡在报错上（评审票 248 / P0-2）。
 * 空值防御写在这里、用用例钉住，取数模块与页面都不再手写第二份判空。
 */
export interface ArticleFavoriteWritePayload {
  is_favor?: boolean | null;
  favor_count?: number | null;
}

export interface ArticleWithFavoriteState {
  is_favor?: boolean | null;
  favor_count?: number | null;
}

/**
 * 返回 patch 后的正文。
 *
 * - `article` 为空：原样返回（还没正文可 patch）；
 * - `payload` 为空或字段不全：原样返回（不 patch、不抛错）。
 *
 * 返回新对象而不是原地改：页面把它赋回 `article.value`，Vue 才触发重渲染，
 * 也避免同一份缓存对象被就地污染。
 */
export function applyArticleFavoriteResult<TArticle extends ArticleWithFavoriteState>(
  article: TArticle | null | undefined,
  payload: ArticleFavoriteWritePayload | null | undefined,
): TArticle | null | undefined {
  if (!article) {
    return article;
  }
  if (!payload || typeof payload.is_favor !== "boolean" || typeof payload.favor_count !== "number") {
    return article;
  }
  return { ...article, is_favor: payload.is_favor, favor_count: payload.favor_count };
}

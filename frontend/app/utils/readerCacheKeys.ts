/**
 * 读者站跨导航缓存键的唯一出口（ADR-0019 决策 4 / 约束 4）。
 *
 * 这些字符串有两个消费点：`useReaderSwrAsyncData` 的 `useAsyncData` key 与
 * `usePagedResourceCache` 的 `cacheKey`（后者内部再拼 `#页码`）。散在各处的字符串
 * 一旦有人改了前缀，就会出现"同一条数据两个键、缓存永远不命中但没人报错"的漂移，
 * 所以集中在这里：**新增缓存源只加函数，不在调用点手写字符串**。
 *
 * 注意：函数返回值就是线上缓存键，改动等于让所有旧缓存失效（30 秒内自然恢复），
 * 不是纯重命名 —— 要不要改先想清楚。
 */

/** 首页四路数据（轮播 / 热门置顶 / 最新文章 / 右栏卡片）：它们的缓存键与页码无关。 */
export const READER_HOME_CACHE_KEYS = {
  banners: "home-banners",
  hotTop: "home-hot-top",
  latestArticles: "home-latest-articles",
  sidebarCards: "home-sidebar-cards",
} as const;

/** 文章正文缓存键（客户端 30s SWR；服务端渲染会禁用客户端缓存，键仍用于 payload）。 */
export function readerArticleCacheKey(articleId: string) {
  return `article-${articleId}`;
}

/** 文章评分摘要缓存键。 */
export function readerArticleScoreCacheKey(articleId: string) {
  return `article-score-summary:${articleId}`;
}

/** 文章作者统计（`author_info`）缓存键：第二波才有 author_id，退化时用文章 id 占位。 */
export function readerArticleAuthorInfoCacheKey(authorIdOrArticleId: string) {
  return `article-author-info-${authorIdOrArticleId}`;
}

/** 文章作者主页资料缓存键（客户端专用，服务端不渲染）。 */
export function readerArticleAuthorBaseCacheKey(authorIdOrArticleId: string) {
  return `article-author-base-${authorIdOrArticleId}`;
}

/** 根评论分页缓存键：`usePagedResourceCache` 会在后面追加 `#页码`。 */
export function readerArticleCommentsCacheKey(articleId: string) {
  return `article-comments:${articleId}`;
}

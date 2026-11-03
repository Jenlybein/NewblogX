import type { Schema } from "./contract";

/**
 * 文章上下文接口（`/articles/{id}/outline-context`）：阅读页左栏取同一棵树。
 *
 * 「来源」=`project:<id>` / `collection:<id>`；缺失时后端回退到第一棵归属树，
 * 都没有则不返回树。请求形状从契约派生，不手写一份。
 */
export type ArticleOutlineContextRequest = Schema<"internal_article_adapter_http.ArticleOutlineContextRequest">;

/** 把 `?from=` 原样透传给后端：格式校验与回退都在服务端一处。 */
export function outlineContextQuery(from?: string): ArticleOutlineContextRequest {
  const trimmed = (from ?? "").trim();
  return trimmed ? { from: trimmed } : {};
}

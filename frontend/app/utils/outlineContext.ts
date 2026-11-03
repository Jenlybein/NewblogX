/**
 * 文章上下文接口（`/articles/{id}/outline-context`）：阅读页左栏取同一棵树。
 *
 * 「来源」=`project:<id>` / `collection:<id>`；缺失时后端回退到第一棵归属树，
 * 都没有则不返回树。请求形状从契约派生，不手写一份。
 */

/**
 * 查询参数：`from` 只是原样透传给后端，格式校验与回退都在服务端一处。
 *
 * 这里没有用 `Schema<...>`：文章上下文接口由票 220 接线（后端路由与契约字段
 * 同票落地），当前契约里还没有这条 operation / schema。先用页面实际发送的
 * 唯一字段声明形状；票 220 合并后应改为从契约派生，避免两份真相。
 */
export interface ArticleOutlineContextRequest {
  from?: string;
}

/** 把 `?from=` 原样透传给后端：格式校验与回退都在服务端一处。 */
export function outlineContextQuery(from?: string): ArticleOutlineContextRequest {
  const trimmed = (from ?? "").trim();
  return trimmed ? { from: trimmed } : {};
}

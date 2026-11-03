import { useNuxtApp } from "#imports";
import type { ArticleOutlineContext } from "~/utils/outlineContext";
import { outlineContextQuery } from "~/utils/outlineContext";

/**
 * 文章上下文（阅读页左栏目录树）。
 *
 * 公开读：左栏不依赖登录身份，匿名取数（`auth: false`）就不会把用户态带进
 * 这条客户端请求。树**只在客户端取数**：调用点（`useArticleOutlineContext`）
 * 负责在 SSR 阶段跳过本请求（ADR-0019 / 票 220）。
 */
export function getArticleOutlineContext(articleId: string | number, from?: string) {
  return useNuxtApp().$api.request<ArticleOutlineContext>(
    `/api/articles/${encodeURIComponent(String(articleId))}/outline-context`,
    {
      auth: false,
      query: outlineContextQuery(from),
    },
  );
}

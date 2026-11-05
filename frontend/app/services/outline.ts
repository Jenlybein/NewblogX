import { useNuxtApp } from "#imports";
import type { ArticleOutlineContext, ProjectOutlineTree } from "~/utils/outlineContext";
import { outlineContextQuery, toArticleOutlineContext } from "~/utils/outlineContext";
import { parseOutlineSource } from "~/utils/outlineTree";

/**
 * 文章上下文（阅读页左栏目录树）。
 *
 * 公开读：左栏不依赖登录身份，匿名取数（`auth: false`）就不会把用户态带进
 * 这条客户端请求。树**只在客户端取数**：调用点（`useArticleOutlineContext`）
 * 负责在 SSR 阶段跳过本请求（ADR-0019 / 票 220）。
 *
 * #276 起 `?from=project:<id>` 改走项目具名端点：把来路与当前文章 ID 一起
 * 透传，响应收成既有左栏读模型，呈现层（抽屉 / 翻页 / 折叠态）逐字不变。
 * 合集来路与无来路仍由 `?from=` 的共用回退回答（合集侧迁移归 #275）。
 */
export async function getArticleOutlineContext(
  articleId: string | number,
  from?: string,
): Promise<ArticleOutlineContext> {
  const source = parseOutlineSource(from);
  if (source?.kind === "project") {
    const tree = await useNuxtApp().$api.request<ProjectOutlineTree>(
      `/api/projects/${encodeURIComponent(source.id)}/outline-tree`,
      {
        auth: false,
        query: { ...outlineContextQuery(from), article_id: String(articleId) },
      },
    );
    return toArticleOutlineContext(tree) ?? { has_tree: false };
  }
  return useNuxtApp().$api.request<ArticleOutlineContext>(
    `/api/articles/${encodeURIComponent(String(articleId))}/outline-context`,
    {
      auth: false,
      query: outlineContextQuery(from),
    },
  );
}

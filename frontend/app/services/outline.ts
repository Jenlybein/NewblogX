import { useNuxtApp } from "#imports";
import type {
  ArticleOutlineContext,
  CollectionOutlineTree,
  ProjectOutlineTree,
} from "~/utils/outlineContext";
import {
  outlineContextQuery,
  toArticleOutlineContext,
} from "~/utils/outlineContext";
import { parseOutlineSource } from "~/utils/outlineTree";

/**
 * 文章上下文（阅读页左栏目录树）。
 *
 * 公开读：左栏不依赖登录身份，匿名取数（`auth: false`）就不会把用户态带进
 * 这条客户端请求。树**只在客户端取数**：调用点（`useArticleOutlineContext`）
 * 负责在 SSR 阶段跳过本请求（ADR-0019 / 票 220）。
 *
 * 拆树迁移（`?from=` 来路）：来源是某一棵具名树时直接向它的具名端点取同一棵树，
 * 并把当前文章 ID 作为 `article_id` 带给后端回答上一章 / 下一篇；来源缺失或
 * 无法解析时仍沿用旧的文章上下文回退（由 #277 收缩）。响应收成左栏现有的
 * 读模型，共用呈现组件 / 版式逐字不变。
 */
export async function getArticleOutlineContext(
  articleId: string | number,
  from?: string,
): Promise<ArticleOutlineContext> {
  const api = useNuxtApp().$api;
  const article = String(articleId ?? "");
  const trimmed = (from ?? "").trim();
  const source = parseOutlineSource(trimmed);

  if (source) {
    const path = source.kind === "collection"
      ? `/api/collections/${encodeURIComponent(source.id)}/outline-tree`
      : `/api/projects/${encodeURIComponent(source.id)}/outline-tree`;
    const data = await api.request<CollectionOutlineTree | ProjectOutlineTree>(path, {
      auth: false,
      query: { from: trimmed, article_id: article },
    });
    return toArticleOutlineContext(data) ?? emptyOutlineContext();
  }

  return api.request<ArticleOutlineContext>(
    `/api/articles/${encodeURIComponent(article)}/outline-context`,
    {
      auth: false,
      query: outlineContextQuery(from),
    },
  );
}

/** 具名响应缺失时左栏按「没有可展示的树」处理（不猜默认归属）。 */
function emptyOutlineContext(): ArticleOutlineContext {
  return {
    has_tree: false,
    owner_kind: 0,
    owner_id: "",
    nodes: [],
    current: null,
    previous: null,
    next: null,
  };
}

import type { Nullable, Operation, Schema } from "~/types/contract";

import type { OutlineSourceKind } from "./outlineKind";
import { outlineSourceKind } from "./outlineKind";
import { outlineArticleHref, outlineSource } from "./outlineTree";

/**
 * 文章上下文接口（`/articles/{id}/outline-context`）：阅读页左栏取同一棵树。
 *
 * 「来源」=`project:<id>` / `collection:<id>`；缺失时后端回退到第一棵归属树，
 * 都没有则不返回树。请求形状与响应形状都从契约派生，不手写一份。
 */

/** 查询参数：`from` 只是原样透传，格式校验与回退都在服务端一处。 */
export type ArticleOutlineContextRequest = NonNullable<
  Operation<"/articles/{id}/outline-context", "get">["parameters"]["query"]
>;

/** 当前 / 上一篇 / 下一篇的一行（后端用指针表达空值，契约里只能由前端声明可空）。 */
export type OutlineArticleRef = Schema<"internal_article_adapter_http.OutlineArticleRefResponse">;

/** 阅读页左栏的读模型；`has_tree=false` 表示不渲染左栏。 */
export type ArticleOutlineContext = Nullable<
  Schema<"internal_article_adapter_http.ArticleOutlineContextResponse">,
  "current" | "previous" | "next"
>;

/** 把 `?from=` 原样透传给后端：格式校验与回退都在服务端一处。 */
export function outlineContextQuery(from?: string): ArticleOutlineContextRequest {
  const trimmed = (from ?? "").trim();
  return trimmed ? { from: trimmed } : {};
}

/**
 * 归属类型（契约里的 owner_kind 数值）→ 来源 kind。
 *
 * 0 / 未知值 / 没有树都返回空串：模板据此不生成链接，不猜一个默认归属。
 */
export function outlineContextKind(
  context: ArticleOutlineContext | null | undefined,
): OutlineSourceKind | "" {
  if (!context?.has_tree) {
    return "";
  }
  return outlineSourceKind(context.owner_kind) ?? "";
}

/** 归属 → 来源参数 `<kind>:<id>`；没有树时给空串。 */
export function outlineContextSource(context: ArticleOutlineContext | null | undefined): string {
  const kind = outlineContextKind(context);
  const ownerId = String(context?.owner_id ?? "").trim();
  if (!kind || !ownerId) {
    return "";
  }
  return outlineSource(kind, ownerId);
}

/**
 * 上一篇 / 下一篇的链接：路径只放目标文章 ID，`from` 沿用当前这棵树。
 *
 * 翻页后阅读页仍显示同一棵树（不串到这篇文章所属的另一棵树）。
 */
export function articleOutlineContextHref(
  context: ArticleOutlineContext | null | undefined,
  articleId: string | number | null | undefined,
): string {
  const from = outlineContextSource(context);
  const target = String(articleId ?? "").trim();
  if (!from || !target) {
    return "";
  }
  return outlineArticleHref(target, from);
}

/** 相邻文章的可见标题（空标题给中性占位，不渲染空按钮）。 */
export function outlineArticleRefTitle(ref: OutlineArticleRef | null | undefined): string {
  return (ref?.article_title ?? "").trim() || "未命名文章";
}

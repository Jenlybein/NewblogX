import type { Nullable, Operation, Schema } from "~/types/contract";

import type { OutlineSourceKind } from "./outlineKind";
import { outlineSourceKind } from "./outlineKind";
import { outlineArticleHref, outlineSource } from "./outlineTree";

/**
 * 阅读页左栏的读模型。
 *
 * 归属由 `?from=<kind>:<id>` 一次给出，取数只经两个具名树端点；没有来源时
 * 左栏按「没有可展示的树」处理，不再有按文章反查归属树的旧共享端点。
 * 形状从具名契约派生，不手写一份。
 */

/** 当前 / 上一篇 / 下一篇的一行（后端用指针表达空值，契约里只能由前端声明可空）。 */
export type OutlineArticleRef = Schema<"internal_article_adapter_http.OutlineArticleRefResponse">;

/** 合集具名树的读模型；current / previous / next 可为 null。 */
export type CollectionOutlineTree = Nullable<
  Schema<"internal_article_adapter_http.CollectionOutlineTreeResponse">,
  "current" | "previous" | "next"
>;

/** 项目具名树的读模型；current / previous / next 可为 null。 */
export type ProjectOutlineTree = Nullable<
  Schema<"internal_article_adapter_http.ProjectOutlineTreeResponse">,
  "current" | "previous" | "next"
>;

/**
 * 阅读页左栏的读模型；`has_tree=false` 表示不渲染左栏。
 *
 * 两个具名形状在左栏需要的字段上逐字同构：从任一份里挑出这些字段，
 * 共用呈现组件 / 版式只消费这一份形状（契约里删字段会在编译期报错）。
 */
export type ArticleOutlineContext = Pick<
  CollectionOutlineTree,
  "has_tree" | "owner_kind" | "owner_id" | "nodes" | "current" | "previous" | "next"
>;

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

// ── 具名端点查询参数 ──────────────────────────────────────────────
//
// 两个具名端点的查询参数都从**本 app 的** openapi.d.ts 派生；owner_kind /
// owner_id 仍在具名响应里，上面的 outlineContextKind / outlineContextSource
// 逐字不变，`?from=` 链接不需要任何兼容层。

/** 合集具名树的查询参数（GET /collections/{id}/outline-tree）。 */
export type CollectionOutlineTreeQuery = NonNullable<
  Operation<"/collections/{id}/outline-tree", "get">["parameters"]["query"]
>;

/** 项目具名树的查询参数（GET /projects/{id}/outline-tree）。 */
export type ProjectOutlineTreeQuery = NonNullable<
  Operation<"/projects/{id}/outline-tree", "get">["parameters"]["query"]
>;

/**
 * 具名树响应 → 阅读页左栏现有的读模型。
 *
 * 两个具名形状在 `has_tree / owner_kind / owner_id / nodes / current /
 * previous / next` 上逐字同构，多出来的 `article_count` / `cover` 只对详情页
 * 有意义。这里显式挑出左栏要的字段，让共用呈现组件、折叠态与翻页链接继续
 * 只消费一份形状；契约里删掉任一字段都会在编译期报错。
 */
export function toArticleOutlineContext(
  value: CollectionOutlineTree | ProjectOutlineTree | null | undefined,
): ArticleOutlineContext | null {
  if (!value) {
    return null;
  }
  return {
    has_tree: value.has_tree ?? false,
    owner_kind: value.owner_kind ?? 0,
    owner_id: value.owner_id ?? "",
    nodes: value.nodes ?? [],
    current: value.current ?? null,
    previous: value.previous ?? null,
    next: value.next ?? null,
  };
}

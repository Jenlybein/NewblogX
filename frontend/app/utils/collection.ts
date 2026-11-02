import type { CollectionArticleItem, CollectionCard } from "~/types/api";

/**
 * 合集（读者站）的纯映射。
 *
 * 为什么抽成纯函数：卡片要处理三种"后端可能给空"的情况——标题空、封面空、
 * 一篇公开文章都没有。它们都是**布局会破**的地方（空标题渲染成一条空白、
 * 空封面渲染成破图），而直接写在模板里就只能靠肉眼回归。
 */

/** 合集卡片的视图模型。 */
export interface CollectionCardView {
  /** 合集 ID（后端契约里是字符串形式的雪花）。 */
  id: string;
  /** 详情页路由（`/collections/<id>`）。 */
  href: string;
  /** 标题；空标题退化成占位文案而不是空白。 */
  title: string;
  /** 描述；空描述给一句"还没有介绍"。 */
  description: string;
  /** 封面 URL；空串表示交给 ArticleCover 的兜底底纹。 */
  cover: string;
  /** "N 篇文章 / 暂无文章"。 */
  countLabel: string;
}

/** 合集详情页路由（路由契约：`/collections/<id>`）。 */
export function collectionHref(id: string | number): string {
  return `/collections/${String(id)}`;
}

/**
 * 把契约里的合集卡片映射成视图模型。
 *
 * 封面回退到"首篇文章的封面"由**后端**做（读模型里已经算好），前端只负责
 * "连首篇都没有"时给空串——此时卡片封面区渲染既有底纹，而不是破图。
 */
export function collectionCardView(card: CollectionCard): CollectionCardView {
  const id = String(card.id ?? "");
  const count = card.article_count ?? 0;
  const title = (card.title ?? "").trim();
  const description = (card.description ?? "").trim();

  return {
    id,
    href: collectionHref(id),
    title: title || "未命名合集",
    description: description || "这个合集还没有介绍。",
    cover: (card.cover ?? "").trim(),
    countLabel: count > 0 ? `${count} 篇文章` : "暂无文章",
  };
}

/** 系列位置文案：读者要能看出"我从第几篇开始读"。 */
export function seriesPositionLabel(sortOrder: number): string {
  return sortOrder > 0 ? `第 ${sortOrder} 篇` : "未编号";
}

/** 合集详情里一篇收录文章的视图模型。 */
export interface CollectionArticleView {
  id: string;
  href: string;
  title: string;
  abstract: string;
  cover: string;
  positionLabel: string;
  /** 作者与分类的展示用副标题（缺失时为空串，调用方自行决定要不要渲染）。 */
  byline: string;
}

/** 把契约里的收录文章映射成视图模型。 */
export function collectionArticleView(item: CollectionArticleItem): CollectionArticleView {
  const title = (item.title ?? "").trim();
  const abstract = (item.abstract ?? "").trim();
  const author = (item.author_nickname ?? "").trim();
  const category = (item.category_title ?? "").trim();

  return {
    id: String(item.article_id ?? ""),
    href: `/article/${String(item.article_id ?? "")}`,
    title: title || "未命名文章",
    abstract,
    cover: (item.cover ?? "").trim(),
    positionLabel: seriesPositionLabel(item.sort_order ?? 0),
    byline: [author, category].filter(Boolean).join(" · "),
  };
}

// 用**同目录相对路径**：后台会跨包 import 本模块（右栏合集推荐卡与预览），
// 而后台的 tsconfig 把 `~` 指向后台自己的 app —— `~/types/api` 在那儿是**后台的**
// 类型文件，没有 `CollectionCard` 这些读者站专属类型。相对路径在两边都指向
// 读者站同一份类型定义。
import type { CollectionCard, CollectionDetail } from "../types/api";
import type { OutlineNode } from "./outlineTree";
import { outlineTreeCount } from "./outlineTree";

/**
 * 合集（读者站）的纯映射。
 *
 * 为什么抽成纯函数：卡片要处理三种"后端可能给空"的情况——标题空、封面空、
 * 一篇公开文章都没有。它们都是**布局会破**的地方（空标题渲染成一条空白、
 * 空封面渲染成破图），而直接写在模板里就只能靠肉眼回归。
 *
 * 详情页的读模型从票 219 起是**嵌套目录树**（`outline`），扁平 articles[] 已退役：
 * 树节点由 `./outlineTree` 统一映射成行，这里只负责把详情响应收敛成页面要的
 * "节点 + 后端算好的公开文章篇数"，不在前端另算一份可见性。
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
  /** 封面 URL；空串表示交给 ArticleCover 渲染一块与父容器同底色的空白占位（票 210）。 */
  cover: string;
  /** "N 篇文章 / 暂无文章"。 */
  countLabel: string;
}

/** 合集详情页的目录树视图。 */
export interface CollectionOutlineView {
  /** 嵌套树根节点；空数组表示"这个合集还没有文章"。 */
  nodes: OutlineNode[];
  /** 公开文章篇数：后端读模型已算好（只数公开文章，不含文件夹）。 */
  articleCount: number;
}

/** 合集详情页路由（路由契约：`/collections/<id>`）。 */
export function collectionHref(id: string | number): string {
  return `/collections/${String(id)}`;
}

/**
 * 把契约里的合集卡片映射成视图模型。
 *
 * 封面回退到"首篇公开文章的封面"由**后端**做（读模型里已经算好），前端只负责
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

/**
 * 合集详情 → 目录树视图。
 *
 * 树节点原样交给 `OutlineTree`（嵌套、层级、来源链接都在 `./outlineTree` 一处
 * 映射）；篇数读后端 `article_count`，不在前端重新遍历一遍可见性——两处判据
 * 一旦分叉，标题里的"共 N 篇"就会和树里的节点对不上。缺字段（老后台响应）时
 * 才回退到树节点计数。
 */
export function collectionOutlineView(
  detail: CollectionDetail | null | undefined,
): CollectionOutlineView {
  const nodes = detail?.outline?.nodes ?? [];
  return {
    nodes,
    articleCount: detail?.article_count ?? outlineTreeCount(nodes),
  };
}

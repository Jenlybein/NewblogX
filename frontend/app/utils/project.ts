// 用**同目录相对路径**：与 collection.ts 同一做法（类型文件在两边都指向读者站
// 同一份定义，不依赖各包的 `~` 别名）。
import type { ProjectArticleItem, ProjectCard } from "../types/api";
import { collectionArticleView, seriesPositionLabel } from "./collection";

/**
 * 项目（读者站）的纯映射。
 *
 * 项目与合集并列、页面形态复用，但新入口内部文案一律写「项目」——
 * 所以占位文案不能直接搬合集的（"未命名合集"这类文案会立刻暴露两套入口混用）。
 *
 * 收录文章的形状与合集完全相同，映射直接复用 `collectionArticleView`：
 * 两处一旦各写一份，"第几篇 / 作者与分类副标题"的格式就会漂移。
 */

/** 项目卡片的视图模型。 */
export interface ProjectCardView {
  /** 项目 ID（后端契约里是字符串形式的雪花）。 */
  id: string;
  /** 详情页路由（`/projects/<id>`）。 */
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

/** 项目详情页路由（路由契约：`/projects/<id>`）。 */
export function projectHref(id: string | number): string {
  return `/projects/${String(id)}`;
}

/**
 * 把契约里的项目卡片映射成视图模型。
 *
 * 封面回退到"首篇公开文章的封面"由**后端**做（读模型里已经算好），前端只负责
 * "连首篇公开文章都没有"时给空串——此时卡片封面区渲染既有底纹，而不是破图。
 */
export function projectCardView(card: ProjectCard): ProjectCardView {
  const id = String(card.id ?? "");
  const count = card.article_count ?? 0;
  const title = (card.title ?? "").trim();
  const description = (card.description ?? "").trim();

  return {
    id,
    href: projectHref(id),
    title: title || "未命名项目",
    description: description || "这个项目还没有介绍。",
    cover: (card.cover ?? "").trim(),
    countLabel: count > 0 ? `${count} 篇文章` : "暂无文章",
  };
}

/** 项目详情里一篇收录文章的视图模型（与合集详情同一形状）。 */
export type ProjectArticleView = ReturnType<typeof collectionArticleView>;

/**
 * 把契约里的收录文章映射成视图模型。
 *
 * 形状与合集完全一致，映射复用合集那一份：位置文案（`seriesPositionLabel`）
 * 与副标题拼法都只有一处真相。
 */
export function projectArticleView(item: ProjectArticleItem): ProjectArticleView {
  return collectionArticleView(item);
}

/** 项目内位置文案：读者要能看出"我从第几篇开始读"（与合集同口径）。 */
export { seriesPositionLabel as projectPositionLabel };

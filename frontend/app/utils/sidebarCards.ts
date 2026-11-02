import type { SiteSidebarCardItem } from "~/types/api";

/**
 * 右栏数据驱动渲染的纯函数层（票 111）。
 *
 * # 为什么单独一层
 *
 * 卡片的种类、顺序、标题、空数据隐藏全是"配置 + 数据 → 渲染模型"的纯计算：
 * 组件只负责把模型画出来，后端只负责把配置与数据发过来。这样规则可以脱离
 * DOM 断言，两端（读者站与后续后台预览）也能各钉同一张契约表。
 *
 * # 契约表
 *
 * `SIDEBAR_CARD_KINDS` / `SIDEBAR_CARD_DEFAULT_TITLES` 必须与后端
 * `internal/site/domain/sidebar.go` 的常量逐字一致：前端靠它们分派渲染，
 * 后端靠它们分派取数。任一端新增/改名一个种类而另一端没跟上，
 * 另一端会把那张卡当成未知种类隐藏 —— 不会画出一个空盒子。
 */
export const SIDEBAR_CARD_KINDS = ["hot_articles", "latest_comments", "tag_cloud"] as const;

/** 内置卡片种类。 */
export type SidebarCardKind = (typeof SIDEBAR_CARD_KINDS)[number];

/** 每种卡的默认标题（运营没填时回退，与后端同一张表）。 */
export const SIDEBAR_CARD_DEFAULT_TITLES: Record<SidebarCardKind, string> = {
  hot_articles: "热门文章",
  latest_comments: "最新评论",
  tag_cloud: "标签云",
};

/** 卡片领域的 eyebrow 小标（纯展示，不参与契约）。 */
export const SIDEBAR_CARD_EYEBROWS: Record<SidebarCardKind, string> = {
  hot_articles: "Hot",
  latest_comments: "Comments",
  tag_cloud: "Tags",
};

/** 热门文章卡的展示项。 */
export interface SidebarArticleView {
  id: string;
  title: string;
  diggCount: number;
  viewCount: number;
}

/** 最新评论卡的展示项。 */
export interface SidebarCommentView {
  id: string;
  content: string;
  articleId: string;
  articleTitle: string;
  userNickname: string;
  createdAt: string;
}

/** 标签云卡的展示项；weight 是 0–1 的字号权重。 */
export interface SidebarTagView {
  id: string;
  title: string;
  articleCount: number;
  weight: number;
}

/** 一张可渲染的卡片：配置（种类/标题/顺序）+ 该种类自己的数据。 */
export interface SidebarCardView {
  key: string;
  kind: SidebarCardKind;
  title: string;
  sortOrder: number;
  articles: SidebarArticleView[];
  comments: SidebarCommentView[];
  tags: SidebarTagView[];
}

// 最小字号 0.75rem、最大 1.25rem：右栏宽度固定，字号跨度再大就会把标签云挤成三行。
const TAG_FONT_MIN_REM = 0.75;
const TAG_FONT_MAX_REM = 1.25;

/** 判断字符串是否是内置卡片种类。 */
export function isSidebarCardKind(value: string): value is SidebarCardKind {
  return (SIDEBAR_CARD_KINDS as readonly string[]).includes(value);
}

/**
 * 标签云的字号权重：按本卡内最大文章数归一。
 *
 * 不按全站最大文章数归一：那样当站点只有一个热标签时，其余标签全挤在最小字号，
 * 标签云会退化成一条平铺列表 —— 而这张卡存在的意义正是"按权重看出大小"。
 */
export function tagCloudWeight(articleCount: number, maxArticleCount: number): number {
  if (!Number.isFinite(articleCount) || !Number.isFinite(maxArticleCount)) {
    return 0;
  }
  if (articleCount <= 0 || maxArticleCount <= 0) {
    return 0;
  }
  return Math.min(1, articleCount / maxArticleCount);
}

/** 把 0–1 的权重映射成 rem 字号（标签云用）。 */
export function tagCloudFontSize(weight: number): string {
  const clamped = Number.isFinite(weight) ? Math.min(1, Math.max(0, weight)) : 0;
  const size = TAG_FONT_MIN_REM + clamped * (TAG_FONT_MAX_REM - TAG_FONT_MIN_REM);
  return `${size.toFixed(2)}rem`;
}

/**
 * 把后端卡片列表收敛成渲染模型：过滤未知种类、按 sort_order 排、空数据隐藏。
 *
 * 三条规则分别对应票面的验收项：
 *   - 只按 kind 读**自己那个数组**（脏数据把评论塞进热门文章卡也不会串门）；
 *   - 空数组 = 这张卡没有数据 = 整块不渲染；
 *   - 标题空白时回退默认标题（后端保存时已回填，这里是二道防线）。
 *
 * 顺序在这里再排一次（后端已经排过）：模型不依赖调用方是否排好序，
 * 前端组件因此不需要知道排序口径。
 */
export function sidebarCardViews(
  cards: readonly SiteSidebarCardItem[] | null | undefined,
): SidebarCardView[] {
  if (!cards?.length) {
    return [];
  }

  const views: Array<{ view: SidebarCardView; index: number }> = [];
  cards.forEach((card, index) => {
    const kind = String(card.kind || "");
    if (!isSidebarCardKind(kind)) {
      return;
    }

    const title = String(card.title || "").trim() || SIDEBAR_CARD_DEFAULT_TITLES[kind];
    const view: SidebarCardView = {
      key: String(card.id || `${kind}-${index}`),
      kind,
      title,
      sortOrder: Number(card.sort_order) || 0,
      articles:
        kind === "hot_articles"
          ? (card.hot_articles || []).map((row) => ({
              id: String(row.id || ""),
              title: String(row.title || ""),
              diggCount: Number(row.digg_count) || 0,
              viewCount: Number(row.view_count) || 0,
            }))
          : [],
      comments:
        kind === "latest_comments"
          ? (card.latest_comments || []).map((row) => ({
              id: String(row.id || ""),
              content: String(row.content || ""),
              articleId: String(row.article_id || ""),
              articleTitle: String(row.article_title || ""),
              userNickname: String(row.user_nickname || ""),
              createdAt: String(row.created_at || ""),
            }))
          : [],
      tags: [],
    };

    if (kind === "tag_cloud") {
      const rows = card.tags || [];
      const maxArticleCount = rows.reduce((max, row) => Math.max(max, Number(row.article_count) || 0), 0);
      view.tags = rows.map((row) => {
        const articleCount = Number(row.article_count) || 0;
        return {
          id: String(row.id || ""),
          title: String(row.title || ""),
          articleCount,
          weight: tagCloudWeight(articleCount, maxArticleCount),
        };
      });
    }

    // 空数据的卡整块隐藏：卡片配置还在（后台可见），只是读者侧不渲染。
    const hasData = view.articles.length + view.comments.length + view.tags.length > 0;
    if (!hasData) {
      return;
    }
    views.push({ view, index });
  });

  return views
    .sort((left, right) => left.view.sortOrder - right.view.sortOrder || left.index - right.index)
    .map((entry) => entry.view);
}

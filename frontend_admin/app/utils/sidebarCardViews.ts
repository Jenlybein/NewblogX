import type { SiteSidebarCardItem } from "~/types/api";

/**
 * 后台预览用的右栏渲染模型（与读者站**同口径、各自实现**）。
 *
 * # 为什么后台不直接 import 读者站那份
 *
 * `deploy/docker/admin/Dockerfile` 的第一行写着「frontend_admin 为自洽应用：构建只依赖
 * 自身 package.json / pnpm-lock.yaml」——镜像里只 `COPY frontend_admin`，构建上下文里
 * 根本没有同级读者站目录。所以后台**不能**在运行时 import 读者站的模块：
 * 本地因为两个目录都在，`nuxt build` 在服务器上才会炸（报错指向读者站的那个
 * 右栏渲染模块，`Could not resolve`）。
 *
 * # 那"两份实现"会不会漂移
 *
 * 不会，因为**契约表共用**：读者站的跨端契约表夹具 被两端用例
 * 一起消费——读者站断言"读者出口 == 契约"，后台断言"预览出口 == 契约"。任一端改了
 * 渲染口径，自己那侧立刻变红。这正是 `#116` 验收项「两端渲染口径由逐字相同的契约表锁定」
 * 要求的机制。
 *
 * # 与读者站唯一的口径差异
 *
 * 富文本卡的正文：读者站要把运营写的那段 HTML 交给浏览器，必须先过 DOMPurify 白名单；
 * 后台预览只是判断版面密度与顺序，不需要在后台再执行一遍清洗，所以这里把它折成**纯文本**
 * （去标签 + 折叠空白）。契约表对富文本卡同样只钉 `contentHtml` 为纯文本形态。
 */

/** 内置卡片种类（与后端 `internal/site/domain/sidebar.go` 的常量逐字一致）。 */
export const SIDEBAR_CARD_KINDS = [
  "hot_articles",
  "latest_comments",
  "tag_cloud",
  "recommended_collections",
  "rich_text",
] as const;

export type SidebarCardKind = (typeof SIDEBAR_CARD_KINDS)[number];

/** 每种卡的默认标题（运营没填时回退，与后端同一张表）。 */
export const SIDEBAR_CARD_DEFAULT_TITLES: Record<SidebarCardKind, string> = {
  hot_articles: "热门文章",
  latest_comments: "最新评论",
  tag_cloud: "标签云",
  recommended_collections: "推荐合集",
  rich_text: "公告",
};

/** 卡片领域的 eyebrow 小标（纯展示，不参与契约）。 */
export const SIDEBAR_CARD_EYEBROWS: Record<SidebarCardKind, string> = {
  hot_articles: "Hot",
  latest_comments: "Comments",
  tag_cloud: "Tags",
  recommended_collections: "Collections",
  rich_text: "Notice",
};

export interface SidebarArticleView {
  id: string;
  title: string;
  diggCount: number;
  viewCount: number;
}

export interface SidebarCommentView {
  id: string;
  content: string;
  articleId: string;
  articleTitle: string;
  userNickname: string;
  createdAt: string;
}

export interface SidebarTagView {
  id: string;
  title: string;
  articleCount: number;
  weight: number;
}

export interface SidebarCollectionView {
  id: string;
  href: string;
  title: string;
  description: string;
  cover: string;
  countLabel: string;
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
  collections: SidebarCollectionView[];
  /** 富文本卡正文（预览用纯文本）；其它种类为空串。 */
  contentHtml: string;
}

export interface SidebarCardViewContext {
  /** 主栏「热门置顶」已经露出的文章 ID；热门文章卡会剔除这些文章。 */
  pinnedArticleIds?: Iterable<string>;
}

const TAG_FONT_MIN_REM = 0.75;
const TAG_FONT_MAX_REM = 1.25;

export function isSidebarCardKind(value: string): value is SidebarCardKind {
  return (SIDEBAR_CARD_KINDS as readonly string[]).includes(value);
}

/** 标签云的字号权重：按本卡内最大文章数归一。 */
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

/** 合集详情路径：与读者站的 `collectionHref` 同一格式（`/collections/{id}`）。 */
function collectionHref(id: string): string {
  return `/collections/${id}`;
}

/** 把一段 HTML 折成纯文本：预览不承担清洗职责，只展示"有多少字"。 */
function toPlainText(html: string): string {
  return html
    .replace(/<(script|style)\b[^>]*>[\s\S]*?<\/\1>/giu, "")
    .replace(/<[^>]*>/gu, "")
    .replace(/&nbsp;/giu, " ")
    .replace(/\s+/gu, " ")
    .trim();
}

export function sidebarCollectionView(
  row: SiteSidebarCardItem["collections"][number],
): SidebarCollectionView {
  const id = String(row.id ?? "");
  const articleCount = Number(row.article_count) || 0;

  return {
    id,
    href: collectionHref(id),
    title: String(row.title ?? "").trim() || "未命名合集",
    description: String(row.description ?? "").trim(),
    cover: String(row.cover ?? "").trim(),
    countLabel: articleCount > 0 ? `${articleCount} 篇文章` : "暂无文章",
  };
}

/**
 * 把后端卡片列表收敛成渲染模型：过滤未知种类、按 sort_order 排、空数据隐藏、置顶去重。
 *
 * 规则与读者站逐条一致（见共享契约表）；差异只有富文本卡正文按纯文本呈现。
 */
export function sidebarCardViews(
  cards: readonly SiteSidebarCardItem[] | null | undefined,
  context: SidebarCardViewContext = {},
): SidebarCardView[] {
  if (!cards?.length) {
    return [];
  }

  const pinnedArticleIds = new Set(Array.from(context.pinnedArticleIds ?? [], (id) => String(id)));

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
          ? (card.hot_articles || [])
              .filter((row) => !pinnedArticleIds.has(String(row.id || "")))
              .map((row) => ({
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
      collections:
        kind === "recommended_collections"
          ? (card.collections || []).map((row) => sidebarCollectionView(row))
          : [],
      contentHtml: kind === "rich_text" ? toPlainText(String(card.content || "")) : "",
    };

    if (kind === "tag_cloud") {
      const rows = card.tags || [];
      const maxArticleCount = rows.reduce(
        (max, row) => Math.max(max, Number(row.article_count) || 0),
        0,
      );
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

    const hasData =
      view.articles.length + view.comments.length + view.tags.length + view.collections.length > 0 ||
      view.contentHtml.length > 0;
    if (!hasData) {
      return;
    }
    views.push({ view, index });
  });

  return views
    .sort((left, right) => left.view.sortOrder - right.view.sortOrder || left.index - right.index)
    .map((entry) => entry.view);
}

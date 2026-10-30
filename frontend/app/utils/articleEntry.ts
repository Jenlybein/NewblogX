// 文章条目（卡片）的统一视图模型。
//
// 为什么要有它：同一张卡片要出现在三处 —— 列表（首页/作者文章流）、搜索结果、
// 用户主页的收藏架 —— 但三处的数据源不同（`SearchArticleItem` 与
// `FavoriteArticleItem`）。如果让卡片组件自己判断"这是哪种数据"，模板里会长出
// 三套分支；把"数据 → 条目视图"收进纯函数后，卡片只消费一种形状，三处天然同形态。
//
// 本模块是纯 TS：不 import Nuxt / Vue，单测直接 import。
import type { FavoriteArticleItem, SearchArticleItem } from "~/types/api";
import { formatCount, formatDateLabel, formatDateTimeLabel } from "~/utils/format";

export type ArticleEntryVariant = "list" | "search" | "shelf";

export interface ArticleEntryTag {
  id: string;
  title: string;
}

export interface ArticleEntryAuthor {
  /** 收藏架数据里没有作者 ID，此时只展示昵称、不生成作者主页链接。 */
  id: string;
  nickname: string;
  avatar: string;
}

export interface ArticleEntryStats {
  digg: number;
  favor: number;
  comment: number;
  view: number;
}

export interface ArticleEntryData {
  id: string;
  /** 文章详情页地址；三处都指向同一路由（路由不改名）。 */
  href: string;
  title: string;
  abstract: string;
  cover: string;
  dateLabel: string;
  categoryLabel: string;
  statusLabel: string;
  author: ArticleEntryAuthor | null;
  tags: ArticleEntryTag[];
  stats: ArticleEntryStats;
  /** 收藏架特有的一行附加信息（收藏时间 / 阅读 / 收藏数），其他变体为空。 */
  shelfMeta: string[];
}

const FALLBACK_TITLE = "未命名文章";
const FALLBACK_ABSTRACT = "这篇文章暂时没有摘要。";

export const EMPTY_ARTICLE_ENTRY: ArticleEntryData = {
  id: "",
  href: "",
  title: FALLBACK_TITLE,
  abstract: FALLBACK_ABSTRACT,
  cover: "",
  dateLabel: "",
  categoryLabel: "",
  statusLabel: "",
  author: null,
  tags: [],
  stats: { digg: 0, favor: 0, comment: 0, view: 0 },
  shelfMeta: [],
};

function normalizeTags(tags?: Array<{ id?: string; title?: string }> | null): ArticleEntryTag[] {
  return (tags || [])
    .map((tag) => ({ id: String(tag.id || ""), title: String(tag.title || "").trim() }))
    .filter((tag) => tag.id && tag.title);
}

function count(value?: number | null) {
  return Number.isFinite(value) ? Number(value) : 0;
}

/** 列表（首页 / 作者文章流）与搜索结果的数据 → 卡片视图。 */
export function fromSearchArticle(article: SearchArticleItem): ArticleEntryData {
  const title = (article.highlight?.title || article.title || "").trim();
  const abstract = (article.highlight?.abstract || article.abstract || "").trim();
  const authorNickname = (article.author?.nickname || "").trim();

  return {
    id: String(article.id || ""),
    href: `/article/${article.id || ""}`,
    title: title || FALLBACK_TITLE,
    abstract: abstract || FALLBACK_ABSTRACT,
    cover: (article.cover || "").trim(),
    dateLabel: formatDateLabel(article.created_at),
    categoryLabel: (article.category?.title || "").trim(),
    statusLabel: "",
    author: authorNickname
      ? {
          id: String(article.author?.id || ""),
          nickname: authorNickname,
          avatar: (article.author?.avatar || "").trim(),
        }
      : null,
    tags: normalizeTags(article.tags),
    stats: {
      digg: count(article.digg_count),
      favor: count(article.favor_count),
      comment: count(article.comment_count),
      view: count(article.view_count),
    },
    shelfMeta: [],
  };
}

/** 收藏架的数据 → 卡片视图（与列表同一形态，只多一行收藏元信息）。 */
export function fromFavoriteArticle(article: FavoriteArticleItem): ArticleEntryData {
  const title = String(article.title || "").trim();
  const abstract = String(article.abstract || "").trim();
  const authorNickname = String(article.user_nickname || "").trim();
  const statusLabel =
    article.article_status === 3 ? "已发布" : article.article_status ? `状态 ${article.article_status}` : "";

  return {
    id: String(article.article_id || ""),
    href: `/article/${article.article_id || ""}`,
    title: title || FALLBACK_TITLE,
    abstract: abstract || FALLBACK_ABSTRACT,
    cover: String(article.cover || "").trim(),
    dateLabel: formatDateTimeLabel(article.favorited_at),
    categoryLabel: "",
    statusLabel,
    author: authorNickname
      ? { id: "", nickname: authorNickname, avatar: String(article.user_avatar || "").trim() }
      : null,
    tags: [],
    stats: {
      digg: count(article.digg_count),
      favor: count(article.favor_count),
      comment: count(article.comment_count),
      view: count(article.view_count),
    },
    shelfMeta: [
      `${formatCount(count(article.view_count))} 阅读`,
      `${formatCount(count(article.favor_count))} 收藏`,
    ],
  };
}

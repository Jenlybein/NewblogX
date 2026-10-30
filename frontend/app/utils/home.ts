// 首页的纯逻辑（从模板里抽出来，便于单测）：右栏分区开关、标签/作者聚合、
// 区块降级态、首屏指标与置顶条目视图。
//
// 为什么抽出来：首页是"接口失败也得体面"的地方，五种降级态散在模板里最容易漏；
// 置顶/最新/右栏三块的数据聚合也不依赖 DOM。模板只做渲染。
import type { SearchArticleItem, SiteRuntimeConfig } from "~/types/api";
import { buildSearchLocation } from "~/utils/chrome";

export interface HomeSidebarVisibility {
  notice: boolean;
  tags: boolean;
  authors: boolean;
}

/**
 * 右栏三个模块的开关。`index_right.list` 为空时视为"全开"（旧行为）；
 * 有配置时按 title 精确匹配 `site_notice` / `hot_tags` / `recommended_authors`。
 */
export function homeSidebarVisibility(runtimeConfig: SiteRuntimeConfig | null): HomeSidebarVisibility {
  const modules = new Set(
    (runtimeConfig?.index_right?.list || [])
      .map((item) => String(item.title || "").trim())
      .filter(Boolean),
  );
  const allEnabled = modules.size === 0;
  return {
    notice: allEnabled || modules.has("site_notice"),
    tags: allEnabled || modules.has("hot_tags"),
    authors: allEnabled || modules.has("recommended_authors"),
  };
}

/** 公告卡里展示的登录方式（只展示已启用的）。 */
export function homeLoginModes(runtimeConfig: SiteRuntimeConfig | null): string[] {
  const login = runtimeConfig?.login;
  const modes: string[] = [];
  if (login?.pwd) modes.push("密码登录");
  if (login?.email) modes.push("邮箱登录");
  if (login?.qq) modes.push("QQ 登录");
  return modes;
}

export interface HomeTagSummary {
  id: string;
  title: string;
  count: number;
}

/** 从最新文章列表里聚合热门标签：按出现次数降序，同频按 id 稳定排序。 */
export function aggregateHotTags(articles: SearchArticleItem[], limit = 8): HomeTagSummary[] {
  const tagMap = new Map<string, HomeTagSummary>();
  for (const article of articles) {
    for (const tag of article.tags || []) {
      const current = tagMap.get(tag.id);
      if (current) {
        current.count += 1;
      } else {
        tagMap.set(tag.id, { id: tag.id, title: tag.title, count: 1 });
      }
    }
  }
  return [...tagMap.values()]
    .sort((left, right) => right.count - left.count || left.id.localeCompare(right.id))
    .slice(0, limit);
}

export interface HomeAuthorSummary {
  id: string;
  nickname: string;
  avatar: string;
  articleCount: number;
  totalViews: number;
}

/** 从最新文章列表里聚合推荐作者：按文章数降序，其次按总阅读降序。 */
export function aggregateRecommendedAuthors(articles: SearchArticleItem[], limit = 4): HomeAuthorSummary[] {
  const authorMap = new Map<string, HomeAuthorSummary>();
  for (const article of articles) {
    const current = authorMap.get(article.author.id);
    if (current) {
      current.articleCount += 1;
      current.totalViews += article.view_count;
      continue;
    }
    authorMap.set(article.author.id, {
      id: article.author.id,
      nickname: article.author.nickname,
      avatar: article.author.avatar,
      articleCount: 1,
      totalViews: article.view_count,
    });
  }
  return [...authorMap.values()]
    .sort((left, right) => right.articleCount - left.articleCount || right.totalViews - left.totalViews)
    .slice(0, limit);
}

export type HomeSectionKind = "loading" | "error" | "empty" | "ready";

export interface HomeSectionState {
  kind: HomeSectionKind;
  badge: string;
  title: string;
  description: string;
  hints: string[];
}

export interface HomeSectionInput {
  pending: boolean;
  failed: boolean;
  itemCount: number;
}

/**
 * 首页各区块（置顶/最新）的降级态。顺序：加载中 → 失败 → 空 → 有数据。
 * 失败但有缓存条目时仍算 ready，徽标说明是缓存，避免把已有内容整块替换成错误态。
 */
export function resolveHomeSectionState(input: HomeSectionInput): HomeSectionState {
  if (input.pending && input.itemCount === 0) {
    return {
      kind: "loading",
      badge: "加载中",
      title: "正在读取内容",
      description: "首页数据正在加载，稍后会自然出现。",
      hints: [],
    };
  }

  if (input.failed && input.itemCount === 0) {
    return {
      kind: "error",
      badge: "暂不可用",
      title: "这块内容暂时取不到",
      description: "接口没有返回数据，可能是后端未启动或网络不可用。",
      hints: ["可以先重试；仍然失败时就先看别的区块，不会影响整页浏览。"],
    };
  }

  if (input.itemCount === 0) {
    return {
      kind: "empty",
      badge: "空",
      title: "还没有公开内容",
      description: "这里会展示最新的公开文章，当前还没有可展示的条目。",
      hints: ["可以去搜索页按关键词或标签浏览，或者过一会儿再回来。"],
    };
  }

  return {
    kind: "ready",
    badge: input.failed ? `${input.itemCount} 篇（缓存）` : `${input.itemCount} 篇`,
    title: "",
    description: "",
    hints: [],
  };
}

/** 首页"去搜索"的固定落点：复用页头同一个 URL 契约（/search，空关键词不拼 ?key=）。 */
export function homeSearchLocation(): ReturnType<typeof buildSearchLocation> {
  return buildSearchLocation("");
}

export interface PinnedArticleView {
  id: string;
  href: string;
  title: string;
  abstract: string;
  cover: string;
  categoryLabel: string;
  authorName: string;
  viewCount: number;
}

/** 置顶卡片的视图（置顶接口只有 7 个字段，和列表卡片的数据源不同，单独归一化）。 */
export function pinnedArticleView(article: {
  id?: string;
  title?: string;
  abstract?: string;
  cover?: string;
  category_title?: string;
  author_nickname?: string;
  view_count?: number;
}): PinnedArticleView {
  const id = String(article.id || "");
  return {
    id,
    href: `/article/${id}`,
    title: String(article.title || "").trim() || "未命名文章",
    abstract: String(article.abstract || "").trim() || "这篇文章暂时没有摘要。",
    cover: String(article.cover || "").trim(),
    categoryLabel: String(article.category_title || "").trim(),
    authorName: String(article.author_nickname || "").trim(),
    viewCount: Number.isFinite(article.view_count) ? Number(article.view_count) : 0,
  };
}

// 首页的纯逻辑（从模板里抽出来，便于单测）：区块降级态、首屏指标与置顶条目视图。
//
// 为什么抽出来：首页是"接口失败也得体面"的地方，五种降级态散在模板里最容易漏；
// 置顶/最新两块的数据聚合也不依赖 DOM。模板只做渲染。
//
// 右栏的渲染模型在 `~/utils/sidebarCards`：卡片种类、顺序、标题、空数据隐藏
// 都由后端卡片数据决定，本文件不再参与（旧的分区开关已随旧配置一起下线）。
import { buildSearchLocation } from "~/utils/chrome";

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
 * 首页「最新文章」区块的降级态。顺序：加载中 → 失败 → 空 → 有数据。
 * 失败但有缓存条目时仍算 ready，徽标说明是缓存，避免把已有内容整块替换成错误态。
 *
 * 只服务最新文章一块：热门置顶是运营位，无数据 / 加载中 / 失败一律整块不渲染
 * （连标题与徽标都不出现），没有降级态可表达，见 ADR-0015 决策 5。
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

/** 置顶卡片的视图（热门置顶接口只有 7 个字段，和列表卡片的数据源不同，单独归一化）。 */
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

import { describe, expect, it } from "vitest";

import type { SearchArticleItem, SiteRuntimeConfig } from "~/types/api";
import {
  aggregateHotTags,
  aggregateRecommendedAuthors,
  homeHeroFacts,
  homeLoginModes,
  homeSearchLocation,
  homeSidebarVisibility,
  pinnedArticleView,
  resolveHomeSectionState,
} from "~/utils/home";

function article(overrides: Record<string, unknown> = {}): SearchArticleItem {
  return {
    id: "a1",
    title: "标题",
    abstract: "",
    cover: "",
    created_at: "2026-10-01T00:00:00Z",
    category: { title: "工程" },
    author: { id: "u1", nickname: "作者甲", avatar: "" },
    tags: [],
    digg_count: 0,
    favor_count: 0,
    comment_count: 0,
    view_count: 0,
    ...overrides,
  } as unknown as SearchArticleItem;
}

const runtimeConfig = {
  site_info: { title: "站点" },
  project: { title: "项目", abstract: "项目摘要" },
  login: { pwd: true, email: false, qq: true },
  index_right: { list: [{ title: "hot_tags" }] },
  article: {},
  comment: {},
} as unknown as SiteRuntimeConfig;

describe("homeSidebarVisibility / homeLoginModes", () => {
  it("配置为空时三个模块全开；按 title 精确匹配关闭其它模块", () => {
    expect(homeSidebarVisibility(null)).toEqual({ notice: true, tags: true, authors: true });

    const emptyList = { ...runtimeConfig, index_right: { list: [] } };
    expect(homeSidebarVisibility(emptyList)).toEqual({ notice: true, tags: true, authors: true });

    const onlyTags = homeSidebarVisibility(runtimeConfig);
    expect(onlyTags).toEqual({ notice: false, tags: true, authors: false });
  });

  it("登录方式只列已启用的", () => {
    expect(homeLoginModes(runtimeConfig)).toEqual(["密码登录", "QQ 登录"]);
    expect(homeLoginModes(null)).toEqual([]);
  });
});

describe("热门标签 / 推荐作者聚合", () => {
  it("标签按出现次数降序、同频按 id 稳定排序，并支持 limit", () => {
    const tags = (titles: Array<[string, string]>) => titles.map(([id, title]) => ({ id, title }));

    const items = [
      article({ id: "a", tags: tags([["t1", "Nuxt"], ["t2", "部署"]]) }),
      article({ id: "b", tags: tags([["t2", "部署"]]) }),
      article({ id: "c", tags: tags([["t3", "Go"], ["t2", "部署"]]) }),
    ];

    const hot = aggregateHotTags(items, 2);
    expect(hot).toHaveLength(2);
    expect(hot[0]).toMatchObject({ id: "t2", count: 3 });
    expect(hot[1]?.id).toBe("t1");
  });

  it("作者按文章数降序、再按总阅读降序", () => {
    const items = [
      article({ id: "a", author: { id: "u1", nickname: "甲", avatar: "" }, view_count: 10 }),
      article({ id: "b", author: { id: "u2", nickname: "乙", avatar: "" }, view_count: 900 }),
      article({ id: "c", author: { id: "u2", nickname: "乙", avatar: "" }, view_count: 100 }),
    ];

    const authors = aggregateRecommendedAuthors(items, 2);
    expect(authors).toHaveLength(2);
    expect(authors[0]).toMatchObject({ id: "u2", articleCount: 2, totalViews: 1000 });
    expect(authors[1]).toMatchObject({ id: "u1", articleCount: 1 });
  });

  it("空列表聚合结果为空数组（不抛错）", () => {
    expect(aggregateHotTags([])).toEqual([]);
    expect(aggregateRecommendedAuthors([])).toEqual([]);
  });
});

describe("首页区块降级态", () => {
  it("加载中：徽标显示加载中，不显示错误/空态文案", () => {
    const state = resolveHomeSectionState({ pending: true, failed: false, itemCount: 0 });
    expect(state.kind).toBe("loading");
    expect(state.badge).toBe("加载中");
    expect(state.title).toContain("正在读取");
  });

  it("请求失败且无数据：错误态 + 可执行的重试指引", () => {
    const state = resolveHomeSectionState({ pending: false, failed: true, itemCount: 0 });
    expect(state.kind).toBe("error");
    expect(state.badge).toBe("暂不可用");
    expect(state.hints.length).toBeGreaterThan(0);
  });

  it("空数据：空态给出下一步（去搜索）", () => {
    const state = resolveHomeSectionState({ pending: false, failed: false, itemCount: 0 });
    expect(state.kind).toBe("empty");
    expect(state.badge).toBe("空");
    expect(state.description).toContain("最新");
  });

  it("有数据：ready；失败但有缓存条目时 ready 且徽标标注缓存", () => {
    expect(resolveHomeSectionState({ pending: false, failed: false, itemCount: 3 })).toMatchObject({
      kind: "ready",
      badge: "3 篇",
    });
    expect(resolveHomeSectionState({ pending: false, failed: true, itemCount: 2 })).toMatchObject({
      kind: "ready",
      badge: "2 篇（缓存）",
    });
  });
});

describe("首屏指标 / 搜索落点 / 置顶卡片视图", () => {
  it("事实条数字来自真实数据", () => {
    expect(homeHeroFacts({ topCount: 3, latestCount: 9, pageSize: 9 })).toEqual([
      { label: "置顶", value: "3" },
      { label: "本页最新", value: "9" },
      { label: "每页", value: "9 篇" },
    ]);
  });

  it("首页搜索落点复用页头契约：/search 且空关键词不带 ?key=", () => {
    expect(homeSearchLocation()).toEqual({ path: "/search", query: {} });
  });

  it("置顶卡片归一化：链接、兜底标题/摘要、缺字段时安全", () => {
    expect(
      pinnedArticleView({
        id: "a1",
        title: "置顶文章",
        abstract: "",
        category_title: "工程",
        author_nickname: "作者甲",
        view_count: 120,
      }),
    ).toMatchObject({
      id: "a1",
      href: "/article/a1",
      title: "置顶文章",
      abstract: "这篇文章暂时没有摘要。",
      categoryLabel: "工程",
      authorName: "作者甲",
      viewCount: 120,
    });

    expect(pinnedArticleView({})).toMatchObject({
      id: "",
      href: "/article/",
      title: "未命名文章",
      abstract: "这篇文章暂时没有摘要。",
      viewCount: 0,
    });
  });
});

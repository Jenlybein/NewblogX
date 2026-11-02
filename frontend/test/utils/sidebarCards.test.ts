import { describe, expect, it } from "vitest";

import type { SiteSidebarCardItem } from "~/types/api";
import {
  SIDEBAR_CARD_DEFAULT_TITLES,
  SIDEBAR_CARD_KINDS,
  isSidebarCardKind,
  sidebarCardViews,
  tagCloudFontSize,
  tagCloudWeight,
} from "~/utils/sidebarCards";

function card(partial: Partial<SiteSidebarCardItem> & Pick<SiteSidebarCardItem, "kind">): SiteSidebarCardItem {
  return {
    id: `card-${partial.kind}`,
    kind: partial.kind,
    title: "",
    sort_order: 0,
    hot_articles: [],
    latest_comments: [],
    tags: [],
    ...partial,
  };
}

function article(id: string, title: string, digg = 0) {
  return { id, title, digg_count: digg, view_count: digg * 10 };
}

function comment(id: string, content: string, articleId = `a-${id}`) {
  return {
    id,
    content,
    article_id: articleId,
    article_title: `文章 ${articleId}`,
    user_nickname: "读者甲",
    created_at: "2026-11-02T00:00:00Z",
  };
}

function tag(id: string, title: string, count = 0) {
  return { id, title, article_count: count };
}

describe("sidebarCardViews：右栏卡片的渲染模型", () => {
  it("只认内置种类，未知种类整张卡忽略（链路里的旧配置不能变成空盒子）", () => {
    const views = sidebarCardViews([
      card({ kind: "hot_articles", hot_articles: [article("1", "热门一")] }),
      card({ kind: "legacy_notice", title: "旧公告" }),
      card({ kind: "ai_card", title: "AI" }),
    ]);

    expect(views.map((view) => view.kind)).toEqual(["hot_articles"]);
  });

  it("按 sort_order 升序渲染，标题用配置里的标题", () => {
    const views = sidebarCardViews([
      card({ kind: "tag_cloud", title: "标签云", sort_order: 2, tags: [tag("t1", "Go", 5)] }),
      card({ kind: "hot_articles", title: "本周热读", sort_order: 0, hot_articles: [article("1", "热读一")] }),
      card({ kind: "latest_comments", title: "大家怎么说", sort_order: 1, latest_comments: [comment("c1", "清楚")] }),
    ]);

    expect(views.map((view) => [view.kind, view.title])).toEqual([
      ["hot_articles", "本周热读"],
      ["latest_comments", "大家怎么说"],
      ["tag_cloud", "标签云"],
    ]);
  });

  it("标题为空时回退到内置默认标题，不渲染空标题卡", () => {
    const views = sidebarCardViews([
      card({ kind: "hot_articles", title: "   ", hot_articles: [article("1", "热门一")] }),
      card({ kind: "latest_comments", title: "最新评论", latest_comments: [comment("c1", "说得好")] }),
    ]);

    expect(views[0]?.title).toBe(SIDEBAR_CARD_DEFAULT_TITLES.hot_articles);
    expect(views[1]?.title).toBe("最新评论");
  });

  it("无数据的卡片整块隐藏（包括停用/空数组两种情况）", () => {
    const views = sidebarCardViews([
      card({ kind: "hot_articles", title: "热门文章", hot_articles: [] }),
      card({ kind: "latest_comments", title: "最新评论", latest_comments: [comment("c1", "内容")] }),
      card({ kind: "tag_cloud", title: "标签云", tags: [tag("t1", "Go", 3)] }),
    ]);

    expect(views.map((view) => view.kind)).toEqual(["latest_comments", "tag_cloud"]);
  });

  it("每张卡只带自己种类的数据，别的数组不串门", () => {
    const views = sidebarCardViews([
      card({
        kind: "hot_articles",
        title: "热门文章",
        hot_articles: [article("1", "热门一", 42)],
        // 脏数据：热门文章卡里塞了评论与标签，也不该被渲染。
        latest_comments: [comment("c1", "不该出现")],
        tags: [tag("t1", "不该出现")],
      }),
    ]);

    expect(views[0]?.articles).toHaveLength(1);
    expect(views[0]?.comments).toEqual([]);
    expect(views[0]?.tags).toEqual([]);
  });

  it("三种内置种类的清单与默认标题都在契约表里（两端各钉同一张表）", () => {
    expect(SIDEBAR_CARD_KINDS).toEqual(["hot_articles", "latest_comments", "tag_cloud"]);
    expect(SIDEBAR_CARD_DEFAULT_TITLES).toEqual({
      hot_articles: "热门文章",
      latest_comments: "最新评论",
      tag_cloud: "标签云",
    });
  });
});

/**
 * 热门文章卡与主栏「热门置顶」去重。
 *
 * 验收口径是"同一篇文章不能两边都出现"：主栏已经承担了置顶文章的露出，
 * 右栏再出现一次就是重复推荐。去重放在渲染模型里，是因为主栏置顶与右栏卡片
 * 是两个独立请求（各走 30s SWR），两边数据可能短暂不同步 —— 模型必须
 * 按**当前**的置顶列表收敛，而不依赖后端恰好在同一时刻排除了它们。
 */
describe("sidebarCardViews：热门文章与热门置顶去重", () => {
  it("置顶文章从热门文章卡里剔除，其余文章保持后端的点赞降序", () => {
    const views = sidebarCardViews(
      [
        card({
          kind: "hot_articles",
          title: "热门文章",
          hot_articles: [article("1", "已置顶", 50), article("2", "热门二", 30), article("3", "热门三", 10)],
        }),
      ],
      { pinnedArticleIds: ["1"] },
    );

    expect(views).toHaveLength(1);
    expect(views[0]?.articles.map((item) => [item.id, item.diggCount])).toEqual([
      ["2", 30],
      ["3", 10],
    ]);
  });

  it("热门文章卡里的文章全部被置顶时，整张卡隐藏（不留空盒子）", () => {
    const views = sidebarCardViews(
      [
        card({ kind: "hot_articles", title: "热门文章", hot_articles: [article("1", "置顶甲"), article("2", "置顶乙")] }),
        card({ kind: "latest_comments", title: "最新评论", latest_comments: [comment("c1", "还在")] }),
      ],
      { pinnedArticleIds: new Set(["1", "2"]) },
    );

    expect(views.map((view) => view.kind)).toEqual(["latest_comments"]);
  });

  it("没有置顶数据时一篇都不少（去重不能变成误删）", () => {
    const views = sidebarCardViews(
      [card({ kind: "hot_articles", title: "热门文章", hot_articles: [article("1", "热门一"), article("2", "热门二")] })],
      { pinnedArticleIds: [] },
    );

    expect(views[0]?.articles.map((item) => item.id)).toEqual(["1", "2"]);
  });

  it("只影响热门文章卡：同一篇文章仍可作为评论卡的所属文章出现", () => {
    const views = sidebarCardViews(
      [
        card({
          kind: "hot_articles",
          title: "热门文章",
          hot_articles: [article("1", "已置顶")],
        }),
        card({
          kind: "latest_comments",
          title: "最新评论",
          latest_comments: [comment("c1", "评论正文", "1")],
        }),
      ],
      { pinnedArticleIds: ["1"] },
    );

    expect(views.map((view) => view.kind)).toEqual(["latest_comments"]);
    expect(views[0]?.comments[0]?.articleId).toBe("1");
  });
});

describe("tagCloudWeight：标签云字号权重", () => {
  it("按最大文章数归一：最大值是 1，其余按比例", () => {
    expect(tagCloudWeight(10, 10)).toBe(1);
    expect(tagCloudWeight(5, 10)).toBe(0.5);
    expect(tagCloudWeight(1, 10)).toBe(0.1);
  });

  it("全部为零或最大值为零时不除零，回退到 0（用最小字号）", () => {
    expect(tagCloudWeight(0, 0)).toBe(0);
    expect(tagCloudWeight(3, 0)).toBe(0);
  });

  it("标签卡的数据里带上了各自权重，供渲染决定字号", () => {
    const views = sidebarCardViews([
      card({
        kind: "tag_cloud",
        title: "标签云",
        tags: [tag("t1", "Go", 10), tag("t2", "Nuxt", 5), tag("t3", "部署", 1)],
      }),
    ]);

    expect(views[0]?.tags.map((t) => [t.title, t.weight])).toEqual([
      ["Go", 1],
      ["Nuxt", 0.5],
      ["部署", 0.1],
    ]);
  });

  it("权重映射成字号：最小 0.75rem、最大 1.25rem，越界值被钳制", () => {
    expect(tagCloudFontSize(0)).toBe("0.75rem");
    expect(tagCloudFontSize(0.5)).toBe("1.00rem");
    expect(tagCloudFontSize(1)).toBe("1.25rem");
    // 脏数据不能让字号跑出右栏能承受的跨度。
    expect(tagCloudFontSize(-1)).toBe("0.75rem");
    expect(tagCloudFontSize(2)).toBe("1.25rem");
    expect(tagCloudFontSize(Number.NaN)).toBe("0.75rem");
  });
});

describe("isSidebarCardKind：种类收窄", () => {
  it("只把三种内置种类收窄成合法类型", () => {
    expect(isSidebarCardKind("hot_articles")).toBe(true);
    expect(isSidebarCardKind("latest_comments")).toBe(true);
    expect(isSidebarCardKind("tag_cloud")).toBe(true);
    expect(isSidebarCardKind("ai_card")).toBe(false);
    expect(isSidebarCardKind("")).toBe(false);
  });
});

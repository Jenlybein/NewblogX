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
    collections: [],
    content: "",
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

function collection(id: string, title: string, count = 0, overrides = {}) {
  return {
    id,
    title,
    description: `简介 ${id}`,
    cover: `https://cdn.example.com/${id}.png`,
    article_count: count,
    ...overrides,
  };
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

  it("内置种类的清单与默认标题都在契约表里（两端各钉同一张表）", () => {
    expect(SIDEBAR_CARD_KINDS).toEqual([
      "hot_articles",
      "latest_comments",
      "tag_cloud",
      "recommended_collections",
      "rich_text",
    ]);
    expect(SIDEBAR_CARD_DEFAULT_TITLES).toEqual({
      hot_articles: "热门文章",
      latest_comments: "最新评论",
      tag_cloud: "标签云",
      recommended_collections: "推荐合集",
      rich_text: "公告",
    });
  });
});

/**
 * 富文本卡：正文是运营在后台写的一段 HTML，读者站必须**先清洗再渲染**。
 *
 * 清洗放在渲染模型里，而不是组件模板里：这样"恶意输入会被剥离"可以在纯函数层
 * 断言，不必等 DOM 挂载；组件只负责把清洗后的 HTML 画出来。
 */
describe("sidebarCardViews：富文本卡与清洗", () => {
  it("合法排版内容原样保留，读者站拿到的就是可渲染的 HTML", () => {
    const views = sidebarCardViews([
      card({
        kind: "rich_text",
        title: "站点公告",
        content: "<h2>更新日志</h2><p>本周 <strong>重点</strong></p><ul><li>第一条</li></ul>",
      }),
    ]);

    expect(views).toHaveLength(1);
    expect(views[0]?.kind).toBe("rich_text");
    expect(views[0]?.contentHtml).toContain("<h2>更新日志</h2>");
    expect(views[0]?.contentHtml).toContain("<strong>重点</strong>");
    expect(views[0]?.contentHtml).toContain("<li>第一条</li>");
  });

  it("script / onerror / javascript: / data: / iframe 都在渲染前被剥离", () => {
    const views = sidebarCardViews([
      card({
        kind: "rich_text",
        title: "公告",
        content: [
          "<p>正文</p>",
          "<script>alert(1)</script>",
          '<img src="https://cdn.example.com/a.png" onerror="alert(1)">',
          '<a href="javascript:alert(1)">点我</a>',
          '<a href="data:text/html;base64,PHNjcmlwdD4=">伪协议</a>',
          '<iframe src="https://evil.example.com"></iframe>',
        ].join(""),
      }),
    ]);

    const html = views[0]?.contentHtml ?? "";
    expect(html).toContain("正文");
    expect(html).not.toContain("<script");
    expect(html.toLowerCase()).not.toContain("onerror");
    expect(html).not.toContain("javascript:");
    expect(html).not.toContain("data:");
    expect(html).not.toContain("iframe");
  });

  it("正文清洗后为空（纯空白或只剩被剥掉的标签）时整张卡隐藏，不留空盒子", () => {
    const views = sidebarCardViews([
      card({ kind: "rich_text", title: "公告", content: "<script>alert(1)</script>   " }),
      card({ kind: "tag_cloud", title: "标签云", tags: [tag("t1", "Go", 3)] }),
    ]);

    expect(views.map((view) => view.kind)).toEqual(["tag_cloud"]);
  });

  it("富文本卡不串用别的种类的数据，别的卡也不读 content", () => {
    const views = sidebarCardViews([
      card({ kind: "rich_text", title: "公告", content: "<p>公告正文</p>", hot_articles: [article("1", "不该出现")] }),
      card({ kind: "hot_articles", title: "热门文章", content: "<p>不该被热门卡渲染</p>", hot_articles: [article("2", "热读一")] }),
    ]);

    expect(views[0]?.contentHtml).toContain("公告正文");
    expect(views[0]?.articles).toEqual([]);
    expect(views[1]?.contentHtml).toBe("");
    expect(views[1]?.articles).toHaveLength(1);
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

describe("推荐合集卡：种类分派与渲染模型", () => {
  it("该种类只读自己的数组，映射出详情链接、标题、简介与文章数文案", () => {
    const views = sidebarCardViews([
      card({
        kind: "recommended_collections",
        title: "往期专题",
        collections: [
          collection("101", "从零搭一个博客", 7),
          collection("102", "工程效率", 0),
        ],
      }),
    ]);

    expect(views).toHaveLength(1);
    expect(views[0]?.collections).toEqual([
      {
        id: "101",
        href: "/collections/101",
        title: "从零搭一个博客",
        description: "简介 101",
        cover: "https://cdn.example.com/101.png",
        countLabel: "7 篇文章",
      },
      {
        id: "102",
        href: "/collections/102",
        title: "工程效率",
        description: "简介 102",
        cover: "https://cdn.example.com/102.png",
        countLabel: "暂无文章",
      },
    ]);
    // 别种数据一个都不沾（合集卡没有文章/评论/标签）。
    expect(views[0]?.articles).toEqual([]);
    expect(views[0]?.comments).toEqual([]);
    expect(views[0]?.tags).toEqual([]);
  });

  it("没有可推荐的合集时整张卡隐藏（空数组即不渲染）", () => {
    const views = sidebarCardViews([
      card({ kind: "recommended_collections", title: "推荐合集", collections: [] }),
      card({ kind: "tag_cloud", title: "标签云", tags: [tag("t1", "Go", 3)] }),
    ]);

    expect(views.map((view) => view.kind)).toEqual(["tag_cloud"]);
  });

  it("标题为空时回退到「推荐合集」，空标题的合集退化成占位标题而不是空白", () => {
    const views = sidebarCardViews([
      card({
        kind: "recommended_collections",
        title: "   ",
        collections: [
          collection("101", "  ", 2, { description: "  ", cover: "" }),
        ],
      }),
    ]);

    expect(views[0]?.title).toBe(SIDEBAR_CARD_DEFAULT_TITLES.recommended_collections);
    expect(views[0]?.collections[0]?.title).toBe("未命名合集");
    expect(views[0]?.collections[0]?.description).toBe("");
    expect(views[0]?.collections[0]?.cover).toBe("");
    expect(views[0]?.collections[0]?.href).toBe("/collections/101");
  });

  it("推荐合集卡按 sort_order 与其它卡一起排序（顺序仍由既有配置控制）", () => {
    const views = sidebarCardViews([
      card({ kind: "tag_cloud", title: "标签云", sort_order: 2, tags: [tag("t1", "Go", 3)] }),
      card({
        kind: "recommended_collections",
        title: "推荐合集",
        sort_order: 1,
        collections: [collection("101", "从零搭一个博客", 5)],
      }),
      card({ kind: "hot_articles", title: "热门文章", sort_order: 0, hot_articles: [article("1", "热读一")] }),
    ]);

    expect(views.map((view) => view.kind)).toEqual([
      "hot_articles",
      "recommended_collections",
      "tag_cloud",
    ]);
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

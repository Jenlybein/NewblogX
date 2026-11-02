import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";

import HomeSidebar from "~/components/home/HomeSidebar.vue";
import type { SearchArticleItem, SiteRuntimeConfig, SiteSidebarCardItem } from "~/types/api";

const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(props, { slots }) {
    // 把 `to` 落到 href 上：右栏的合集推荐卡是"整行进详情"的链接，
    // 断言 href 是唯一能钉住"点进去到哪个页面"的方式。
    const href = typeof props.to === "string" ? props.to : String((props.to as { path?: string })?.path || "");
    return () => h("a", { href }, slots.default?.());
  },
});

const MotionStub = defineComponent({
  props: { transition: { type: Object, default: () => ({}) } },
  setup(_props, { slots }) {
    return () => h("div", slots.default?.());
  },
});

function article(id: string, overrides: Record<string, unknown> = {}): SearchArticleItem {
  return {
    id,
    title: `文章 ${id}`,
    abstract: "",
    cover: "",
    created_at: "2026-10-01T00:00:00Z",
    category: { title: "工程" },
    author: { id: "u1", nickname: "作者甲", avatar: "" },
    tags: [{ id: "t1", title: "Nuxt" }],
    digg_count: 0,
    favor_count: 0,
    comment_count: 0,
    view_count: 10,
    ...overrides,
  } as unknown as SearchArticleItem;
}

// fixture 必须用后端真实发出的键（`conf/site.enter.go` 的 json tag）：
// 之前这里是 `login: { pwd/email/qq }`，与后端 `qq_login` / `username_pwd_login` /
// `email_login` 对不上，于是组件断言绿、线上 chips 永远空（#56）。
const runtimeConfig = {
  site_info: { title: "站点" },
  project: { title: "果冻盒" },
  login: { username_pwd_login: true, email_login: false, qq_login: false },
  index_right: { list: [] },
} as unknown as SiteRuntimeConfig;

function mountSidebar(props: Record<string, unknown> = {}) {
  return mount(HomeSidebar, {
    props: { runtimeConfig, articles: [], ...props },
    global: { stubs: { NuxtLink: NuxtLinkStub, Motion: MotionStub } },
  });
}

describe("HomeSidebar：右栏（热门标签 / 推荐作者）", () => {
  it("旧「站点公告」硬编码区已移除，标签与作者两块分区仍在（不再有 AI 区块）", () => {
    const wrapper = mountSidebar();

    expect(wrapper.attributes("data-home-sidebar")).toBeDefined();
    // 公告不再由模板硬编码：它改成可在后台配置的富文本卡，这里没有配置时就不该出现。
    expect(wrapper.find("[data-home-section='notice']").exists()).toBe(false);
    expect(wrapper.find("[data-home-section='tags']").exists()).toBe(true);
    expect(wrapper.find("[data-home-section='authors']").exists()).toBe(true);
    expect(wrapper.find("[data-home-section='ai-placeholder']").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("打开对话框");
    // 旧公告卡里的占位简介与登录方式 chips 一起退场。
    expect(wrapper.text()).not.toContain("为开发者写作、搜索与知识整理提供稳定的内容入口。");
    expect(wrapper.text()).not.toContain("密码登录");
  });

  it("模块开关：index_right 只开 hot_tags 时，推荐作者不渲染", () => {
    const wrapper = mountSidebar({
      runtimeConfig: { ...runtimeConfig, index_right: { list: [{ title: "hot_tags" }] } },
    });

    expect(wrapper.find("[data-home-section='tags']").exists()).toBe(true);
    expect(wrapper.find("[data-home-section='authors']").exists()).toBe(false);
  });

  it("文章为空：标签/作者区不消失，改成「加载后出现」的说明（右栏不塌陷）", () => {
    const wrapper = mountSidebar({ articles: [] });

    expect(wrapper.get("[data-home-section='tags']").text()).toContain("文章加载后");
    expect(wrapper.get("[data-home-section='authors']").text()).toContain("文章加载后");
  });

  it("有文章：聚合出热门标签链接与作者行（整行可点）", () => {
    const wrapper = mountSidebar({
      articles: [
        article("a1"),
        article("a2", {
          author: { id: "u2", nickname: "作者乙", avatar: "" },
          tags: [{ id: "t2", title: "部署" }],
        }),
      ],
    });

    expect(wrapper.get("[data-home-section='tags']").text()).toContain("Nuxt");
    expect(wrapper.get("[data-home-section='tags']").text()).toContain("部署");
    expect(wrapper.findAll("a")).toHaveLength(2 + 2); // 两个标签 + 两位作者
    expect(wrapper.get("[data-home-section='authors']").text()).toContain("作者甲");
    expect(wrapper.get("[data-home-section='authors']").text()).toContain("1 篇文章");
  });

  it("加载中：主侧栏给骨架，而不是空态先闪", () => {
    const wrapper = mountSidebar({ pending: true });
    expect(wrapper.find("[data-home-section='sidebar-skeleton']").exists()).toBe(true);
  });
});

/**
 * 数据驱动卡片（#111）：读者站不再按模板写死"公告 / 热门标签 / 推荐作者"，
 * 而是按后端返回的卡片列表渲染 —— 顺序、标题、种类都来自接口，空数据的卡整块隐藏。
 */
describe("HomeSidebar：数据驱动卡片（#111）", () => {
  function sidebarCard(
    kind: SiteSidebarCardItem["kind"],
    overrides: Partial<SiteSidebarCardItem> = {},
  ): SiteSidebarCardItem {
    return {
      id: `card-${kind}`,
      kind,
      title: "",
      sort_order: 0,
      hot_articles: [],
      latest_comments: [],
      tags: [],
      collections: [],
      content: "",
      ...overrides,
    };
  }

  const configuredCards: SiteSidebarCardItem[] = [
    sidebarCard("tag_cloud", {
      title: "常用标签",
      sort_order: 2,
      tags: [{ id: "t1", title: "Go", article_count: 9 }],
    }),
    sidebarCard("hot_articles", {
      title: "本周热读",
      sort_order: 0,
      hot_articles: [{ id: "a1", title: "热读一", digg_count: 12, view_count: 120 }],
    }),
    sidebarCard("latest_comments", {
      title: "大家怎么说",
      sort_order: 1,
      latest_comments: [
        {
          id: "c1",
          content: "写得清楚",
          article_id: "a2",
          article_title: "文章二",
          user_nickname: "读者甲",
          created_at: "2026-11-02T00:00:00Z",
        },
      ],
    }),
  ];

  it("按配置的顺序与标题渲染，各卡带自己的数据；旧三卡不再出现", () => {
    const wrapper = mountSidebar({ sidebarCards: configuredCards });

    const cards = wrapper.findAll("[data-sidebar-card]");
    expect(cards).toHaveLength(3);
    expect(cards.map((section) => section.attributes("data-sidebar-card"))).toEqual([
      "hot_articles",
      "latest_comments",
      "tag_cloud",
    ]);
    expect(cards.map((section) => section.get("[data-sidebar-card-title]").text())).toEqual([
      "本周热读",
      "大家怎么说",
      "常用标签",
    ]);
    expect(cards[0].text()).toContain("热读一");
    expect(cards[1].text()).toContain("写得清楚");
    expect(cards[1].text()).toContain("读者甲");
    expect(cards[2].text()).toContain("Go");

    // 旧配置路径（热门标签/推荐作者）在数据驱动模式下完全不渲染。
    expect(wrapper.find("[data-home-section='tags']").exists()).toBe(false);
    expect(wrapper.find("[data-home-section='authors']").exists()).toBe(false);
  });

  it("无数据的卡整块隐藏，有数据的卡照常渲染（右栏不出现空盒子）", () => {
    const wrapper = mountSidebar({
      sidebarCards: [
        sidebarCard("hot_articles", { hot_articles: [] }),
        sidebarCard("latest_comments", {
          latest_comments: [
            {
              id: "c1",
              content: "只保留这一张",
              article_id: "a2",
              article_title: "文章二",
              user_nickname: "读者甲",
              created_at: "2026-11-02T00:00:00Z",
            },
          ],
        }),
      ],
    });

    const cards = wrapper.findAll("[data-sidebar-card]");
    expect(cards).toHaveLength(1);
    expect(cards[0].attributes("data-sidebar-card")).toBe("latest_comments");
  });

  it("配置了卡片但全部没数据：只隐藏卡片，不回落旧三卡", () => {
    const wrapper = mountSidebar({
      sidebarCards: [
        sidebarCard("hot_articles", { hot_articles: [] }),
        sidebarCard("latest_comments", { latest_comments: [] }),
        sidebarCard("tag_cloud", { tags: [] }),
      ],
    });

    expect(wrapper.findAll("[data-sidebar-card]")).toHaveLength(0);
    // 回退只看"有没有配置"：新配置生效中，旧卡片不该重新出现。
    expect(wrapper.find("[data-home-section='tags']").exists()).toBe(false);
    expect(wrapper.find("[data-home-section='authors']").exists()).toBe(false);
  });

  it("没有卡片配置时回退到旧右栏（标签与作者仍兼容、不报错）", () => {
    const wrapper = mountSidebar({ sidebarCards: [] });

    expect(wrapper.find("[data-home-section='tags']").exists()).toBe(true);
    expect(wrapper.find("[data-home-section='authors']").exists()).toBe(true);
    expect(wrapper.findAll("[data-sidebar-card]")).toHaveLength(0);
  });

  it("标签云按文章数给字号：权重高的标签字号更大", () => {
    const wrapper = mountSidebar({
      sidebarCards: [
        sidebarCard("tag_cloud", {
          title: "标签云",
          tags: [
            { id: "t1", title: "Go", article_count: 10 },
            { id: "t2", title: "部署", article_count: 1 },
          ],
        }),
      ],
    });

    const links = wrapper.findAll("[data-sidebar-card='tag_cloud'] a");
    expect(links).toHaveLength(2);
    const sizeOf = (index: number) =>
      Number((links[index].attributes("style") || "").match(/font-size:\s*([\d.]+)rem/)?.[1] || 0);
    expect(sizeOf(0)).toBeGreaterThan(sizeOf(1));
  });

  it("热门文章卡不与主栏热门置顶重复：置顶过的文章不再出现在右栏", () => {
    const wrapper = mountSidebar({
      sidebarCards: [
        sidebarCard("hot_articles", {
          title: "热门文章",
          hot_articles: [
            { id: "a1", title: "主栏已经置顶的文章", digg_count: 50, view_count: 500 },
            { id: "a2", title: "右栏热读", digg_count: 20, view_count: 200 },
          ],
        }),
      ],
      pinnedArticleIds: ["a1"],
    });

    const links = wrapper.findAll("[data-sidebar-card='hot_articles'] a");
    expect(links).toHaveLength(1);
    expect(links[0].text()).toContain("右栏热读");
    expect(wrapper.text()).not.toContain("主栏已经置顶的文章");
  });
});

/**
 * 右栏的「推荐合集」卡：数据来自合集读面（只含展示中的合集），
 * 点击整行进合集详情；没有可推荐的合集时整块隐藏。
 */
describe("HomeSidebar：推荐合集卡", () => {
  function sidebarCard(
    kind: SiteSidebarCardItem["kind"],
    overrides: Partial<SiteSidebarCardItem> = {},
  ): SiteSidebarCardItem {
    return {
      id: `card-${kind}`,
      kind,
      title: "",
      sort_order: 0,
      hot_articles: [],
      latest_comments: [],
      tags: [],
      collections: [],
      content: "",
      ...overrides,
    };
  }

  const collectionCard = sidebarCard("recommended_collections", {
    title: "往期专题",
    collections: [
      {
        id: "101",
        title: "从零搭一个博客",
        description: "一套完整的建站路径",
        cover: "https://cdn.example.com/blog.png",
        article_count: 7,
      },
      {
        id: "102",
        title: "工程效率",
        description: "",
        cover: "",
        article_count: 0,
      },
    ],
  });

  it("每行是一个合集详情链接，带标题、简介与文章数", () => {
    const wrapper = mountSidebar({ sidebarCards: [collectionCard] });

    const section = wrapper.get("[data-sidebar-card='recommended_collections']");
    expect(section.get("[data-sidebar-card-title]").text()).toBe("往期专题");
    const links = section.findAll("a");
    expect(links).toHaveLength(2);
    expect(links[0].attributes("href")).toBe("/collections/101");
    expect(links[1].attributes("href")).toBe("/collections/102");
    expect(links[0].text()).toContain("从零搭一个博客");
    expect(links[0].text()).toContain("一套完整的建站路径");
    expect(links[0].text()).toContain("7 篇文章");
    expect(links[1].text()).toContain("工程效率");
    expect(links[1].text()).toContain("暂无文章");
  });

  it("有封面的合集渲染缩略图，没有封面就不留破图位", () => {
    const wrapper = mountSidebar({ sidebarCards: [collectionCard] });

    const images = wrapper.findAll("[data-sidebar-card='recommended_collections'] img");
    expect(images).toHaveLength(1);
    expect(images[0].attributes("src")).toBe("https://cdn.example.com/blog.png");
  });

  it("没有可推荐的合集时整块隐藏（不出现只有标题的空盒子）", () => {
    const wrapper = mountSidebar({
      sidebarCards: [
        sidebarCard("recommended_collections", { title: "推荐合集", collections: [] }),
        sidebarCard("hot_articles", {
          title: "热门文章",
          hot_articles: [{ id: "a1", title: "热读一", digg_count: 12, view_count: 120 }],
        }),
      ],
    });

    expect(wrapper.find("[data-sidebar-card='recommended_collections']").exists()).toBe(false);
    expect(wrapper.findAll("[data-sidebar-card]")).toHaveLength(1);
  });
});

/**
 * 右栏富文本卡：运营在后台写的一段 HTML，读者站渲染前先过同一份白名单。
 *
 * 组件只负责把渲染模型里**已清洗**的 `contentHtml` 交给 `v-html`，
 * 所以这里断言两件事：合法排版真的渲染出来了、恶意内容真的不在 DOM 里。
 */
describe("HomeSidebar：富文本卡", () => {
  function richTextCard(content: string, title = "站点公告"): SiteSidebarCardItem {
    return {
      id: "card-rich-text",
      kind: "rich_text",
      title,
      sort_order: 0,
      hot_articles: [],
      latest_comments: [],
      tags: [],
      collections: [],
      content,
    };
  }

  it("合法排版内容按 HTML 渲染：标题、粗体、列表、链接都在", () => {
    const wrapper = mountSidebar({
      sidebarCards: [
        richTextCard(
          '<h2>更新日志</h2><p>本周 <strong>重点</strong></p><ul><li>第一条</li></ul><p><a href="https://example.com/post">链接</a></p>',
        ),
      ],
    });

    const section = wrapper.get("[data-sidebar-card='rich_text']");
    const body = section.get("[data-sidebar-rich-text]");
    expect(section.get("[data-sidebar-card-title]").text()).toBe("站点公告");
    expect(body.find("h2").text()).toBe("更新日志");
    expect(body.find("strong").text()).toBe("重点");
    expect(body.findAll("li")).toHaveLength(1);
    expect(body.get("a").attributes("href")).toBe("https://example.com/post");
  });

  it("恶意内容不会进 DOM：script / onerror / javascript: / iframe 全部被剥离", () => {
    const wrapper = mountSidebar({
      sidebarCards: [
        richTextCard(
          [
            "<p>正文</p>",
            "<script>alert(1)</script>",
            '<img src="https://cdn.example.com/a.png" onerror="alert(1)">',
            '<a href="javascript:alert(1)">点我</a>',
            '<iframe src="https://evil.example.com"></iframe>',
          ].join(""),
        ),
      ],
    });

    const html = wrapper.get("[data-sidebar-card='rich_text']").html();
    expect(html).toContain("正文");
    expect(html).not.toContain("<script");
    expect(html.toLowerCase()).not.toContain("onerror");
    expect(html).not.toContain("javascript:");
    expect(html).not.toContain("iframe");
    expect(wrapper.findAll("[data-sidebar-card='rich_text'] script")).toHaveLength(0);
  });

  it("清洗后没有正文时整张卡不渲染（不留空盒子）", () => {
    const wrapper = mountSidebar({
      sidebarCards: [
        richTextCard("<script>alert(1)</script>"),
        {
          id: "card-tag",
          kind: "tag_cloud",
          title: "标签云",
          sort_order: 1,
          hot_articles: [],
          latest_comments: [],
          tags: [{ id: "t1", title: "Go", article_count: 3 }],
          collections: [],
          content: "",
        },
      ],
    });

    expect(wrapper.find("[data-sidebar-card='rich_text']").exists()).toBe(false);
    expect(wrapper.findAll("[data-sidebar-card]")).toHaveLength(1);
  });
});

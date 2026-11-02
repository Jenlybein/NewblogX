import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";

import HomeSidebar from "~/components/home/HomeSidebar.vue";
import type { SiteSidebarCardItem } from "~/types/api";

const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(props, { slots }) {
    // 把 `to` 落到 href 上：右栏的合集推荐卡是"整行进详情"的链接，
    // 断言 href 是唯一能钉住"点进去到哪个页面"的方式。
    const href =
      typeof props.to === "string" ? props.to : String((props.to as { path?: string })?.path || "");
    return () => h("a", { href }, slots.default?.());
  },
});

const MotionStub = defineComponent({
  props: { transition: { type: Object, default: () => ({}) } },
  setup(_props, { slots }) {
    return () => h("div", slots.default?.());
  },
});

/**
 * 旧右栏配置（`site.index_right`）的残留形状：三个"标题 + 启用"开关。
 *
 * 它已经从后端配置、种子 YAML 与前端投影里删除；这里故意把它塞进组件 attr，
 * 断言右栏渲染完全不看它 —— 有卡片才渲染，没卡片整块不出现。
 */
const legacyIndexRight = {
  index_right: {
    list: [
      { title: "site_notice", enable: true },
      { title: "hot_tags", enable: true },
      { title: "recommended_authors", enable: true },
    ],
  },
};

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
    ...overrides,
  };
}

const commentCard = sidebarCard("latest_comments", {
  title: "大家怎么说",
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
});

function mountSidebar(
  props: Record<string, unknown> = {},
  attrs: Record<string, unknown> = {},
) {
  return mount(HomeSidebar, {
    props: { sidebarCards: [], ...props },
    attrs,
    global: {
      stubs: { NuxtLink: NuxtLinkStub, Motion: MotionStub },
      config: { warnHandler: () => {} },
    },
  });
}

/**
 * 数据驱动卡片：读者站不再按模板写死"公告 / 热门标签 / 推荐作者"，
 * 而是按后端返回的卡片列表渲染 —— 顺序、标题、种类都来自接口，空数据的卡整块隐藏。
 * 右栏本身是否出现只看卡片数据：没有启用中的卡片就不渲染。
 */
describe("HomeSidebar：右栏只由卡片数据决定", () => {
  it("没有启用卡片时右栏整块不渲染（旧三卡不再回退）", () => {
    const wrapper = mountSidebar();

    expect(wrapper.find("[data-home-sidebar]").exists()).toBe(false);
    expect(wrapper.find("[data-home-section]").exists()).toBe(false);
    expect(wrapper.findAll("[data-sidebar-card]")).toHaveLength(0);
  });

  it("旧 index_right 开关不再影响渲染：三个旧开关都开着，也只认卡片数据", () => {
    const wrapper = mountSidebar({ sidebarCards: [] }, legacyIndexRight);

    expect(wrapper.find("[data-home-sidebar]").exists()).toBe(false);
    expect(wrapper.find("[data-home-section='notice']").exists()).toBe(false);
    expect(wrapper.find("[data-home-section='tags']").exists()).toBe(false);
    expect(wrapper.find("[data-home-section='authors']").exists()).toBe(false);
    expect(wrapper.findAll("[data-sidebar-card]")).toHaveLength(0);
  });

  it("有启用卡片就渲染：一张卡也能撑起右栏", () => {
    const wrapper = mountSidebar({ sidebarCards: [commentCard] });

    expect(wrapper.find("[data-home-sidebar]").exists()).toBe(true);
    expect(wrapper.findAll("[data-sidebar-card]")).toHaveLength(1);
    expect(wrapper.find("[data-home-section='notice']").exists()).toBe(false);
  });

  it("按配置的顺序与标题渲染，各卡带自己的数据", () => {
    const wrapper = mountSidebar({
      sidebarCards: [
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
        commentCard,
      ],
    });

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
    expect(cards[2].text()).toContain("Go");

    // 旧右栏（公告 / 热门标签 / 推荐作者）在数据驱动模式下完全不渲染。
    expect(wrapper.find("[data-home-section='notice']").exists()).toBe(false);
    expect(wrapper.find("[data-home-section='tags']").exists()).toBe(false);
    expect(wrapper.find("[data-home-section='authors']").exists()).toBe(false);
  });

  it("无数据的卡整块隐藏，有数据的卡照常渲染（右栏不出现空盒子）", () => {
    const wrapper = mountSidebar({
      sidebarCards: [
        sidebarCard("hot_articles", { hot_articles: [] }),
        commentCard,
      ],
    });

    const cards = wrapper.findAll("[data-sidebar-card]");
    expect(cards).toHaveLength(1);
    expect(cards[0].attributes("data-sidebar-card")).toBe("latest_comments");
  });

  it("有卡片但全部没数据：只隐藏卡片，不回落旧三卡", () => {
    const wrapper = mountSidebar({
      sidebarCards: [
        sidebarCard("hot_articles", { hot_articles: [] }),
        sidebarCard("latest_comments", { latest_comments: [] }),
        sidebarCard("tag_cloud", { tags: [] }),
      ],
    });

    expect(wrapper.findAll("[data-sidebar-card]")).toHaveLength(0);
    expect(wrapper.find("[data-home-section]").exists()).toBe(false);
  });

  it("加载中：右栏已因卡片而渲染时给骨架", () => {
    const wrapper = mountSidebar({
      pending: true,
      sidebarCards: [
        sidebarCard("tag_cloud", {
          tags: [{ id: "t1", title: "Go", article_count: 9 }],
        }),
      ],
    });

    expect(wrapper.find("[data-home-section='sidebar-skeleton']").exists()).toBe(true);
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

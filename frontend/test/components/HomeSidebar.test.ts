import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";

import HomeSidebar from "~/components/home/HomeSidebar.vue";
import type { SearchArticleItem, SiteRuntimeConfig, SiteSidebarCardItem } from "~/types/api";

const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(_props, { slots }) {
    return () => h("a", slots.default?.());
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

describe("HomeSidebar：右栏（公告 / 热门标签 / 推荐作者）", () => {
  it("右栏三块分区都在，且不再有 AI 区块（入口已由右下角捏捏接管）", () => {
    const wrapper = mountSidebar();

    expect(wrapper.attributes("data-home-sidebar")).toBeDefined();
    expect(wrapper.find("[data-home-section='notice']").exists()).toBe(true);
    expect(wrapper.find("[data-home-section='tags']").exists()).toBe(true);
    expect(wrapper.find("[data-home-section='authors']").exists()).toBe(true);
    expect(wrapper.find("[data-home-section='ai-placeholder']").exists()).toBe(false);
    expect(wrapper.text()).not.toContain("打开对话框");
    expect(wrapper.get("[data-home-section='notice']").text()).toContain("果冻盒");
    expect(wrapper.get("[data-home-section='notice']").text()).toContain("密码登录");
  });

  it("模块开关：index_right 只开 hot_tags 时，公告与推荐作者不渲染", () => {
    const wrapper = mountSidebar({
      runtimeConfig: { ...runtimeConfig, index_right: { list: [{ title: "hot_tags" }] } },
    });

    expect(wrapper.find("[data-home-section='notice']").exists()).toBe(false);
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
 * 公告卡的简介来自哪里（#39）。
 *
 * 后端 `site` 段里没有任何"站点简介"字段：`SiteInfo` 只有 title / logo / beian，
 * `Project` 只有 title / icon / web_path。所以简介只能是一句固定文案 —— 之前这条链
 * 先挂 `project.abstract`、再挂 `site_info.subtitle`，两者后端都没有，读出来永远是空，
 * 于是整条链在生产里恒等于最后那句默认值（两个死分支）。下面两条用例把"死字段不再被消费"
 * 钉住：即使有人从别处把这两段塞进运行时配置，公告卡也不该跟着变。
 */
describe("HomeSidebar：公告卡简介（#39 死字段清理）", () => {
  const DEFAULT_ABSTRACT = "为开发者写作、搜索与知识整理提供稳定的内容入口。";

  it("配置里残留 site_info.subtitle 也不再被读：显示默认文案", () => {
    // 去掉 project.abstract，否则它会盖住 subtitle，这条用例就变成"永远绿"的空断言。
    const wrapper = mountSidebar({
      runtimeConfig: {
        ...runtimeConfig,
        project: { title: "果冻盒" },
        site_info: { title: "站点", subtitle: "旧副标题" },
      },
    });
    const notice = wrapper.get("[data-home-section='notice']").text();

    expect(notice).toContain(DEFAULT_ABSTRACT);
    expect(notice).not.toContain("旧副标题");
  });

  it("project.abstract 也不再被读（后端 Project 没有该字段）：显示默认文案", () => {
    // 基础 fixture 里就带着 project.abstract = "项目摘要"，正好当残留值用。
    const notice = mountSidebar().get("[data-home-section='notice']").text();

    expect(notice).toContain(DEFAULT_ABSTRACT);
    expect(notice).not.toContain("项目摘要");
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

    // 旧配置路径（公告/热门标签/推荐作者）在数据驱动模式下完全不渲染。
    expect(wrapper.find("[data-home-section='notice']").exists()).toBe(false);
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
    // 回退只看"有没有配置"：新配置生效中，旧三卡不该重新出现。
    expect(wrapper.find("[data-home-section='notice']").exists()).toBe(false);
    expect(wrapper.find("[data-home-section='tags']").exists()).toBe(false);
    expect(wrapper.find("[data-home-section='authors']").exists()).toBe(false);
  });

  it("没有卡片配置时回退到旧右栏（旧配置仍兼容、不报错）", () => {
    const wrapper = mountSidebar({ sidebarCards: [] });

    expect(wrapper.find("[data-home-section='notice']").exists()).toBe(true);
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
});

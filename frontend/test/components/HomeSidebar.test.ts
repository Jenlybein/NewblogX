import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";

import HomeSidebar from "~/components/home/HomeSidebar.vue";
import type { SearchArticleItem, SiteRuntimeConfig } from "~/types/api";

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

const runtimeConfig = {
  site_info: { title: "站点" },
  project: { title: "果冻盒", abstract: "项目摘要" },
  login: { pwd: true, email: false, qq: false },
  index_right: { list: [] },
  article: {},
  comment: {},
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

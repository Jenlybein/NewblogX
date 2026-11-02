import { mount } from "@vue/test-utils";
import { defineComponent, h } from "vue";
import { describe, expect, it } from "vitest";

import ArticleFeedItem from "~/components/article/ArticleFeedItem.vue";
import type { SearchArticleItem } from "~/types/api";
import type { ArticleEntryData, ArticleEntryVariant } from "~/utils/articleEntry";

const NuxtLinkStub = defineComponent({
  props: { to: { type: [String, Object], default: "" } },
  setup(_props, { slots }) {
    return () => h("a", slots.default?.());
  },
});

const baseEntry: ArticleEntryData = {
  id: "a1",
  href: "/article/a1",
  title: "Nuxt 部署笔记",
  abstract: "讲清楚 SSR 与静态生成的选择。",
  cover: "",
  crop: null,
  dateLabel: "2026/10/01",
  categoryLabel: "工程",
  statusLabel: "",
  author: { id: "u1", nickname: "作者甲", avatar: "" },
  tags: [
    { id: "t1", title: "Nuxt" },
    { id: "t2", title: "部署" },
  ],
  stats: { digg: 12, favor: 34, comment: 5, view: 1200 },
  shelfMeta: [],
};

function mountCard(options: { entry?: ArticleEntryData; variant?: ArticleEntryVariant; compact?: boolean } = {}) {
  return mount(ArticleFeedItem, {
    props: {
      entry: options.entry ?? baseEntry,
      variant: options.variant ?? "list",
      ...(options.compact === undefined ? {} : { compact: options.compact }),
    },
    global: {
      stubs: {
        NuxtLink: NuxtLinkStub,
        IconThumbUp: true,
        IconHeart: true,
        IconMessageCircle2: true,
        IconEye: true,
      },
    },
  });
}

describe("ArticleFeedItem：列表 / 搜索 / 收藏架三处同一卡片形态", () => {
  it("列表变体渲染标题、摘要、标签与四项统计", () => {
    const wrapper = mountCard({ variant: "list" });

    expect(wrapper.attributes("data-variant")).toBe("list");
    expect(wrapper.get(".feed-entry__title").text()).toContain("Nuxt 部署笔记");
    expect(wrapper.get(".feed-entry__abstract").text()).toContain("讲清楚 SSR");
    expect(wrapper.findAll(".feed-entry__tag").map((tag) => tag.text())).toEqual(["Nuxt", "部署"]);
    expect(wrapper.findAll(".feed-entry__stat")).toHaveLength(4);
    expect(wrapper.get('[data-stat="digg"]').text()).toContain("12");
    expect(wrapper.get('[data-stat="favor"]').text()).toContain("34");
    expect(wrapper.get('[data-stat="comment"]').text()).toContain("5");
    expect(wrapper.get('[data-stat="view"]').text()).toContain("1.2k");
    expect(wrapper.get(".feed-entry__author-link").text()).toContain("作者甲");
  });

  it("搜索变体复用同一形态，只是更紧凑（不再有第二种卡片）", () => {
    const wrapper = mountCard({ variant: "search" });

    expect(wrapper.attributes("data-variant")).toBe("search");
    expect(wrapper.classes()).toContain("feed-entry--search");
    expect(wrapper.get(".feed-entry__title").text()).toContain("Nuxt 部署笔记");
    expect(wrapper.get(".feed-entry__abstract").text()).toContain("讲清楚 SSR");
    expect(wrapper.findAll(".feed-entry__tag")).toHaveLength(2);
    expect(wrapper.findAll(".feed-entry__stat")).toHaveLength(4);
  });

  it("旧 compact 参数仍映射到搜索密度（首页/创作区调用不用改）", () => {
    const wrapper = mountCard({ variant: "list", compact: true });

    expect(wrapper.attributes("data-variant")).toBe("search");
  });

  it("收藏架变体展示状态、收藏元信息与无链接作者，形态与列表一致", () => {
    const wrapper = mountCard({
      variant: "shelf",
      entry: {
        ...baseEntry,
        id: "f1",
        href: "/article/f1",
        statusLabel: "已发布",
        shelfMeta: ["40 阅读", "2 收藏"],
        tags: [],
        author: { id: "", nickname: "原作者", avatar: "" },
        stats: { digg: 0, favor: 2, comment: 0, view: 40 },
      },
    });

    expect(wrapper.attributes("data-variant")).toBe("shelf");
    expect(wrapper.get(".feed-entry__status").text()).toBe("已发布");
    expect(wrapper.get(".feed-entry__shelf-meta").text()).toContain("40 阅读");
    expect(wrapper.get(".feed-entry__author-link--static").text()).toContain("原作者");
    expect(wrapper.find(".feed-entry__author-link a").exists()).toBe(false);
    expect(wrapper.findAll(".feed-entry__stat")).toHaveLength(4);
  });

  it("空数据时降级渲染（不抛错、仍有标题/摘要/统计占位）", () => {
    const wrapper = mount(ArticleFeedItem, {
      global: {
        stubs: {
          NuxtLink: NuxtLinkStub,
          IconThumbUp: true,
          IconHeart: true,
          IconMessageCircle2: true,
          IconEye: true,
        },
      },
    });

    expect(wrapper.attributes("data-empty")).toBe("true");
    expect(wrapper.get(".feed-entry__title").text()).toBe("未命名文章");
    expect(wrapper.get(".feed-entry__abstract").text()).toBe("这篇文章暂时没有摘要。");
    expect(wrapper.findAll(".feed-entry__tag")).toHaveLength(0);
    expect(wrapper.get('[data-stat="view"]').text()).toContain("0");
  });

  it("旧调用方式（article 原始数据）仍能渲染，不需要改调用方", () => {
    const article = {
      id: "a9",
      title: "旧调用方的文章",
      abstract: "摘要",
      created_at: "2026-10-01T00:00:00Z",
      tags: [{ id: "t9", title: "旧标签" }],
      digg_count: 1,
      favor_count: 2,
      comment_count: 3,
      view_count: 4,
    } as unknown as SearchArticleItem;

    const wrapper = mount(ArticleFeedItem, {
      props: { article },
      global: {
        stubs: {
          NuxtLink: NuxtLinkStub,
          IconThumbUp: true,
          IconHeart: true,
          IconMessageCircle2: true,
          IconEye: true,
        },
      },
    });

    expect(wrapper.get(".feed-entry__title").text()).toContain("旧调用方的文章");
    expect(wrapper.findAll(".feed-entry__tag").map((tag) => tag.text())).toEqual(["旧标签"]);
  });
});

/**
 * 列表封面要尊重作者在创作页调好的取景。
 *
 * crop 随 ES 文档投影过来（`article_sync` 写、搜索接口返回），这里钉住最后一段接线：
 * 卡片把 crop 交给 ArticleCover，图片样式才会带上非居中的 object-position 与缩放。
 */
describe("ArticleFeedItem：列表封面按取景渲染", () => {
  function mountWithCrop(crop: { focus_x: number; focus_y: number; zoom: number } | null) {
    return mount(ArticleFeedItem, {
      props: {
        entry: { ...baseEntry, cover: "https://cdn.example.com/a1.png", crop },
      },
      global: { stubs: { NuxtLink: NuxtLinkStub } },
    });
  }

  it("有取景时图片带上百分比 object-position 与缩放", () => {
    const wrapper = mountWithCrop({ focus_x: 0.5, focus_y: 0.25, zoom: 1.4 });
    const style = wrapper.get(".feed-entry__cover img").attributes("style") ?? "";

    expect(style).toContain("object-position: 50% 25%");
    expect(style).toContain("scale(1.4)");
  });

  it("没有取景时退回居中、缩放 1", () => {
    const wrapper = mountWithCrop(null);
    const style = wrapper.get(".feed-entry__cover img").attributes("style") ?? "";

    expect(style).toContain("object-position: 50% 50%");
    expect(style).toContain("scale(1)");
  });
});
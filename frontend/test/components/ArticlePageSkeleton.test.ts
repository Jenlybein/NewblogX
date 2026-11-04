import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import ArticlePageSkeleton from "~/components/article/ArticlePageSkeleton.vue";

describe("ArticlePageSkeleton：切页先出骨架", () => {
  it("pending 状态可观察，且骨架延用了文章页的纸面版式", () => {
    const wrapper = mount(ArticlePageSkeleton);

    expect(wrapper.attributes("data-role")).toBe("article-page-skeleton");
    expect(wrapper.attributes("data-state")).toBe("pending");
    // 版式复用文章页既有的纸面容器（骨架不该另起一套布局）。
    expect(wrapper.findAll(".article-paper").length).toBeGreaterThanOrEqual(2);
    expect(wrapper.findAll("[data-role='skeleton-bar']").length).toBeGreaterThan(0);
  });

  it("给读屏留一句加载提示（骨架视觉层对读屏隐藏）", () => {
    const wrapper = mount(ArticlePageSkeleton);

    expect(wrapper.get(".sr-only").text()).toContain("文章");
    expect(wrapper.find("[aria-hidden='true']").exists()).toBe(true);
  });

  it("骨架恒定两栏：目录是贴左边缘的抽屉，不再预留左栏占位（否则加载完会跳版）", () => {
    const wrapper = mount(ArticlePageSkeleton);

    expect(wrapper.get(".article-layout").classes()).not.toContain("article-layout--with-outline");
    expect(wrapper.find('[data-role="article-outline-column-skeleton"]').exists()).toBe(false);
    // 骨架不接受"有没有归属树"的开关：形态恒定，树到达后也不跳版。
    expect(Object.keys(wrapper.props())).not.toContain("withOutline");
  });

  it("右栏骨架列序与真实版式同步（票 236）：作者简介 → 互动条 → 目录", () => {
    const wrapper = mount(ArticlePageSkeleton);

    const aside = wrapper.get(".article-aside");
    const slots = aside
      .findAll("[data-role$='-skeleton']")
      .map((section) => section.attributes("data-role"));

    expect(slots).toEqual([
      "article-aside-author-skeleton",
      "article-aside-actions-skeleton",
      "article-aside-toc-skeleton",
    ]);
    // 互动条骨架用的是真实互动条卡片的容器类：占位高度与真卡片对齐。
    expect(aside.get('[data-role="article-aside-actions-skeleton"]').classes()).toContain("article-actions-card");
  });
});

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

  it("有归属树时多出一栏目录占位：与真实三栏版式同构，树到达后不跳版", () => {
    const wrapper = mount(ArticlePageSkeleton, { props: { withOutline: true } });

    const layout = wrapper.get(".article-layout");
    expect(layout.classes()).toContain("article-layout--with-outline");
    const outlineColumn = wrapper.get('[data-role="article-outline-column-skeleton"]');
    expect(outlineColumn.exists()).toBe(true);
    expect(outlineColumn.findAll("[data-role='skeleton-bar']").length).toBeGreaterThan(0);
  });

  it("无归属树时不预留空左栏（默认仍是两栏骨架）", () => {
    const wrapper = mount(ArticlePageSkeleton);

    expect(wrapper.get(".article-layout").classes()).not.toContain("article-layout--with-outline");
    expect(wrapper.find('[data-role="article-outline-column-skeleton"]').exists()).toBe(false);
  });
});

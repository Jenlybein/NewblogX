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
});

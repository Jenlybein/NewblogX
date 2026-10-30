import { mount } from "@vue/test-utils";
import { h } from "vue";
import { describe, expect, it } from "vitest";

import ProfileEmptyState from "~/components/profile/ProfileEmptyState.vue";

function mountEmptyState(props: Record<string, unknown> = {}, withAction = false) {
  return mount(ProfileEmptyState, {
    props: {
      title: "这个收藏夹还是空的",
      description: "当前分组还没有公开文章。",
      hints: ["换一个收藏夹继续看看。"],
      ...props,
    },
    slots: withAction
      ? { default: () => h("a", { href: "/search" }, "去搜索文章") }
      : {},
  });
}

describe("ProfileEmptyState：统一空态结构", () => {
  it("默认 md 尺寸：固定最小高度、标题/说明/指引齐全", () => {
    const wrapper = mountEmptyState();

    expect(wrapper.attributes("data-empty-state")).toBe("true");
    expect(wrapper.attributes("data-state")).toBe("empty");
    expect(wrapper.classes()).toContain("min-h-[240px]");
    expect(wrapper.get(".empty-state__title").text()).toBe("这个收藏夹还是空的");
    expect(wrapper.get(".empty-state__body").text()).toContain("当前分组还没有公开文章");
    expect(wrapper.findAll(".empty-state__hints li")).toHaveLength(1);
    expect(wrapper.find(".empty-state__actions").exists()).toBe(false);
  });

  it("尺寸档位映射到显式 min-height（空态不会塌成一行字）", () => {
    expect(mountEmptyState({ size: "sm" }).classes()).toContain("min-h-[160px]");
    expect(mountEmptyState({ size: "lg" }).classes()).toContain("min-h-[320px]");
  });

  it("加载态标记为 pending（页面据此渲染“正在读取”而不是“空”）", () => {
    const wrapper = mountEmptyState({ pending: true });

    expect(wrapper.attributes("data-state")).toBe("pending");
    expect(wrapper.classes()).toContain("is-pending");
  });

  it("actions 插槽渲染下一步入口（搜索/首页等按钮由调用方给）", () => {
    const wrapper = mountEmptyState({ size: "lg" }, true);

    expect(wrapper.get(".empty-state__actions").text()).toContain("去搜索文章");
  });

  it("几何记号已退役：空态不再渲染三角/方/圆三色 SVG", () => {
    const wrapper = mountEmptyState();

    expect(wrapper.find(".empty-state__mark").exists()).toBe(false);
    expect(wrapper.findAll("svg")).toHaveLength(0);
  });
});

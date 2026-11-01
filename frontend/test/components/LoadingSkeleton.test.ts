import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import LoadingSkeleton from "~/components/common/LoadingSkeleton.vue";

function mountSkeleton(props: Record<string, unknown> = {}) {
  return mount(LoadingSkeleton, { props });
}

describe("LoadingSkeleton：数据未到时的最小占位", () => {
  it("默认渲染三行正文条，且骨架条对读屏隐藏（只是装饰）", () => {
    const wrapper = mountSkeleton();

    expect(wrapper.attributes("data-role")).toBe("skeleton");
    expect(wrapper.findAll("[data-role='skeleton-bar']")).toHaveLength(3);
    expect(wrapper.find("[aria-hidden='true']").exists()).toBe(true);
  });

  it("rows 可调，card 变体用于列表/侧栏卡片块", () => {
    const wrapper = mountSkeleton({ rows: 2, variant: "card" });

    expect(wrapper.findAll("[data-role='skeleton-bar']")).toHaveLength(2);
    expect(wrapper.get("[data-role='skeleton-bar']").classes()).toContain("skeleton-bar--card");
  });

  it("label 提供可读的加载态说明（骨架条本身仍隐藏）", () => {
    const wrapper = mountSkeleton({ label: "正在读取内容" });

    expect(wrapper.get(".sr-only").text()).toBe("正在读取内容");
  });
});

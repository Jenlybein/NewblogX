import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";

import HomeAiPlaceholder from "~/components/home/HomeAiPlaceholder.vue";

describe("HomeAiPlaceholder：AI 卡片本轮只降级为占位", () => {
  it("启用时：展示助手名、占位说明、能力 chips 与打开入口", () => {
    const wrapper = mount(HomeAiPlaceholder, {
      props: { aiInfo: { enable: true, nickname: "小助手", abstract: "可以问搜索" } as never },
    });

    expect(wrapper.get("[data-role='ai-status']").text()).toBe("占位入口");
    expect(wrapper.get("h3").text()).toBe("小助手");
    expect(wrapper.get("[data-role='ai-note']").text()).toContain("捏捏");
    expect(wrapper.findAll(".blueprint-chip").length).toBeGreaterThan(3);
    expect(wrapper.get("[data-role='ai-open']").text()).toBe("打开对话框");
  });

  it("未启用：入口禁用并说明状态，不假装可用", () => {
    const wrapper = mount(HomeAiPlaceholder, { props: { aiInfo: null } });

    expect(wrapper.get("[data-role='ai-status']").text()).toBe("未启用");
    expect(wrapper.get("[data-role='ai-open']").text()).toBe("AI 暂未启用");
    expect(wrapper.get("[data-role='ai-open']").attributes("disabled")).toBeDefined();
  });

  it("点击打开入口时向外抛 open（真实打开逻辑由侧栏/页面处理）", async () => {
    const wrapper = mount(HomeAiPlaceholder, {
      props: { aiInfo: { enable: true } as never },
    });

    await wrapper.get("[data-role='ai-open']").trigger("click");
    expect(wrapper.emitted("open")).toHaveLength(1);
  });
});

import { mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";

import HomeInkFluid from "~/components/home/HomeInkFluid.vue";

afterEach(() => {
  vi.restoreAllMocks();
});

/**
 * 指针墨流的降级路径：happy-dom 没有 WebGL2，正好用来证明"特效失败不会把首屏拖垮"。
 * 真实 GPU 路径仍由类型检查 + 浏览器实测覆盖。
 */
describe("HomeInkFluid：无 WebGL2 时优雅降级", () => {
  it("标记 data-ink-fallback，保留空画布，不抛错", () => {
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);

    const wrapper = mount(HomeInkFluid);

    expect(wrapper.get("[data-home-ink]").attributes("data-ink-fallback")).toBe("1");
    expect(wrapper.get("canvas").attributes("aria-hidden")).toBe("true");
    expect(wrapper.findAll("a, button")).toHaveLength(0);
  });
});

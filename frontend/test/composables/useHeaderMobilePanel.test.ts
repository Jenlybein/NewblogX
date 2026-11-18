import { mount } from "@vue/test-utils";
import { defineComponent, h, nextTick } from "vue";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useHeaderMobilePanel } from "~/composables/useHeaderMobilePanel";

/**
 * 页头手机断点的水合安全（票 437 评审）。
 *
 * SSR 没有视口，只能按桌面口径渲染；如果客户端 setup 阶段就同步读 `matchMedia`，
 * 首次渲染立刻变成 `inert`，与 SSR 的 markup 不一致（Vue 会把这种属性跳变当成
 * 水合错误）。契约：**setup 阶段恒 false，挂载后才按真实视口补**。
 */

function stubViewport(matches: boolean) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: matches && query.includes("max-width: 639px"),
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useHeaderMobilePanel：水合安全", () => {
  it("水合首帧按桌面口径（false），挂载后才按真实视口补上", async () => {
    stubViewport(true);

    let api!: ReturnType<typeof useHeaderMobilePanel>;
    const Probe = defineComponent({
      setup() {
        api = useHeaderMobilePanel();
        // setup 阶段（SSR 与水合首帧走同一段代码）必须是桌面口径。
        expect(api.isMobile.value).toBe(false);
        return () => h("div", { class: api.isMobile.value ? "mobile" : "desktop" });
      },
    });

    const wrapper = mount(Probe);
    // 初始渲染仍是桌面口径（与水合前的 SSR 一致）。
    expect(wrapper.classes()).toContain("desktop");

    await nextTick();
    // 挂载后按真实视口补上手机口径。
    expect(api.isMobile.value).toBe(true);
    expect(wrapper.classes()).toContain("mobile");
  });

  it("桌面视口下挂载后仍是 false（不会因为 onMounted 一律置 true）", async () => {
    stubViewport(false);

    let api!: ReturnType<typeof useHeaderMobilePanel>;
    const Probe = defineComponent({
      setup() {
        api = useHeaderMobilePanel();
        return () => h("div");
      },
    });

    mount(Probe);
    await nextTick();

    expect(api.isMobile.value).toBe(false);
  });
});

import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import BrandLogo from "~/components/brand/Logo.vue";

/**
 * Logo 资产的三种形态（issue #12：暗色下 logo 必须清晰）。
 *
 * `render` 是常规蓝字标；`flat` 是扁平矢量（放奶油白方板上用）；
 * `white` 是反白 PNG —— 暗色克莱因蓝底上用它，蓝字标会糊进底色里。
 */
const stubs = {};

beforeEach(() => {
  vi.stubGlobal("__useRuntimeConfigMock", () => ({ app: { baseURL: "/" }, public: {} }));
});

function mountLogo(variant?: "render" | "flat" | "white", decorative = false) {
  return mount(BrandLogo, {
    props: { size: 40, ...(variant ? { variant } : {}), decorative },
    global: { stubs },
  });
}

describe("BrandLogo：资产形态与可访问性", () => {
  it("默认用常规蓝色 logo 资产", () => {
    const wrapper = mountLogo();

    expect(wrapper.get("img").attributes("src")).toContain("logo-256.png");
    expect(wrapper.get("img").attributes("alt")).toContain("JellyBin");
  });

  it("flat 用扁平矢量（奶油白方板/页脚场景）", () => {
    expect(mountLogo("flat").get("img").attributes("src")).toContain("jelly-flat.svg");
  });

  it("white 用反白 PNG（暗色蓝底场景）", () => {
    expect(mountLogo("white").get("img").attributes("src")).toContain("logo-512-white.png");
  });

  it("decorative 时 alt 为空且对读屏隐藏（旁边的字标已承担语义）", () => {
    const wrapper = mountLogo("render", true);

    expect(wrapper.get("img").attributes("alt")).toBe("");
    expect(wrapper.get("img").attributes("aria-hidden")).toBe("true");
  });
});

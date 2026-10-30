import { readFileSync } from "node:fs";
import { join } from "node:path";

import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import BrandLogo from "~/components/brand/Logo.vue";

/**
 * Logo 资产的两种形态（issue #19：暗色白边修复）。
 *
 * `render` 是常规蓝字标（`logo-256.png`，带 alpha 通道的透底资产）；
 * `flat` 是扁平矢量（放奶油白方板上用）。暗色专用的 `white` 变体已删除：
 * 那是一个无 alpha 通道的白底 PNG，正是暗色页头"白瓷砖"的来源。
 */
const stubs = {};

beforeEach(() => {
  vi.stubGlobal("__useRuntimeConfigMock", () => ({ app: { baseURL: "/" }, public: {} }));
});

function mountLogo(variant?: "render" | "flat", decorative = false) {
  return mount(BrandLogo, {
    props: { size: 40, ...(variant ? { variant } : {}), decorative },
    global: { stubs },
  });
}

describe("BrandLogo：资产形态与可访问性", () => {
  it("默认用常规蓝色 logo 资产（带 alpha 通道）", () => {
    const wrapper = mountLogo();

    expect(wrapper.get("img").attributes("src")).toContain("logo-256.png");
    expect(wrapper.get("img").attributes("alt")).toContain("JellyBin");
  });

  it("flat 用扁平矢量（奶油白方板/页脚场景）", () => {
    expect(mountLogo("flat").get("img").attributes("src")).toContain("jelly-flat.svg");
  });

  it("历史 white 输入不再产出白底 PNG，回落到通用透底资产", () => {
    const wrapper = mountLogo("white" as never);
    const src = wrapper.get("img").attributes("src");

    expect(src).toContain("logo-256.png");
    expect(src).not.toContain("logo-512-white.png");
  });

  it("decorative 时 alt 为空且对读屏隐藏（旁边的字标已承担语义）", () => {
    const wrapper = mountLogo("render", true);

    expect(wrapper.get("img").attributes("alt")).toBe("");
    expect(wrapper.get("img").attributes("aria-hidden")).toBe("true");
  });

  it("组件只声明「通用/扁平」两种形态，源码里没有暗色专用变体", () => {
    const source = readFileSync(join(process.cwd(), "app/components/brand/Logo.vue"), "utf8");

    expect(source).toMatch(/variant\?:\s*"render"\s*\|\s*"flat"/u);
    expect(source).not.toMatch(/logoWhite|logo-512-white/u);
    expect(source).not.toMatch(/white/u);
  });
});

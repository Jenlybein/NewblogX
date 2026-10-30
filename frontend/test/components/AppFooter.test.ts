import { readFileSync } from "node:fs";
import { join } from "node:path";

import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

import AppFooter from "~/components/layout/AppFooter.vue";
import { brandCopyright } from "~/utils/brand";
import { parseTokenBlocks, resolveTokenValue } from "~/utils/markdownTheme";

/**
 * 页脚测试：深墨面铺色（中性石墨阶）+ 品牌（倒色字标）+ 版权/备案/少量链接。
 *
 * 对比度用 WCAG 公式从 tokens.css 里**真实取色**算，而不是相信"看起来够黑"。
 */
const projectDir = process.cwd();
const footerSource = readFileSync(join(projectDir, "app/components/layout/AppFooter.vue"), "utf8");
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
const tokenBlocks = parseTokenBlocks(tokensCss);

function tokenHex(mode: "light" | "dark", name: string): string {
  return resolveTokenValue(tokenBlocks, mode, name);
}

function compositeOver(foreground: string, background: string): string {
  const rgba = /rgba\((\d+),\s*(\d+),\s*(\d+),\s*([\d.]+)\)/u.exec(foreground);
  if (!rgba) return foreground;
  const alpha = Number.parseFloat(rgba[4]!);
  const base = background.replace("#", "");
  const channels = [Number(rgba[1]), Number(rgba[2]), Number(rgba[3])].map((value, index) =>
    Math.round(value * alpha + Number.parseInt(base.slice(index * 2, index * 2 + 2), 16) * (1 - alpha)),
  );
  return `#${channels.map((value) => value.toString(16).padStart(2, "0")).join("")}`;
}

function channelLuminance(channel: number): number {
  const value = channel / 255;
  // WCAG 相对亮度：sRGB 线性化后 (value+0.05) / 1.05 再取 2.4 次方。
  return value <= 0.03928 ? value / 12.92 : ((value + 0.05) / 1.05) ** 2.4;
}

function relativeLuminance(hex: string): number {
  const value = hex.replace("#", "");
  const r = channelLuminance(Number.parseInt(value.slice(0, 2), 16));
  const g = channelLuminance(Number.parseInt(value.slice(2, 4), 16));
  const b = channelLuminance(Number.parseInt(value.slice(4, 6), 16));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrastRatio(foreground: string, background: string): number {
  const lighter = Math.max(relativeLuminance(foreground), relativeLuminance(background));
  const darker = Math.min(relativeLuminance(foreground), relativeLuminance(background));
  return (lighter + 0.05) / (darker + 0.05);
}
const stubs = {
  NuxtLink: {
    props: ["to"],
    template: '<a :href="typeof to === \'string\' ? to : \'/\'"><slot /></a>',
  },
};

function mountFooter() {
  return mount(AppFooter, { global: { stubs } });
}

let scrollToSpy: ReturnType<typeof vi.fn>;

beforeEach(() => {
  scrollToSpy = vi.fn();
  vi.stubGlobal("scrollTo", scrollToSpy);
  // Logo 组件要读 app.baseURL 拼品牌资产路径。
  vi.stubGlobal("__useRuntimeConfigMock", () => ({ app: { baseURL: "/" }, public: {} }));
});

describe("页脚：品牌与文案", () => {
  it("是 footer 地标，品牌字标用倒色（深墨面上的反白字）", () => {
    const wrapper = mountFooter();

    expect(wrapper.element.tagName).toBe("FOOTER");
    expect(wrapper.get(".brand-wordmark").text()).toContain("JellyBin");
    expect(wrapper.get(".brand-wordmark").text()).toContain("果冻盒");
  });

  it("版权行来自品牌模块（brandCopyright），不在模板里硬编码", () => {
    const wrapper = mountFooter();
    const expected = brandCopyright(new Date().getFullYear());

    expect(wrapper.text()).toContain(expected);
    // 反向证明：模板里没有可直接写死的年份/品牌串，改了品牌名这里会跟着变。
    expect(footerSource).toContain("brandCopyright");
  });

  it("备案行集中一处，并带上少量站内链接", () => {
    const wrapper = mountFooter();
    const nav = wrapper.get('nav[aria-label="页脚导航"]');

    expect(wrapper.text()).toMatch(/粤ICP备 \d+ 号/u);
    expect(nav.findAll("a").map((link) => link.text())).toEqual(["首页", "搜索"]);
  });
});

describe("页脚：对比度达 WCAG AA（明暗两态）", () => {
  it.each(["light", "dark"] as const)("%s：主文字（反白 / 深墨面）≥ 4.5:1", (mode) => {
    const surface = tokenHex(mode, "--color-surface-inverse");
    const ink = tokenHex(mode, "--color-on-inverse");

    expect(contrastRatio(ink, surface)).toBeGreaterThanOrEqual(4.5);
  });

  it.each(["light", "dark"] as const)("%s：次级文字（半透明反白 / 深墨面）也 ≥ 4.5:1", (mode) => {
    const surface = tokenHex(mode, "--color-surface-inverse");
    const muted = tokenHex(mode, "--color-on-inverse-muted");

    expect(contrastRatio(compositeOver(muted, surface), surface)).toBeGreaterThanOrEqual(4.5);
  });

  it("铺色与字色都来自语义 token，组件里没有裸色值 / 克莱因蓝", () => {
    expect(footerSource).toContain("var(--color-surface-inverse)");
    expect(footerSource).toContain("var(--color-on-inverse)");
    expect(footerSource).toContain("var(--color-on-inverse-muted)");
    expect(footerSource).not.toMatch(/--jelly-/u);
    expect(footerSource).not.toMatch(/#[0-9a-fA-F]{3,8}\b/u);
    expect(footerSource).not.toMatch(/rgba?\(/u);
  });
});

describe("页脚：回到顶部（结束锚点）", () => {
  it("点击回到顶部；普通模式下平滑滚动", async () => {
    const wrapper = mountFooter();

    await wrapper.get("button[data-chrome=to-top]").trigger("click");
    expect(scrollToSpy).toHaveBeenCalledWith({ top: 0, behavior: "smooth" });
  });

  it("开启「减少动态」时改为瞬间跳转（不做平滑滚动）", async () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query === "(prefers-reduced-motion: reduce)",
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
    }));

    const wrapper = mountFooter();
    await wrapper.get("button[data-chrome=to-top]").trigger("click");

    expect(scrollToSpy).toHaveBeenCalledWith({ top: 0, behavior: "auto" });
  });
});

import * as fs from "node:fs";
import * as path from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 首屏「像素尾迹」（ADR-0031）的静态门禁。
 *
 * 这一层是首屏唯一的指针装饰（原来的指针墨流保留为可切换的备选），所以除了功能
 * 本身，还要钉住它的三条纪律：装饰层不吃点击、颜色只从 token 读、减少动态下不启动。
 */

const appRoot = path.resolve(__dirname, "../../app");

function read(rel: string): string {
  return fs.readFileSync(path.join(appRoot, rel), "utf8");
}

const composable = read("composables/usePixelTrail.ts");
const component = read("components/home/HomePixelTrail.vue");
const indexVue = read("pages/index.vue");

describe("像素尾迹：实现与接线", () => {
  it("组件是纯装饰层：aria-hidden、pointer-events: none、不接收 Tab 焦点", () => {
    expect(component).toContain('aria-hidden="true"');
    expect(component).toContain("pointer-events: none");
    expect(component).toContain("data-home-pixels");
  });

  it("像素格与淡出只画在 canvas 上，颜色从 token 读（不写死色值）", () => {
    expect(composable).toContain("fillRect");
    expect(composable).toContain("getComputedStyle");
    expect(composable).toContain("--jelly-yellow");
    // 色值字面量（#rrggbb / rgb(...) / rgba(...)）一个都不许有。
    expect(composable).not.toMatch(/#[0-9a-fA-F]{3,8}/);
    expect(composable).not.toMatch(/rgba?\(/);
  });

  it("减少动态时不启动：不挂监听、不画帧", () => {
    expect(composable).toContain("prefersReducedMotion.value");
    expect(composable).toMatch(/if\s*\(listening\s*\|\|\s*prefersReducedMotion\.value\)\s*return/);
    expect(composable).toContain("watch(prefersReducedMotion");
  });

  it("有停帧条件：标签页隐藏 / 画布离屏 / 无存活方块且指针静止", () => {
    expect(composable).toContain("visibilitychange");
    expect(composable).toContain("IntersectionObserver");
    expect(composable).toMatch(/cells\.size\s*>\s*0\s*\|\|\s*!idle/);
  });
});

describe("首屏接线：默认像素尾迹，可切回墨流", () => {
  it("默认读像素尾迹，只有显式 smoke 才换成指针墨流", () => {
    expect(indexVue).toContain("HomePixelTrail");
    expect(indexVue).toContain("HomeInkFluid");
    expect(indexVue).toContain('deco?.home_trail === "smoke" ? "smoke" : "pixel"');
    expect(indexVue).toContain("v-if=\"trailMode === 'pixel'\"");
  });

  it("两层挂在同一个首屏装饰类上（图层顺序不变）", () => {
    expect(indexVue).toMatch(/HomePixelTrail[^>]*class="home-cover__ink"/);
    expect(indexVue).toMatch(/HomeInkFluid[\s\S]{0,200}class="home-cover__ink"/);
  });
});

describe("后台装饰设置：尾迹样式可选", () => {
  const settings = fs.readFileSync(
    path.resolve(__dirname, "../../../frontend_admin/app/components/settings/SettingsDeco.vue"),
    "utf8",
  );

  it("后台有尾迹选择器，值来自统一常量", () => {
    expect(settings).toContain("HOME_TRAIL_OPTIONS");
    expect(settings).toContain("config.home_trail");
  });
});

import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter } from "../helpers/designFiles";

/**
 * 捏捏的几何常量只有一个来源（票 437 评审）。
 *
 * 问题：composable 里已经有 `JELLY_LAUNCHER_SIZE` / `JELLY_HANDLE_*`，但组件 CSS 又
 * 把 `56px` / `44px` / `16px` 重抄了一遍 —— 两处同值靠人记得同步。契约：
 *   - 组件把 composable 的常量写成 CSS 自定义属性；
 *   - 样式块只消费 `var(--jelly-*)`，不出现这些像素值；
 *   - 收起/展开**不改根元素宽度**（那是布局动画），只动 transform/opacity。
 */

const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");

const launcher = read("app/components/jelly/JellyLauncher.vue");
const composable = read("app/composables/useJellyAssistant.ts");

function cssBlock(selector: string): string {
  return blockAfter(launcher, `${selector} {`);
}

describe("JellyLauncher：几何常量由 composable 注入 CSS 变量", () => {
  it("composable 仍是常量唯一来源", () => {
    expect(composable).toContain("export const JELLY_LAUNCHER_SIZE = 56");
    expect(composable).toContain("export const JELLY_HANDLE_WIDTH = 16");
    expect(composable).toContain("export const JELLY_HANDLE_HEIGHT = 56");
    expect(composable).toContain("export const JELLY_HANDLE_HIT_WIDTH = 44");
  });

  it("组件把常量写成 CSS 自定义属性（内联 style）", () => {
    const rootStyle = launcher.slice(
      launcher.indexOf("const rootStyle"),
      launcher.indexOf("const bodyStyle"),
    );

    for (const name of [
      "JELLY_LAUNCHER_SIZE",
      "JELLY_VIEWPORT_MARGIN",
      "JELLY_HANDLE_WIDTH",
      "JELLY_HANDLE_HEIGHT",
      "JELLY_HANDLE_HIT_WIDTH",
    ]) {
      expect(rootStyle, `rootStyle 没有注入 ${name}`).toContain(name);
    }
    for (const customProperty of [
      "--jelly-size",
      "--jelly-margin",
      "--jelly-handle-width",
      "--jelly-handle-height",
      "--jelly-handle-hit",
    ]) {
      expect(rootStyle, `rootStyle 没有写 ${customProperty}`).toContain(customProperty);
    }
  });

  it("样式块只消费变量：不再重抄 56 / 44 / 16 这些像素值", () => {
    const root = cssBlock(".jelly-launcher");
    const handle = cssBlock(".jelly-launcher__handle");
    const capsule = cssBlock(".jelly-launcher__handle-capsule");

    // 根元素尺寸来自 composable。
    expect(root).toContain("width: var(--jelly-size)");
    expect(root).toContain("height: var(--jelly-size)");
    expect(root).not.toContain("width: 56px");
    expect(root).not.toContain("--jelly-size: 56px");

    // 把手命中区与可见胶囊同源。
    expect(handle).toContain("width: var(--jelly-handle-hit)");
    expect(handle).toContain("height: var(--jelly-handle-height)");
    expect(handle).not.toContain("width: 44px");
    expect(capsule).toContain("width: var(--jelly-handle-width)");
    expect(capsule).toContain("height: var(--jelly-handle-height)");
    expect(capsule).not.toMatch(/width:\s*16px/u);

    // hover 的 20px = 16 × 1.25：只放大 transform，不再补间 width（布局属性）。
    expect(launcher).toContain("scaleX(1.25)");
    expect(launcher).toContain("transform-origin: 100% 50%");
    expect(launcher).not.toMatch(/transition:[^;\n]*\bwidth\b/u);
  });
});

describe("JellyLauncher：收起 / 展开只动 transform / opacity", () => {
  it("收起态不改根元素宽度（不再有 56 → 44 的布局动画）", () => {
    // 收起态现在只调整可交互面，不声明 width / height。
    const collapsedSelectors = launcher.match(/\.jelly-launcher--collapsed[^{]*\{/gu) ?? [];
    expect(collapsedSelectors.length).toBeGreaterThan(0);

    for (const selector of collapsedSelectors) {
      const block = cssBlock(selector.replace(/\{$/u, "").trim());
      expect(block, `${selector} 不应改根元素尺寸`).not.toMatch(/\bwidth:/u);
      expect(block, `${selector} 不应改根元素尺寸`).not.toMatch(/\bheight:/u);
    }
  });

  it("把手在尺寸恒定的根元素内右对齐，命中区 44 宽由按钮自己拿到", () => {
    const handle = cssBlock(".jelly-launcher__handle");

    expect(handle).toMatch(/position:\s*absolute;/u);
    expect(handle).toContain("right: 0");
    expect(handle).toContain("width: var(--jelly-handle-hit)");
    // 没有负 inset 外扩：贴边元素不能外扩 outline。
    expect(handle).not.toMatch(/inset:\s*-/u);
  });

  it("收起态的根元素 inline 过渡只声明 transform / opacity", () => {
    const rootStyle = launcher.slice(
      launcher.indexOf("const rootStyle"),
      launcher.indexOf("const bodyStyle"),
    );

    expect(rootStyle).toContain(
      "transform var(--motion-base) var(--motion-ease), opacity var(--motion-base) var(--motion-ease)",
    );
    // 位置直写期（拖拽 / 落位）没有过渡。
    expect(rootStyle).toContain('"none"');
  });

  it("收起 / 展开角色本体：200ms transform + opacity，滑到把手位置后动画结束才卸载", () => {
    // 角色不是 v-if 瞬时换装：Vue 过渡类负责位移与淡出。
    expect(launcher).toContain(".jelly-character-enter-active");
    expect(launcher).toContain(".jelly-character-leave-to");
    expect(launcher).toContain("translateX(calc(100% - var(--jelly-handle-width)))");
    expect(launcher).toContain("transform var(--motion-base) var(--motion-ease)");
    expect(launcher).toContain("opacity: 0");
  });
});

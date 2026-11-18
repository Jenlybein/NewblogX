import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, stripComments } from "../helpers/designFiles";

/**
 * 捏捏的几何常量只有一个来源（票 437 评审），以及固定定位 / 六态这些新口径。
 *
 * 契约：
 *   - 组件把 composable 的常量写成 CSS 自定义属性；
 *   - 样式块只消费 `var(--jelly-*)`，不出现这些像素值；
 *   - 根元素固定右下角（`right` / `bottom` 带安全区），不再用 `translate3d` 定位；
 *   - 收起把手固定在右边缘垂直中部（`top: 45%` + 垂直居中），`z-index` 50；
 *   - 拖拽 / 位置持久化整体退场；视觉状态是六态。
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
    expect(composable).toContain("export const JELLY_VIEWPORT_MARGIN = 12");
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
    // 固定定位后 rootStyle 不再有坐标 / 过渡：位置只由样式表与安全区决定。
    expect(rootStyle).not.toContain("translate3d");
    expect(rootStyle).not.toContain("--jelly-y");
    expect(rootStyle).not.toContain("transition");
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

describe("JellyLauncher：固定右下角与右边缘中部的收起把手", () => {
  it("根元素纯 CSS 固定右下角：right / bottom 带安全区，根节点不再用 translate3d 定位", () => {
    const root = cssBlock(".jelly-launcher");

    expect(root).toContain("position: fixed");
    expect(root).toContain("right: max(var(--jelly-margin), env(safe-area-inset-right))");
    expect(root).toContain("bottom: calc(var(--jelly-margin) + env(safe-area-inset-bottom))");
    // 不再有 JS 坐标、也不再叠安全区让位的 translate。
    expect(root).not.toContain("translate3d");
    expect(root).not.toContain("--jelly-y");
    expect(root).not.toMatch(/^\s*translate:/mu);
    expect(stripComments(launcher)).not.toContain("translate3d");
  });

  it("收起把手贴在右边缘垂直中部（top 45% + 垂直居中），避开底部进度条 / 页脚", () => {
    const handle = cssBlock(".jelly-launcher__handle");

    expect(handle).toContain("position: fixed");
    expect(handle).toContain("top: 45%");
    // 独立 translate 属性做垂直居中：不会被进场 transform 覆盖。
    expect(handle).toMatch(/translate:\s*0\s+-50%/u);
    expect(handle).toContain("right: max(var(--jelly-margin), env(safe-area-inset-right))");
    // 不贴底部：把手上没有 bottom 声明。
    expect(handle).not.toMatch(/\bbottom:/u);
  });

  it("把手 z-index 50：高于页面普通 chrome，仍低于评论面板 / 弹窗的 60", () => {
    const handle = cssBlock(".jelly-launcher__handle");
    const collapsedRoot = cssBlock(".jelly-launcher--collapsed");

    expect(handle).toContain("z-index: 50");
    // 根元素的 z-index 40 会形成层叠上下文，所以收起态的根一起抬到 50，
    // 把手的 50 才真正生效（同层级内仍低于 60 的浮层）。
    expect(collapsedRoot).toContain("z-index: 50");
  });

  it("收起态提示也贴在把手左侧（不像旧口径那样挂在右下角）", () => {
    const collapsedHint = cssBlock(".jelly-launcher--collapsed .jelly-hint");

    expect(collapsedHint).toContain("position: fixed");
    expect(collapsedHint).toContain("top: 45%");
    expect(collapsedHint).toMatch(/translate:\s*0\s+-50%/u);
  });
});

describe("JellyLauncher：收起 / 展开只动 transform / opacity", () => {
  it("收起态不改根元素宽度（不再有 56 → 44 的布局动画）", () => {
    // 收起态只调整可交互面 / 层叠，不声明 width / height。
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

    expect(handle).toContain("width: var(--jelly-handle-hit)");
    // 没有负 inset 外扩：贴边元素不能外扩 outline。
    expect(handle).not.toMatch(/inset:\s*-/u);
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

describe("JellyLauncher：六态与拖拽退场", () => {
  it("视觉状态是六态：idle / hover / press / open / collapsed / disabled", () => {
    const code = stripComments(composable);
    const start = code.indexOf("export type JellyVisualState");
    const end = code.indexOf(";", start);
    const typeBlock = code.slice(start, end);

    for (const state of ["idle", "hover", "press", "open", "collapsed", "disabled"]) {
      expect(typeBlock, `六态里缺 ${state}`).toContain(`"${state}"`);
    }
    expect(typeBlock).not.toContain("dragging");
  });

  it("组件不再有拖拽 / 位置写入逻辑", () => {
    const code = stripComments(launcher);

    for (const gone of [
      "DRAG_THRESHOLD_PX",
      "pointerOffset",
      "draggingByPointer",
      "attachPointerListeners",
      "detachPointerListeners",
      "onPointerMove",
      "settleOffset",
      "SPRING_LAUNCHER_SETTLE",
      "releaseVelocity",
    ]) {
      expect(code, `拖拽逻辑残留：${gone}`).not.toContain(gone);
    }
    expect(code).not.toContain('"dragging"');
  });

  it("composable 不再有位置持久化 / 拖拽 / 视口钳制入口", () => {
    const code = stripComments(composable);

    for (const gone of [
      "JELLY_POSITION_KEY",
      "jelly:launcher-position",
      "JellyPosition",
      "clampJellyPosition",
      "collapsedHandlePosition",
      "persistPosition",
      "restorePosition",
      "moveTo",
      "beginDrag",
      "endDrag",
      "isDragged",
      "handleViewportResize",
    ]) {
      expect(code, `位置 / 拖拽入口残留：${gone}`).not.toContain(gone);
    }
  });

  it("提示文案收短，且不自称「捏捏」；aria-label 换成中性说法", () => {
    expect(composable).toContain('export const JELLY_COLLAPSE_HINT = "我先躲起来"');
    expect(composable).toContain('export const JELLY_EXPAND_HINT = "我回来了"');

    const code = stripComments(launcher);
    expect(code).not.toContain("展开捏捏");
    expect(code).not.toContain("收起捏捏");
    expect(code).toContain("展开 AI 助手");
    expect(code).toContain("收起 AI 助手");
    // 角色的读屏名也不再从品牌昵称（「捏捏」）拼。
    expect(code).not.toContain("brandAiName");
  });
});

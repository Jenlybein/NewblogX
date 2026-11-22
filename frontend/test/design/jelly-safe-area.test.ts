import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, stripComments } from "../helpers/designFiles";

/**
 * 捏捏的安全区设计测试（票 426，spec 的 D20）。
 *
 * 为什么单列一个文件：安全区是**纯 CSS 的渲染层让位**，JS 侧一行都不该动 ——
 * 固定定位之后更是如此：根元素用 `right/bottom` 直接吃 `env(safe-area-inset-*)`，
 * 不再有坐标 / translate 让位。这里的断言守住这条边界：
 *   1. `env(safe-area-inset-right/bottom)` 只出现在根元素与把手的固定定位里；
 *   2. 收起把手在右边缘垂直中部，不被底部进度条 / 页脚 / iPhone 手势条压住；
 *   3. composable 不掺安全区（读了也不会用）；
 *   4. 组件不再写 `translate3d` / `--jelly-y`。
 *
 * 不断言像素值（无法也没有必要在这里看真实渲染），也不模拟 iPhone——真机安全区
 * 属于"真机未验"的验收项，见实现报告。
 */
const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");

const launcherSource = read("app/components/jelly/JellyLauncher.vue");
const composableSource = read("app/composables/useJellyAssistant.ts");

function launcherRootBlock(): string {
  return blockAfter(launcherSource, ".jelly-launcher {");
}

describe("捏捏安全区 · 固定定位与 JS 坐标语义的边界", () => {
  it("根元素把 safe-area-inset-right / bottom 直接写进固定定位", () => {
    const block = launcherRootBlock();

    expect(block).toContain("position: fixed");
    expect(block).toContain("right: max(var(--jelly-margin), env(safe-area-inset-right))");
    expect(block).toContain("bottom: calc(var(--jelly-margin) + env(safe-area-inset-bottom))");
    // 固定定位后不再有 JS 坐标与 translate 让位。
    expect(block).not.toContain("--jelly-safe-bottom");
    expect(block).not.toContain("--jelly-y");
    expect(block).not.toMatch(/^\s*translate:/mu);
  });

  it("收起把手避开底部：右边缘垂直中部 + 安全区，不贴 bottom", () => {
    const handleBlock = blockAfter(launcherSource, ".jelly-launcher__handle {");

    expect(handleBlock).toContain("position: fixed");
    expect(handleBlock).toContain("bottom: calc(var(--jelly-margin) + env(safe-area-inset-bottom))");
    expect(handleBlock).not.toMatch(/translate:\s*0\s+-50%/u);
    expect(handleBlock).toMatch(/right:\s*0;/u);
    expect(handleBlock).toContain("bottom: calc(var(--jelly-margin) + env(safe-area-inset-bottom))");
    // 不另开第二套底部让位变量。
    expect(handleBlock).not.toContain("dvh");
  });

  it("JS 坐标语义退场：composable 不掺安全区，组件不写 translate3d", () => {
    const composableCode = stripComments(composableSource);
    const launcherCode = stripComments(launcherSource);

    expect(composableCode).not.toContain("safe-area");
    expect(composableCode).not.toContain("env(");

    expect(launcherCode).not.toContain("translate3d");
    expect(launcherCode).not.toContain("--jelly-y");
    expect(launcherCode).not.toContain("100dvh");
  });

  it("收起提示跟着把手：固定在垂直中部左侧，而不是右下角", () => {
    const block = blockAfter(launcherSource, ".jelly-launcher--collapsed .jelly-hint {");

    expect(block).toContain("position: fixed");
    expect(block).toContain("top: 45%");
    expect(block).toContain("calc(max(var(--jelly-margin), env(safe-area-inset-right))");
    expect(block).toMatch(/translate:\s*0\s+-50%/u);
  });
});



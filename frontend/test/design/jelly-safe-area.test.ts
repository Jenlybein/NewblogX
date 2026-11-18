import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, stripComments } from "../helpers/designFiles";

/**
 * 捏捏的安全区让位设计测试（票 426，spec #423 的 D20）。
 *
 * 为什么单列一个文件：安全区是**纯 CSS 的渲染层让位**，JS 侧一行都不该动 ——
 * 位置语义（左上角 transform）、`clampJellyPosition`、拖拽采样与持久化必须保持
 * 与 #425 完全一致。这里的断言就是守住这条边界：
 *   1. `env(safe-area-inset-bottom)` 只注入到根元素的 CSS 变量里；
 *   2. 让位量只在角色贴视口底时非零（有 0 下限，按视口高度算），不是整体上移；
 *   3. 根元素包住本体与把手，所以两者共享同一个让位量；
 *   4. composable 不掺安全区（读了也不会用）。
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

describe("捏捏安全区让位 · 渲染层与 JS 坐标语义的边界", () => {
  it("根元素把 env(safe-area-inset-bottom) 注入成 CSS 变量", () => {
    const block = launcherRootBlock();

    expect(block).toContain("--jelly-safe-bottom: env(safe-area-inset-bottom, 0px)");
    // 让位量是 translate 的一部分（与 inline 的 transform 叠加，不是替换它）。
    expect(block).toContain("translate:");
    expect(block).toContain("var(--jelly-safe-bottom)");
  });

  it("只在贴底时上抬：有 0 下限、以视口高度为界，不是把角色整体上移", () => {
    const block = launcherRootBlock();

    // 纵向坐标由组件以只读 CSS 变量抄给样式表。
    expect(block).toContain("--jelly-y");
    // 让位量按「角色底边 - (视口底 - 安全区)」算，因此高处为 0、贴底最多抬一个安全区。
    expect(block).toContain("100dvh");
    expect(block).toMatch(/max\(\s*0px/u);
    expect(block).toContain("var(--jelly-size)");
    expect(block).toContain("var(--jelly-margin)");
  });

  it("本体与把手共享同一个让位量：整个组件只有根元素带 translate，把手不另开 fixed 定位", () => {
    // `.jelly-launcher` 根元素包住 cluster（本体 / 把手都在里面）；只有一个 translate。
    const translateDeclarations = launcherSource.match(/^\s*translate:/gmu) ?? [];
    expect(translateDeclarations).toHaveLength(1);

    const handleBlock = blockAfter(launcherSource, ".jelly-launcher__handle {");
    expect(handleBlock).not.toContain("position: fixed");
    expect(handleBlock).not.toContain("bottom:");
  });

  it("JS 坐标语义不变：clampJellyPosition / 持久化里不掺安全区，仍只信 inline transform", () => {
    const composableCode = stripComments(composableSource);

    expect(composableCode).not.toContain("safe-area");
    expect(composableCode).not.toContain("env(");

    // 位置仍然由组件 inline 的 translate3d 写入；CSS 只负责叠一个让位量。
    expect(launcherSource).toContain(
      "transform: `translate3d(${target.x + offset.x}px, ${target.y + offset.y}px, 0)`",
    );
    // `--jelly-y` 只是同一坐标的只读副本，两者必须写在同一份 rootStyle 里。
    expect(launcherSource).toContain('"--jelly-y": `${target.y + offset.y}px`');
  });
});

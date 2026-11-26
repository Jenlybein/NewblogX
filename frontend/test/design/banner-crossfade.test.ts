import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { blockAfter, stripComments } from "../helpers/designFiles";

/**
 * 首屏轮播换图的静态动效契约（#432）。
 *
 * 换图是 160ms 交叉淡入，不是硬切：旧帧以 `is-leaving` 淡出、新帧直接落在它下面，
 * 视觉上就是一次 A→B 的交叉。这里锁三件事：
 *   1. 时长只有一个来源 —— `utils/motion.ts` 的命名表达预设，不在组件里写裸毫秒；
 *   2. 只动 `opacity`，且用 transition 而不是 keyframes（中途反向才不会被重播打断）；
 *   3. 减少动态下直接切（`transition: none`），不是"放慢"。
 *
 * 同时锁住暂停控件的两个静态约定：命中区 ≥44px、它仍住在渐黑区那一行控件里。
 */
const projectDir = process.cwd();
const carousel = readFileSync(
  join(projectDir, "app/components/article/BannerCarousel.vue"),
  "utf8",
);
const motion = readFileSync(join(projectDir, "app/utils/motion.ts"), "utf8");

/** `<template>` 段（不扫 `<style>`，避免把声明当成结构）。 */
function templateOf(source: string): string {
  const start = source.indexOf("<template>");
  // 模板内部有嵌套的 `<template v-for>`，整段结尾取最后一个闭合标签。
  const end = source.lastIndexOf("</template>");
  expect(start, "组件没有 <template> 段").toBeGreaterThan(-1);
  expect(end, "组件没有 </template> 段").toBeGreaterThan(start);
  return source.slice(start, end);
}

/** 取出包含某个选择器的 `@media (prefers-reduced-motion: reduce)` 块。 */
function reducedMediaBlock(anchor: string): string {
  let from = 0;
  for (;;) {
    const start = carousel.indexOf("@media (prefers-reduced-motion: reduce)", from);
    expect(start, `找不到包含 ${anchor} 的减少动态块`).toBeGreaterThan(-1);
    const open = carousel.indexOf("{", start);
    let depth = 0;
    for (let index = open; index < carousel.length; index += 1) {
      if (carousel[index] === "{") depth += 1;
      if (carousel[index] === "}") {
        depth -= 1;
        if (depth === 0) {
          const block = carousel.slice(open + 1, index);
          if (block.includes(anchor)) return block;
          from = start + 1;
          break;
        }
      }
    }
  }
}

describe("换图交叉淡入：160ms、只动 opacity、可被打断", () => {
  it("160ms 是 motion.ts 里登记的命名预设，不是组件里的裸毫秒", () => {
    expect(motion).toMatch(/bannerCrossfade:\s*160/u);
    expect(carousel).toContain("MOTION_EXPRESSIVE_MS.bannerCrossfade");
    // 组件把时长落成 CSS 变量供 transition 消费，不直接写 `opacity 160ms`。
    expect(carousel).toMatch(/--banner-crossfade/u);
    expect(carousel).not.toMatch(/transition:\s*opacity\s+\d+ms/u);
  });

  it("旧帧用 transition 淡出（只动 opacity），不用 keyframes", () => {
    const leaving = blockAfter(carousel, ".banner-photo.is-leaving {");

    expect(leaving).toContain("opacity: 0");
    const transitions = [...leaving.matchAll(/transition:\s*([^;]+);/gu)].map((match) =>
      match[1]!.trim(),
    );
    expect(transitions).toHaveLength(1);
    expect(transitions[0], "交叉淡入只能动 opacity").toMatch(/^opacity\s+/u);
    expect(transitions[0]).toContain("var(--banner-crossfade)");
    expect(transitions[0]).toContain("var(--motion-ease)");
    expect(leaving).not.toContain("transform");

    // 整块轮播里没有任何 keyframes：交叉淡入、图标状态切换都必须走 transition
    // （可中途反向、连点不重播）。标题换字是硬切 —— 单侧淡入只会先空一帧。
    expect(stripComments(carousel), "轮播里不该再有 keyframes").not.toContain("@keyframes");
  });

  it("当前帧不挂过渡：入场是直接可见的，过渡只属于出场帧", () => {
    const photo = blockAfter(carousel, ".banner-photo {");

    expect(photo).not.toMatch(/transition:/u);
  });

  it("减少动态直接切：出场帧没有过渡，不是放慢", () => {
    const reduced = reducedMediaBlock(".banner-photo.is-leaving");

    expect(reduced).toContain(".banner-photo.is-leaving");
    expect(reduced).toMatch(/transition:\s*none/u);
  });
});

describe("暂停控件：命中区 ≥44 且仍住在渐黑区一行里", () => {
  it("播放/暂停按钮的命中区不小于 44px", () => {
    const playback = blockAfter(carousel, ".banner-playback {");

    expect(playback).toContain("min-height: 44px");
    expect(playback).toContain("min-width: 44px");
  });

  it("按钮是控件行里的内容，不是另起一颗浮层", () => {
    const template = templateOf(carousel);
    const controlsAt = template.indexOf('class="banner-controls"');
    const playbackAt = template.indexOf('class="banner-playback"');
    const veilAt = template.indexOf('class="banner-veil"');

    expect(veilAt, "模板里找不到 .banner-veil").toBeGreaterThan(-1);
    expect(controlsAt, "模板里找不到 .banner-controls").toBeGreaterThan(veilAt);
    expect(playbackAt, "控件行里找不到 .banner-playback").toBeGreaterThan(controlsAt);
    // 从控件行开到播放按钮之间没有闭合标签：它就在这一行里。
    expect(template.slice(controlsAt, playbackAt)).not.toContain("</div>");
  });
});

import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";



/**
 * 首页 "首屏封面 + 列表分级淡入" 的结构与降级断言。
 *
 * 动效地基（#3）把"减少动态 = 归零，而不是变慢"定在 utils/motion；#23 又补了
 * `useSectionReveal`（区块滚动进场）与 `useParallax`（视差 ≤10px）。#24 的首屏封面
 * 是这两者的第一个"分层"消费者：装饰层走 reveal、文字层走 stagger、记号走视差，
 * 所以这里锁定四件事：
 *   1. 首页首屏的装饰层真的接了对应 composable（而不是各写一套动画）；
 *   2. 正文列表**不以 `opacity: 0` 起步**（票 443：内容可见优先于入场效果）；
 *   3. reduce 下时长/延迟为 0，initial/animate 不变（不因 reduce 改 markup 而抖动 hydration）；
 *   4. 首屏封面没有自建关键帧动画（视差与进场都归口到 #3/#23 的预算）。
 *
 * 票 441 补一条边界：**首页正文区块（列表 / 置顶 / 右栏）不走进场**。它们原来也挂
 * `.motion-reveal`，初态 `opacity: 0`，进入站点时正好落在首屏下沿 —— 看起来就是
 * "列表和右栏被隐藏了"。正文常显，装饰才进场。
 */
const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");

const pageSource = read("app/pages/index.vue");
const feedSource = read("app/components/home/HomeFeedSection.vue");
const sidebarSource = read("app/components/home/HomeSidebar.vue");
const appSource = read("app/app.vue");

describe("首页分级淡入：首屏与列表", () => {
  it("列表首批的初态就是可见的：不做 opacity:0 的错峰入场（票 443）", () => {
    // 真实故障反推：SSR 把条目内联成 opacity:0，水合完成后才逐条出现 ——
    // 弱网/JS 失败时首页正文是空的。正文可见优先于入场效果。
    expect(feedSource).not.toContain("useStaggerEntrance");
    expect(feedSource).not.toContain("motion-stagger-item");
    expect(feedSource).toMatch(/initial: \{ opacity: 1, y: 0 \}/u);
    // 翻页后的新批次允许淡入，但必须从「可读」起步，不能从 0 起。
    expect(feedSource).toMatch(/initial: \{ opacity: 0\.35, y: 0 \}/u);
    expect(feedSource).not.toMatch(/initial: \{ opacity: 0,/u);
  });

  it("首屏不走进场动画：h1 是关键内容，不能带 SSR initial（opacity:0）", () => {
    // 这条是从真实故障反推出来的：h1 曾被 stagger 包住，动画没跑起来时首页等于没有标题。
    expect(pageSource).not.toContain("useStaggerEntrance");
    expect(pageSource).not.toContain("motion-stagger-item");
  });

  it("右栏标签云同样不以 opacity:0 起步（桌面端与正文同屏）", () => {
    expect(sidebarSource).not.toContain("useStaggerEntrance");
    expect(sidebarSource).not.toContain("motion-stagger-item");
  });

  it("无 JS 兜底只覆盖首屏色带（正文已不再需要兜底）", () => {
    expect(appSource).toContain("motion-reveal{opacity:1!important;transform:none!important}");
    expect(appSource).not.toContain("motion-stagger-item");
  });

  it("正文区块不挂 reveal：列表 / 置顶 / 右栏进入站点时就可见（票 441）", () => {
    for (const source of [feedSource, sidebarSource, read("app/components/home/HomePinnedSection.vue")]) {
      expect(source).not.toContain("useSectionReveal");
      expect(source).not.toContain("motion-reveal");
      expect(source).not.toContain("revealStyle");
    }
  });
});

describe("首屏封面：分层进场与视差（#23 口径）", () => {
  it("封面装饰层走 useSectionReveal（motion-reveal），文字层常显不走进场", () => {
    expect(pageSource).toContain("useSectionReveal");
    expect(pageSource).toContain("revealStyle");
    expect(pageSource).toContain("motion-reveal");
    expect(pageSource).not.toContain("motion-stagger-item");
  });

  it("封面记号接 useParallax（≤10px，reduced-motion 归零）", () => {
    expect(pageSource).toContain("useParallax");
    expect(pageSource).toContain("parallaxStyle");
    expect(pageSource).toContain("motion-parallax");
  });

  it("封面不自己写关键帧动画", () => {
    expect(pageSource).not.toContain("@keyframes");
    expect(pageSource).not.toMatch(/animation(?:-name)?\s*:/u);
  });
});

describe("减少动态：翻页淡入归零而不是变慢", () => {
  it("新批次淡入的时长取自 motion 预算，reduce 下为 0", () => {
    // 时长不是裸毫秒：走 MOTION_DURATIONS_MS.fast + prefersReducedMotion 三元。
    expect(feedSource).toContain("motionSeconds(MOTION_DURATIONS_MS.fast)");
    expect(feedSource).toContain("prefersReducedMotion.value ? 0");
  });
});

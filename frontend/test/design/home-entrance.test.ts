import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { staggerItemMotion } from "~/utils/motion";

/**
 * 首页 "首屏封面 + 列表分级淡入" 的结构与降级断言。
 *
 * 动效地基（#3）把"减少动态 = 归零，而不是变慢"定在 utils/motion；#23 又补了
 * `useSectionReveal`（区块滚动进场）与 `useParallax`（视差 ≤10px）。#24 的首屏封面
 * 是这两者的第一个"分层"消费者：装饰层走 reveal、文字层走 stagger、记号走视差，
 * 所以这里锁定四件事：
 *   1. 首页首屏、最新列表与右栏真的接了对应 composable（而不是各写一套动画）；
 *   2. 每个入场元素都带 `.motion-stagger-item` / `.motion-reveal`（SSR initial 与
 *      noscript 兜底依赖这些类）；
 *   3. reduce 下时长/延迟为 0，initial/animate 不变（不因 reduce 改 markup 而抖动 hydration）；
 *   4. 首屏封面没有自建关键帧动画（视差与进场都归口到 #3/#23 的预算）。
 */
const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");

const pageSource = read("app/pages/index.vue");
const feedSource = read("app/components/home/HomeFeedSection.vue");
const sidebarSource = read("app/components/home/HomeSidebar.vue");
const appSource = read("app/app.vue");

describe("首页分级淡入：首屏与列表", () => {
  it("列表区块走 useStaggerEntrance（非关键内容），入场元素带 motion-stagger-item", () => {
    expect(feedSource).toContain("useStaggerEntrance");
    expect(feedSource).toContain("entranceProps");
    expect(feedSource).toContain("motion-stagger-item");
  });

  it("首屏不走进场动画：h1 是关键内容，不能带 SSR initial（opacity:0）", () => {
    // 这条是从真实故障反推出来的：h1 曾被 stagger 包住，动画没跑起来时首页等于没有标题。
    expect(pageSource).not.toContain("useStaggerEntrance");
    expect(pageSource).not.toContain("motion-stagger-item");
  });

  it("右栏只有热门标签做错峰入场（非关键内容），其它区块不加", () => {
    expect(sidebarSource).toContain("useStaggerEntrance");
    expect(sidebarSource).toContain("motion-stagger-item");
  });

  it("无 JS 兜底：app.vue 的 noscript 样式把 stagger 条目直接置为可见", () => {
    expect(appSource).toContain("motion-stagger-item{opacity:1!important;transform:none!important}");
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

describe("减少动态：归零而不是变慢", () => {
  it("reduce 下时长/延迟为 0，initial/animate 不变", () => {
    const reduced = staggerItemMotion(1, true);
    expect(reduced.transition.duration).toBe(0);
    expect(reduced.transition.delay).toBe(0);
    expect(reduced.initial).toMatchObject({ opacity: 0, y: 8 });
    expect(reduced.animate).toMatchObject({ opacity: 1, y: 0 });
  });

  it("普通模式仍按 40ms/档错峰、200ms 完成", () => {
    const motion = staggerItemMotion(1, false);
    expect(motion.transition.delay).toBe(0.04);
    expect(motion.transition.duration).toBe(0.2);
  });
});

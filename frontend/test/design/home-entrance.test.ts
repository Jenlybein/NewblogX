import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { staggerItemMotion } from "~/utils/motion";

/**
 * 首页 "首屏 + 列表分级淡入" 的结构与降级断言。
 *
 * 动效地基（#3）把"减少动态 = 归零，而不是变慢"定在 utils/motion；这里锁定三件事：
 *   1. 首页首屏与最新列表真的接了 useStaggerEntrance（而不是各写一套动画）；
 *   2. 每个入场元素都带 `.motion-stagger-item`（SSR initial 与 noscript 兜底依赖这个类）；
 *   3. reduce 下时长/延迟为 0，initial/animate 不变（不因 reduce 改 markup 而抖动 hydration）。
 */
const projectDir = process.cwd();
const read = (path: string) => readFileSync(join(projectDir, path), "utf8");

const pageSource = read("app/pages/index.vue");
const feedSource = read("app/components/home/HomeFeedSection.vue");
const sidebarSource = read("app/components/home/HomeSidebar.vue");
const appSource = read("app/app.vue");

describe("首页分级淡入：首屏与列表", () => {
  it("首屏与最新列表都走 useStaggerEntrance，且入场元素带 motion-stagger-item", () => {
    for (const source of [pageSource, feedSource]) {
      expect(source).toContain("useStaggerEntrance");
      expect(source).toContain("entranceProps");
      expect(source).toContain("motion-stagger-item");
    }
  });

  it("右栏只有热门标签做错峰入场（非关键内容），其它区块不加", () => {
    expect(sidebarSource).toContain("useStaggerEntrance");
    expect(sidebarSource).toContain("motion-stagger-item");
  });

  it("无 JS 兜底：app.vue 的 noscript 样式把 stagger 条目直接置为可见", () => {
    expect(appSource).toContain("motion-stagger-item{opacity:1!important;transform:none!important}");
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

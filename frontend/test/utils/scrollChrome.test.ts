import { describe, expect, it } from "vitest";

import {
  SCROLL_CHROME_DIRECTION_DELTA_PX,
  SCROLL_CHROME_RESTING,
  SCROLL_CHROME_THRESHOLD_PX,
  nextScrollChromeState,
  type ScrollChromeState,
} from "~/utils/scrollChrome";

/**
 * 吸顶页头的状态机（纯函数）。
 *
 * 为什么值得测：方向感知最容易写成"每次 scroll 都翻状态"，结果页头在触控板
 * 的微小抖动上疯狂闪现。这里锁定三件事：阈值之前的滚动不动页头、方向判定有
 * 迟滞（小幅反向不立刻翻）、回到顶部永远可见。
 */

/** 从静止状态滚到某个位置（一次事件内的位移 = 目标值）。 */
function scrollTo(scrollY: number, options?: Parameters<typeof nextScrollChromeState>[3]): ScrollChromeState {
  return nextScrollChromeState(SCROLL_CHROME_RESTING, 0, scrollY, options);
}

describe("页头 chrome 状态机：阈值", () => {
  it("阈值之内不转玻璃、不隐藏（顶部的小滚动不该让页头动）", () => {
    expect(scrollTo(SCROLL_CHROME_THRESHOLD_PX - 1)).toEqual({ scrolled: false, hidden: false });
  });

  it("超过阈值后转为玻璃并随下滚隐藏", () => {
    const state = scrollTo(SCROLL_CHROME_THRESHOLD_PX + 12);

    expect(state.scrolled).toBe(true);
    expect(state.hidden).toBe(true);
  });

  it("阈值可配置（组件与测试共用同一个判定）", () => {
    const state = scrollTo(40, { threshold: 64 });

    expect(state.scrolled).toBe(false);
    expect(state.hidden).toBe(false);
  });

  it("溢出回弹产生的负 scrollY 按 0 处理（不会把页头顶出视野）", () => {
    expect(nextScrollChromeState(SCROLL_CHROME_RESTING, 0, -24)).toEqual({
      scrolled: false,
      hidden: false,
    });
  });
});

describe("页头 chrome 状态机：方向感知", () => {
  it("上滚恢复显示（超过方向迟滞才算数）", () => {
    const scrolled = scrollTo(320);
    const backUp = nextScrollChromeState(scrolled, 320, 320 - SCROLL_CHROME_DIRECTION_DELTA_PX - 4);

    expect(backUp.hidden).toBe(false);
    expect(backUp.scrolled).toBe(true);
  });

  it("小幅反向不翻状态（迟滞避免触控板抖动导致页头闪烁）", () => {
    const scrolled = scrollTo(320);
    const tiny = nextScrollChromeState(scrolled, 320, 320 - 2);

    expect(tiny.hidden).toBe(true);
    expect(tiny.scrolled).toBe(true);
  });

  it("继续下滚保持隐藏", () => {
    const hidden = scrollTo(320);
    const deeper = nextScrollChromeState(hidden, 320, 360);

    expect(deeper.hidden).toBe(true);
  });

  it("回到阈值以内：玻璃退掉且页头必须可见", () => {
    const hidden = scrollTo(320);
    const backTop = nextScrollChromeState(hidden, 320, SCROLL_CHROME_THRESHOLD_PX - 1);

    expect(backTop).toEqual({ scrolled: false, hidden: false });
  });
});

describe("页头 chrome 状态机：减少动态", () => {
  it("减少动态下完全不启用（不转玻璃、不隐藏）", () => {
    for (const scrollY of [SCROLL_CHROME_THRESHOLD_PX + 12, 320, 1200]) {
      expect(scrollTo(scrollY, { reducedMotion: true })).toEqual(SCROLL_CHROME_RESTING);
    }
  });

  it("已隐藏时打开减少动态也立刻回到静止（不会卡在隐藏态）", () => {
    const hidden = scrollTo(320);
    const reduced = nextScrollChromeState(hidden, 320, 400, { reducedMotion: true });

    expect(reduced).toEqual(SCROLL_CHROME_RESTING);
  });
});

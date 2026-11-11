import { describe, expect, it } from "vitest";

import {
  MOTION_REVEAL_OFFSET_PX,
  PARALLAX_MAX_PX,
  clampParallaxOffset,
  sectionRevealStyle,
} from "~/utils/motion";

/**
 * 动效档位 C 的两个纯函数（ADR-0012 决策 7）。
 *
 * 锁定三件事：只动 transform/opacity（不碰布局属性）、位移与视差在上限内、
 * 减少动态下整段归零（不是放慢）。
 */
describe("sectionRevealStyle：区块滚动进场", () => {
  it("未进场时只动 opacity 与 transform（translateY 不超过上限）", () => {
    const style = sectionRevealStyle(false, false);

    expect(Object.keys(style).sort()).toEqual(["opacity", "transform", "transition"]);
    expect(style.opacity).toBe("0");
    expect(style.transform).toBe(`translateY(${MOTION_REVEAL_OFFSET_PX}px)`);
    expect(style.transition).toContain("var(--motion-base)");
    expect(style.transition).toContain("var(--motion-ease)");
  });

  it("进场后回到原位（transform: none，不残留位移）", () => {
    expect(sectionRevealStyle(true, false)).toMatchObject({ opacity: "1", transform: "none" });
  });

  it("位移钳到 ≤12px（传大值也不会越过预算）", () => {
    expect(sectionRevealStyle(false, false, 40).transform).toBe(
      `translateY(${MOTION_REVEAL_OFFSET_PX}px)`,
    );
  });

  it("负值 / NaN 钳到 0（不做反向进场）", () => {
    expect(sectionRevealStyle(false, false, -8).transform).toBe("translateY(0px)");
    expect(sectionRevealStyle(false, false, Number.NaN).transform).toBe("translateY(0px)");
  });

  it("减少动态下归零：直接可见、无过渡", () => {
    expect(sectionRevealStyle(false, true)).toEqual({
      opacity: "1",
      transform: "none",
      transition: "none",
    });
  });
});

describe("clampParallaxOffset：视差上限 10px", () => {
  it("在 ±10px 内原样返回", () => {
    expect(clampParallaxOffset(4, false)).toBe(4);
    expect(clampParallaxOffset(-6, false)).toBe(-6);
  });

  it("超出上限被钳住（视差不许超过 10px）", () => {
    expect(clampParallaxOffset(40, false)).toBe(PARALLAX_MAX_PX);
    expect(clampParallaxOffset(-40, false)).toBe(-PARALLAX_MAX_PX);
  });

  it("非法输入与减少动态都归零", () => {
    expect(clampParallaxOffset(Number.NaN, false)).toBe(0);
    expect(clampParallaxOffset(8, true)).toBe(0);
    expect(clampParallaxOffset(-8, true)).toBe(0);
  });
});

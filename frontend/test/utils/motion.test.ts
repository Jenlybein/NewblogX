import { describe, expect, it } from "vitest";

import {
  MOTION_DURATIONS_MS,
  MOTION_EASE,
  MOTION_EASE_CSS,  MOTION_STAGGER_MS,
  motionDurationMs,
  motionSeconds,
  motionTransition,
  prefersReducedMotionFrom,
  staggerDelayMs,
  staggerItemMotion,
  staggerTransition,
} from "~/utils/motion";

/**
 * 动效预算的判定逻辑。
 *
 * 这里测的是"可观察的取值约定"（三档时长 / 单一缓动 / stagger 节奏 / 减少动态的降级），
 * 不是 CSS 怎么写。组件只消费这些函数，改实现不改测试。
 */
describe("动效 token 取值（spec 决策 3：120 / 200 / 320ms + 单一缓动 + stagger 40ms）", () => {
  it("三档时长固定为 120 / 200 / 320ms", () => {
    expect(MOTION_DURATIONS_MS).toEqual({ fast: 120, base: 200, slow: 320 });
  });

  it("全站只有一条缓动曲线：motion-v 用贝塞尔数组，CSS 用同一条曲线的字符串", () => {
    expect(MOTION_EASE).toEqual([0.2, 0.8, 0.2, 1]);
    expect(MOTION_EASE_CSS).toBe("cubic-bezier(0.2, 0.8, 0.2, 1)");
  });

  it("列表 stagger 节奏是 40ms", () => {
    expect(MOTION_STAGGER_MS).toBe(40);
  });
});

describe("motionDurationMs：开启减少动态后退化为 0", () => {
  it("正常取值", () => {
    expect(motionDurationMs("fast", false)).toBe(120);
    expect(motionDurationMs("base", false)).toBe(200);
    expect(motionDurationMs("slow", false)).toBe(320);
  });

  it("prefers-reduced-motion 时全部归零（静态，而不是放慢）", () => {
    expect(motionDurationMs("fast", true)).toBe(0);
    expect(motionDurationMs("base", true)).toBe(0);
    expect(motionDurationMs("slow", true)).toBe(0);
  });
});

describe("staggerDelayMs：第 index 个条目的入场延迟", () => {
  it("按 40ms 递增", () => {
    expect(staggerDelayMs(0, false)).toBe(0);
    expect(staggerDelayMs(1, false)).toBe(40);
    expect(staggerDelayMs(3, false)).toBe(120);
  });

  it("减少动态时没有错峰延迟", () => {
    expect(staggerDelayMs(3, true)).toBe(0);
  });

  it("索引非法时钳到 0（不产生负延迟 / NaN）", () => {
    expect(staggerDelayMs(-2, false)).toBe(0);
    expect(staggerDelayMs(Number.NaN, false)).toBe(0);
  });
});

describe("motion-v 需要秒为单位的过渡描述", () => {
  it("毫秒换算成秒（保留三位精度）", () => {
    expect(motionSeconds(120)).toBe(0.12);
    expect(motionSeconds(200)).toBe(0.2);
    expect(motionSeconds(320)).toBe(0.32);
    expect(motionSeconds(0)).toBe(0);
  });

  it("单个过渡：时长 + 唯一缓动", () => {
    expect(motionTransition("fast", false)).toEqual({ duration: 0.12, delay: 0, ease: MOTION_EASE });
    expect(motionTransition("slow", true)).toEqual({ duration: 0, delay: 0, ease: MOTION_EASE });
  });

  it("列表条目过渡：时长 + delay（index × 40ms）+ 唯一缓动", () => {
    expect(staggerTransition(0, false)).toEqual({ duration: 0.2, delay: 0, ease: MOTION_EASE });
    expect(staggerTransition(2, false)).toEqual({ duration: 0.2, delay: 0.08, ease: MOTION_EASE });
    expect(staggerTransition(2, true)).toEqual({ duration: 0, delay: 0, ease: MOTION_EASE });
  });
});

describe("staggerItemMotion：列表入场的 initial/animate", () => {
  it("只动 opacity 与 y（即 transform），不碰布局属性", () => {
    const motion = staggerItemMotion(0, false);

    expect(Object.keys(motion.initial).sort()).toEqual(["opacity", "y"]);
    expect(Object.keys(motion.animate).sort()).toEqual(["opacity", "y"]);
    expect(motion.initial).toEqual({ opacity: 0, y: 8 });
    expect(motion.animate).toEqual({ opacity: 1, y: 0 });
  });

  it("把索引带进过渡 delay，方便列表错峰", () => {
    expect(staggerItemMotion(3, false).transition).toEqual({ duration: 0.2, delay: 0.12, ease: MOTION_EASE });
  });

  it("减少动态时同一份 initial/animate 但过渡为 0——SSR 与客户端标记一致，不会 hydration 抖动", () => {
    const reduced = staggerItemMotion(1, true);

    expect(reduced.initial).toEqual({ opacity: 0, y: 8 });
    expect(reduced.transition).toEqual({ duration: 0, delay: 0, ease: MOTION_EASE });
  });
});

describe("prefersReducedMotionFrom：没有 matchMedia 时（SSR / 老浏览器）安全兜底", () => {
  it("matchMedia 不可用时按「不减少动态」处理", () => {
    expect(prefersReducedMotionFrom(undefined)).toBe(false);
    expect(prefersReducedMotionFrom(null)).toBe(false);
  });

  it("按 matchMedia 的 matches 取值", () => {
    expect(prefersReducedMotionFrom({ matches: false })).toBe(false);
    expect(prefersReducedMotionFrom({ matches: true })).toBe(true);
  });
});

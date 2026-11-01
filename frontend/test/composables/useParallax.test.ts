import { describe, expect, it, vi } from "vitest";

import { useParallax, type ScrollNumberValue } from "~/composables/useParallax";

/**
 * 滚动视差的接线（motion-v 的 useScroll → translateY ≤10px）。
 *
 * 这里测接线契约而不是 motion-v 本身：注入假滚动值与假 media，断言
 * 「滚动量 → 位移」的映射、钳位、减少动态归零，以及**卸载时按 MotionValue 的
 * 真实 API 取消订阅**——motion-v / framer-motion 只有 `on()` 返回的取消函数、
 * 没有 `off`，在 dispose 里调 `off` 会在路由切换卸载时抛未捕获异常并中断视图
 * 切换（#80 的 P0 崩溃）。
 */
function fakeMedia(initialMatches = false) {
  const listeners = new Set<(event: { matches: boolean }) => void>();
  const media = {
    matches: initialMatches,
    addEventListener: (_type: "change", listener: (event: { matches: boolean }) => void) => {
      listeners.add(listener);
    },
    removeEventListener: (_type: "change", listener: (event: { matches: boolean }) => void) => {
      listeners.delete(listener);
    },
  };

  return {
    media,
    /** 模拟系统切换「减少动态」。 */
    set(matches: boolean) {
      media.matches = matches;
      for (const listener of listeners) listener({ matches });
    },
  };
}

function fakeScroll(initial = 0) {
  let value = initial;
  const listeners = new Set<(value: number) => void>();
  // MotionValue 的契约：on() 返回取消订阅函数，而不是 off()。
  const stop = vi.fn(() => listeners.clear());
  const scroll: ScrollNumberValue = {
    get: () => value,
    on: vi.fn((_type: "change", callback: (value: number) => void) => {
      listeners.add(callback);
      return stop;
    }),
  };

  return {
    scroll,
    stop,
    listeners,
    emit(next: number) {
      value = next;
      for (const listener of listeners) listener(next);
    },
  };
}

const reducedMotion = (matches: boolean) => ({
  matches,
  addEventListener: () => undefined,
  removeEventListener: () => undefined,
});

describe("useParallax：接线", () => {
  it("卸载（路由切换销毁）时按 MotionValue 真实 API 取消订阅，不抛异常（#80 回归）", () => {
    const { scroll, stop } = fakeScroll(0);
    const parallax = useParallax({ scroll, motion: { matchMedia: () => reducedMotion(false) } });

    expect(() => parallax.dispose()).not.toThrow();
    expect(stop).toHaveBeenCalledTimes(1);
  });

  it("滚动量按 0.02 的比例换算成位移（500px 用满 10px 预算）", () => {
    const { scroll, emit } = fakeScroll(0);
    const parallax = useParallax({ scroll, motion: { matchMedia: () => reducedMotion(false) } });

    expect(parallax.offset.value).toBe(0);

    emit(250);
    expect(parallax.offset.value).toBeCloseTo(5);

    emit(500);
    expect(parallax.offset.value).toBeCloseTo(10);
  });

  it("位移钳在 ±10px：滚动再远也不超出预算", () => {
    const { scroll, emit } = fakeScroll(0);
    const parallax = useParallax({ scroll, motion: { matchMedia: () => reducedMotion(false) } });

    emit(2000);
    expect(parallax.offset.value).toBe(10);

    emit(-2000);
    expect(parallax.offset.value).toBe(-10);
  });

  it("样式契约：归零时 transform: none，否则只给 translateY", () => {
    const { scroll, emit } = fakeScroll(0);
    const parallax = useParallax({ scroll, motion: { matchMedia: () => reducedMotion(false) } });

    expect(parallax.parallaxStyle.value).toEqual({ transform: "none" });

    emit(400);
    expect(parallax.parallaxStyle.value).toEqual({ transform: "translateY(8px)" });
  });

  it("减少动态：位移归零，不跟随滚动", () => {
    const { scroll, emit } = fakeScroll(400);
    const parallax = useParallax({ scroll, motion: { matchMedia: () => reducedMotion(true) } });

    expect(parallax.offset.value).toBe(0);

    emit(800);
    expect(parallax.offset.value).toBe(0);
  });

  it("系统切换「减少动态」时立刻归零（不是等下一次滚动）", () => {
    const motionMedia = fakeMedia(false);
    const { scroll, emit } = fakeScroll(0);
    const parallax = useParallax({
      scroll,
      motion: { matchMedia: () => motionMedia.media },
    });

    emit(400);
    expect(parallax.offset.value).toBe(8);

    motionMedia.set(true);
    expect(parallax.offset.value).toBe(0);
  });

  it("销毁后不再跟随滚动（订阅确实被摘掉）", () => {
    const { scroll, emit, listeners } = fakeScroll(0);
    const parallax = useParallax({ scroll, motion: { matchMedia: () => reducedMotion(false) } });

    expect(listeners.size).toBe(1);

    emit(400);
    expect(parallax.offset.value).toBe(8);

    parallax.dispose();
    expect(listeners.size).toBe(0);

    emit(900);
    expect(parallax.offset.value).toBe(8);
  });
});

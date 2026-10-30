import { describe, expect, it, vi } from "vitest";

import { useMotionPreference } from "~/composables/useMotionPreference";

/**
 * 「减少动态」的取值来源。
 *
 * 判定逻辑本身是纯函数（utils/motion 的单测），这里测的是**接线**：
 * 读到初值、监听系统设置变化、组件销毁时摘掉监听、没有 matchMedia 时安全兜底。
 * 用注入的假 MediaQueryList，不碰真实浏览器。
 */

type Listener = (event: { matches: boolean }) => void;

function fakeMediaQuery(matches: boolean) {
  const listeners = new Set<Listener>();
  return {
    query: {
      matches,
      addEventListener: vi.fn((_type: "change", listener: Listener) => listeners.add(listener)),
      removeEventListener: vi.fn((_type: "change", listener: Listener) => listeners.delete(listener)),
    },
    emit(next: boolean) {
      for (const listener of listeners) listener({ matches: next });
    },
    listeners,
  };
}

describe("useMotionPreference", () => {
  it("读到系统初始设置", () => {
    const reduced = fakeMediaQuery(true);
    const motion = useMotionPreference({ matchMedia: () => reduced.query });

    expect(motion.prefersReducedMotion.value).toBe(true);
  });

  it("系统设置变化时跟着变（不需要重新加载页面）", () => {
    const reduced = fakeMediaQuery(false);
    const motion = useMotionPreference({ matchMedia: () => reduced.query });

    expect(motion.prefersReducedMotion.value).toBe(false);

    reduced.emit(true);
    expect(motion.prefersReducedMotion.value).toBe(true);

    reduced.emit(false);
    expect(motion.prefersReducedMotion.value).toBe(false);
  });

  it("销毁时摘掉监听（否则切页后旧监听会泄漏）", () => {
    const reduced = fakeMediaQuery(false);
    const motion = useMotionPreference({ matchMedia: () => reduced.query });

    motion.dispose();

    expect(reduced.listeners.size).toBe(0);
    expect(reduced.query.removeEventListener).toHaveBeenCalledTimes(1);
  });

  it("matchMedia 不可用 / 返回 null 时按「不减少动态」处理，且不报错", () => {
    expect(useMotionPreference({ matchMedia: () => null }).prefersReducedMotion.value).toBe(false);
    expect(useMotionPreference({ matchMedia: () => undefined }).prefersReducedMotion.value).toBe(false);
  });

  it("缺省实现读 window.matchMedia（测试环境由 setup.ts 桩成 matches=false）", () => {
    const motion = useMotionPreference();

    expect(motion.prefersReducedMotion.value).toBe(false);
  });

  it("缺省实现读到系统开启减少动态时取 true", () => {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query === "(prefers-reduced-motion: reduce)",
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
    }));

    const motion = useMotionPreference();

    expect(motion.prefersReducedMotion.value).toBe(true);
  });
});

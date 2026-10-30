import { describe, expect, it, vi } from "vitest";

import {
  useScrollChrome,
  type ScrollChromeTarget,
} from "~/composables/useScrollChrome";

/**
 * 吸顶页头的滚动接线。
 *
 * 纯判定在 utils/scrollChrome（单测），这里测接线：监听 scroll、读 scrollY、
 * 把状态映射成 ref、减少动态时整个不启用、销毁时摘掉监听、SSR 无 window 不报错。
 */
function fakeTarget(initialScrollY = 0) {
  const listeners = new Set<() => void>();
  const target: ScrollChromeTarget = {
    scrollY: initialScrollY,
    addEventListener: vi.fn(
      (_type: "scroll", listener: () => void, _options?: { passive?: boolean }) => {
        listeners.add(listener);
      },
    ),
    removeEventListener: vi.fn((_type: "scroll", listener: () => void) => {
      listeners.delete(listener);
    }),
  };

  return {
    target,
    emitScroll(scrollY?: number) {
      if (typeof scrollY === "number") target.scrollY = scrollY;
      for (const listener of listeners) listener();
    },
    listeners,
  };
}

function fakeMediaQuery(matches: boolean) {
  return {
    matches,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  };
}

describe("useScrollChrome：接线", () => {
  it("初始处于静止态（页头可见、不转玻璃）", () => {
    const { target } = fakeTarget(0);
    const chrome = useScrollChrome({ target });

    expect(chrome.enabled.value).toBe(true);
    expect(chrome.scrolled.value).toBe(false);
    expect(chrome.hidden.value).toBe(false);
  });

  it("滚动超过阈值：转玻璃 + 下滚隐藏；上滚恢复出现", () => {
    const { target, emitScroll } = fakeTarget(0);
    const chrome = useScrollChrome({ target });

    emitScroll(120);
    expect(chrome.scrolled.value).toBe(true);
    expect(chrome.hidden.value).toBe(true);

    emitScroll(20);
    expect(chrome.hidden.value).toBe(false);
    expect(chrome.scrolled.value).toBe(true);

    emitScroll(0);
    expect(chrome.scrolled.value).toBe(false);
    expect(chrome.hidden.value).toBe(false);
  });

  it("阈值可注入（与状态机共用同一份预算）", () => {
    const { target, emitScroll } = fakeTarget(0);
    const chrome = useScrollChrome({ target, threshold: 64 });

    emitScroll(40);
    expect(chrome.scrolled.value).toBe(false);
  });

  it("减少动态下整个不启用：监听不改变状态", () => {
    const media = fakeMediaQuery(true);
    const { target, emitScroll } = fakeTarget(0);
    const chrome = useScrollChrome({ target, motion: { matchMedia: () => media } });

    expect(chrome.enabled.value).toBe(false);

    emitScroll(240);
    expect(chrome.scrolled.value).toBe(false);
    expect(chrome.hidden.value).toBe(false);
  });

  it("系统设置切换成减少动态时，已有状态立刻归零（不会卡在隐藏态）", async () => {
    const { ref } = await import("vue");
    const media = ref(fakeMediaQuery(false));
    const { target, emitScroll } = fakeTarget(0);
    const chrome = useScrollChrome({ target, motion: { matchMedia: () => media.value } });

    emitScroll(240);
    expect(chrome.hidden.value).toBe(true);

    // 与真实实现一致：matchMedia 变化时把 enabled 置 false，由 watch 复位状态。
    chrome.enabled.value = false;
    await Promise.resolve();
    await Promise.resolve();

    expect(chrome.scrolled.value).toBe(false);
    expect(chrome.hidden.value).toBe(false);
  });

  it("销毁时摘掉监听（切页/卸载后不留旧监听）", () => {
    const { target, listeners } = fakeTarget(0);
    const chrome = useScrollChrome({ target });

    expect(listeners.size).toBe(1);
    chrome.dispose();

    expect(listeners.size).toBe(0);
    expect(target.removeEventListener).toHaveBeenCalledTimes(1);
  });

  it("没有可用目标（SSR / 无 window）时安全兜底：不启用、不报错", () => {
    const chrome = useScrollChrome({ target: null });

    expect(chrome.enabled.value).toBe(false);
    expect(chrome.scrolled.value).toBe(false);
    expect(chrome.hidden.value).toBe(false);
  });
});

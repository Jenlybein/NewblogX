import { describe, expect, it } from "vitest";

import { THEME_SWITCH_CLASS, THEME_SWITCH_FRAMES, beginThemeSwitch } from "~/utils/themeTransition";

/**
 * 主题切换的同帧守卫（票 428，spec 423 第 4 批第 30 条）。
 *
 * 明暗切换时纸面与结构线必须同一帧换。CSS 变量本身是瞬时的，但页面上到处是
 * `transition-colors` 之类的颜色过渡 —— 它们会让各处面自己渐变，于是出现
 * "底已经换、面还在路上"的半明半暗瞬间。守卫的做法是：切换那一帧给根挂上
 * `.theme-switching`（CSS 里把全站 transition 关掉），过两帧再放开。
 *
 * 这里只用假 document root + 假帧调度验证可观察行为（class 的加与撤），
 * 不碰真实 rAF —— 用真帧调度会让用例依赖机器时序。
 */
function makeRoot(): HTMLElement {
  return document.createElement("div");
}

function makeFrames(): {
  frame: (callback: () => void) => void;
  flushOne: () => void;
  flush: () => void;
  pending: () => number;
} {
  const queue: Array<() => void> = [];
  return {
    frame: (callback) => queue.push(callback),
    flushOne: () => {
      const next = queue.shift();
      next?.();
    },
    flush: () => {
      while (queue.length > 0) queue.shift()!();
    },
    pending: () => queue.length,
  };
}

describe("beginThemeSwitch：切换帧关过渡，随后放开", () => {
  it("挂上 .theme-switching 后，至少等到第二帧才撤掉（保证切换的那次绘制在守卫内）", () => {
    const root = makeRoot();
    const frames = makeFrames();

    beginThemeSwitch({ root, requestFrame: frames.frame });

    expect(root.classList.contains(THEME_SWITCH_CLASS)).toBe(true);
    expect(THEME_SWITCH_FRAMES).toBeGreaterThanOrEqual(2);

    // 第一帧：还不能撤（Vue 的根类更新与浏览器重算都在这一帧里发生）。
    frames.flushOne();
    expect(frames.pending(), "一帧之后应该还有下一帧").toBeGreaterThan(0);

    frames.flush();
    expect(root.classList.contains(THEME_SWITCH_CLASS)).toBe(false);
  });

  it("连续切换两次：第二次接管收尾，不会出现「提前撤守卫」的窗口", () => {
    const root = makeRoot();
    const frames = makeFrames();

    beginThemeSwitch({ root, requestFrame: frames.frame });
    beginThemeSwitch({ root, requestFrame: frames.frame });
    expect(root.classList.contains(THEME_SWITCH_CLASS)).toBe(true);

    frames.flush();

    expect(root.classList.contains(THEME_SWITCH_CLASS)).toBe(false);
    expect(frames.pending()).toBe(0);
  });

  it("没有可用的根元素时不抛错（SSR / 无 document 环境直接跳过）", () => {
    expect(() =>
      beginThemeSwitch({
        root: null,
        requestFrame: () => {
          throw new Error("不该被调用");
        },
      }),
    ).not.toThrow();
  });
});

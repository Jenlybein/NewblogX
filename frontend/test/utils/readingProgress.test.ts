import { describe, expect, it } from "vitest";

import { READING_HEADING_THRESHOLD, resolveActiveHeading, resolveProgressPercent } from "~/utils/readingProgress";

describe("阅读进度：目录当前章节（纯函数）", () => {
  it("没有标题时返回空串", () => {
    expect(resolveActiveHeading([], {})).toBe("");
  });

  it("取最后一个已越过阈值的标题", () => {
    // 阈值 164：标题元素 top 小于等于 164 视为"读到了"。
    expect(
      resolveActiveHeading(["a", "b", "c"], { a: -100, b: 20, c: 200 }),
    ).toBe("b");
    expect(READING_HEADING_THRESHOLD).toBe(164);
  });

  it("一个都没读到阈值时仍停在第一个（目录不空高亮）", () => {
    expect(resolveActiveHeading(["a", "b"], { a: 400, b: 900 })).toBe("a");
  });

  it("尚未测量到的标题会被跳过，不打断后续判断", () => {
    expect(resolveActiveHeading(["a", "b", "c"], { b: -10, c: 40 })).toBe("c");
  });

  it("阈值可注入（页面顶部有吸顶页头时可以调整）", () => {
    expect(resolveActiveHeading(["a", "b"], { a: 30, b: 240 }, 40)).toBe("a");
  });
});

describe("阅读进度百分比（纯函数）", () => {
  it("顶部为 0，底部为 100", () => {
    expect(resolveProgressPercent(0, 5000, 1000)).toBe(0);
    expect(resolveProgressPercent(4000, 5000, 1000)).toBe(100);
  });

  it("中间线性推进", () => {
    expect(resolveProgressPercent(1000, 5000, 1000)).toBe(25);
  });

  it("越界值被钳制在 0..100（负滚动 / overscroll 不会画出负数）", () => {
    expect(resolveProgressPercent(-120, 5000, 1000)).toBe(0);
    expect(resolveProgressPercent(9999, 5000, 1000)).toBe(100);
  });

  it("内容不足一屏时进度为 0（不除零）", () => {
    expect(resolveProgressPercent(0, 800, 1000)).toBe(0);
    expect(resolveProgressPercent(400, 800, 1000)).toBe(0);
  });
});

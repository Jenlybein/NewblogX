import { describe, expect, it } from "vitest";
import { hasNextPage } from "~/utils/pagination";

describe("hasNextPage", () => {
  it("有剩余数据时返回 true", () => {
    expect(hasNextPage(1, 10, 23)).toBe(true);
    expect(hasNextPage(2, 10, 23)).toBe(true);
  });

  it("刚好整页时返回 false", () => {
    // 23 条、每页 10 条：第 3 页装 3 条，是最后一页。
    expect(hasNextPage(3, 10, 23)).toBe(false);
    // 20 条刚好两页，第 2 页之后就没有了。
    expect(hasNextPage(2, 10, 20)).toBe(false);
  });

  it("空列表时返回 false", () => {
    expect(hasNextPage(1, 10, 0)).toBe(false);
  });

  it("非法入参一律返回 false，不抛错", () => {
    expect(hasNextPage(0, 10, 100)).toBe(false);
    expect(hasNextPage(1, 0, 100)).toBe(false);
    expect(hasNextPage(1, 10, Number.NaN)).toBe(false);
    expect(hasNextPage(Number.POSITIVE_INFINITY, 10, 100)).toBe(false);
  });
});

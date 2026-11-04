import { describe, expect, it } from "vitest";

import { firstQueryValue } from "~/utils/queryValue";

/**
 * 路由 query 取值（评审 #248 / P1-2）。
 *
 * 这份实现以前在 `utils/chat.ts` 与 `utils/messageCenter.ts` 各有一份，
 * 一份只认字符串、一份也认数字：同一条 `?group=2` 在两个入口拿到不同类型。
 * 现在共用一份，宽进（数组取第一项、数字转字符串）。
 */

describe("firstQueryValue", () => {
  it("字符串原样返回", () => {
    expect(firstQueryValue("chat")).toBe("chat");
  });

  it("数组取第一项（重复参数的常见形状）", () => {
    expect(firstQueryValue(["chat", "site"])).toBe("chat");
    expect(firstQueryValue([])).toBe("");
  });

  it("数字转字符串（Nuxt 会把纯数字 query 归一化成 number）", () => {
    expect(firstQueryValue(2)).toBe("2");
  });

  it("缺省与不可展示的值一律空串", () => {
    expect(firstQueryValue(undefined)).toBe("");
    expect(firstQueryValue(null)).toBe("");
    expect(firstQueryValue({})).toBe("");
    expect(firstQueryValue([null, "x"])).toBe("");
  });
});

import { describe, expect, it } from "vitest";

import { HEADER_NAV_ICONS, headerNavIcon } from "~/utils/navIcon";

/**
 * 页头第四项（合集）的图标选择逻辑。
 *
 * 这里钉的是"每个入口都有自己的图标"：旧实现按 key 三元选择，除首页外一律
 * 搜索图标——加新入口时那行代码不会报错，只是静默错。改成显式映射之后，
 * 漏配一个 key 就是类型错误；这条用例再把"四个 key 都有值且互不相同"锁住。
 */
describe("headerNavIcon：入口到图标的显式映射", () => {
  it("四个入口都有图标", () => {
    expect(Object.keys(HEADER_NAV_ICONS).sort()).toEqual([
      "collections",
      "home",
      "search",
      "write",
    ]);
  });

  it("每个入口的图标互不相同（合集不能复用搜索图标）", () => {
    const components = Object.values(HEADER_NAV_ICONS);
    expect(new Set(components).size).toBe(components.length);
  });

  it("headerNavIcon 与映射表一致", () => {
    expect(headerNavIcon("collections")).toBe(HEADER_NAV_ICONS.collections);
    expect(headerNavIcon("home")).toBe(HEADER_NAV_ICONS.home);
  });
});

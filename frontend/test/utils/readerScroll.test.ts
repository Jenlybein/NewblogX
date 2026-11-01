import { describe, expect, it } from "vitest";

import { resolveReaderScrollPosition } from "~/utils/readerScroll";

/**
 * 路由滚动行为（spec #79 用户故事 3）：返回停在原滚动位置，其它导航回顶。
 * 只测纯函数，不依赖真浏览器 / Vue Router —— 接线在 app/router.options.ts。
 */
describe("resolveReaderScrollPosition：savedPosition 优先，否则回顶", () => {
  it("浏览器返回给出保存位置时原样恢复（用户回到原来的滚动位置）", () => {
    expect(resolveReaderScrollPosition({ left: 0, top: 482 })).toEqual({ left: 0, top: 482 });
    expect(resolveReaderScrollPosition({ left: 12, top: 900 })).toEqual({ left: 12, top: 900 });
  });

  it("没有保存位置（点链接去新页面 / 首屏进入）回顶", () => {
    expect(resolveReaderScrollPosition(null)).toEqual({ left: 0, top: 0 });
    expect(resolveReaderScrollPosition(undefined)).toEqual({ left: 0, top: 0 });
  });

  it("缺分量 / 非数值的脏位置不猜，回顶（避免 NaN 滚出页面）", () => {
    expect(resolveReaderScrollPosition({ top: 300 })).toEqual({ left: 0, top: 0 });
    expect(resolveReaderScrollPosition({ left: 0 })).toEqual({ left: 0, top: 0 });
    expect(resolveReaderScrollPosition({ left: Number.NaN, top: 100 })).toEqual({ left: 0, top: 0 });
  });

  it("0 是合法位置（页面顶部也是保存位置，不算缺失）", () => {
    expect(resolveReaderScrollPosition({ left: 0, top: 0 })).toEqual({ left: 0, top: 0 });
  });
});

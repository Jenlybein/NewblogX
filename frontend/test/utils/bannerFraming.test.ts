import { describe, expect, it } from "vitest";

import { BANNER_FRAME_RATIO, bannerFramingStyle } from "~/utils/bannerFraming";

/**
 * 取景参数 → 样式对象的纯函数（ADR-0015）。
 *
 * 这里只测"可观察的取值约定"，不测组件或 CSS 怎么写：
 * 缺省 = 居中、缩放 1；焦点落在 transform-origin（相对坐标 → 百分比）；
 * 越界值一律钳制，保证任何输入都不露出面板底色（缩放下界是 1）。
 */
describe("bannerFramingStyle：取景参数 → 样式对象", () => {
  it("缺省取景（无 crop / 空对象 / 历史数据）＝居中、缩放 1，与引入取景前一致", () => {
    const expected = {
      objectFit: "cover",
      objectPosition: "50% 50%",
      transformOrigin: "50% 50%",
      transform: "scale(1)",
    };

    expect(bannerFramingStyle()).toMatchObject(expected);
    expect(bannerFramingStyle(null)).toMatchObject(expected);
    expect(bannerFramingStyle({})).toMatchObject(expected);
  });

  it("焦点是相对坐标：同时落成 object-position 与 transform-origin 的百分比", () => {
    // 两个属性必须取同一个点：前者把图上焦点对到框内同一点，后者让缩放围着它，
    // 这就是后台取景窗预览与阅读站线上渲染同一套几何的前提。
    const quarters = bannerFramingStyle({ focus_x: 0.25, focus_y: 0.75 });
    expect(quarters.objectPosition).toBe("25% 75%");
    expect(quarters.transformOrigin).toBe("25% 75%");

    // 只缺一个分量时，另一个按居中缺省。
    const half = bannerFramingStyle({ focus_x: 0.2 });
    expect(half.objectPosition).toBe("20% 50%");
    expect(half.transformOrigin).toBe("20% 50%");
  });

  it("缩放放大到 scale(zoom)", () => {
    expect(bannerFramingStyle({ zoom: 1.5 }).transform).toBe("scale(1.5)");
    expect(bannerFramingStyle({ zoom: 2 }).transform).toBe("scale(2)");
  });

  it("极限值：焦点钳到 0–1、缩放钳到 1–2（任何取值都不露底色）", () => {
    const low = bannerFramingStyle({ focus_x: -2, focus_y: -2, zoom: 0.2 });
    const high = bannerFramingStyle({ focus_x: 3, focus_y: 3, zoom: 5 });

    expect(low.objectPosition).toBe("0% 0%");
    expect(low.transformOrigin).toBe("0% 0%");
    // 缩放的下界是 1：小于 1 才会露出面板底色。
    expect(low.transform).toBe("scale(1)");
    expect(high.objectPosition).toBe("100% 100%");
    expect(high.transformOrigin).toBe("100% 100%");
    expect(high.transform).toBe("scale(2)");
  });

  it("非有限值退化到缺省，不让 NaN / Infinity 进样式", () => {
    const style = bannerFramingStyle({ focus_x: Number.NaN, focus_y: Number.POSITIVE_INFINITY, zoom: Number.NaN });

    expect(style.transformOrigin).toBe("50% 50%");
    expect(style.transform).toBe("scale(1)");
  });

  it("固定 16:9 是唯一一份比例常量（两端静态断言引用它）", () => {
    expect(BANNER_FRAME_RATIO).toBe("16 / 9");
  });
});

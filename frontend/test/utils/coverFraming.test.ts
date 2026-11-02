import { describe, expect, it } from "vitest";

import {
  COVER_FRAME_RATIO,
  DEFAULT_FRAMING,
  clampFraming,
  framingStyle,
  isAspectCompliant,
  nudgeFraming,
  panFraming,
  stepFramingZoom,
} from "~/utils/coverFraming";

/**
 * 文章封面取景算术（与后台轮播取景窗同一套换算）。
 *
 * 为什么先测算术而不是先挂组件：取景不上传第二份图，线上是按「焦点 + 缩放」
 * 三个数用 CSS 复现同一块取景。于是"这块图会露出什么"完全由算术决定；
 * 算术只写在组件里时，只能靠量像素来测——量不准，也拦不住"拖到角落露白"。
 *
 * 这里钉住三件事：
 *   1. 16:9 合规判定（±1% 容差，边界值算合规）；
 *   2. 参数钳制（焦点 0–1、缩放 1–2），任何取值都不露白；
 *   3. 预览样式与后台、阅读站同一份逐条语义（百分比 + scale，无像素值）。
 */

describe("isAspectCompliant：16:9 锁定判定", () => {
  it("标准 16:9 与常见等比尺寸都合规", () => {
    expect(isAspectCompliant(1920, 1080)).toBe(true);
    expect(isAspectCompliant(1280, 720)).toBe(true);
    expect(isAspectCompliant(3840, 2160)).toBe(true);
  });

  it("±1% 以内的噪声（1919×1079 这类）不弹取景窗", () => {
    expect(isAspectCompliant(1919, 1079)).toBe(true);
    expect(isAspectCompliant(1921, 1080)).toBe(true);
    // 1616×900 正好落在 +1% 边界上：闭区间算合规。
    expect(isAspectCompliant(1616, 900)).toBe(true);
    expect(isAspectCompliant(1584, 900)).toBe(true);
  });

  it("超出 1% 的横图与竖图都必须先取景", () => {
    expect(isAspectCompliant(1617, 900)).toBe(false);
    expect(isAspectCompliant(1583, 900)).toBe(false);
    expect(isAspectCompliant(1200, 800)).toBe(false);
    expect(isAspectCompliant(1080, 1920)).toBe(false);
  });

  it("尺寸非法（0 / 负数 / NaN / Infinity）一律不合规", () => {
    expect(isAspectCompliant(0, 0)).toBe(false);
    expect(isAspectCompliant(-1920, 1080)).toBe(false);
    expect(isAspectCompliant(1920, 0)).toBe(false);
    expect(isAspectCompliant(Number.NaN, 1080)).toBe(false);
    expect(isAspectCompliant(1920, Number.POSITIVE_INFINITY)).toBe(false);
  });
});

describe("取景参数：钳制与交互", () => {
  it("焦点恒在 0–1、缩放恒在 1–2，非法值回缺省", () => {
    expect(clampFraming({ focus_x: -2, focus_y: 3, zoom: 5 })).toEqual({
      focus_x: 0,
      focus_y: 1,
      zoom: 2,
    });
    expect(clampFraming({ focus_x: Number.NaN, focus_y: Number.POSITIVE_INFINITY, zoom: 0 })).toEqual(
      DEFAULT_FRAMING,
    );
    expect(clampFraming(null)).toEqual(DEFAULT_FRAMING);
  });

  it("拖动是推图：拖到右边会看到更靠左的区域", () => {
    const frame = { width: 800, height: 450 };
    const image = { width: 1600, height: 450 };
    const moved = panFraming(DEFAULT_FRAMING, 400, 0, frame, image);

    expect(moved.focus_x).toBeLessThan(0.5);
    expect(moved.focus_x).toBeCloseTo(0, 2);
    expect(moved.zoom).toBe(1);
  });

  it("方向键移动的是取景范围（与拖动方向相反），步进 2%", () => {
    const right = nudgeFraming(DEFAULT_FRAMING, 1, 0);

    expect(right.focus_x).toBeCloseTo(0.52, 4);
    expect(right.focus_y).toBe(0.5);
  });

  it("缩放按 0.05 步进且钳在 1–2", () => {
    expect(stepFramingZoom(DEFAULT_FRAMING, 1).zoom).toBe(1.05);
    expect(stepFramingZoom(DEFAULT_FRAMING, -1).zoom).toBe(1);
    expect(stepFramingZoom({ focus_x: 0.5, focus_y: 0.5, zoom: 2 }, 1).zoom).toBe(2);
  });
});

describe("framingStyle：预览几何 = 最终渲染几何", () => {
  it("只输出百分比与 scale，不含像素值", () => {
    const style = framingStyle({ focus_x: 0.25, focus_y: 0.75, zoom: 1.5 });

    expect(style.objectFit).toBe("cover");
    expect(style.objectPosition).toBe("25% 75%");
    expect(style.transformOrigin).toBe("25% 75%");
    expect(style.transform).toBe("scale(1.5)");
    expect(Object.values(style).join(" ")).not.toMatch(/px|vh|vw/u);
  });

  it("缺省取景＝居中、缩放 1（历史图与首次打开一致）", () => {
    const style = framingStyle();

    expect(style.objectPosition).toBe("50% 50%");
    expect(style.transform).toBe("scale(1)");
  });
});

describe("比例常量", () => {
  it("固定 16:9 是唯一取值点（与后台取景窗一致）", () => {
    expect(COVER_FRAME_RATIO).toBe("16 / 9");
  });
});

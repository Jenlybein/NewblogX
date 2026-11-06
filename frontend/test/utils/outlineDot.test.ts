import { describe, expect, it } from "vitest";

import {
  OUTLINE_DOT_ARC_MAX_PX,
  OUTLINE_GUIDE_OFFSET_REM,
  OUTLINE_INDENT_REM,
  outlineDotArcOffset,
  outlineDotPointAt,
  outlineDotXPx,
  outlineDotXRem,
} from "~/utils/outlineDot";

/**
 * 沟槽圆点的几何（纯函数接缝）。
 *
 * 期望值来自 spec 的**文字公式**而不是实现里的算式：
 *   横向 `x = depth × 1.5rem − 0.75rem`（depth 0 取 0）；
 *   弧线法向偏移的模 = `min(0.8 × 位移长度, 14px)`（参考量级 `min(0.8, 14/distance)`
 *   乘上位移向量），进度 t 处再乘 `sin(πt)`。
 */

describe("缩进口径", () => {
  it("每层 1.5rem、引导线在缩进起点左侧 0.75rem（与树的 CSS 同源）", () => {
    expect(OUTLINE_INDENT_REM).toBe(1.5);
    expect(OUTLINE_GUIDE_OFFSET_REM).toBe(0.75);
  });
});

describe("outlineDotXRem：圆点横向压在所属层级的引导线上", () => {
  it("depth 0 落在最外侧缩进起点（x = 0）", () => {
    expect(outlineDotXRem(0)).toBe(0);
  });

  it("depth ≥ 1 落在该层的发丝线上（depth × 1.5 − 0.75）", () => {
    expect(outlineDotXRem(1)).toBe(0.75);
    expect(outlineDotXRem(2)).toBe(2.25);
    expect(outlineDotXRem(3)).toBe(3.75);
  });

  it("跨层级是单调横移，不重复落在上一层的线上", () => {
    const xs = [0, 1, 2].map((depth) => outlineDotXRem(depth));

    expect(xs).toEqual([0, 0.75, 2.25]);
    expect(new Set(xs).size).toBe(3);
  });

  it("缺失 / 非法层级按根层级兜底，不产生 NaN 坐标", () => {
    expect(outlineDotXRem(undefined)).toBe(0);
    expect(outlineDotXRem(null)).toBe(0);
    expect(outlineDotXRem(Number.NaN)).toBe(0);
    expect(outlineDotXRem(-2)).toBe(0);
    expect(outlineDotXRem(1.8)).toBe(0.75);
  });
});

describe("outlineDotXPx：rem 按根字号换算成 px", () => {
  it("16px 根字号下 depth 1 = 12px、depth 2 = 36px", () => {
    expect(outlineDotXPx(1, 16)).toBe(12);
    expect(outlineDotXPx(2, 16)).toBe(36);
    expect(outlineDotXPx(0, 16)).toBe(0);
  });

  it("根字号变化时按比例缩放（rem 口径不写死像素）", () => {
    expect(outlineDotXPx(2, 20)).toBe(45);
  });

  it("根字号缺失时按 16px 兜底", () => {
    expect(outlineDotXPx(1, Number.NaN)).toBe(12);
    expect(outlineDotXPx(1, 0)).toBe(12);
  });
});

describe("outlineDotArcOffset：斜向移动才画弧", () => {
  it("只有纵向变化（同层换行）退化为直线：不横移、不离开引导线", () => {
    expect(outlineDotArcOffset(0, 120)).toEqual({ x: 0, y: 0 });
  });

  it("只有横向变化同样退化为直线", () => {
    expect(outlineDotArcOffset(48, 0)).toEqual({ x: 0, y: 0 });
  });

  it("位移向量与原点的退化输入不产生 NaN", () => {
    expect(outlineDotArcOffset(0, 0)).toEqual({ x: 0, y: 0 });
    expect(outlineDotArcOffset(Number.NaN, 10)).toEqual({ x: 0, y: 0 });
  });

  it("长距离斜移：偏移量被 14px 上限钳住，方向垂直于位移向量", () => {
    // 3-4-5 三角形：distance = 150
    const offset = outlineDotArcOffset(120, 90);

    expect(Math.hypot(offset.x, offset.y)).toBeCloseTo(OUTLINE_DOT_ARC_MAX_PX, 5);
    // 垂直：点积为 0
    expect(offset.x * 120 + offset.y * 90).toBeCloseTo(0, 5);
    // 顺时针法向（位移右下 → 弧往左下鼓）
    expect(offset.x).toBeLessThan(0);
    expect(offset.y).toBeGreaterThan(0);
  });

  it("短距离斜移：偏移量取位移长度的 0.8 倍（参考量级 min(0.8, 14/distance)）", () => {
    // distance = 10
    const offset = outlineDotArcOffset(6, 8);

    expect(Math.hypot(offset.x, offset.y)).toBeCloseTo(8, 5);
    expect(offset.x * 6 + offset.y * 8).toBeCloseTo(0, 5);
  });
});

describe("outlineDotPointAt：弹簧驱动的 0→1 进度取点", () => {
  const from = { x: 12, y: 40 };
  const to = { x: 36, y: 140 };

  it("进度 0 在起点、进度 1 在终点（落位无残留偏移）", () => {
    expect(outlineDotPointAt(from, to, 0)).toEqual(from);
    expect(outlineDotPointAt(from, to, 1)).toEqual(to);
  });

  it("进度 0.5 在位移中点外加满额弧偏移（sin(π/2) = 1）", () => {
    const offset = outlineDotArcOffset(to.x - from.x, to.y - from.y);
    const middle = outlineDotPointAt(from, to, 0.5);

    expect(middle.x).toBeCloseTo((from.x + to.x) / 2 + offset.x, 5);
    expect(middle.y).toBeCloseTo((from.y + to.y) / 2 + offset.y, 5);
  });

  it("同层换行（x 不变）时整条路径都落在引导线上", () => {
    for (const progress of [0, 0.25, 0.5, 0.75, 1]) {
      expect(outlineDotPointAt(from, { x: from.x, y: 300 }, progress).x).toBe(from.x);
    }
  });

  it("进度越界 / 非数值钳在两端，不回弹到起点之外", () => {
    expect(outlineDotPointAt(from, to, -0.4)).toEqual(from);
    expect(outlineDotPointAt(from, to, 1.6)).toEqual(to);
    expect(outlineDotPointAt(from, to, Number.NaN)).toEqual(from);
  });
});

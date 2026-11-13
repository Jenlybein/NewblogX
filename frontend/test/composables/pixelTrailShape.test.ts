import { describe, expect, it } from "vitest";

import { TRAIL_PEAK_AT, trailAlpha, trailShape } from "~/composables/usePixelTrail";

/**
 * 尾迹形状：起点 0.5em → TRAIL_PEAK_AT 处峰值 1em → 远端 0.8em。
 *
 * 断言用导出的 `TRAIL_PEAK_AT` 而不是写死数字：峰值位置是个观感旋钮，会反复调，
 * 但"起点最小 / 峰值最大 / 两端单调"这些关系不该跟着变。
 */
describe("尾迹形状曲线", () => {
  it("起点是 0.5em（全局最小值）", () => {
    expect(trailShape(0)).toBeCloseTo(0.5, 5);
  });

  it("TRAIL_PEAK_AT 处是峰值 1em", () => {
    expect(trailShape(TRAIL_PEAK_AT)).toBeCloseTo(1, 5);
  });

  it("远端是 0.8em", () => {
    expect(trailShape(1)).toBeCloseTo(0.8, 5);
  });

  it("峰值位置在一个尾迹半径之内（不会跑到范围外）", () => {
    expect(TRAIL_PEAK_AT).toBeGreaterThan(0);
    expect(TRAIL_PEAK_AT).toBeLessThan(1);
  });

  it("前段单调升、后段单调降（峰值处没有折角）", () => {
    const steps = 20;
    let prev = trailShape(0);
    for (let i = 1; i <= steps; i += 1) {
      const next = trailShape((TRAIL_PEAK_AT * i) / steps);
      expect(next).toBeGreaterThanOrEqual(prev - 1e-9);
      prev = next;
    }
    prev = trailShape(TRAIL_PEAK_AT);
    for (let i = 1; i <= steps; i += 1) {
      const next = trailShape(TRAIL_PEAK_AT + ((1 - TRAIL_PEAK_AT) * i) / steps);
      expect(next).toBeLessThanOrEqual(prev + 1e-9);
      prev = next;
    }
  });

  it("超出 0–1 的输入被钳住", () => {
    expect(trailShape(-1)).toBeCloseTo(0.5, 5);
    expect(trailShape(2)).toBeCloseTo(0.8, 5);
  });

  it("起点是全局最小值（贴指针最小最淡）", () => {
    expect(trailShape(0)).toBeLessThan(trailShape(1));
    expect(trailShape(0)).toBeLessThan(trailShape(TRAIL_PEAK_AT));
    expect(trailAlpha(0)).toBeLessThan(trailAlpha(1));
  });

  it("透明度同形：起点 0 → 峰值 0.92 → 远端 0.6", () => {
    expect(trailAlpha(0)).toBeCloseTo(0, 5);
    expect(trailAlpha(TRAIL_PEAK_AT)).toBeCloseTo(0.92, 5);
    expect(trailAlpha(1)).toBeCloseTo(0.6, 5);
  });
});

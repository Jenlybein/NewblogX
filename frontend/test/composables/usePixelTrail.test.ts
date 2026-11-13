import { describe, expect, it } from "vitest";

import { resolvePixelTrailOptions } from "~/composables/usePixelTrail";

/**
 * 参数合并的兜底（这次的真实 bug）：
 *
 * 组件只透传了 `cellSize`、`lifeMs` 没给时，曾经把 `lifeMs: undefined` 直接
 * 展开进配置，覆盖成 undefined → 寿命算成 NaN → 一格都不画。表现是"调参完全
 * 没效果"，而不是报错。这条用例把「未给的键一律回落默认值」钉住。
 */
describe("像素尾迹参数合并", () => {
  it("空参数 → 全部走默认值", () => {
    expect(resolvePixelTrailOptions()).toEqual({
      cellSize: 16,
      trailLength: 9,
      lifeMs: 180,
      maxCells: 560,
      spread: 30,
      colorVar: "--color-on-inverse",
    });
  });

  it("显式 undefined 等同于没给（不覆盖默认值）", () => {
    expect(resolvePixelTrailOptions({ cellSize: 24, lifeMs: undefined }).lifeMs).toBe(180);
    expect(resolvePixelTrailOptions({ cellSize: 24 }).cellSize).toBe(24);
  });

  it("显式给的值原样生效", () => {
    const merged = resolvePixelTrailOptions({ cellSize: 20, lifeMs: 300, trailLength: 5 });
    expect(merged.cellSize).toBe(20);
    expect(merged.lifeMs).toBe(300);
    expect(merged.trailLength).toBe(5);
    expect(merged.colorVar).toBe("--color-on-inverse");
  });
});

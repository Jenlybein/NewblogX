import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { findColorLiterals } from "../../../scripts/check-design-tokens.mjs";

import { blockAfter } from "../helpers/designFiles";

/**
 * 沟槽圆点的**视觉**口径（票 295 返工，2026-11-07）。
 *
 * 几何（横向压线 / 纵向居中 / 弧线）由 `utils/outlineDot.ts` 的纯函数与
 * `test/components/OutlineTree.test.ts` 的假布局用例锁住；这里锁 CSS 的三件事：
 *
 *   1. **尺寸**：0.75rem（12px）是 1px 发丝线的 12 倍。8px 的小点在 44px 行高旁读不出
 *      "点压线"（用户返工的原话），所以这里把下限钉在 0.625rem，防止又被调小；
 *   2. **颜色**：只用语义 token，且圆点填 `--color-ink`——不引入新色相（ADR-0012 约束 1）；
 *   3. **叠压关系**：圆点抬到当前行的记号笔底之上，正中留一道 1px 纸色缝，让上下两段
 *      发丝线在视觉上接成一条（"线从圆点里穿过"）；根层没有引导线，不留缝。
 *
 * 位置动画由弹簧驱动 transform，所以这里还断言圆点自己**没有** CSS transition——
 * 两套动画同时作用会互相打架（CSS 会把每一帧的弹簧位置再缓动一次）。
 */

const componentPath = () => join(process.cwd(), "app/components/outline/OutlineTree.vue");
const source = () => readFileSync(componentPath(), "utf8");

/** 圆点的尺寸变量与两块规则。 */
function dotStyles() {
  const css = source();
  return {
    css,
    tree: blockAfter(css, ".outline-tree {"),
    dot: blockAfter(css, ".outline-tree__dot {"),
    seam: blockAfter(css, ".outline-tree__dot::after"),
    rootSeam: blockAfter(css, '.outline-tree__dot[data-outline-dot-depth="0"]::after'),
  };
}

describe("沟槽圆点：尺寸 / 颜色 / 与发丝线的叠压", () => {
  it("圆点直径 0.75rem（12px），且不小于 0.625rem——不做回「线上的毛刺」", () => {
    const { tree } = dotStyles();
    const size = Number(/--outline-dot-size:\s*([\d.]+)rem/u.exec(tree)?.[1]);

    expect(Number.isFinite(size)).toBe(true);
    expect(size).toBeGreaterThanOrEqual(0.625);
    expect(size).toBeLessThanOrEqual(1);
  });

  it("实心墨点：填 --color-ink，不引入新色相，也不吃点击", () => {
    const { dot } = dotStyles();

    expect(dot).toMatch(/background:\s*var\(--color-ink\);/u);
    expect(dot).toMatch(/pointer-events:\s*none;/u);
    expect(dot).toMatch(/border-radius:\s*var\(--radius-pill\);/u);
  });

  it("圆心落在 (x, y)：top / left 取半个圆点尺寸的负偏移", () => {
    const { dot } = dotStyles();

    expect(dot).toMatch(/top:\s*calc\(var\(--outline-dot-size\)\s*\/\s*-2\);/u);
    expect(dot).toMatch(/left:\s*calc\(var\(--outline-dot-size\)\s*\/\s*-2\);/u);
    expect(dot).toMatch(/width:\s*var\(--outline-dot-size\);/u);
    expect(dot).toMatch(/height:\s*var\(--outline-dot-size\);/u);
  });

  it("压在线的上面：z-index 抬到当前行的记号笔底之上", () => {
    const { dot } = dotStyles();
    const zIndex = Number(/z-index:\s*(-?\d+);/u.exec(dot)?.[1]);

    expect(zIndex).toBeGreaterThanOrEqual(1);
  });

  it("线从圆点里穿过：正中 1px 纸色缝，与发丝线同一像素列", () => {
    const { dot, seam } = dotStyles();

    expect(dot).toMatch(/overflow:\s*hidden;/u);
    expect(seam).toMatch(/top:\s*0;/u);
    expect(seam).toMatch(/bottom:\s*0;/u);
    // 圆心落在线的左沿（`x = depth × 1.5rem − 0.75rem`），所以缝取 50% 才与线同列。
    expect(seam).toMatch(/left:\s*50%;/u);
    expect(seam).toMatch(/width:\s*1px;/u);
    expect(seam).toMatch(/background:\s*var\(--color-surface\);/u);
  });

  it("根层没有引导线：depth 0 不留缝（否则像被切开的点）", () => {
    const { rootSeam } = dotStyles();

    expect(rootSeam).toMatch(/content:\s*none;/u);
  });

  it("位置动画只归弹簧：圆点自己不许写 CSS transition / animation", () => {
    const { dot, seam } = dotStyles();

    expect(dot).not.toMatch(/(?:^|[\s;])(?:transition|animation)\s*:/u);
    expect(seam).not.toMatch(/(?:^|[\s;])(?:transition|animation)\s*:/u);
  });

  it("行高与行距仍是被测坐标的前提（假布局用 44 / 48）", () => {
    const { css } = dotStyles();

    expect(blockAfter(css, ".outline-tree__list {")).toMatch(/gap:\s*0\.25rem;/u);
    expect(css).toMatch(/min-height:\s*2\.75rem;/u);
  });

  it("整个组件没有裸色值（颜色只从语义 token 来）", () => {
    expect(findColorLiterals(source())).toEqual([]);
  });
});

import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { SPRING_NECK } from "~/utils/motion";
import {
  NECK_CLOSED_GAP,
  NECK_RADIUS_ACTIVE_PX,
  NECK_RADIUS_IDLE_PX,
  NECK_SEPARATION_PX,
  isNeckSeamOpen,
  neckGradientStops,
  neckMotionKind,
  neckPath,
  neckSegmentGap,
  neckSegmentMotion,
  neckTransition,
} from "~/utils/subnavNeck";

/**
 * 分段条颈部（术语见 GLOSSARY「颈部」，口径见 grill 记录 §三 3.7 / §三 3.8）。
 *
 * 这里测的是**纯几何与纯接线**：颈部路径怎么画、哪两条接缝该张开、段位与圆角到位时
 * 是什么值、弹簧参数从哪来。视觉像素一律不断言（ADR-0012 已否决截图比对）。
 */

/** 路径里的全部数字（按出现顺序，坐标成对出现）。 */
function numbersIn(path: string): number[] {
  return [...path.matchAll(/-?\d+(?:\.\d+)?/gu)].map((match) => Number(match[0]));
}

describe("neckPath：颈部路径是纯函数", () => {
  it("间隙为 0 / 负数 / 非有限值时返回空串（不产生 NaN 坐标）", () => {
    for (const gap of [0, -1, -12.5, Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY]) {
      expect(neckPath(gap, 100), `gap=${gap}`).toBe("");
    }
  });

  it("高度为 0 / 负数 / 非有限值时返回空串", () => {
    for (const span of [0, -1, Number.NaN, Number.POSITIVE_INFINITY]) {
      expect(neckPath(NECK_SEPARATION_PX, span), `span=${span}`).toBe("");
    }
  });

  it("合法输入给出封闭路径，坐标全部有限且落在 [0, gap] × [0, span] 内", () => {
    const path = neckPath(NECK_SEPARATION_PX, 100);

    expect(path.startsWith("M0 0")).toBe(true);
    expect(path.endsWith("Z")).toBe(true);
    expect(path).not.toMatch(/NaN|Infinity|undefined/u);

    const numbers = numbersIn(path);
    expect(numbers.length % 2).toBe(0);
    for (const value of numbers) expect(Number.isFinite(value)).toBe(true);

    const xs = numbers.filter((_, index) => index % 2 === 0);
    const ys = numbers.filter((_, index) => index % 2 === 1);
    expect(Math.min(...xs)).toBe(0);
    expect(Math.max(...xs)).toBe(NECK_SEPARATION_PX);
    expect(Math.min(...ys)).toBe(0);
    expect(Math.max(...ys)).toBe(100);
  });

  it("两侧各是一条凹曲线：控制点向颈部中轴内收，中段收腰到间隙的一半", () => {
    const gap = 12;
    const numbers = numbersIn(neckPath(gap, 100));

    // 结构：M(0,0) → C(b,1/3) (b,2/3) (0,span) → L(gap,span) → C(镜像) → Z
    expect(numbers).toHaveLength(16);
    const [bowLeft, bowRight] = [numbers[2]!, numbers[12]!];

    // 向内收：控制点严格落在两条边之间
    expect(bowLeft).toBeGreaterThan(0);
    expect(bowLeft).toBeLessThan(gap / 2);
    // 左右镜像：右曲线的控制点距右边缘同样是 bowLeft
    expect(bowRight).toBeCloseTo(gap - bowLeft, 5);
    // 三次贝塞尔在两控制点同 x 时，t=0.5 的横坐标是 0.75×控制点偏移 → 收腰 = gap/2
    expect(0.75 * bowLeft).toBeCloseTo(gap / 4, 5);
  });

  it("同一组输入永远给同一条路径（纯函数，无隐藏状态）", () => {
    expect(neckPath(12, 100)).toBe(neckPath(12, 100));
    expect(neckPath(12, 100)).not.toBe(neckPath(12, 120));
  });
});

describe("接缝：只有激活段两侧张开", () => {
  it("每段拿的是它左侧接缝的间隙；第一段没有左接缝", () => {
    expect(neckSegmentGap(0, 2)).toBe(0);
    expect(neckSegmentGap(0, 0)).toBe(0);
  });

  it("激活段与它右侧一段的接缝张开，其余接缝闭合（重叠 1px 消除发丝缝）", () => {
    const gaps = [0, 1, 2, 3, 4].map((index) => neckSegmentGap(index, 2));

    expect(gaps).toEqual([0, NECK_CLOSED_GAP, NECK_SEPARATION_PX, NECK_SEPARATION_PX, NECK_CLOSED_GAP]);
  });

  it("首段激活时只有一条接缝张开；末段激活时同理", () => {
    expect([0, 1, 2, 3, 4].map((index) => neckSegmentGap(index, 0))).toEqual([
      0,
      NECK_SEPARATION_PX,
      NECK_CLOSED_GAP,
      NECK_CLOSED_GAP,
      NECK_CLOSED_GAP,
    ]);
    expect([0, 1, 2, 3, 4].map((index) => neckSegmentGap(index, 4))).toEqual([
      0,
      NECK_CLOSED_GAP,
      NECK_CLOSED_GAP,
      NECK_CLOSED_GAP,
      NECK_SEPARATION_PX,
    ]);
  });

  it("闭合接缝的间隙是负的 1px（贴合，不留发丝缝）", () => {
    expect(NECK_CLOSED_GAP).toBe(-1);
    expect(isNeckSeamOpen(1, 2)).toBe(false);
    expect(isNeckSeamOpen(2, 2)).toBe(true);
    expect(isNeckSeamOpen(3, 2)).toBe(true);
  });

  it("找不到激活段（activeIndex 越界）时全部闭合，不抛错", () => {
    expect([0, 1, 2, 3, 4].map((index) => neckSegmentGap(index, -1))).toEqual([
      0,
      NECK_CLOSED_GAP,
      NECK_CLOSED_GAP,
      NECK_CLOSED_GAP,
      NECK_CLOSED_GAP,
    ]);
  });
});

describe("段位与圆角：到位后的值 + 弹簧来源", () => {
  it("激活块抬高到张开量、圆角涨到圆角阶梯的上一档；闭合段落在重叠位", () => {
    expect(neckSegmentMotion(2, 2)).toEqual({
      marginLeft: `${NECK_SEPARATION_PX}px`,
      borderRadius: `${NECK_RADIUS_ACTIVE_PX}px`,
      "--neck-gap": `${NECK_SEPARATION_PX}px`,
    });
    expect(neckSegmentMotion(1, 2)).toEqual({
      marginLeft: `${NECK_CLOSED_GAP}px`,
      borderRadius: `${NECK_RADIUS_IDLE_PX}px`,
      "--neck-gap": "0px",
    });
  });

  it("被拉开的邻段只是让位，不涨圆角（圆角只跟激活走）", () => {
    expect(neckSegmentMotion(3, 2).borderRadius).toBe(`${NECK_RADIUS_IDLE_PX}px`);
    expect(neckSegmentMotion(0, 0).borderRadius).toBe(`${NECK_RADIUS_ACTIVE_PX}px`);
    expect(neckSegmentMotion(0, 0).marginLeft).toBe("0px");
  });

  it("弹簧参数只来自 motion.ts 的 SPRING_NECK", () => {
    expect(neckTransition(false)).toEqual({ type: "spring", ...SPRING_NECK });
    expect(neckMotionKind(false)).toBe("spring");
  });

  it("减少动态下是落位（时长 0），不是放慢的弹簧", () => {
    expect(neckTransition(true)).toEqual({ type: "tween", duration: 0, delay: 0 });
    expect(neckMotionKind(true)).toBe("jump");
  });

  it("圆角档位与 tokens.css 的圆角阶梯同源（8 / 14 两档）", () => {
    const tokens = readFileSync(join(process.cwd(), "app/assets/css/tokens.css"), "utf8");

    expect(tokens).toContain(`--radius-sm: ${NECK_RADIUS_IDLE_PX}px`);
    expect(tokens).toContain(`--radius-md: ${NECK_RADIUS_ACTIVE_PX}px`);
  });
});

describe("颈部渐变：两端分别取左右相邻段的填充色", () => {
  const activeFill = "var(--color-surface-inverse)";
  const idleFill = "var(--color-surface)";

  it("左邻是激活段 → 从深墨渐变到纸面", () => {
    expect(neckGradientStops(2, 1)).toEqual({ from: activeFill, to: idleFill });
  });

  it("右邻是激活段 → 从纸面渐变到深墨", () => {
    expect(neckGradientStops(2, 2)).toEqual({ from: idleFill, to: activeFill });
  });

  it("两个端点都只消费语义 token（没有裸色值）", () => {
    for (const stops of [neckGradientStops(1, 0), neckGradientStops(1, 1)]) {
      expect(stops.from).toMatch(/^var\(--color-[a-z-]+\)$/u);
      expect(stops.to).toMatch(/^var\(--color-[a-z-]+\)$/u);
    }
  });
});

import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { SPRING_NECK } from "~/utils/motion";
import {
  NECK_CLOSED_GAP,
  NECK_CORNER_PX,
  NECK_NOMINAL_SPAN,
  NECK_SEPARATION_PX,
  NECK_VIEW_BOX,
  NECK_WAIST,
  isNeckSeamOpen,
  neckGradientStops,
  neckMotionKind,
  neckPath,
  neckSeamCorner,
  neckSegmentGap,
  neckSegmentMotion,
  neckTransition,
} from "~/utils/subnavNeck";

/**
 * 分段条颈部（术语见 GLOSSARY「颈部」，口径见 grill 记录 §三 3.7 / §三 3.8）。
 *
 * 这里测的是**纯几何与纯接线**：颈部路径怎么画、哪两条接缝该张开、圆角给在哪一侧、
 * 段位到位时是什么值、弹簧参数从哪来。视觉像素一律不断言（ADR-0012 已否决截图比对）。
 */

/** 路径里的全部数字（按出现顺序，坐标成对出现）。 */
function numbersIn(path: string): number[] {
  return [...path.matchAll(/-?\d+(?:\.\d+)?/gu)].map((match) => Number(match[0]));
}

describe("neckPath：颈部路径是纯函数", () => {
  it("腰宽为 0 / 负数 / 不小于标称高度时返回空串（不产生非法坐标）", () => {
    for (const waist of [0, -1, -28, NECK_NOMINAL_SPAN, NECK_NOMINAL_SPAN + 10, Number.NaN]) {
      expect(neckPath(waist, NECK_NOMINAL_SPAN), `waist=${waist}`).toBe("");
    }
  });

  it("标称高度为 0 / 负数 / 非有限值时返回空串", () => {
    for (const span of [0, -1, Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY]) {
      expect(neckPath(NECK_WAIST, span), `span=${span}`).toBe("");
    }
  });

  it("合法输入给出封闭路径，坐标全部有限且落在标称方框内", () => {
    const path = neckPath(NECK_WAIST, NECK_NOMINAL_SPAN);

    expect(path.startsWith("M0 0")).toBe(true);
    expect(path.endsWith("Z")).toBe(true);
    expect(path).not.toMatch(/NaN|Infinity|undefined/u);

    const numbers = numbersIn(path);
    expect(numbers.length % 2).toBe(0);
    for (const value of numbers) expect(Number.isFinite(value)).toBe(true);

    const xs = numbers.filter((_, index) => index % 2 === 0);
    const ys = numbers.filter((_, index) => index % 2 === 1);
    expect(Math.min(...xs)).toBe(0);
    expect(Math.max(...xs)).toBe(NECK_NOMINAL_SPAN);
    expect(Math.min(...ys)).toBe(0);
    expect(Math.max(...ys)).toBe(NECK_NOMINAL_SPAN);
  });

  it("两端满高、中点收腰：上下两条凹曲线都往中轴收，腰部厚度等于 waist", () => {
    const path = neckPath(NECK_WAIST, NECK_NOMINAL_SPAN);
    const numbers = numbersIn(path);

    // 结构：M(0,0) → Q(mid, span-waist) (span,0) → L(span,span) → Q(mid, waist) (0,span) → Z
    expect(numbers).toHaveLength(12);
    const [topMidX, topMidY, bottomMidX, bottomMidY] = [
      numbers[2]!,
      numbers[3]!,
      numbers[8]!,
      numbers[9]!,
    ];

    // 两个控制点都落在中轴
    expect(topMidX).toBeCloseTo(NECK_NOMINAL_SPAN / 2, 5);
    expect(bottomMidX).toBeCloseTo(NECK_NOMINAL_SPAN / 2, 5);
    // 上边界被压到 span-waist、下边界抬到 waist，于是中点的厚度正好是 waist
    expect(topMidY).toBeCloseTo(NECK_NOMINAL_SPAN - NECK_WAIST, 5);
    expect(bottomMidY).toBeCloseTo(NECK_WAIST, 5);
    expect(NECK_NOMINAL_SPAN - topMidY).toBeCloseTo(bottomMidY, 5);
  });

  it("两端是满高（贴住相邻段的上／下沿），不是从半高起步", () => {
    const numbers = numbersIn(neckPath(NECK_WAIST, NECK_NOMINAL_SPAN));
    // 起点 (0,0) 与终点的镜像 (0,span) 都在整条高度上
    expect(numbers.slice(0, 2)).toEqual([0, 0]);
    expect(numbers.slice(-2)).toEqual([0, NECK_NOMINAL_SPAN]);
  });

  it("同一组输入永远给同一条路径（纯函数，无隐藏状态）", () => {
    expect(neckPath(28, 100)).toBe(neckPath(28, 100));
    expect(neckPath(28, 100)).not.toBe(neckPath(28, 120));
  });

  it("viewBox 用同一对标称值，横向与纵向都靠 CSS 拉伸", () => {
    expect(NECK_VIEW_BOX).toBe(`0 0 ${NECK_NOMINAL_SPAN} ${NECK_NOMINAL_SPAN}`);
  });
});

describe("接缝：只有激活段两侧张开", () => {
  it("每段拿的是它左侧接缝的间隙；第一段没有左接缝", () => {
    expect(neckSegmentGap(0, 2)).toBe(0);
    expect(neckSegmentGap(0, 0)).toBe(0);
  });

  it("张开量取 md 档的 20px（参考组件的 separation）", () => {
    expect(NECK_SEPARATION_PX).toBe(20);
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

describe("圆角：只给敞口的一侧", () => {
  const count = 5;

  it("控件两端永远敞口（圆的）", () => {
    expect(neckSeamCorner(0, 2, count)).toBe(NECK_CORNER_PX);
    expect(neckSeamCorner(count, 2, count)).toBe(NECK_CORNER_PX);
    expect(NECK_CORNER_PX).toBe(14);
  });

  it("张开的接缝旁是圆的，闭合的接缝旁恒为 0（否则贴合处会啃出缺口）", () => {
    expect(neckSeamCorner(1, 2, count)).toBe(0);
    expect(neckSeamCorner(2, 2, count)).toBe(NECK_CORNER_PX);
    expect(neckSeamCorner(3, 2, count)).toBe(NECK_CORNER_PX);
    expect(neckSeamCorner(4, 2, count)).toBe(0);
  });
});

describe("段位与圆角：到位后的值 + 弹簧来源", () => {
  const count = 5;

  it("激活段让开张开量、四角都是敞口圆角；闭合段压在重叠位、贴合侧不带圆角", () => {
    expect(neckSegmentMotion(2, 2, count)).toEqual({
      marginLeft: `${NECK_SEPARATION_PX}px`,
      borderTopLeftRadius: `${NECK_CORNER_PX}px`,
      borderTopRightRadius: `${NECK_CORNER_PX}px`,
      borderBottomRightRadius: `${NECK_CORNER_PX}px`,
      borderBottomLeftRadius: `${NECK_CORNER_PX}px`,
      "--neck-gap": `${NECK_SEPARATION_PX}px`,
    });

    expect(neckSegmentMotion(1, 2, count)).toEqual({
      marginLeft: `${NECK_CLOSED_GAP}px`,
      // 左接缝闭合（0）、右接缝张开（圆）—— 贴合一侧必须方，不然相邻两段的圆角会互相啃
      borderTopLeftRadius: "0px",
      borderBottomLeftRadius: "0px",
      borderTopRightRadius: `${NECK_CORNER_PX}px`,
      borderBottomRightRadius: `${NECK_CORNER_PX}px`,
      "--neck-gap": "0px",
    });
  });

  it("首段没有左接缝：贴在第 0 位、左角仍是敞口圆角（控件的那一头）", () => {
    const first = neckSegmentMotion(0, 2, count);

    expect(first.marginLeft).toBe("0px");
    expect(first.borderTopLeftRadius).toBe(`${NECK_CORNER_PX}px`);
    expect(first.borderTopRightRadius).toBe("0px");
  });

  it("闭合接缝的颈部宽度是 0（不画，也不留缝）", () => {
    expect(neckSegmentMotion(4, 2, count)["--neck-gap"]).toBe("0px");
    expect(neckSegmentMotion(4, 2, count).marginLeft).toBe(`${NECK_CLOSED_GAP}px`);
  });

  it("弹簧参数只来自 motion.ts 的 SPRING_NECK", () => {
    expect(neckTransition(false)).toEqual({ type: "spring", ...SPRING_NECK });
    expect(neckMotionKind(false)).toBe("spring");
  });

  it("减少动态下是落位（时长 0），不是放慢的弹簧", () => {
    expect(neckTransition(true)).toEqual({ type: "tween", duration: 0, delay: 0 });
    expect(neckMotionKind(true)).toBe("jump");
  });

  it("敞口圆角与 tokens.css 的圆角阶梯同源（--radius-md）", () => {
    const tokens = readFileSync(join(process.cwd(), "app/assets/css/tokens.css"), "utf8");

    expect(tokens).toContain(`--radius-md: ${NECK_CORNER_PX}px`);
  });
});

describe("颈部渐变：两端分别取左右相邻段的填充色", () => {
  const activeFill = "var(--color-surface-inverse)";
  const idleFill = "var(--color-surface-raised)";

  it("左邻是激活段 → 从深墨渐变到轨道面", () => {
    expect(neckGradientStops(2, 1)).toEqual({ from: activeFill, to: idleFill });
  });

  it("右邻是激活段 → 从轨道面渐变到深墨", () => {
    expect(neckGradientStops(2, 2)).toEqual({ from: idleFill, to: activeFill });
  });

  it("闭合接缝两侧都不是激活块 → 两端同为轨道面（连上时不闪第二种颜色）", () => {
    expect(neckGradientStops(1, 2)).toEqual({ from: idleFill, to: idleFill });
    expect(neckGradientStops(4, 2)).toEqual({ from: idleFill, to: idleFill });
  });

  it("两个端点都只消费语义 token（没有裸色值）", () => {
    for (const stops of [neckGradientStops(1, 0), neckGradientStops(1, 1)]) {
      expect(stops.from).toMatch(/^var\(--color-[a-z-]+\)$/u);
      expect(stops.to).toMatch(/^var\(--color-[a-z-]+\)$/u);
    }
  });
});

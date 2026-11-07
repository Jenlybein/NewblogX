import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { SPRING_NECK } from "~/utils/motion";
import {
  SEAM_CLOSED_GAP,
  SEAM_CORNER_PX,
  SEAM_OPEN_PX,
  isSeamOpen,
  seamCorner,
  seamGap,
  seamMotionKind,
  seamSegmentMotion,
  seamTransition,
} from "~/utils/subnavNeck";

/**
 * 分段条接缝（口径见 grill 记录 §三 3.7 / §三 3.8）。
 *
 * 这里测的是**纯几何与纯接线**：哪两条接缝该张开、张开多少、圆角给在哪一侧、
 * 段位到位时是什么值、弹簧参数从哪来。视觉像素一律不断言（ADR-0012 已否决截图比对）。
 *
 * 颈部已按审稿意见整体退役，所以这里没有"路径怎么画""渐变端点取哪支"这一类断言 ——
 * 留下的全部是让五段贴在一条线上、且只有激活块自己张开的那点几何。
 */

describe("接缝：只有激活段两侧张开", () => {
  it("每段拿的是它左侧接缝的间隙；第一段没有左接缝", () => {
    expect(seamGap(0, 2)).toBe(0);
    expect(seamGap(0, 0)).toBe(0);
  });

  it("张开量是 10px：去掉颈部后留成干净的间隙，不是一道被掰开的缝", () => {
    expect(SEAM_OPEN_PX).toBe(10);
  });

  it("激活段与它右侧一段的接缝张开，其余接缝闭合（重叠 1px 消除发丝缝）", () => {
    const gaps = [0, 1, 2, 3, 4].map((index) => seamGap(index, 2));

    expect(gaps).toEqual([0, SEAM_CLOSED_GAP, SEAM_OPEN_PX, SEAM_OPEN_PX, SEAM_CLOSED_GAP]);
  });

  it("首段激活时只有一条接缝张开；末段激活时同理", () => {
    expect([0, 1, 2, 3, 4].map((index) => seamGap(index, 0))).toEqual([
      0,
      SEAM_OPEN_PX,
      SEAM_CLOSED_GAP,
      SEAM_CLOSED_GAP,
      SEAM_CLOSED_GAP,
    ]);
    expect([0, 1, 2, 3, 4].map((index) => seamGap(index, 4))).toEqual([
      0,
      SEAM_CLOSED_GAP,
      SEAM_CLOSED_GAP,
      SEAM_CLOSED_GAP,
      SEAM_OPEN_PX,
    ]);
  });

  it("闭合接缝的间隙是负的 1px（贴合，不留发丝缝）", () => {
    expect(SEAM_CLOSED_GAP).toBe(-1);
    expect(isSeamOpen(1, 2)).toBe(false);
    expect(isSeamOpen(2, 2)).toBe(true);
    expect(isSeamOpen(3, 2)).toBe(true);
  });

  it("找不到激活段（activeIndex 越界）时全部闭合，不抛错", () => {
    expect([0, 1, 2, 3, 4].map((index) => seamGap(index, -1))).toEqual([
      0,
      SEAM_CLOSED_GAP,
      SEAM_CLOSED_GAP,
      SEAM_CLOSED_GAP,
      SEAM_CLOSED_GAP,
    ]);
  });
});

describe("圆角：只给敞口的一侧", () => {
  const count = 5;

  it("控件两端永远敞口（圆的）", () => {
    expect(seamCorner(0, 2, count)).toBe(SEAM_CORNER_PX);
    expect(seamCorner(count, 2, count)).toBe(SEAM_CORNER_PX);
    expect(SEAM_CORNER_PX).toBe(14);
  });

  it("张开的接缝旁是圆的，闭合的接缝旁恒为 0（否则贴合处会啃出缺口）", () => {
    expect(seamCorner(1, 2, count)).toBe(0);
    expect(seamCorner(2, 2, count)).toBe(SEAM_CORNER_PX);
    expect(seamCorner(3, 2, count)).toBe(SEAM_CORNER_PX);
    expect(seamCorner(4, 2, count)).toBe(0);
  });
});

describe("段位与圆角：到位后的值 + 弹簧来源", () => {
  const count = 5;

  it("激活块四角都是敞口圆角、左右各让开张开量；闭合段压在重叠位、贴合侧不带圆角", () => {
    expect(seamSegmentMotion(2, 2, count)).toEqual({
      borderTopLeftRadius: `${SEAM_CORNER_PX}px`,
      borderTopRightRadius: `${SEAM_CORNER_PX}px`,
      borderBottomRightRadius: `${SEAM_CORNER_PX}px`,
      borderBottomLeftRadius: `${SEAM_CORNER_PX}px`,
      "--seam-gap": `${SEAM_OPEN_PX}px`,
    });

    expect(seamSegmentMotion(1, 2, count)).toEqual({
      // 左接缝闭合（0）、右接缝张开（圆）—— 贴合一侧必须方，不然相邻两段的圆角会互相啃
      borderTopLeftRadius: "0px",
      borderBottomLeftRadius: "0px",
      borderTopRightRadius: `${SEAM_CORNER_PX}px`,
      borderBottomRightRadius: `${SEAM_CORNER_PX}px`,
      "--seam-gap": `${SEAM_CLOSED_GAP}px`,
    });
  });

  it("只有一份间隙量：`--seam-gap` 保留负值，段位在 CSS 里从它派生（不各写一份 margin-left）", () => {
    // 负值必须原样留着：段位要的就是 -1px 的重叠。
    expect(seamSegmentMotion(1, 2, count)["--seam-gap"]).toBe(`${SEAM_CLOSED_GAP}px`);
    expect(seamSegmentMotion(1, 2, count)).not.toHaveProperty("marginLeft");
  });

  it("首段没有左接缝：贴在第 0 位、左角仍是敞口圆角（控件的那一头）", () => {
    const first = seamSegmentMotion(0, 2, count);

    expect(first["--seam-gap"]).toBe("0px");
    expect(first.borderTopLeftRadius).toBe(`${SEAM_CORNER_PX}px`);
    expect(first.borderTopRightRadius).toBe("0px");
  });

  it("闭合接缝没有间隙（段位重叠 1px）", () => {
    expect(seamSegmentMotion(4, 2, count)["--seam-gap"]).toBe(`${SEAM_CLOSED_GAP}px`);
  });

  it("弹簧参数只来自 motion.ts 的 SPRING_NECK", () => {
    expect(seamTransition(false)).toEqual({ type: "spring", ...SPRING_NECK });
    expect(seamMotionKind(false)).toBe("spring");
  });

  it("减少动态下是落位（时长 0），不是放慢的弹簧", () => {
    expect(seamTransition(true)).toEqual({ type: "tween", duration: 0, delay: 0 });
    expect(seamMotionKind(true)).toBe("jump");
  });

  it("敞口圆角与 tokens.css 的圆角阶梯同源（--radius-md）", () => {
    const tokens = readFileSync(join(process.cwd(), "app/assets/css/tokens.css"), "utf8");

    expect(tokens).toContain(`--radius-md: ${SEAM_CORNER_PX}px`);
  });
});

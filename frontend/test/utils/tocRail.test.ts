import { describe, expect, it } from "vitest";

import {
  TOC_RAIL_BASE_X_PX,
  TOC_RAIL_INDENT_PX,
  TOC_RAIL_PAD_TOP_PX,
  TOC_RAIL_ROW_PX,
  TOC_RAIL_WIDTH_PX,
  buildTocRailNodes,
  buildTocRailPath,
  tocRailAngleAtLength,
  tocRailLengthAtY,
  tocRailReadFraction,
} from "~/utils/tocRail";

/**
 * 标题大纲轨道的几何（纯函数，票 296）。
 *
 * 这里断言的是**输入输出**：给标题层级与「测到的行中心 y」，拿到节点坐标、路径、
 * 按 y 求长度、切线角度。不断言像素级观感（渲染长什么样由人眼验收），也不比对
 * 整条 path 字符串 —— 那是实现细节，唯一例外是「起笔点」这种协议值。
 */

const headings = [
  { id: "a", level: 2 },
  { id: "b", level: 3 },
  { id: "c", level: 2 },
];

describe("buildTocRailNodes：层级定缩进，测量定 y", () => {
  it("按层级缩进：h2 顶格、h3 一级、h4+ 两级（每级 14px）", () => {
    const nodes = buildTocRailNodes(
      [
        { id: "a", level: 2 },
        { id: "b", level: 3 },
        { id: "c", level: 4 },
        { id: "d", level: 5 },
      ],
      [20, 60, 100, 140],
    );

    expect(nodes.map((node) => node.x)).toEqual([
      TOC_RAIL_BASE_X_PX,
      TOC_RAIL_BASE_X_PX + TOC_RAIL_INDENT_PX,
      TOC_RAIL_BASE_X_PX + 2 * TOC_RAIL_INDENT_PX,
      TOC_RAIL_BASE_X_PX + 2 * TOC_RAIL_INDENT_PX,
    ]);
    // 测得准就直接用测得的值，不做二次加工。
    expect(nodes.map((node) => node.y)).toEqual([20, 60, 100, 140]);
    expect(nodes.map((node) => node.id)).toEqual(["a", "b", "c", "d"]);
  });

  it("没有测量（SSR / 首帧）时按等距兜底，保证首屏轨道不是一坨叠在一起的点", () => {
    const nodes = buildTocRailNodes(headings);

    expect(nodes.map((node) => node.y)).toEqual([
      TOC_RAIL_PAD_TOP_PX,
      TOC_RAIL_PAD_TOP_PX + TOC_RAIL_ROW_PX,
      TOC_RAIL_PAD_TOP_PX + 2 * TOC_RAIL_ROW_PX,
    ]);
  });

  it("测量退化（全是 0 / 不递增 / 条数对不上）时也走等距兜底", () => {
    const fallback = [TOC_RAIL_PAD_TOP_PX, TOC_RAIL_PAD_TOP_PX + TOC_RAIL_ROW_PX];

    for (const rows of [
      [0, 0],
      [40, 40],
      [80, 40],
      [30],
      [Number.NaN, 40],
      [],
    ]) {
      const nodes = buildTocRailNodes(
        [
          { id: "a", level: 2 },
          { id: "b", level: 2 },
        ],
        rows,
      );
      expect(nodes.map((node) => node.y), `${JSON.stringify(rows)}`).toEqual(fallback);
    }
  });

  it("空标题 → 空节点表（不抛异常、不造出幽灵节点）", () => {
    expect(buildTocRailNodes([], [])).toEqual([]);
    expect(buildTocRailNodes([], [12, 40])).toEqual([]);
  });
});

describe("buildTocRailPath：只向下走的折线", () => {
  it("起点在第一个节点，拐点落在每个节点上，节点数不变", () => {
    const path = buildTocRailPath(buildTocRailNodes(headings, [20, 56, 100]));

    expect(path.nodes).toHaveLength(3);
    expect(path.d.startsWith(`M ${TOC_RAIL_BASE_X_PX} 20`)).toBe(true);
    expect(path.d.match(/L/gu)).toHaveLength(2);
    // 中段与末段终点都落在节点上（只判断坐标出现，不比对整串）。
    expect(path.d).toContain(`${TOC_RAIL_BASE_X_PX + TOC_RAIL_INDENT_PX} 56`);
    expect(path.d.endsWith(`${TOC_RAIL_BASE_X_PX} 100`)).toBe(true);
  });

  it("长度是各段直线长度之和（两段折线 ≈ 38.63 + 46.17）", () => {
    const path = buildTocRailPath(buildTocRailNodes(headings, [20, 56, 100]));

    // 手算：√(14² + 36²) + √((-14)² + 44²) = 38.626… + 46.174…
    expect(path.totalLength).toBeCloseTo(84.8, 1);
    expect(path.segments).toHaveLength(2);
  });

  it("svg 画布：宽度固定、高度够装下最后一个节点（含底部留白）", () => {
    const path = buildTocRailPath(buildTocRailNodes(headings, [20, 56, 100]));

    expect(path.width).toBe(TOC_RAIL_WIDTH_PX);
    expect(path.height).toBeGreaterThan(100);
  });

  it("单层一个标题：没有线段，路径只有一个起笔点", () => {
    const path = buildTocRailPath(buildTocRailNodes([{ id: "only", level: 2 }], [20]));

    expect(path.segments).toEqual([]);
    expect(path.totalLength).toBe(0);
    expect(path.d).toBe(`M ${TOC_RAIL_BASE_X_PX} 20`);
  });

  it("空标题 → 空路径（d 为空串，长度为 0）", () => {
    const path = buildTocRailPath([]);

    expect(path.d).toBe("");
    expect(path.totalLength).toBe(0);
    expect(path.segments).toEqual([]);
  });
});

describe("tocRailLengthAtY：按 y 求已经走过多少", () => {
  const path = buildTocRailPath(buildTocRailNodes(headings, [20, 56, 100]));

  it("起点处为 0，第一个拐点处等于第一段长度", () => {
    expect(tocRailLengthAtY(path, 20)).toBe(0);
    expect(tocRailLengthAtY(path, 56)).toBeCloseTo(38.63, 1);
  });

  it("末节点之后钳到全长，起点之上钳到 0（滚动越界不会算出负长度）", () => {
    expect(tocRailLengthAtY(path, 9999)).toBeCloseTo(path.totalLength, 5);
    expect(tocRailLengthAtY(path, -100)).toBe(0);
  });

  it("读进度是 0..1 的比例：末节点为 1，中途在 0 和 1 之间且单调递增", () => {
    expect(tocRailReadFraction(path, 100)).toBe(1);
    expect(tocRailReadFraction(path, 20)).toBe(0);
    expect(tocRailReadFraction(path, 78)).toBeGreaterThan(0);
    expect(tocRailReadFraction(path, 78)).toBeLessThan(1);
    expect(tocRailReadFraction(path, 78)).toBeGreaterThan(tocRailReadFraction(path, 40));
  });

  it("零长路径（只有一个标题）不打 NaN：比例按 0 收口", () => {
    const single = buildTocRailPath(buildTocRailNodes([{ id: "only", level: 2 }], [20]));

    expect(tocRailReadFraction(single, 20)).toBe(0);
  });
});

describe("tocRailAngleAtLength：飞机朝向", () => {
  const path = buildTocRailPath(buildTocRailNodes(headings, [20, 56, 100]));

  it("切线角是沿轨道往下的度数：右下段 69°，左下段 108°", () => {
    // 手算：atan2(36, 14) = 68.75°；atan2(44, -14) = 107.65°
    expect(Math.round(tocRailAngleAtLength(path, 10))).toBe(69);
    expect(Math.round(tocRailAngleAtLength(path, 60))).toBe(108);
  });

  it("超界长度钳在两端（弹簧过冲也不会让机头朝向跳档）", () => {
    expect(Math.round(tocRailAngleAtLength(path, -50))).toBe(69);
    expect(Math.round(tocRailAngleAtLength(path, 10_000))).toBe(108);
  });

  it("零长路径（只有一个标题）没有线段：按「向下」给角度，不返回 NaN", () => {
    const single = buildTocRailPath(buildTocRailNodes([{ id: "only", level: 2 }], [20]));

    expect(tocRailAngleAtLength(single, 0)).toBe(90);
  });
});

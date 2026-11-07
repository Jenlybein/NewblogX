import { describe, expect, it } from "vitest";

import {
  TOC_RAIL_BASE_X_PX,
  TOC_RAIL_BEND_RATIO,
  TOC_RAIL_FIRST_ROW_CENTER_PX,
  TOC_RAIL_INDENT_PX,
  TOC_RAIL_LABEL_GAP_PX,
  TOC_RAIL_ROW_PX,
  TOC_RAIL_WIDTH_PX,
  buildTocRailNodes,
  buildTocRailPath,
  tocRailAngleAtLength,
  tocRailLabelInsetPx,
  tocRailLengthAtY,
  tocRailNodeX,
  tocRailPlaneRotationDeg,
  tocRailPointAtLength,
  tocRailReadFraction,
  tocRailRowCenterPx,
} from "~/utils/tocRail";

/**
 * 标题大纲轨道的几何（纯函数，票 296 返工版）。
 *
 * 这里断言的是**输入输出**：给标题层级与「测到的行中心 y」，拿到节点坐标、折线
 * 路径、按 y 求长度、按长度求点/切线/机身角度。不断言像素级观感（渲染长什么样由
 * 人眼验收），也不比对整条 path 字符串 —— 那是实现细节，例外是「起笔点」「折点」
 * 这类协议值。
 *
 * 返工的核心：上一版相邻节点直接连线，等于每个节点都在自己那一小段上，整条轨道
 * 并没有"随层级移动"的形状。这一版每段横坐标不同的相邻节点之间插两个折点（跨度
 * 的 30% / 70%），所以一段 = 竖 + 斜 + 竖，整条轨道是**折线**。
 */

const headings = [
  { id: "a", level: 2 },
  { id: "b", level: 3 },
  { id: "c", level: 2 },
];

describe("buildTocRailNodes：层级定缩进，测量定 y", () => {
  it("按层级缩进：h2 顶格、h3 一级、h4+ 两级（每级 14px，起点 5）", () => {
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

  it("没有测量（SSR / 首帧）时按等距兜底，且兜底就落在真实行中心上", () => {
    const nodes = buildTocRailNodes(headings);

    expect(nodes.map((node) => node.y)).toEqual([
      TOC_RAIL_FIRST_ROW_CENTER_PX,
      TOC_RAIL_FIRST_ROW_CENTER_PX + TOC_RAIL_ROW_PX,
      TOC_RAIL_FIRST_ROW_CENTER_PX + 2 * TOC_RAIL_ROW_PX,
    ]);
  });

  it("测量退化（全是 0 / 不递增 / 条数对不上）时也走等距兜底", () => {
    const fallback = [
      TOC_RAIL_FIRST_ROW_CENTER_PX,
      TOC_RAIL_FIRST_ROW_CENTER_PX + TOC_RAIL_ROW_PX,
    ];

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

describe("对齐算式：节点落在行中心，文字 = 自己的节点 + 固定间距", () => {
  it("节点 y 就是行盒的垂直中心（不是行顶、不是行底）", () => {
    // 行盒：高 40、行距 4 → 第一行 top=0，第二行 top=44。
    const centers = [
      tocRailRowCenterPx(0, 40, 0),
      tocRailRowCenterPx(44, 40, 0),
    ];

    expect(centers).toEqual([20, 64]);
    expect(buildTocRailNodes(headings.slice(0, 2), centers).map((node) => node.y)).toEqual(centers);
  });

  it("容器不在视口顶部时中心仍然相对容器算（减掉 origin）", () => {
    expect(tocRailRowCenterPx(300, 40, 280)).toBe(40);
  });

  it("节点 x 只有 0/1/2 三档：5 / 19 / 33", () => {
    expect([0, 1, 2].map(tocRailNodeX)).toEqual([5, 19, 33]);
  });

  it("文字内边距 = 自己的节点 x + 14px：任何层级间距都一样", () => {
    for (const depth of [0, 1, 2]) {
      expect(tocRailLabelInsetPx(depth)).toBe(tocRailNodeX(depth) + TOC_RAIL_LABEL_GAP_PX);
      expect(tocRailLabelInsetPx(depth) - tocRailNodeX(depth)).toBe(14);
    }

    expect([0, 1, 2].map(tocRailLabelInsetPx)).toEqual([19, 33, 47]);
  });

  it("层级越深缩进越多，但间距恒定（不会越缩越远）", () => {
    const gaps = [0, 1, 2].map((depth) => tocRailLabelInsetPx(depth) - tocRailNodeX(depth));

    expect(new Set(gaps).size).toBe(1);
  });
});

describe("buildTocRailPath：横坐标一变就拐弯的折线", () => {
  // a(5,20) → b(19,56) → c(5,100)
  const path = buildTocRailPath(buildTocRailNodes(headings, [20, 56, 100]));

  it("节点仍是折线顶点：起点与各节点坐标都出现在 path 上", () => {
    expect(path.nodes).toHaveLength(3);
    expect(path.d.startsWith(`M ${TOC_RAIL_BASE_X_PX} 20`)).toBe(true);
    expect(path.d).toContain(`${TOC_RAIL_BASE_X_PX + TOC_RAIL_INDENT_PX} 56`);
    expect(path.d.endsWith(`${TOC_RAIL_BASE_X_PX} 100`)).toBe(true);
  });

  it("相邻节点 x 不同 → 插入两个折点（跨度的 30% / 70%），一段拆成三段", () => {
    // a → b：dy = 36，折点在 20 + 10.8 = 30.8 与 56 - 10.8 = 45.2
    expect(path.d).toContain(`${TOC_RAIL_BASE_X_PX} 30.8`);
    expect(path.d).toContain(`${TOC_RAIL_BASE_X_PX + TOC_RAIL_INDENT_PX} 45.2`);
    // b → c：dy = 44，折点在 69.2 与 86.8（回程往左）
    expect(path.d).toContain(`${TOC_RAIL_BASE_X_PX + TOC_RAIL_INDENT_PX} 69.2`);
    expect(path.d).toContain(`${TOC_RAIL_BASE_X_PX} 86.8`);

    // 两段过渡 × 三段 = 6 段（竖 / 斜 / 竖各两次）。
    expect(path.segments).toHaveLength(6);
    expect(path.d.match(/L/gu)).toHaveLength(6);
  });

  it("折点比例就是 30%：跨度乘 0.3 落在折点上", () => {
    const firstBend = path.segments[0];
    expect(firstBend).toBeDefined();
    expect((firstBend?.y2 ?? 0) - 20).toBeCloseTo(36 * TOC_RAIL_BEND_RATIO, 5);
  });

  it("长度是各段长度之和（10.8 + 20.08 + 10.8 + 13.2 + 22.49 + 13.2）", () => {
    // 手算：竖 10.8 / 斜 √(14² + 14.4²) ≈ 20.08 / 竖 10.8，回程 13.2 + √(14² + 17.6²) ≈ 22.49 + 13.2
    expect(path.segments.map((segment) => Number(segment.length.toFixed(2)))).toEqual([
      10.8, 20.08, 10.8, 13.2, 22.49, 13.2,
    ]);
    expect(path.totalLength).toBeCloseTo(90.57, 1);
  });

  it("只有竖段与斜段：每个斜段都真的横向移动了，整条轨道仍然只向下", () => {
    const slanted = path.segments.filter((segment) => segment.x1 !== segment.x2);

    expect(slanted).toHaveLength(2);
    expect(slanted.map((segment) => segment.x2 - segment.x1)).toEqual([
      TOC_RAIL_INDENT_PX,
      -TOC_RAIL_INDENT_PX,
    ]);
    for (const segment of path.segments) {
      expect(segment.y2).toBeGreaterThan(segment.y1);
    }
  });

  it("同层相邻标题（x 相同）不加折点：就是一条竖线", () => {
    const flat = buildTocRailPath(
      buildTocRailNodes(
        [
          { id: "a", level: 2 },
          { id: "b", level: 2 },
        ],
        [20, 60],
      ),
    );

    expect(flat.segments).toHaveLength(1);
    expect(flat.d).toBe(`M ${TOC_RAIL_BASE_X_PX} 20 L ${TOC_RAIL_BASE_X_PX} 60`);
  });

  it("纵向跨度极小时折点不交叉（不会画出回头的折线）", () => {
    const tight = buildTocRailPath(
      buildTocRailNodes(
        [
          { id: "a", level: 2 },
          { id: "b", level: 3 },
        ],
        [20, 21],
      ),
    );

    for (const segment of tight.segments) {
      expect(segment.y2).toBeGreaterThanOrEqual(segment.y1);
    }
    expect(tight.totalLength).toBeGreaterThan(0);
  });

  it("svg 画布：宽度固定、高度够装下最后一个节点（含底部留白与飞机机尾）", () => {
    expect(path.width).toBe(TOC_RAIL_WIDTH_PX);
    expect(path.height).toBeGreaterThan(100);
  });

  it("单个标题：没有线段，路径只有一个起笔点", () => {
    const single = buildTocRailPath(buildTocRailNodes([{ id: "only", level: 2 }], [20]));

    expect(single.segments).toEqual([]);
    expect(single.totalLength).toBe(0);
    expect(single.d).toBe(`M ${TOC_RAIL_BASE_X_PX} 20`);
  });

  it("空标题 → 空路径（d 为空串，长度为 0）", () => {
    const empty = buildTocRailPath([]);

    expect(empty.d).toBe("");
    expect(empty.totalLength).toBe(0);
    expect(empty.segments).toEqual([]);
  });
});

describe("tocRailLengthAtY：按 y 求已经走过多少", () => {
  const path = buildTocRailPath(buildTocRailNodes(headings, [20, 56, 100]));

  it("起点处为 0，第一个折点处等于竖段长度", () => {
    expect(tocRailLengthAtY(path, 20)).toBe(0);
    expect(tocRailLengthAtY(path, 30.8)).toBeCloseTo(10.8, 2);
  });

  it("节点处等于走到该节点的累计长度（斜段按纵向比例切分）", () => {
    expect(tocRailLengthAtY(path, 45.2)).toBeCloseTo(30.88, 1);
    expect(tocRailLengthAtY(path, 56)).toBeCloseTo(41.68, 1);
    expect(tocRailLengthAtY(path, 100)).toBeCloseTo(path.totalLength, 5);
  });

  it("末节点之后钳到全长，起点之上钳到 0（滚动越界不会算出负长度）", () => {
    expect(tocRailLengthAtY(path, 9999)).toBeCloseTo(path.totalLength, 5);
    expect(tocRailLengthAtY(path, -100)).toBe(0);
  });

  it("y 在斜段里时长度也单调增长（不会因为拐弯而回退）", () => {
    let previous = -1;
    for (let y = 20; y <= 100; y += 2) {
      const length = tocRailLengthAtY(path, y);
      expect(length).toBeGreaterThanOrEqual(previous);
      previous = length;
    }
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

describe("tocRailPointAtLength：飞机停在哪", () => {
  const path = buildTocRailPath(buildTocRailNodes(headings, [20, 56, 100]));
  const [segVertical, segSlant] = path.segments;

  it("长度 0 落在起点，竖段尽头落在上折点", () => {
    expect(tocRailPointAtLength(path, 0)).toEqual({ x: TOC_RAIL_BASE_X_PX, y: 20 });

    const bend = tocRailPointAtLength(path, segVertical?.length ?? 0);
    expect(bend?.x).toBeCloseTo(TOC_RAIL_BASE_X_PX, 5);
    expect(bend?.y).toBeCloseTo(30.8, 5);
  });

  it("斜段中点落在两折点的正中间（横坐标走了一半缩进）", () => {
    const half = (segVertical?.length ?? 0) + (segSlant?.length ?? 0) / 2;

    const point = tocRailPointAtLength(path, half);
    expect(point?.x).toBeCloseTo(TOC_RAIL_BASE_X_PX + TOC_RAIL_INDENT_PX / 2, 5);
    expect(point?.y).toBeCloseTo(38, 5);
  });

  it("走到某个节点时点就是那个节点（飞机停在节点上，不悬空）", () => {
    for (const node of path.nodes) {
      const point = tocRailPointAtLength(path, tocRailLengthAtY(path, node.y));

      expect(point?.x).toBeCloseTo(node.x, 5);
      expect(point?.y).toBeCloseTo(node.y, 5);
    }
  });

  it("越界钳到两端；空节点表没有可落的点", () => {
    expect(tocRailPointAtLength(path, -50)).toEqual({ x: TOC_RAIL_BASE_X_PX, y: 20 });
    expect(tocRailPointAtLength(path, 10_000)).toEqual({ x: TOC_RAIL_BASE_X_PX, y: 100 });
    expect(tocRailPointAtLength(buildTocRailPath([]), 10)).toBeNull();
  });
});

describe("tocRailAngleAtLength / tocRailPlaneRotationDeg：飞机朝向", () => {
  const path = buildTocRailPath(buildTocRailNodes(headings, [20, 56, 100]));
  const [segVertical, segSlant] = path.segments;

  it("切线角沿轨道走：竖段 90°，右下斜段 ≈46°，左下斜段 ≈129°", () => {
    // 手算：atan2(14.4, 14) = 45.81°；atan2(17.6, -14) = 128.50°
    expect(Math.round(tocRailAngleAtLength(path, 5))).toBe(90);
    expect(Math.round(tocRailAngleAtLength(path, 20.84))).toBe(46);
    expect(Math.round(tocRailAngleAtLength(path, 66.13))).toBe(129);
  });

  it("机身角度 = 切线 + 90°（图形机头朝上，转到切线要多转一个直角）", () => {
    expect(tocRailPlaneRotationDeg(path, 5)).toBeCloseTo(180, 5);
    expect(tocRailPlaneRotationDeg(path, 20.84)).toBeCloseTo(135.81, 1);
    expect(tocRailPlaneRotationDeg(path, 66.13)).toBeCloseTo(218.5, 1);
  });

  it("超界长度钳在两端（弹簧过冲也不会让机头朝向跳档）", () => {
    expect(Math.round(tocRailAngleAtLength(path, -50))).toBe(90);
    // 末段是回到 c 的那截竖线，所以走向仍然是"向下"。
    expect(Math.round(tocRailAngleAtLength(path, 10_000))).toBe(90);
    expect(tocRailPlaneRotationDeg(path, 10_000)).toBe(180);
  });

  it("零长路径（只有一个标题）没有线段：按「向下」给角度，不返回 NaN", () => {
    const single = buildTocRailPath(buildTocRailNodes([{ id: "only", level: 2 }], [20]));

    expect(tocRailAngleAtLength(single, 0)).toBe(90);
    expect(tocRailPlaneRotationDeg(single, 0)).toBe(180);
  });

  it("每个长度都有确定的点与角度（斜段两侧不出现 NaN 或跳空）", () => {
    // 覆盖全部段边界附近：折点、节点、以及对半处。
    const probes = [0, segVertical?.length ?? 0, segSlant?.length ?? 0, path.totalLength];

    for (const length of probes) {
      const point = tocRailPointAtLength(path, length);
      expect(Number.isFinite(point?.x)).toBe(true);
      expect(Number.isFinite(point?.y)).toBe(true);
      expect(Number.isFinite(tocRailAngleAtLength(path, length))).toBe(true);
    }
  });
});

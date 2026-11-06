import { tocIndentLevel } from "~/utils/toc";

/**
 * 标题大纲「轨道 + 飞行器」的几何（票 296，纯函数，不碰 DOM）。
 *
 * 分工：这个模块只负责**算**——层级换算成缩进、每个标题一个节点、节点之间连成
 * 一条只往下走的折线，以及「某个 y 之前已经走过多少长度」「某个长度处的坐标与
 * 切线角度」。组件负责**量**（每个链接行的中心 y）与**画**（SVG + 弹簧）。
 *
 * 为什么几何要独立成纯函数：轨道的观感全靠这几个量（虚线描边、已读段实线填、
 * 飞机停在当前长度、机头贴切线），但它们全是"输入数字 → 输出数字"，不需要浏览器
 * 就能钉死边界（单层、空标题、测量退化、滚动越界）。
 *
 * 为什么测量退化要兜底：SSR 与首帧没有布局，`offsetTop` 全是 0。若直接把 0 当
 * 真值，轨道会缩成一个点。兜底按等距排布，让首屏就有一条完整的轨道，客户端量到
 * 真值后再改（这也是参考组件自己做的那件事）。
 */

/** 每级标题的横向缩进（px）：与左栏目录树的缩进口径区分开，这里是轨道自己的量。 */
export const TOC_RAIL_INDENT_PX = 14;

/** 兜底行距（px）：测量不可用时按这个间距等距排布。 */
export const TOC_RAIL_ROW_PX = 28;

/** 轨道起点距 svg 左边缘的距离（px）：给飞机机身留出横向余量。 */
export const TOC_RAIL_BASE_X_PX = 8;

/** 轨道顶部留白（px）。 */
export const TOC_RAIL_PAD_TOP_PX = 12;

/** 轨道底部留白（px）：最后一个节点下方还要能放下飞机尾迹。 */
export const TOC_RAIL_PAD_BOTTOM_PX = 16;

/** 节点圆点半径（px）。 */
export const TOC_RAIL_NODE_RADIUS_PX = 3.5;

/** svg 画布宽度（px）：兜住 3 档缩进 + 节点半径 + 飞机横向余量。 */
export const TOC_RAIL_WIDTH_PX = 44;

/** 折线只认标题的这两个字段（`ArticleHeadingAnchor` 的子集，便于测试直接喂字面量）。 */
export interface TocRailHeading {
  id: string;
  level: number;
}

/** 一个标题在轨道上的位置：缩进档位 + 画布坐标。 */
export interface TocRailNode {
  id: string;
  /** 缩进档位（0..2），与 `tocIndentLevel` 同源。 */
  depth: number;
  x: number;
  y: number;
}

/** 折线的一段：用于"按 y 求长度"与切线角。 */
export interface TocRailSegment {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  /** 这段起点在整条轨道上的累计长度。 */
  startLength: number;
  /** 这段自身的长度。 */
  length: number;
  /** 切线角（度，SVG 坐标：0 = 向右，90 = 向下）。 */
  angleDeg: number;
}

/** 整条轨道：path 字符串 + 分段 + 尺寸。 */
export interface TocRailPath {
  d: string;
  nodes: TocRailNode[];
  segments: TocRailSegment[];
  totalLength: number;
  width: number;
  height: number;
}

/** 保留两位小数并去掉多余的 0（path 字符串里的数字不该是 8.000000001）。 */
function round(value: number): number {
  return Math.round(value * 100) / 100;
}

/**
 * 测量值是否可信：条数对得上、全是有限非负数、且严格递增。
 *
 * 不递增意味着"两个标题中心重合"——那会把折线画成回头的竖线，比等距兜底更糟。
 */
function isUsableRows(rows: readonly (number | null | undefined)[], count: number): boolean {
  if (rows.length !== count || count === 0) {
    return false;
  }

  let previous = Number.NEGATIVE_INFINITY;
  for (const row of rows) {
    if (typeof row !== "number" || !Number.isFinite(row) || row < 0 || row <= previous) {
      return false;
    }
    previous = row;
  }

  return true;
}

/**
 * 标题序列 → 轨道节点。
 *
 * @param headings 标题（顺序即文章顺序）
 * @param rows 每个标题行的中心 y（相对轨道容器顶部）；缺省或不可信时等距兜底
 */
export function buildTocRailNodes(
  headings: readonly TocRailHeading[],
  rows: readonly (number | null | undefined)[] = [],
): TocRailNode[] {
  const usable = isUsableRows(rows, headings.length);

  return headings.map((heading, index) => {
    const depth = tocIndentLevel(heading.level);

    return {
      id: heading.id,
      depth,
      x: TOC_RAIL_BASE_X_PX + depth * TOC_RAIL_INDENT_PX,
      y: usable ? (rows[index] as number) : TOC_RAIL_PAD_TOP_PX + index * TOC_RAIL_ROW_PX,
    };
  });
}

/** 节点序列 → 折线（相邻节点直接连线，所以只会横向走一次、整体只向下）。 */
export function buildTocRailPath(nodes: readonly TocRailNode[]): TocRailPath {
  if (nodes.length === 0) {
    return {
      d: "",
      nodes: [],
      segments: [],
      totalLength: 0,
      width: TOC_RAIL_WIDTH_PX,
      height: 0,
    };
  }

  const first = nodes[0];
  const commands = [`M ${round(first.x)} ${round(first.y)}`];
  const segments: TocRailSegment[] = [];
  let totalLength = 0;

  for (let index = 1; index < nodes.length; index += 1) {
    const from = nodes[index - 1];
    const to = nodes[index];
    const dx = to.x - from.x;
    const dy = to.y - from.y;
    const length = Math.hypot(dx, dy);

    segments.push({
      x1: from.x,
      y1: from.y,
      x2: to.x,
      y2: to.y,
      startLength: totalLength,
      length,
      angleDeg: (Math.atan2(dy, dx) * 180) / Math.PI,
    });

    totalLength += length;
    commands.push(`L ${round(to.x)} ${round(to.y)}`);
  }

  const last = nodes[nodes.length - 1];

  return {
    d: commands.join(" "),
    nodes: [...nodes],
    segments,
    totalLength,
    width: TOC_RAIL_WIDTH_PX,
    height: Math.ceil(last.y + TOC_RAIL_PAD_BOTTOM_PX + TOC_RAIL_NODE_RADIUS_PX),
  };
}

/** 把长度钳在 [0, totalLength]。 */
function clampLength(path: TocRailPath, length: number): number {
  if (!Number.isFinite(length) || length <= 0) {
    return 0;
  }
  return Math.min(path.totalLength, length);
}

/**
 * 走到某个 y 时在轨道上已经走了多长（px）。
 *
 * 折线的 y 单调递增，所以 y 只可能落在一段里：找到那一段，按纵向比例切分它的长度。
 * 越界（滚动到底部之后 / 顶部之前）钳到两端。
 */
export function tocRailLengthAtY(path: TocRailPath, y: number): number {
  if (path.totalLength <= 0 || !Number.isFinite(y)) {
    return 0;
  }

  const first = path.nodes[0];
  const last = path.nodes[path.nodes.length - 1];
  if (y <= first.y) {
    return 0;
  }
  if (y >= last.y) {
    return path.totalLength;
  }

  for (const segment of path.segments) {
    if (y <= segment.y2) {
      const span = segment.y2 - segment.y1;
      const progress = span <= 0 ? 1 : (y - segment.y1) / span;
      return segment.startLength + segment.length * progress;
    }
  }

  return path.totalLength;
}

/** 已读比例（0..1）：零长路径按 0 收口，不产生 NaN。 */
export function tocRailReadFraction(path: TocRailPath, y: number): number {
  if (path.totalLength <= 0) {
    return 0;
  }

  return tocRailLengthAtY(path, y) / path.totalLength;
}

/** 某个长度处的坐标（飞机停机点）。零段路径落在唯一节点上。 */
export function tocRailPointAtLength(path: TocRailPath, length: number): { x: number; y: number } {
  const node = path.nodes[0];
  if (!node) {
    return { x: TOC_RAIL_BASE_X_PX, y: 0 };
  }

  const target = clampLength(path, length);
  if (path.totalLength <= 0) {
    return { x: node.x, y: node.y };
  }

  for (const segment of path.segments) {
    if (target <= segment.startLength + segment.length) {
      const progress =
        segment.length <= 0 ? 1 : (target - segment.startLength) / segment.length;
      return {
        x: segment.x1 + (segment.x2 - segment.x1) * progress,
        y: segment.y1 + (segment.y2 - segment.y1) * progress,
      };
    }
  }

  const last = path.nodes[path.nodes.length - 1];
  return { x: last.x, y: last.y };
}

/** 某个长度处的切线角（度）：飞机机头就贴这个角度。没有线段时按"向下"给。 */
export function tocRailAngleAtLength(path: TocRailPath, length: number): number {
  if (path.segments.length === 0) {
    return 90;
  }

  const target = clampLength(path, length);

  for (const segment of path.segments) {
    if (target <= segment.startLength + segment.length) {
      return segment.angleDeg;
    }
  }

  return path.segments[path.segments.length - 1].angleDeg;
}

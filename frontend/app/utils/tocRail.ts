import { tocIndentLevel } from "~/utils/toc";

/**
 * 标题大纲「轨道 + 飞行器」的几何（票 296，纯函数，不碰 DOM）。
 *
 * 分工：这个模块只负责**算**——层级换算成缩进、每个标题一个节点、节点之间连成
 * 一条**会拐弯的折线**，以及「某个 y 之前已经走过多少长度」「某个长度处在哪、
 * 切线朝哪」。组件负责**量**（每个链接行的中心 y）与**画**（SVG + 弹簧）。
 *
 * 为什么轨道必须拐弯：参考组件的节点横坐标是 `5 + depth * 14`，层级一变 x 就变，
 * 于是两个节点之间**不能**用一条固定 x 的竖线连起来——那样节点会悬在竖线外面，
 * 看起来像"点没长在轨道上"（上一版被用户否掉的正是这一点）。这里把每段相邻节点
 * 拆成「竖 → 斜 → 竖」三小段：斜段夹在两个折点之间，折点分别位于这段纵向跨度的
 * 30% 与 70% 处，所以拐弯是一条短斜线，而不是直角台阶。
 *
 * 为什么几何要独立成纯函数：轨道的观感全靠这几个量（虚线描边、已读段实线填、
 * 飞机停在当前长度、机头贴切线），但它们全是"输入数字 → 输出数字"，不需要浏览器
 * 就能钉死边界（单层、空标题、测量退化、滚动越界）。
 *
 * 为什么测量退化要兜底：SSR 与首帧没有布局，`offsetTop` 全是 0。若直接把 0 当
 * 真值，轨道会缩成一个点。兜底按等距排布，让首屏就有一条完整的轨道，客户端量到
 * 真值后再改（这也是参考组件自己做的那件事）。
 */

/** 轨道起点距 svg 左边缘的距离（px）：与参考组件的 `RAIL_X` 同口径。 */
export const TOC_RAIL_BASE_X_PX = 5;

/** 每级标题的横向缩进（px）：节点 x = 起点 + depth × 本值，与参考组件一致。 */
export const TOC_RAIL_INDENT_PX = 14;

/** 兜底行距（px）：测量不可用时按这个间距等距排布。 */
export const TOC_RAIL_ROW_PX = 28;

/**
 * 折点位置（占相邻两节点纵向跨度的比例）：0.3 表示上折点在 30%、下折点在 70%，
 * 中间 40% 留给斜线过渡。取值与参考组件的"30% 处放两个折点"一致。
 */
export const TOC_RAIL_BEND_RATIO = 0.3;

/** 轨道顶部留白（px）。 */
export const TOC_RAIL_PAD_TOP_PX = 12;

/** 轨道底部留白（px）：最后一个节点下方还要能放下飞机机尾。 */
export const TOC_RAIL_PAD_BOTTOM_PX = 16;

/** 节点圆点半径（px）：已读 = 实心 3px（参考组件同值）。 */
export const TOC_RAIL_NODE_RADIUS_PX = 3;

/** 未读节点的半径（px）：比已读略大一点，空心描边才不显细（参考组件同值）。 */
export const TOC_RAIL_NODE_RADIUS_HOLLOW_PX = 3.25;

/** svg 画布宽度（px）：兜住最深一层的节点 x（5 + 2 × 14）与飞机横向半宽。 */
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

/** 折线上的一点（节点、折点、以及任意长度处取样都用它）。 */
export interface TocRailPoint {
  x: number;
  y: number;
}

/** 折线的一段：用于"按 y 求长度""按长度求点/切线"。 */
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

/**
 * 两个节点的横坐标不同时，它们之间的两个折点。
 *
 * 上折点留在出发节点的 x 上（纵向跨度的 30%），下折点已经落在到达节点的 x 上
 * （70% 处），两点之间就是那条斜线。x 相同时返回空数组——同层相邻标题之间本来就
 * 是一条竖线，不该为了"好看"硬加一段折。
 *
 * `near` 额外钳到跨度的一半以内：测量退化成"两个标题几乎重合"时也不会让两个折点
 * 交叉（那会画出回头的折线）。
 */
function bendPoints(from: TocRailNode, to: TocRailNode): TocRailPoint[] {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  if (Math.abs(dx) < 0.01 || !(dy > 0)) {
    return [];
  }

  const near = Math.min(TOC_RAIL_BEND_RATIO * dy, dy / 2);
  if (!(near > 0)) {
    return [];
  }

  return [
    { x: from.x, y: from.y + near },
    { x: to.x, y: to.y - near },
  ];
}

/** 节点序列 → 折线（相邻节点之间横坐标不同就插两个折点，整体只向下）。 */
export function buildTocRailPath(nodes: readonly TocRailNode[]): TocRailPath {
  const first = nodes[0];
  if (!first) {
    return {
      d: "",
      nodes: [],
      segments: [],
      totalLength: 0,
      width: TOC_RAIL_WIDTH_PX,
      height: 0,
    };
  }

  // 先把折点摊平成一条点序列：节点本身仍是折线的顶点（点一定落在轨道上），
  // 折点只是节点之间的插值点。
  const points: TocRailPoint[] = [{ x: first.x, y: first.y }];
  let previous = first;
  for (const to of nodes.slice(1)) {
    points.push(...bendPoints(previous, to), { x: to.x, y: to.y });
    previous = to;
  }

  const commands = [`M ${round(first.x)} ${round(first.y)}`];
  const segments: TocRailSegment[] = [];
  let totalLength = 0;

  for (let index = 1; index < points.length; index += 1) {
    const from = points[index - 1];
    const to = points[index];
    if (!from || !to) {
      continue;
    }
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

  return {
    d: commands.join(" "),
    nodes: [...nodes],
    segments,
    totalLength,
    width: TOC_RAIL_WIDTH_PX,
    height: Math.ceil(previous.y + TOC_RAIL_PAD_BOTTOM_PX + TOC_RAIL_NODE_RADIUS_HOLLOW_PX),
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
 * 折线的 y 单调递增（节点 y 严格递增，折点都夹在两个节点之间），所以 y 只可能落在
 * 一段里：找到那一段，按纵向比例切分它的长度。斜段里"纵向比例 = 弧长比例"成立，
 * 因为段内是直线。越界（滚动到底部之后 / 顶部之前）钳到两端。
 */
export function tocRailLengthAtY(path: TocRailPath, y: number): number {
  if (path.totalLength <= 0 || !Number.isFinite(y)) {
    return 0;
  }

  const first = path.nodes[0];
  const last = path.nodes[path.nodes.length - 1];
  if (!first || !last) {
    return 0;
  }
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

/**
 * 某个长度处在画布上的点（飞机就停在这里）。
 *
 * 越界钳到两端；空节点表没有可落的点，返回 null（组件据此不画飞机）。
 */
export function tocRailPointAtLength(path: TocRailPath, length: number): TocRailPoint | null {
  const first = path.nodes[0];
  if (!first) {
    return null;
  }

  const target = clampLength(path, length);

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

  const last = path.segments[path.segments.length - 1];
  return last ? { x: last.x2, y: last.y2 } : { x: first.x, y: first.y };
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

  const last = path.segments[path.segments.length - 1];
  // 空线段已在上面提前返回；显式兜底只为满足严格空值检查（行为不变）。
  return last ? last.angleDeg : 90;
}

/**
 * 飞机的旋转角（度）：切线角 + 90。
 *
 * 为什么加 90：机身是按"机头朝上"画的（`M12 2 …` 的尖角在正上方），而切线角
 * 用的是 SVG 口径（0 = 向右、90 = 向下）。把朝上的图形转到切线方向，就要多转
 * 一个 90°——这也正是参考组件的算法。角度不取模：负角/超过 360° 的旋转在 SVG
 * 里等价，保留原值便于测试直接读，不必和归一化纠缠。
 */
export function tocRailPlaneRotationDeg(path: TocRailPath, length: number): number {
  return tocRailAngleAtLength(path, length) + 90;
}

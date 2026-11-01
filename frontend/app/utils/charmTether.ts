/**
 * 挂绳的绘制几何：链节点 → 一条可填充的带形路径（纯函数）。
 *
 * 为什么从 `decoCharm.ts` 拆出来：那个模块同时装着"参数与钳位""启用门控"
 * "Verlet 链""绘制几何"四件事，票 #75 加完指针端偏移后超过了仓库的单文件
 * 800 行门槛（`frontend/eslint.config.mjs` 的 `max-lines`）。拆的边界选在
 * "绘制"这一侧：这里不持有状态、不推进物理，输入一串点、输出可以直接写进
 * SVG 的 `d`。
 *
 * 依赖方向是单向的：本模块只从 `decoCharm.ts` 取线宽钳位与收尾比例，
 * `decoCharm.ts` **不**反过来引用它 —— 没有循环依赖。
 */
import { CHARM_TAPER_RATIO, clampCharmLineWidth, type CharmPoint } from "./decoCharm";

/**
 * 弧垂的几何系数。
 *
 * 松弛量与矢高不是线性关系：把绳子近似成一段圆弧 / 抛物线，弧长 L、弦长 c、
 * 矢高 s 满足 L − c ≈ (8/3) s² / c，所以 **s ≈ √(3 c (L − c) / 8)**。
 * 这解释了为什么"一点点松弛"就能看出一条明显的弧：松弛 0.1px 时矢高已有
 * 约 2px。松弛为 0 时矢高严格为 0（竖直悬挂就是直线）。
 */
const CHARM_SAG_NUMERATOR = 3;
const CHARM_SAG_DENOMINATOR = 8;

/**
 * 中心线的采样密度：每个链节点区间取 4 个 Catmull-Rom 采样点。
 *
 * 4 段/区间已经让 10 节点的折线在 1px 级看不出棱角；再加密只是让每帧的
 * `d` 字符串变长，视觉收益为零。
 */
const CHARM_CURVE_SAMPLES_PER_SEGMENT = 4;

/** 挂绳的几何结果：中心线、每点宽度、松弛量、弧垂与可直接写进 SVG 的带形路径。 */
export interface CharmTetherGeometry {
  /** 平滑中心线（经过链节点；原始精度，不取整）。 */
  centerline: CharmPoint[];
  /** 每个中心线采样点处的描边宽度（px）；首项 = line_width，末项 = 0.6 × line_width。 */
  widths: number[];
  /** 松驰量（px）= max(0, 绳长 − 指针到站标的实际距离)。 */
  slackPx: number;
  /** 中心线相对头尾弦的最大横向偏移（px）：这就是肉眼看到的"弧垂"。 */
  maxDeviationPx: number;
  /** 闭合的带形多边形（SVG `<path d>` 用 `fill`，不是 `stroke`）。 */
  path: string;
}

/** 单个 Catmull-Rom 采样点。 */
function catmullRomPoint(p0: CharmPoint, p1: CharmPoint, p2: CharmPoint, p3: CharmPoint, t: number): CharmPoint {
  const t2 = t * t;
  const t3 = t2 * t;
  return {
    x:
      0.5 *
      (2 * p1.x +
        (-p0.x + p2.x) * t +
        (2 * p0.x - 5 * p1.x + 4 * p2.x - p3.x) * t2 +
        (-p0.x + 3 * p1.x - 3 * p2.x + p3.x) * t3),
    y:
      0.5 *
      (2 * p1.y +
        (-p0.y + p2.y) * t +
        (2 * p0.y - 5 * p1.y + 4 * p2.y - p3.y) * t2 +
        (-p0.y + 3 * p1.y - 3 * p2.y + p3.y) * t3),
  };
}

/**
 * 把链节点插值成一条经过每个节点的平滑折线（Catmull-Rom）。
 *
 * 为什么要插值而不直接连节点：10 个节点的折线在甩动时会露出棱角，
 * 而"有重量的绳子"读起来必须是连续的。采样点保留原始精度，取整只发生在
 * 写进 `d` 的那一刻。
 */
function smoothCenterline(points: readonly CharmPoint[]): CharmPoint[] {
  const count = points.length;
  if (count === 0) return [];
  if (count === 1) return [{ ...points[0]! }];
  const samples: CharmPoint[] = [];
  for (let index = 0; index < count - 1; index += 1) {
    const p0 = points[Math.max(0, index - 1)]!;
    const p1 = points[index]!;
    const p2 = points[index + 1]!;
    const p3 = points[Math.min(count - 1, index + 2)]!;
    for (let sample = 0; sample < CHARM_CURVE_SAMPLES_PER_SEGMENT; sample += 1) {
      samples.push(catmullRomPoint(p0, p1, p2, p3, sample / CHARM_CURVE_SAMPLES_PER_SEGMENT));
    }
  }
  samples.push({ ...points[count - 1]! });
  return samples;
}

/** 保留一位小数（非有限值一律当 0）：避免长浮点每帧写出不同的 `d` 字符串。 */
function round1(value: number): number {
  return Number.isFinite(value) ? Number(value.toFixed(1)) : 0;
}

/**
 * 挂绳的绘制几何。
 *
 * 三件事在这里同时决定，所以单独成一个纯函数、供测试直接喂"几何链"：
 *
 * 1. **弧垂只由松弛驱动，绝不来自重力**。松弛 = 绳长 − 指针到站标的实际
 *    距离；两端距离恰好等于绳长时矢高为 0（竖直悬挂就是一条直线），
 *    松弛越大中段偏离弦越多。重力只影响链的瞬态姿态（节点的实际位置），
 *    从不进入这个函数 —— 调大重力不会让静止的绳子弯下来。
 * 2. **收尾描边恰好是起始的 0.6 倍**。一根 `stroke` 变不了宽度，所以这里
 *    生成一条**填充的带形多边形**：两侧边界沿中心线法向各偏移半个宽度，
 *    宽度从 `line_width` 线性收到 `0.6 × line_width`。每帧仍然只写一个
 *    `d`，不增加 DOM 写入次数。
 * 3. 坐标保留一位小数，避免每帧写出长浮点制造 diff 噪声。
 */
export function charmTetherGeometry(
  points: readonly CharmPoint[],
  ropeLengthPx: number,
  lineWidthPx: number,
): CharmTetherGeometry {
  const widthStart = clampCharmLineWidth(lineWidthPx);
  const ropeLength = Number.isFinite(ropeLengthPx) && ropeLengthPx > 0 ? ropeLengthPx : 0;
  const centerline = smoothCenterline(points);
  const count = centerline.length;
  if (count === 0) {
    return { centerline: [], widths: [], slackPx: 0, maxDeviationPx: 0, path: "" };
  }

  // 两端之间的弦：它是"实际距离"，弧垂的量尺。
  const head = centerline[0]!;
  const tail = centerline[count - 1]!;
  const chordX = tail.x - head.x;
  const chordY = tail.y - head.y;
  const chord = Math.hypot(chordX, chordY);
  const slackPx = Math.max(0, ropeLength - chord);

  // 弧的方向：顺着链自己的横向偏移（物理形变的方向）；纯直线的合成几何
  // 没有方向可说，就取弦的左侧作为确定性默认。两端点处权重为 0。
  const normalX = chord > 1e-9 ? -chordY / chord : 0;
  const normalY = chord > 1e-9 ? chordX / chord : 1;
  let side = 0;
  for (const point of centerline) {
    side += (point.x - head.x) * normalX + (point.y - head.y) * normalY;
  }
  const direction = side < 0 ? -1 : 1;
  // 松弛 → 矢高：s ≈ √(3 c slack / 8)。两端恰好等于绳长时 slack = 0 → 直线。
  const sagPeakPx = Math.sqrt((CHARM_SAG_NUMERATOR * chord * slackPx) / CHARM_SAG_DENOMINATOR);

  // 宽度与弧垂都按"沿链的归一化参数"插值：首尾宽度固定，中段弧垂最大。
  const widths: number[] = [];
  const offsetSamples: CharmPoint[] = [];
  for (let index = 0; index < count; index += 1) {
    const u = count > 1 ? index / (count - 1) : 1;
    const point = centerline[index]!;
    const bowPx = sagPeakPx * 4 * u * (1 - u);
    const bowed = {
      x: point.x + normalX * direction * bowPx,
      y: point.y + normalY * direction * bowPx,
    };
    centerline[index] = bowed;
    offsetSamples.push(bowed);
    widths.push(widthStart * (1 - (1 - CHARM_TAPER_RATIO) * u));
  }

  // 弧垂 = 中心线相对弦的最大横向偏移（用同一把尺子量，测试与显示一致）。
  let maxDeviationPx = 0;
  const chordUnitX = chord > 1e-9 ? chordX / chord : 0;
  const chordUnitY = chord > 1e-9 ? chordY / chord : 1;
  for (const point of centerline) {
    const dx = point.x - head.x;
    const dy = point.y - head.y;
    maxDeviationPx = Math.max(maxDeviationPx, Math.abs(dx * chordUnitY - dy * chordUnitX));
  }

  // 带形多边形：中心线两侧各偏移半个宽度。两侧各走一遍，尾端用一条
  // `L` 连起来，首端由 `Z` 闭合。
  const left: CharmPoint[] = [];
  const right: CharmPoint[] = [];
  for (let index = 0; index < count; index += 1) {
    const point = centerline[index]!;
    const previous = centerline[Math.max(0, index - 1)]!;
    const next = centerline[Math.min(count - 1, index + 1)]!;
    const tangentX = next.x - previous.x;
    const tangentY = next.y - previous.y;
    const tangentLength = Math.hypot(tangentX, tangentY);
    const unitX = tangentLength > 1e-9 ? tangentX / tangentLength : 0;
    const unitY = tangentLength > 1e-9 ? tangentY / tangentLength : 1;
    const half = widths[index]! / 2;
    const nx = -unitY * half;
    const ny = unitX * half;
    left.push({ x: point.x + nx, y: point.y + ny });
    right.push({ x: point.x - nx, y: point.y - ny });
  }

  const leftCommands = left.slice(1).map((point) => `L${round1(point.x)} ${round1(point.y)}`);
  const rightCommands = right
    .slice(0, -1)
    .reverse()
    .map((point) => `L${round1(point.x)} ${round1(point.y)}`);
  const path =
    `M${round1(left[0]!.x)} ${round1(left[0]!.y)}` +
    leftCommands.join("") +
    `L${round1(right[right.length - 1]!.x)} ${round1(right[right.length - 1]!.y)}` +
    rightCommands.join("") +
    "Z";

  return { centerline, widths, slackPx, maxDeviationPx, path };
}

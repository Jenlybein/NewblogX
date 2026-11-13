import { onBeforeUnmount, onMounted, watch, type Ref } from "vue";
import { useMotionPreference } from "~/composables/useMotionPreference";
import { collectOccluders, hitsRoundedBox } from "~/utils/occluders";
import {
  resolveSpringMotion,
  SPRING_PIXEL_POP,
  SPRING_TRAIL,
  SPRING_TRAIL_TAIL,
} from "~/utils/motion";

/**
 * 首屏「像素尾迹」：指针划过时在深墨底上留下一条会淡出的像素方块轨迹。
 *
 * 观感参照 reactbits.dev/animations/pixel-trail，实现是自己写的（不引第三方包）。
 * 流畅度靠的是**真弹簧**，不是一阶跟随：
 *
 *   - 尾巴是一串弹簧结点：头结点弹向指针，后面每一节弹向**前一节**的位置；
 *   - 每节都是速度 + 加速度积分（半隐式欧拉），所以有惯性、有连续减速度，
 *     不是"每帧追一点"的 lerp —— lerp 只有延迟没有弹性，快速甩动会显得一顿一顿；
 *   - 弹簧参数来自共享预设 `SPRING_TRAIL` / `SPRING_TRAIL_TAIL`（近临界阻尼，
 *     跟得紧但不回弹；尾巴逐级软一档形成连续减速）；
 *   - 结点之间按半格插值盖章，快速甩动时尾巴不会断成一段段。
 *
 * 与首屏其它装饰同一套纪律：
 *   - 只画 canvas，不碰任何 DOM 布局属性；
 *   - `prefers-reduced-motion: reduce` 下不挂监听、不画一帧；
 *   - 标签页隐藏 / 画布离屏时停帧；没有存活方块且指针静止时也停帧，不留常驻 rAF；
 *   - 颜色只从 CSS token 读（默认 `--color-link`，终端蓝的码雨），不写任何色值。
 */

/** 尾迹参数。 */
export interface PixelTrailOptions {
  /** 像素格边长（CSS px）。 */
  cellSize?: number;
  /** 尾巴上的弹簧结点数量（也是"轨迹条数"）。 */
  trailLength?: number;
  /** 方块寿命（毫秒）。 */
  lifeMs?: number;
  /** 最多同时存活多少格（防止快速甩动时铺满画布）。 */
  maxCells?: number;
  /** 扇形散开的横向最大偏移（CSS px）：0 表示所有结点走同一条线。 */
  spread?: number;
  /** 取颜色的 CSS 变量名。 */
  colorVar?: string;
}

const DEFAULTS: Required<PixelTrailOptions> = {
  cellSize: 16,
  trailLength: 9,
  lifeMs: 180,
  maxCells: 560,
  spread: 30,
  colorVar: "--color-on-inverse",
};

/**
 * 合并调用方给的参数与默认值。
 *
 * 逐键用 `??` 兜底：调用方显式给 `undefined`（比如组件只透传了 `cellSize`、
 * `lifeMs` 没传）不能被当成"覆盖成 undefined"，否则寿命算成 NaN、一格都不画。
 * 这个 bug 曾经让"调寿命"看起来完全没生效。
 */
export function resolvePixelTrailOptions(input: PixelTrailOptions = {}): Required<PixelTrailOptions> {
  return {
    cellSize: input.cellSize ?? DEFAULTS.cellSize,
    trailLength: input.trailLength ?? DEFAULTS.trailLength,
    lifeMs: input.lifeMs ?? DEFAULTS.lifeMs,
    maxCells: input.maxCells ?? DEFAULTS.maxCells,
    spread: input.spread ?? DEFAULTS.spread,
    colorVar: input.colorVar ?? DEFAULTS.colorVar,
  };
}

/** 后一节等待前一节的时长（毫秒）：1ms 基本等于"同一帧起步"，只保留极轻的节拍。 */
const NODE_DELAY_MS = 1;

/** 元素留白距离（px）：字符中心离元素外框不到这么多就隐身。 */
const OCCLUDER_PADDING = 4;

/** 从某节的位置历史里取 `when` 时刻的坐标（线性插值）；历史不足时退回当前位置。 */
/** 新格子被"踢"起的初始缩放速度（每秒）：配合 SPRING_PIXEL_POP 约放大 40%。 */
const POP_KICK = 15;

/**
 * 尾迹形状的三个控制点（字号，单位 em = 格边）：
 * **起点（贴指针）0.5em、峰值 1em（峰值位置见 TRAIL_PEAK_AT）、远端 0.8em**。
 *
 * 起点是全局最小值：贴指针的那几格必须**最小最淡**，否则"越近越大越亮"，
 * 看起来就是一条越远越小的普通拖尾，而不是从指针处起笔的一团码雨。
 */
const GLYPH_START_EM = 0.5;
const GLYPH_PEAK_EM = 1;
const GLYPH_END_EM = 0.8;
/** 峰值出现的相对位置（0 = 指针，1 = 一个尾迹半径之外）。 */
/** 峰值出现的相对位置（0 = 指针，1 = 一个尾迹半径之外）。 */
export const TRAIL_PEAK_AT = 0.85;

/**
 * 形状参考半径，单位是**格数**（实际半径 = cellSize × 这个数）。
 *
 * 它和 `trailLength` 解耦：轨迹节数只决定"有几条线"，参考半径决定
 * "多远算远端"。想整体放大/缩小这条光带就改这里，不要动节数。
 */
export const TRAIL_RADIUS_CELLS = 14;

/** 透明度（再乘 glow 弹簧）：起点完全透明（不做"贴指针的一坨"）、峰值最实、远端再次变淡。 */
const ALPHA_START = 0;
const ALPHA_PEAK = 0.92;
const ALPHA_END = 0.6;

/**
 * 两段 smoothstep 拼接的曲线：`t=0` 取 start、`t=TRAIL_PEAK_AT` 取 peak、`t=1` 取 end。
 *
 * 两段各自平滑且单调，所以中间最突出、两端收细，峰值处没有折角。
 */
function trailCurve(t: number, start: number, peak: number, end: number): number {
  const clamped = Math.min(1, Math.max(0, t));
  const smooth = (x: number) => x * x * (3 - 2 * x);
  if (clamped <= TRAIL_PEAK_AT) {
    const k = clamped / TRAIL_PEAK_AT;
    return start + (peak - start) * smooth(k);
  }
  const k = (clamped - TRAIL_PEAK_AT) / (1 - TRAIL_PEAK_AT);
  return peak + (end - peak) * smooth(k);
}

/** 尾迹形状：相对位置 `t`（0 = 贴着指针，1 = 尾端）→ 字号系数（em）。 */
export function trailShape(t: number): number {
  return trailCurve(t, GLYPH_START_EM, GLYPH_PEAK_EM, GLYPH_END_EM);
}

/** 尾迹形状：相对位置 `t` → 基础透明度（再乘 glow 弹簧）。 */
export function trailAlpha(t: number): number {
  return trailCurve(t, ALPHA_START, ALPHA_PEAK, ALPHA_END);
}

/** 同一格两次"弹一下"的最小间隔（毫秒）：让字号周期性变大 → 缩回常态，而不是卡在最大档。 */
const POP_INTERVAL_MS = 130;

/** 新格子亮度的初始上升速度（每秒）：配 SPRING_PIXEL_GLOW 冲到约 1.5，随即被按回 1。 */
const POP_GLOW_KICK = 46;

/** 新格子雾范围的初始扩张速度（每秒）：先用软弹簧张到最大，再弹性收小到 0。 */
const POP_HALO_KICK = 32;

/** 字符池：只要数字、英文与符号，混入其它语言会让"码雨"走味。 */
const GLYPHS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz!@#$%&*+-=/\\<>?{}[]()";

/** 随机取一个字符。 */
function randomGlyph(): string {
  return GLYPHS[Math.floor(Math.random() * GLYPHS.length)] ?? "0";
}

/** 从某节的位置历史里取 `when` 时刻的坐标（线性插值）；历史不足时退回最早/最新一帧。 */
function sampleHistory(node: SpringNode, when: number): { x: number; y: number } {
  const hist = node.hist;
  const first = hist[0];
  if (!first) return { x: node.x, y: node.y };
  const last = hist[hist.length - 1];
  if (!last || when <= first.t) return { x: first.x, y: first.y };
  if (when >= last.t) return { x: last.x, y: last.y };
  for (let i = 1; i < hist.length; i += 1) {
    const a = hist[i - 1];
    const b = hist[i];
    if (!a || !b || when > b.t) continue;
    const span = b.t - a.t || 1;
    const k = (when - a.t) / span;
    return { x: a.x + (b.x - a.x) * k, y: a.y + (b.y - a.y) * k };
  }
  return { x: last.x, y: last.y };
}

/**
 * 把 token 颜色包成 `rgba(r, g, b, a)`。
 *
 * 借助 canvas 把任意写法（token 变量、hex、rgb 字符串）归一成 `rgb()/rgba()`，
 * 再拼出带透明度的梯度色标 —— 组件里因此不需要写任何色值。
 */
function withAlpha(canvasEl: HTMLCanvasElement | null, value: string, alpha: number): string {
  let normalized = value;
  const probe = canvasEl?.getContext("2d");
  if (probe) {
    probe.fillStyle = value;
    normalized = String(probe.fillStyle);
  }
  const match = normalized.match(/^rgba?\(([^)]+)\)$/);
  const parts = (match?.[1] ?? "255, 255, 255").split(",").map((part) => part.trim());
  const [r = "255", g = "255", b = "255"] = parts;
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

/**
 * 预渲染一张"雾"贴图（64×64 的径向渐变圆）。
 *
 * 每格每帧只做一次 `drawImage` 缩放，比每帧现建 gradient 便宜得多；
 * 颜色仍然只来自 token。
 */
function buildHaze(
  value: string,
  canvasEl: HTMLCanvasElement | null = null,
): HTMLCanvasElement | null {
  if (typeof document === "undefined") return null;
  const size = 64;
  const sheet = document.createElement("canvas");
  sheet.width = size;
  sheet.height = size;
  const ctx = sheet.getContext("2d");
  if (!ctx) return null;
  const center = size / 2;
  const grad = ctx.createRadialGradient(center, center, 1, center, center, center - 1);
  grad.addColorStop(0, withAlpha(canvasEl, value, 0.85));
  grad.addColorStop(0.4, withAlpha(canvasEl, value, 0.35));
  grad.addColorStop(1, withAlpha(canvasEl, value, 0));
  ctx.fillStyle = grad;
  ctx.fillRect(0, 0, size, size);
  return sheet;
}

/**
 * 挡住装饰的元素外框与命中判定都放在 `~/utils/occluders`：墨流也要用同一套
 * 边界（它把元素当成流场里的墙），两处各写一份迟早对不上。
 */

/**
 * 第 i 条轨迹的横向偏移系数（-1..1）。
 *
 * `|u|^1.4`（u 是从中间到两端的归一化位置）：中间几条偏移小、两端偏移大，
 * 看起来像"正态分布"那样中间密、两侧疏；再乘一个 sin 透镜，让首尾收拢、
 * 中段最宽 —— 也就是"从出发点一起拉宽、到终点再收缩"的笔触。
 */
function fanCoefficient(index: number, count: number): number {
  if (count <= 1) return 0;
  const u = (index / (count - 1)) * 2 - 1;
  const bell = Math.sign(u) * Math.pow(Math.abs(u), 1.4);
  const taper = Math.sin((Math.PI * (index + 0.5)) / count);
  return bell * taper;
}

interface SpringNode {
  x: number;
  y: number;
  vx: number;
  vy: number;
  /** 位置历史（ms 时间戳 + 坐标）：后一节按 10ms 的"旧位置"追前一节。 */
  hist: Array<{ t: number; x: number; y: number }>;
}

interface PixelCell {
  /** 剩余寿命（1 → 0）。 */
  life: number;
  /** 当前缩放（1 = 原始大小）。 */
  scale: number;
  /** 缩放速度（每秒）。 */
  velocity: number;
  /** 当前"亮度 / 雾量"（1 = 常态）：激活时被踢高，再弹回。 */
  glow: number;
  /** 亮度的变化速度（每秒）。 */
  glowVelocity: number;
  /** 当前"雾的范围"（1 = 常态）：激活时先张大、再弹性收小。 */
  halo: number;
  /** 雾范围的变化速度（每秒）。 */
  haloVelocity: number;
  /** 最近一次盖章来自第几节（0 = 头结点）：越靠后字号越小，拉开层次。 */
  rank: number;
  /** 当前显示的字符（黑客帝国式 ASCII 码）。 */
  glyph: string;
  /** 下一次换字符的时间戳（ms）。 */
  nextGlyph: number;
  /**
   * 下一次"弹一下"的时间戳（ms）。
   *
   * 指针停在一格上时，这一格每帧都会被重新盖章；如果每次盖章都重置弹簧，
   * 字号就会永远停在"最大"那一档、看起来根本没弹过。所以加一个冷却：
   * 到点了才重新张一次，于是移动中的每一格都在**周期性**地"变大 → 缩回常态"。
   */
  nextPop: number;
}

/**
 * 把像素尾迹挂到一块画布上。
 *
 * `root` 只用来取画布尺寸与指针坐标原点（画布本身 pointer-events: none，
 * 指针事件从 window 收，这样划过内容层时也照样有尾迹）。
 */
export function usePixelTrail(
  canvas: Ref<HTMLCanvasElement | null>,
  root: Ref<HTMLElement | null>,
  options: () => PixelTrailOptions = () => ({}),
  prefersReducedMotion: Ref<boolean>,
): { reset: () => void } {
  const { dispose: disposeMotionQuery } = useMotionPreference();

  const config = () => resolvePixelTrailOptions(options());
  /** 键是 "gx,gy"；每个格子记寿命 + 当前缩放 + 缩放速度（激活时弹一下）。 */
  const cells = new Map<string, PixelCell>();
  const pointer = { x: -9999, y: -9999, active: false, lastMove: 0 };
  /** 指针最近一次的运动方向（单位向量）；静止时保持上一次的方向。 */
  let dirX = 0;
  let dirY = 1;
  let nodes: SpringNode[] = [];
  let raf = 0;
  let lastTs = 0;
  let color = "";
  /** 预渲染的"雾"贴图（径向渐变）：每帧建 gradient 太贵，一张贴图按格缩放贴上。 */
  let haze: HTMLCanvasElement | null = null;
  /** 换字节奏（ms）：移动越快换得越快，停下后放慢，速度本身也走弹簧。 */
  let flickerMs = 140;
  let flickerVel = 0;
  let viewport = { width: 0, height: 0 };
  /** 首屏里"会挡字"的元素外框（已外拓、带圆角）：尾迹到这附近就隐身，给元素留白。 */
  let occluders: Array<{ x: number; y: number; w: number; h: number; radius: number }> = [];
  let occluderScanAt = 0;
  let listening = false;
  let observer: IntersectionObserver | null = null;
  let resizeObserver: ResizeObserver | null = null;
  let visible = true;

  // 「减少动态」经共享解析器落地：reduce 时拿到 duration 0 的落位过渡，直接不挂。
  const headSpring = resolveSpringMotion(SPRING_TRAIL, prefersReducedMotion.value);
  const tailSpring = resolveSpringMotion(SPRING_TRAIL_TAIL, prefersReducedMotion.value);
  const popSpring = resolveSpringMotion(SPRING_PIXEL_POP, prefersReducedMotion.value);
  const glowSpring = resolveSpringMotion(SPRING_PIXEL_GLOW, prefersReducedMotion.value);
  const head = headSpring.type === "spring" ? headSpring : null;
  const tail = tailSpring.type === "spring" ? tailSpring : null;
  const pop = popSpring.type === "spring" ? popSpring : null;
  const glow = glowSpring.type === "spring" ? glowSpring : null;

  /** 在 (x,y) 附近盖一格：抖动让边缘参差，满额后不再新增。 */
  function stamp(x: number, y: number, jitter: number, rank: number): void {
    const cfg = config();
    const gx = Math.floor(x / cfg.cellSize);
    const gy = Math.floor(y / cfg.cellSize);
    if (cells.size >= cfg.maxCells && !cells.has(`${gx},${gy}`)) {
      return;
    }
    const jx = jitter > 0 && Math.random() < 0.3 ? (Math.random() < 0.5 ? -jitter : jitter) : 0;
    const key = `${gx + Math.round(jx)},${gy}`;
    const existing = cells.get(key);
    if (!existing) {
      // 新格子：从原大小被"踢"一下，欠阻尼弹簧会先放大、再弹回原大小。
      cells.set(key, {
        life: 1,
        scale: 1,
        velocity: POP_KICK,
        glow: 1,
        glowVelocity: POP_GLOW_KICK,
        halo: 1,
        haloVelocity: POP_HALO_KICK,
        rank,
        glyph: randomGlyph(),
        nextGlyph: 0,
        nextPop: performance.now() + POP_INTERVAL_MS,
      });
      return;
    }
    // 已有格子：寿命补满。冷却到点（或快过期）才重新弹一次 —— 每次盖章都重置弹簧
    // 会让字号永远卡在最大档，看起来"根本没弹"。
    const now = performance.now();
    const revived = existing.life < 0.35 || now > existing.nextPop;
    existing.life = 1;
    // rank 记的是"**当前**是哪一节在给它盖章"，也就是它离指针有多远。
    // 这里必须直接覆盖，不能用 Math.min 取更小的：
    // 头结点会把整条路都扫一遍，取最小值等于把每一格永久钉在 rank 0，
    // 于是所有格子都取形状曲线的起点值 —— 看起来就是"每处都一样大"。
    existing.rank = rank;
    if (revived) {
      existing.scale = 1;
      existing.velocity = POP_KICK;
      existing.glow = 1;
      existing.glowVelocity = POP_GLOW_KICK;
      existing.halo = 1;
      existing.haloVelocity = POP_HALO_KICK;
      existing.nextPop = now + POP_INTERVAL_MS;
    }
  }

  /** 沿两点补格（半格步长）：快速甩动时尾巴不断线。 */
  function stampSegment(fromX: number, fromY: number, toX: number, toY: number, jitter: number, rank: number): void {
    const cfg = config();
    const dist = Math.hypot(toX - fromX, toY - fromY);
    const steps = Math.min(32, Math.max(1, Math.ceil(dist / (cfg.cellSize * 0.5))));
    for (let i = 0; i <= steps; i += 1) {
      const t = i / steps;
      stamp(fromX + (toX - fromX) * t, fromY + (toY - fromY) * t, jitter, rank);
    }
  }

  /**
   * 扫一遍首屏里"会挡装饰"的元素，记下它们相对画布的外框。
   *
   * 判据（哪些元素算、怎么排除布局容器）放在 `~/utils/occluders`，
   * 墨流用的是同一套：元素在墨流里就是"墙"，两边边界必须一致。
   */
  function refreshOccluders(): void {
    const cover = (root.value?.closest("[data-home-hero], .home-cover") as HTMLElement | null) ?? null;
    occluderScanAt = performance.now();
    occluders = collectOccluders(cover, root.value);
  }


  /**
   * 点是否落在某个元素外框的"外拓 + 圆角"范围内。
   *
   * 先按外拓后的矩形排除，再单独排除圆角外的两小块 —— 视觉上就是"快到元素边缘
   * 就迅速看不见"，而不是把整块元素盖住。
   */
  function isOccluded(x: number, y: number, padding: number): boolean {
    return occluders.some((box) => hitsRoundedBox(x, y, box, padding));
  }

  function resize(): void {
    const element = root.value;
    const el = canvas.value;
    if (!element || !el) return;
    const rect = element.getBoundingClientRect();
    const dpr = Math.min(2, Math.max(1, globalThis.devicePixelRatio || 1));
    viewport = { width: rect.width, height: rect.height };
    el.width = Math.round(rect.width * dpr);
    el.height = Math.round(rect.height * dpr);
    const ctx = el.getContext("2d");
    if (ctx) {
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      color = getComputedStyle(element).getPropertyValue(config().colorVar).trim() || "currentColor";
      haze = buildHaze(color, el);
    }
    refreshOccluders();
  }

  /**
   * 一节弹簧：把结点从当前位置弹向目标。
   *
   * 半隐式欧拉：先算加速度、更新速度、再更新位置。dt 已经在上层钳过，
   * 这里再做一次子步拆分（每步 ≤ 16ms），避免高刷屏上大 dt 直接发散。
   */
  function springStep(node: SpringNode, target: { x: number; y: number }, preset: { stiffness: number; damping: number; mass: number }, dt: number): void {
    let remaining = Math.min(dt, 0.05);
    while (remaining > 0) {
      const stepDt = Math.min(remaining, 0.016);
      remaining -= stepDt;
      const ax = (preset.stiffness * (target.x - node.x) - preset.damping * node.vx) / preset.mass;
      const ay = (preset.stiffness * (target.y - node.y) - preset.damping * node.vy) / preset.mass;
      node.vx += ax * stepDt;
      node.vy += ay * stepDt;
      node.x += node.vx * stepDt;
      node.y += node.vy * stepDt;
    }
  }

  function draw(timestamp: number): void {
    const el = canvas.value;
    const ctx = el?.getContext("2d");
    if (!el || !ctx || !head || !tail || !pop || !glow) return;

    const dt = Math.min(48, Math.max(0, timestamp - lastTs || 16)) / 1000;
    lastTs = timestamp;
    const cfg = config();
    // 元素位置会随视差 / 轮播切换移动：每 700ms 重新扫一遍外框，开销可忽略。
    if (timestamp - occluderScanAt > 700) refreshOccluders();

    if (nodes.length === 0) {
      nodes = Array.from({ length: cfg.trailLength }, () => ({ x: pointer.x, y: pointer.y, vx: 0, vy: 0, hist: [] }));
    }

    // 指针停下后**不立刻停笔**：结点还有速度，就继续盖章，让尾巴一路跟到
    // 指针停住的位置；等结点真正停稳（或指针离开）才停止盖章、只等方块淡出。
    const pointerRecent = pointer.active && timestamp - pointer.lastMove < 120;
    let maxSpeed = 0;
    const prev = nodes.map((n) => ({ x: n.x, y: n.y }));
    // 横向（垂直于运动方向）的偏移方向；指针没动过时退回竖直方向。
    const perpX = -dirY;
    const perpY = dirX;
    for (let i = 0; i < nodes.length; i += 1) {
      const node = nodes[i];
      if (!node) continue;
      // 后段等待前一节 NODE_DELAY_MS（=5ms）：基准取前一节"5ms 前"的位置。
      // 等待短到看不出卡顿，但仍是一节接一节跟上，而不是整条同时动。
      const base = i === 0 ? pointer : sampleHistory(nodes[i - 1] ?? node, timestamp - NODE_DELAY_MS);
      // 再按「正态分布 + 首尾收拢」的系数沿垂直方向散开，形成扇形笔触。
      const offset = cfg.spread * fanCoefficient(i, nodes.length);
      const target = {
        x: base.x + perpX * offset,
        y: base.y + perpY * offset,
      };
      springStep(node, target, i === 0 ? head : tail, dt);
      const speed = Math.hypot(node.vx, node.vy);
      maxSpeed = Math.max(maxSpeed, speed);
      node.hist.push({ t: timestamp, x: node.x, y: node.y });
      const oldest = node.hist[0]?.t ?? timestamp;
      while (node.hist.length > 0 && timestamp - oldest > 200) {
        node.hist.shift();
      }

      if (pointerRecent || speed > 4) {
        // 越靠后的结点盖得越稀，尾巴尖端淡得快，不会拖出一条硬边。
        const jitter = i >= 2 ? 1 : 0;
        stampSegment(prev[i]?.x ?? node.x, prev[i]?.y ?? node.y, node.x, node.y, jitter, i);
      }
    }

    // 换字节奏也做成弹簧：移动越快换得越快、停下后放慢；不是匀速跳字。
    const flickerTarget = pointerRecent || maxSpeed > 4 ? 62 : 320;
    if (head) {
      const acc = (head.stiffness * (flickerTarget - flickerMs) - head.damping * flickerVel) / head.mass;
      flickerVel += acc * dt;
      flickerMs += flickerVel * dt;
      flickerMs = Math.min(500, Math.max(30, flickerMs));
    }

    // 到达终点（结点停稳 / 指针离开）后加速淡出：尾巴先跟到位，再迅速散掉，
    // 而不是留在原地慢慢化开。
    const settled = !pointer.active || (maxSpeed <= 4 && timestamp - pointer.lastMove > 160);
    const decay = ((dt * 1000) / cfg.lifeMs) * (settled ? 2.6 : 1);
    for (const [key, cell] of cells) {
      cell.life -= decay;
      if (cell.life <= 0) {
        cells.delete(key);
        continue;
      }
      // 缩放弹簧：每格独立地"放大 → 弹回"，欠阻尼会来回一两次再停。
      const ax = (pop.stiffness * (1 - cell.scale) - pop.damping * cell.velocity) / pop.mass;
      cell.velocity += ax * dt;
      cell.scale += cell.velocity * dt;
      // 亮度 / 雾量用**另一组**过阻尼参数：冲到顶就立刻按下去（约 90ms 落回），
      // 不是跟着尺寸一路晃。和尺寸共用一条弹簧会让亮度只是"顺带变一下"，看不出来。
      const gax = (glow.stiffness * (1 - cell.glow) - glow.damping * cell.glowVelocity) / glow.mass;
      cell.glowVelocity += gax * dt;
      cell.glow = Math.min(2.4, Math.max(0, cell.glow + cell.glowVelocity * dt));
      // 雾的范围用**软弹簧**（和尺寸同一组）：先张到最大，再越过常态收小一点，
      // 最后落回 —— 这才是"弹性缩小"，而不是跟着亮度线性变小。
      // 雾的范围用软弹簧、目标 **0**：张到最大后一路弹性收小，最后收到 0（不再有光晕），
      // 而不是停在一个常驻的底噪光晕上。
      const hax = (pop.stiffness * (0 - cell.halo) - pop.damping * cell.haloVelocity) / pop.mass;
      cell.haloVelocity += hax * dt;
      cell.halo = Math.min(2.6, Math.max(0, cell.halo + cell.haloVelocity * dt));
      // 到点换字：每格有各自的下一跳时刻（带随机抖动），避免整屏同时闪。
      if (timestamp > cell.nextGlyph) {
        cell.glyph = randomGlyph();
        cell.nextGlyph = timestamp + flickerMs * (0.6 + Math.random() * 0.8);
      }
    }

    ctx.clearRect(0, 0, viewport.width, viewport.height);
    if (color) {
      ctx.fillStyle = color;
      ctx.textAlign = "center";
      ctx.textBaseline = "middle";
      for (const [key, cell] of cells) {
        const comma = key.indexOf(",");
        const gx = Number(key.slice(0, comma));
        const gy = Number(key.slice(comma + 1));
        const baseSize = cfg.cellSize * Math.min(1.9, Math.max(0.35, cell.scale));
        const cx = gx * cfg.cellSize + cfg.cellSize / 2;
        const cy = gy * cfg.cellSize + cfg.cellSize / 2;
        // 碰到首屏其它元素（外拓 4px + 圆角）就先隐身：给元素留白，不把画面堆脏。
        // 只影响"画不画"，不影响结点运动 —— 尾迹的运行方式完全不变。
        if (isOccluded(cx, cy, OCCLUDER_PADDING)) continue;
        // 字号与透明度按「**离指针的实际距离**」取值，不看 rank：
        // rank 是"哪一节在盖章"，和眼睛看到的远近不是一回事（一条拖尾上同一节会
        // 反复盖章），按 rank 上色就会出现"近处最大最亮"这种反直觉结果。
        const dist = Math.hypot(cx - pointer.x, cy - pointer.y);
        const t = Math.min(1, dist / Math.max(1, cfg.cellSize * TRAIL_RADIUS_CELLS));
        const shape = trailShape(t);
        // 同一条曲线归一化到 0–1（峰值 = 1）：亮度与光晕范围都乘它，
        // 于是"最亮最大"只发生在峰值那一圈，贴指针与远端都被压下去。
        const shapeNorm = Math.min(
          1,
          Math.max(0, (shape - GLYPH_START_EM) / (GLYPH_PEAK_EM - GLYPH_START_EM)),
        );
        const distanceGain = 0.18 + 0.82 * shapeNorm;
        // glow 是弹簧量：常态 1，激活时冲高、随后被快速按下去；再乘距离增益。
        const glowLevel = Math.min(1.7, Math.max(0.1, cell.glow)) * distanceGain;
        const restAlpha = Math.min(1, cell.life) * trailAlpha(t);
        const alpha = Math.min(1, restAlpha * glowLevel);
        const fontPx = Math.max(3, Math.round(baseSize * shape));
        // 雾：加法混合叠上去，峰值时是一团真的"亮起来"，而不只是同一个白点的透明度变化。
        // 半径同时受 halo 弹簧与距离增益影响（halo → 0 时贴图为 0px，等于没有光晕）。
        if (haze && cell.halo > 0.02) {
          const haloLevel = Math.min(2.2, cell.halo);
          // 光晕尺寸跟格边挂钩而不是跟字号：字号很小，跟字号走会让光晕几乎看不见。
          const hazeSize = cfg.cellSize * (0.35 + 1.05 * shapeNorm) * haloLevel;
          ctx.globalCompositeOperation = "lighter";
          ctx.globalAlpha = Math.min(
            1,
            restAlpha * Math.min(1, haloLevel * 1.4) * (0.35 + 0.65 * glowLevel),
          );
          ctx.drawImage(haze, cx - hazeSize / 2, cy - hazeSize / 2, hazeSize, hazeSize);
          ctx.globalCompositeOperation = "source-over";
        }
        ctx.globalAlpha = alpha;
        ctx.font = `700 ${fontPx}px "JetBrains Mono", ui-monospace, "SFMono-Regular", monospace`;
        ctx.fillText(cell.glyph, cx, cy);
      }
      ctx.globalAlpha = 1;
    }

    if (visible && (cells.size > 0 || !settled)) {
      raf = requestAnimationFrame(draw);
    } else {
      raf = 0;
    }
  }

  function kick(): void {
    if (raf !== 0 || !visible || prefersReducedMotion.value) return;
    lastTs = performance.now();
    raf = requestAnimationFrame(draw);
  }

  function onPointerMove(event: PointerEvent): void {
    if (event.pointerType !== "" && event.pointerType !== "mouse") return;
    const element = root.value;
    if (!element) return;
    const rect = element.getBoundingClientRect();
    const x = event.clientX - rect.left;
    const y = event.clientY - rect.top;
    if (x < -40 || y < -40 || x > rect.width + 40 || y > rect.height + 40) {
      pointer.active = false;
      return;
    }
    if (pointer.active) {
      // 记录运动方向：横向散开的偏移是沿「垂直于运动方向」铺的。
      const dx = x - pointer.x;
      const dy = y - pointer.y;
      const len = Math.hypot(dx, dy);
      if (len > 2) {
        dirX = dx / len;
        dirY = dy / len;
      }
    }
    pointer.x = x;
    pointer.y = y;
    pointer.active = true;
    pointer.lastMove = performance.now();
    kick();
  }

  function onPointerLeave(): void {
    pointer.active = false;
  }

  function onVisibility(): void {
    if (document.hidden) {
      pointer.active = false;
      cancelLoop();
    } else {
      kick();
    }
  }

  function cancelLoop(): void {
    if (raf !== 0) {
      cancelAnimationFrame(raf);
      raf = 0;
    }
  }

  function reset(): void {
    cancelLoop();
    cells.clear();
    const ctx = canvas.value?.getContext("2d");
    if (ctx) ctx.clearRect(0, 0, viewport.width, viewport.height);
  }

  function start(): void {
    if (listening || prefersReducedMotion.value) return;
    resize();
    nodes = Array.from({ length: config().trailLength }, () => ({
      x: pointer.x,
      y: pointer.y,
      vx: 0,
      vy: 0,
      hist: [],
    }));

    globalThis.addEventListener("pointermove", onPointerMove, { passive: true });
    globalThis.addEventListener("pointerleave", onPointerLeave, { passive: true });
    document.addEventListener("visibilitychange", onVisibility);

    if (root.value && "ResizeObserver" in globalThis) {
      resizeObserver = new ResizeObserver(() => resize());
      resizeObserver.observe(root.value);
    }
    if (root.value && "IntersectionObserver" in globalThis) {
      observer = new IntersectionObserver((entries) => {
        visible = entries.some((entry) => entry.isIntersecting);
        if (visible) kick();
        else cancelLoop();
      });
      observer.observe(root.value);
    }
    listening = true;
  }

  function stop(): void {
    if (!listening) return;
    globalThis.removeEventListener("pointermove", onPointerMove);
    globalThis.removeEventListener("pointerleave", onPointerLeave);
    document.removeEventListener("visibilitychange", onVisibility);
    resizeObserver?.disconnect();
    resizeObserver = null;
    observer?.disconnect();
    observer = null;
    listening = false;
    reset();
  }

  onMounted(() => {
    // 等一帧让首屏布局稳定，再量画布尺寸。
    requestAnimationFrame(() => start());
  });

  watch(prefersReducedMotion, (reduced) => {
    if (reduced) stop();
    else start();
  });

  onBeforeUnmount(() => {
    stop();
    disposeMotionQuery();
  });

  return { reset };
}

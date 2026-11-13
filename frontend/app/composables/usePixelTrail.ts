import { onBeforeUnmount, onMounted, watch, type Ref } from "vue";

import { useMotionPreference } from "~/composables/useMotionPreference";
import { resolveSpringMotion, SPRING_TRAIL, SPRING_TRAIL_TAIL } from "~/utils/motion";

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
 *   - 颜色只从 CSS token 读（默认 `--jelly-yellow`），不写任何色值。
 */

/** 尾迹参数。 */
export interface PixelTrailOptions {
  /** 像素格边长（CSS px）。 */
  cellSize?: number;
  /** 尾巴上的弹簧结点数量。 */
  trailLength?: number;
  /** 方块寿命（毫秒）。 */
  lifeMs?: number;
  /** 最多同时存活多少格（防止快速甩动时铺满画布）。 */
  maxCells?: number;
  /** 取颜色的 CSS 变量名。 */
  colorVar?: string;
}

const DEFAULTS: Required<PixelTrailOptions> = {
  cellSize: 14,
  trailLength: 8,
  lifeMs: 700,
  maxCells: 480,
  colorVar: "--jelly-yellow",
};

interface SpringNode {
  x: number;
  y: number;
  vx: number;
  vy: number;
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

  const config = () => ({ ...DEFAULTS, ...options() });
  /** 键是 "gx,gy"，值是剩余寿命（1 → 0）。 */
  const cells = new Map<string, number>();
  const pointer = { x: -9999, y: -9999, active: false, lastMove: 0 };
  let nodes: SpringNode[] = [];
  let raf = 0;
  let lastTs = 0;
  let color = "";
  let viewport = { width: 0, height: 0 };
  let listening = false;
  let observer: IntersectionObserver | null = null;
  let resizeObserver: ResizeObserver | null = null;
  let visible = true;

  // 「减少动态」经共享解析器落地：reduce 时拿到 duration 0 的落位过渡，直接不挂。
  const headSpring = resolveSpringMotion(SPRING_TRAIL, prefersReducedMotion.value);
  const tailSpring = resolveSpringMotion(SPRING_TRAIL_TAIL, prefersReducedMotion.value);
  const head = headSpring.type === "spring" ? headSpring : null;
  const tail = tailSpring.type === "spring" ? tailSpring : null;

  /** 在 (x,y) 附近盖一格：抖动让边缘参差，满额后不再新增。 */
  function stamp(x: number, y: number, jitter: number): void {
    const cfg = config();
    const gx = Math.floor(x / cfg.cellSize);
    const gy = Math.floor(y / cfg.cellSize);
    if (cells.size >= cfg.maxCells && !cells.has(`${gx},${gy}`)) {
      return;
    }
    const jx = jitter > 0 && Math.random() < 0.3 ? (Math.random() < 0.5 ? -jitter : jitter) : 0;
    cells.set(`${gx + Math.round(jx)},${gy}`, 1);
  }

  /** 沿两点补格（半格步长）：快速甩动时尾巴不断线。 */
  function stampSegment(fromX: number, fromY: number, toX: number, toY: number, jitter: number): void {
    const cfg = config();
    const dist = Math.hypot(toX - fromX, toY - fromY);
    const steps = Math.min(32, Math.max(1, Math.ceil(dist / (cfg.cellSize * 0.5))));
    for (let i = 0; i <= steps; i += 1) {
      const t = i / steps;
      stamp(fromX + (toX - fromX) * t, fromY + (toY - fromY) * t, jitter);
    }
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
    }
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
    if (!el || !ctx || !head || !tail) return;

    const dt = Math.min(48, Math.max(0, timestamp - lastTs || 16)) / 1000;
    lastTs = timestamp;
    const cfg = config();

    if (nodes.length === 0) {
      nodes = Array.from({ length: cfg.trailLength }, () => ({ x: pointer.x, y: pointer.y, vx: 0, vy: 0 }));
    }

    // 目标一律取**上一帧**的结点位置，链式跟随才是一条连续的尾巴；
    // 若取本帧刚算完的位置，后面几节会瞬间追上，尾巴就"缩"回指针了。
    const prev = nodes.map((n) => ({ x: n.x, y: n.y }));
    for (let i = 0; i < nodes.length; i += 1) {
      const node = nodes[i];
      if (!node) continue;
      const target = i === 0 ? pointer : (prev[i - 1] ?? pointer);
      springStep(node, target, i === 0 ? head : tail, dt);

      // 越靠后的结点盖得越稀，尾巴尖端淡得快，不会拖出一条硬边。
      const jitter = i >= 2 ? 1 : 0;
      stampSegment(prev[i]?.x ?? node.x, prev[i]?.y ?? node.y, node.x, node.y, jitter);
    }

    const decay = (dt * 1000) / cfg.lifeMs;
    for (const [key, life] of cells) {
      const next = life - decay;
      if (next <= 0) cells.delete(key);
      else cells.set(key, next);
    }

    ctx.clearRect(0, 0, viewport.width, viewport.height);
    if (color) {
      ctx.fillStyle = color;
      for (const [key, life] of cells) {
        const comma = key.indexOf(",");
        const gx = Number(key.slice(0, comma));
        const gy = Number(key.slice(comma + 1));
        const x = gx * cfg.cellSize;
        const y = gy * cfg.cellSize;
        const alpha = Math.min(1, life);
        ctx.globalAlpha = alpha * 0.16;
        ctx.fillRect(x - 2, y - 2, cfg.cellSize + 4, cfg.cellSize + 4);
        ctx.globalAlpha = alpha * 0.9;
        ctx.fillRect(x + 1, y + 1, cfg.cellSize - 2, cfg.cellSize - 2);
      }
      ctx.globalAlpha = 1;
    }

    const idle = !pointer.active || timestamp - pointer.lastMove > 900;
    if (visible && (cells.size > 0 || !idle)) {
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

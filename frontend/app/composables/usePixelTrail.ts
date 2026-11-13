import { onBeforeUnmount, onMounted, watch, type Ref } from "vue";

import { useMotionPreference } from "~/composables/useMotionPreference";

/**
 * 首屏「像素尾迹」：指针划过时在深墨底上留下一条会淡出的像素方块轨迹。
 *
 * 观感参照 reactbits.dev/animations/pixel-trail，实现是自己写的（不引第三方包）：
 *   - 尾巴由若干个**追着指针跑**的墨点组成，越靠后的点跟得越慢；
 *   - 每个墨点按网格坐标「盖章」，盖章位置带正弦摆动与随机抖动 → 方块边缘参差；
 *   - 每个方块有寿命（约 700ms），逐帧淡出；
 *   - 两层绘制（外圈大而淡、内圈小而实）模拟原效果的辉光，而不是用 shadowBlur
 *     （每格一次模糊太贵，快速甩动时会掉帧）。
 *
 * 与首屏其它装饰同一套纪律：
 *   - 只画 canvas，不碰任何 DOM 布局属性；
 *   - `prefers-reduced-motion: reduce` 下不挂监听、不画一帧；
 *   - 标签页隐藏 / 画布离屏时停帧；没有存活方块且指针静止时也停帧，不留常驻 rAF；
 *   - 颜色只从 CSS token 读（默认 `--jelly-yellow`，与首屏黄色色带同一支），不写任何色值。
 */

/** 尾迹参数。 */
export interface PixelTrailOptions {
  /** 像素格边长（CSS px）。 */
  cellSize?: number;
  /** 尾巴上的墨点数量。 */
  trailLength?: number;
  /** 方块寿命（毫秒）。 */
  lifeMs?: number;
  /** 最多同时存活多少格（防止快速甩动时铺满画布）。 */
  maxCells?: number;
  /** 取颜色的 CSS 变量名。 */
  colorVar?: string;
  /** 正弦摆动幅度（像素格的倍数）。 */
  wave?: number;
}

const DEFAULTS: Required<PixelTrailOptions> = {
  cellSize: 14,
  trailLength: 8,
  lifeMs: 700,
  maxCells: 480,
  colorVar: "--jelly-yellow",
  wave: 0.55,
};

interface Follower {
  x: number;
  y: number;
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
  let followers: Follower[] = [];
  let raf = 0;
  let lastTs = 0;
  let color = "";
  let viewport = { width: 0, height: 0 };
  let listening = false;
  let observer: IntersectionObserver | null = null;
  let resizeObserver: ResizeObserver | null = null;
  let visible = true;

  /** 在 (x,y) 附近盖一格：抖动让边缘参差，满额后不再新增。 */
  function stamp(x: number, y: number, jitter: number): void {
    const cfg = config();
    const gx = Math.floor(x / cfg.cellSize);
    const gy = Math.floor(y / cfg.cellSize);
    if (cells.size >= cfg.maxCells && !cells.has(`${gx},${gy}`)) {
      return;
    }
    const jx = jitter > 0 && Math.random() < 0.45 ? (Math.random() < 0.5 ? -jitter : jitter) : 0;
    cells.set(`${gx + Math.round(jx)},${gy}`, 1);
  }

  /** 沿两点补格：快速甩动时尾巴不断线。 */
  function stampSegment(fromX: number, fromY: number, toX: number, toY: number, jitter: number): void {
    const cfg = config();
    const dist = Math.hypot(toX - fromX, toY - fromY);
    const steps = Math.min(24, Math.max(1, Math.ceil(dist / cfg.cellSize)));
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

  function draw(timestamp: number): void {
    const el = canvas.value;
    const ctx = el?.getContext("2d");
    if (!el || !ctx) return;

    const dt = Math.min(48, Math.max(0, timestamp - lastTs || 16));
    lastTs = timestamp;
    const cfg = config();

    if (followers.length === 0) {
      followers = Array.from({ length: cfg.trailLength }, () => ({ x: pointer.x, y: pointer.y }));
    }

    for (let i = 0; i < followers.length; i += 1) {
      const f = followers[i];
      const lead = i === 0 ? pointer : followers[i - 1];
      if (!f || !lead) continue;
      // 越靠后的点跟得越慢；第一点也留一点滞后，尾迹才有"追"的感觉。
      const lag = 0.36 * (1 - i / (followers.length + 2));
      f.x += (lead.x - f.x) * lag;
      f.y += (lead.y - f.y) * lag;

      const wave = Math.sin(timestamp * 0.004 + i * 0.7) * cfg.cellSize * cfg.wave;
      stampSegment(f.x - (lead.x - f.x) * 4, f.y + wave, f.x, f.y + wave, i > 0 ? 1 : 0);
    }

    const decay = dt / cfg.lifeMs;
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
    followers = Array.from({ length: config().trailLength }, () => ({ x: pointer.x, y: pointer.y }));

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

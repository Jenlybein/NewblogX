import { computed, getCurrentScope, onScopeDispose, ref, watch, type Ref } from "vue";

import {
  REDUCED_MOTION_QUERY,
  defaultMatchMedia,
  type ReducedMotionMediaQuery,
} from "~/composables/useMotionPreference";
import {
  CHARM_FINE_POINTER_QUERY,
  CHARM_IDLE_FRAME_INTERVAL_MS,
  advanceCharmRope,
  charmLayerEnabled,
  charmRopeSettled,
  charmRopeTail,
  charmTetherGeometry,
  createCharmRope,
  normalizeCharmRopeConfig,
  type CharmPoint,
  type CharmRopeConfig,
  type CharmRopeState,
  type CharmTetherGeometry,
} from "~/utils/decoCharm";

/** 指针事件里这里用到的最小结构（便于测试注入假事件）。 */
export interface CursorCharmPointerEvent {
  clientX: number;
  clientY: number;
  relatedTarget?: unknown;
}

/** 宿主监听器的最小签名（window / document 都喂得进来）。 */
type HostListener = (event?: unknown) => void;

/**
 * window（或等价宿主）的最小接口。
 *
 * 只声明挂件真正用到的三种事件：指针移动、指针离开文档（pointerout 且
 * relatedTarget 为空）、窗口失焦。可见性变化在 document 上，见下一个接口。
 */
export interface CursorCharmTarget {
  addEventListener?: (type: string, listener: HostListener, options?: { passive: boolean }) => void;
  removeEventListener?: (type: string, listener: HostListener) => void;
}

/** document 的最小接口：可见性事件 + hidden 标志。 */
export interface CursorCharmVisibilityTarget {
  hidden?: boolean;
  addEventListener?: (type: string, listener: HostListener) => void;
  removeEventListener?: (type: string, listener: HostListener) => void;
}

export interface UseCursorCharmOptions {
  /** 注入 matchMedia（测试与 SSR 用）。 */
  matchMedia?: (query: string) => ReducedMotionMediaQuery | null | undefined;
  /** 注入 window 级事件目标；缺省读 `globalThis`（SSR 下没有，静默关闭）。 */
  target?: CursorCharmTarget | null;
  /** 注入 document 级可见性目标；缺省读 `globalThis.document`。 */
  visibilityTarget?: CursorCharmVisibilityTarget | null;
  /** 后台总开关（从运行时配置来）。缺省视为关闭。 */
  configEnabled?: () => boolean;
  /** 物理参数（从运行时配置来）。缺省回落默认值。 */
  config?: () => Partial<CharmRopeConfig> | null | undefined;
  /** 注入动画帧请求/取消与时钟（测试与 SSR 用）。 */
  requestAnimationFrame?: ((callback: HostListener) => number) | null;
  cancelAnimationFrame?: ((id: number) => void) | null;
  /** 注入定时器（怠速车道用；测试里与动画帧共用同一个虚拟时钟）。 */
  setTimeout?: ((callback: HostListener, delayMs: number) => number) | null;
  clearTimeout?: ((id: number) => void) | null;
  now?: () => number;
}

export interface CursorCharm {
  /** 三条门控同时满足才为 true（精确指针 + 未减少动态 + 后台总开关）。 */
  enabled: Ref<boolean>;
  /** 首次指针移动后才为 true —— 挂件据此淡入。 */
  visible: Ref<boolean>;
  /** 指针当前视口坐标；首移之前是原点。 */
  pointer: Ref<CharmPoint>;
  /** 挂绳路径（SVG 的 `d`）；每帧只写这一个字符串。 */
  tetherPath: Ref<string>;
  /** 本帧的挂绳几何（中心线 / 宽度 / 松弛 / 弧垂）；测试与调试用。 */
  tetherGeometry: Ref<CharmTetherGeometry | null>;
  /** 站标中心（最后一个链节点）；每帧只写这一个坐标。 */
  logoCenter: Ref<CharmPoint>;
  /** 已经推进的物理帧数：怠速车道下约 20/s，满速车道下约 60/s。 */
  physicsFrames: Ref<number>;
  dispose: () => void;
}

/** SSR 下没有 window：返回 null，调用方据此静默关闭。 */
function defaultTarget(): CursorCharmTarget | null {
  return typeof globalThis.addEventListener === "function"
    ? (globalThis as unknown as CursorCharmTarget)
    : null;
}

/** SSR 下没有 document：返回 null，可见性监听静默关闭。 */
function defaultVisibilityTarget(): CursorCharmVisibilityTarget | null {
  const doc = (globalThis as { document?: CursorCharmVisibilityTarget }).document;
  return doc && typeof doc.addEventListener === "function" ? doc : null;
}

function defaultRequestAnimationFrame(callback: HostListener): number {
  return globalThis.requestAnimationFrame(callback as FrameRequestCallback);
}

function defaultCancelAnimationFrame(id: number): void {
  globalThis.cancelAnimationFrame(id);
}

function defaultSetTimeout(callback: HostListener, delayMs: number): number {
  return globalThis.setTimeout(callback as TimerHandler, delayMs);
}

function defaultClearTimeout(id: number): void {
  globalThis.clearTimeout(id);
}

function defaultNow(): number {
  return typeof performance !== "undefined" ? performance.now() : Date.now();
}

/** 把事件坐标收敛成有限点（NaN 传进 SVG 会让整条路径消失）。 */
function eventPoint(event: unknown): CharmPoint {
  const move = event as { clientX?: number; clientY?: number } | undefined;
  return {
    x: Number.isFinite(move?.clientX) ? Number(move?.clientX) : 0,
    y: Number.isFinite(move?.clientY) ? Number(move?.clientY) : 0,
  };
}

/**
 * 光标挂件的接线：门控、指针坐标、Verlet 链循环、停帧、生命周期。
 *
 * 为什么要有这个 composable 而不是把逻辑塞进组件：这段有四件事必须能单测断言 ——
 * "关闭时不绑监听"（运行时成本）、"首移才开跑"（首屏观感）、
 * "收敛后取消动画帧"（静态页面零帧）、"卸载时全部摘掉"（HMR / 路由重挂
 * 不会留下第二份监听或第二个循环）。注入假 media、假宿主、假动画帧与假时钟
 * 之后，这四件事都不依赖真浏览器。
 *
 * 物理状态刻意放在模块内的**普通对象**里（不是 ref）：它每秒被改写 120 次，
 * 让它进响应式系统是纯白烧 CPU；对外只暴露每帧会变的路径与站标坐标。
 */
export function useCursorCharm(options: UseCursorCharmOptions = {}): CursorCharm {
  const getMediaQuery = options.matchMedia ?? defaultMatchMedia;
  const target = options.target === undefined ? defaultTarget() : options.target;
  const visibilityTarget =
    options.visibilityTarget === undefined ? defaultVisibilityTarget() : options.visibilityTarget;
  const configEnabled = options.configEnabled ?? (() => false);
  const requestFrame =
    options.requestAnimationFrame === undefined
      ? defaultRequestAnimationFrame
      : options.requestAnimationFrame;
  const cancelFrame =
    options.cancelAnimationFrame === undefined
      ? defaultCancelAnimationFrame
      : options.cancelAnimationFrame;
  const setTimeoutFrame =
    options.setTimeout === undefined ? defaultSetTimeout : options.setTimeout;
  const clearTimeoutFrame =
    options.clearTimeout === undefined ? defaultClearTimeout : options.clearTimeout;
  const now = options.now ?? defaultNow;

  const finePointerQuery = getMediaQuery(CHARM_FINE_POINTER_QUERY) ?? null;
  const reducedQuery = getMediaQuery(REDUCED_MOTION_QUERY) ?? null;

  const finePointerHover = ref(Boolean(finePointerQuery?.matches));
  const prefersReducedMotion = ref(Boolean(reducedQuery?.matches));
  const pointer = ref<CharmPoint>({ x: 0, y: 0 });
  const visible = ref(false);
  const tetherPath = ref("");
  const tetherGeometry = ref<CharmTetherGeometry | null>(null);
  const logoCenter = ref<CharmPoint>({ x: 0, y: 0 });
  const physicsFrames = ref(0);

  // ── 物理状态：普通对象，不进响应式系统 ──────────────────────────
  let rope: CharmRopeState | null = null;
  let ropeSignature = "";
  /** 已请求、尚未执行（或正在执行）的帧 id；非 null 即"循环在跑"。 */
  let frameId: number | null = null;
  /** 当前待执行帧来自哪条车道：收敛 / 停用时按对应 API 取消。 */
  let frameKind: "raf" | "timer" = "raf";
  /** 上一帧的真实时间（ms）；null 表示循环没在跑。 */
  let lastFrameTime: number | null = null;
  let bound = false;
  let paused = Boolean(visibilityTarget?.hidden);
  /** 失焦 / 指针离开文档后冻结指针端：绝不瞬移，只让链自然收敛。 */
  let frozen = false;
  let disposed = false;

  const switchOn = computed(() => Boolean(configEnabled()));
  const enabled = computed(() =>
    charmLayerEnabled({
      finePointerHover: finePointerHover.value,
      prefersReducedMotion: prefersReducedMotion.value,
      configEnabled: switchOn.value,
    }),
  );

  function handleFinePointerChange(event: { matches: boolean }) {
    finePointerHover.value = event.matches;
  }

  function handleReducedChange(event: { matches: boolean }) {
    prefersReducedMotion.value = event.matches;
  }

  function subscribeMedia() {
    finePointerQuery?.addEventListener?.("change", handleFinePointerChange);
    reducedQuery?.addEventListener?.("change", handleReducedChange);
  }

  function unsubscribeMedia() {
    finePointerQuery?.removeEventListener?.("change", handleFinePointerChange);
    reducedQuery?.removeEventListener?.("change", handleReducedChange);
  }

  /** 建链 / 重建：只有拓扑（节点数、绳长）变了才重建，其余参数原地生效。 */
  function ensureRope(head: CharmPoint): CharmRopeState {
    const config = normalizeCharmRopeConfig(options.config?.());
    const signature =
      `${config.ropeLengthPx}|${config.segments}|${config.gravity}|${config.damping}` +
      `|${config.lineWidthPx}|${config.idleSwayHz}|${config.idleSwayAmp}`;
    if (!rope) {
      rope = createCharmRope(head, config);
      ropeSignature = signature;
      return rope;
    }
    if (signature === ropeSignature) return rope;

    const topologyChanged =
      rope.config.segments !== config.segments || rope.config.ropeLengthPx !== config.ropeLengthPx;
    if (topologyChanged) {
      // 节点数 / 绳长变了，旧节点数组没有意义 —— 整条链在新参数下重建。
      rope = createCharmRope(head, config);
    } else {
      rope.config.gravity = config.gravity;
      rope.config.damping = config.damping;
      // 线宽与轻晃参数也不改拓扑：原地换掉，下一帧的几何 / 力就用新值。
      rope.config.lineWidthPx = config.lineWidthPx;
      rope.config.idleSwayHz = config.idleSwayHz;
      rope.config.idleSwayAmp = config.idleSwayAmp;
    }
    ropeSignature = signature;
    return rope;
  }

  function publish(state: CharmRopeState) {
    // 每帧只产出一份几何：路径 `d` 与站标 `transform` 都从它来，
    // 不会出现"画的是这一份、量的是另一份"。
    const geometry = charmTetherGeometry(
      state.nodes,
      state.config.ropeLengthPx,
      state.config.lineWidthPx,
    );
    tetherGeometry.value = geometry;
    tetherPath.value = geometry.path;
    logoCenter.value = charmRopeTail(state);
  }

  function stopLoop() {
    if (frameId !== null) {
      // 按车道取消：满速车道是 cancelAnimationFrame，怠速车道是 clearTimeout。
      // 浏览器里对已触发的 id 调用 cancel / clear 是空操作，但它让"停帧"
      // 这件事可被测试观察到。两种都保证"待执行帧数归零"。
      if (frameKind === "timer") clearTimeoutFrame?.(frameId);
      else cancelFrame?.(frameId);
      frameId = null;
    }
    lastFrameTime = null;
  }

  /**
   * 排下一帧：轻晃开启时走 ~20fps 的怠速车道（setTimeout 50ms），
   * 否则走满速动画帧车道。裁决要求怠速车道不补帧追赶 —— 这里每次只排
   * 一帧，时间差由固定步长累加器按真实流逝时间推进。
   */
  function scheduleNext(state: CharmRopeState | null) {
    if (state && state.config.idleSwayAmp > 0 && setTimeoutFrame) {
      frameKind = "timer";
      frameId = setTimeoutFrame(frame, CHARM_IDLE_FRAME_INTERVAL_MS);
      return;
    }
    if (!requestFrame) {
      frameId = null;
      return;
    }
    frameKind = "raf";
    frameId = requestFrame(frame);
  }

  function startLoop() {
    if (disposed || !enabled.value || paused || frameId !== null) return;
    lastFrameTime = now();
    scheduleNext(rope);
  }

  function frame() {
    if (disposed) {
      frameId = null;
      return;
    }
    if (!enabled.value || paused) {
      stopLoop();
      return;
    }
    const time = now();
    const deltaSeconds = lastFrameTime === null ? 0 : Math.max(0, time - lastFrameTime) / 1000;
    lastFrameTime = time;

    const state = ensureRope(pointer.value);
    // 掉帧 / 断点回来时由固定步长累加器截断，绝不会把十分钟后台时间补进物理；
    // 怠速车道的一帧是 50ms，会推进 6 个固定子步，同样不追赶。
    advanceCharmRope(state, deltaSeconds, pointer.value);
    publish(state);
    physicsFrames.value += 1;

    const swayOn = state.config.idleSwayAmp > 0;
    // 轻晃开着时不停帧（那会把链冻在一个歪姿势上）；关掉后才允许按能量收敛。
    if (!swayOn && charmRopeSettled(state)) {
      stopLoop();
      return;
    }
    scheduleNext(state);
  }

  function handlePointerMove(event?: unknown) {
    if (!enabled.value || paused) return;
    // 冻结期间（窗口失焦 / 指针已离开文档）不更新指针端：
    // 让它停在最后位置，链自然收敛；等 focus / 指针重新进入再解冻。
    if (frozen) return;
    const point = eventPoint(event);
    pointer.value = point;
    // 首次移动才淡入：页面刚打开时不该先冒出一个突兀的东西，
    // 触屏用户（永远匹配不上精确指针）也因此永远看不到它。
    if (!visible.value) visible.value = true;

    publish(ensureRope(point));
    // 首帧即使能量为 0 也要请求：下一帧才会把头节点钉到新位置，
    // 链的惯性从"手动了"这一刻开始，而不是从某个预设状态开始。
    startLoop();
  }

  function handlePointerOut(event?: unknown) {
    // 元素之间移动时 pointerout 也会触发（relatedTarget 非空）；
    // 只有 relatedTarget 为空才是"离开文档"。
    if ((event as { relatedTarget?: unknown } | undefined)?.relatedTarget != null) return;
    frozen = true;
  }

  function handleBlur() {
    frozen = true;
  }

  function handleThaw() {
    // 窗口重新获得焦点 / 指针重新进入文档：从冻结的位置继续，不做任何补算。
    frozen = false;
  }

  function handleVisibilityChange() {
    paused = Boolean(visibilityTarget?.hidden);
    if (paused) {
      stopLoop();
      return;
    }
    // 恢复可见后从现有状态继续：只重置时间基准，不补算后台时间。
    if (visible.value && rope && !charmRopeSettled(rope)) startLoop();
  }

  function bind() {
    if (bound || !enabled.value) return;
    target?.addEventListener?.("pointermove", handlePointerMove, { passive: true });
    target?.addEventListener?.("pointerover", handleThaw, { passive: true });
    target?.addEventListener?.("pointerout", handlePointerOut, { passive: true });
    target?.addEventListener?.("blur", handleBlur);
    target?.addEventListener?.("focus", handleThaw);
    visibilityTarget?.addEventListener?.("visibilitychange", handleVisibilityChange);
    bound = true;
  }

  function unbind() {
    if (!bound) return;
    target?.removeEventListener?.("pointermove", handlePointerMove);
    target?.removeEventListener?.("pointerover", handleThaw);
    target?.removeEventListener?.("pointerout", handlePointerOut);
    target?.removeEventListener?.("blur", handleBlur);
    target?.removeEventListener?.("focus", handleThaw);
    visibilityTarget?.removeEventListener?.("visibilitychange", handleVisibilityChange);
    bound = false;
  }

  // 媒体监听跟着总开关走：关着的时候连"系统偏好变了"都不需要知道。
  const stopSwitchWatch = watch(
    switchOn,
    (value) => {
      if (value) subscribeMedia();
      else unsubscribeMedia();
    },
    { immediate: true, flush: "sync" },
  );

  // 监听随门控挂/摘，而不是"先绑上再在回调里 early return"：
  // 关掉总开关之后不该还留着一个每次移动都跑的回调。
  const stopWatch = watch(
    enabled,
    (value) => {
      if (value) {
        bind();
      } else {
        unbind();
        stopLoop();
      }
    },
    { immediate: true, flush: "sync" },
  );

  function dispose() {
    if (disposed) return;
    disposed = true;
    stopSwitchWatch();
    stopWatch();
    unbind();
    unsubscribeMedia();
    stopLoop();
  }

  // 组件内自动清理；裸调用（测试）由调用方手动 dispose。
  if (getCurrentScope()) onScopeDispose(dispose);

  return {
    enabled,
    visible,
    pointer,
    tetherPath,
    tetherGeometry,
    logoCenter,
    physicsFrames,
    dispose,
  };
}

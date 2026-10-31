import { computed, getCurrentScope, onScopeDispose, ref, watch, type Ref } from "vue";

import {
  REDUCED_MOTION_QUERY,
  defaultMatchMedia,
  type ReducedMotionMediaQuery,
} from "~/composables/useMotionPreference";
import {
  CHARM_FINE_POINTER_QUERY,
  charmLayerEnabled,
  type CharmPoint,
} from "~/utils/decoCharm";

/** 指针事件里这里用到的最小结构（便于测试注入假事件）。 */
export interface CursorCharmPointerEvent {
  clientX: number;
  clientY: number;
}

/**
 * window 的最小接口。
 *
 * 只声明 pointermove：挂件本票不做物理、不跑动画帧，所以也**不**需要
 * requestAnimationFrame —— 指针动一次就重画一次。
 */
export interface CursorCharmTarget {
  addEventListener?: (
    type: "pointermove",
    listener: (event: CursorCharmPointerEvent) => void,
    options?: { passive: boolean },
  ) => void;
  removeEventListener?: (
    type: "pointermove",
    listener: (event: CursorCharmPointerEvent) => void,
  ) => void;
}

export interface UseCursorCharmOptions {
  /** 注入 matchMedia（测试与 SSR 用）。 */
  matchMedia?: (query: string) => ReducedMotionMediaQuery | null | undefined;
  /** 注入 window 级事件目标；缺省读 `globalThis`（SSR 下没有，静默关闭）。 */
  target?: CursorCharmTarget | null;
  /** 后台总开关（从运行时配置来）。缺省视为关闭。 */
  configEnabled?: () => boolean;
}

export interface CursorCharm {
  /** 三条门控同时满足才为 true（精确指针 + 未减少动态 + 后台总开关）。 */
  enabled: Ref<boolean>;
  /** 首次指针移动后才为 true —— 挂件据此淡入。 */
  visible: Ref<boolean>;
  /** 指针当前视口坐标；首移之前是原点。 */
  pointer: Ref<CharmPoint>;
  dispose: () => void;
}

/** SSR 下没有 window：返回 null，调用方据此静默关闭。 */
function defaultTarget(): CursorCharmTarget | null {
  return typeof globalThis.addEventListener === "function"
    ? (globalThis as unknown as CursorCharmTarget)
    : null;
}

/**
 * 光标挂件的接线：门控、指针坐标、首次淡入、生命周期。
 *
 * 为什么要有这个 composable 而不是把逻辑塞进组件：这段有三件事必须能单测断言 ——
 * "关闭时不绑监听"（运行时成本）、"首移才淡入"（首屏观感）、
 * "卸载时全部摘掉"（HMR / 路由重挂不会留下第二份监听）。注入假 media 与假 window
 * 之后，这三件事都不依赖真浏览器。
 *
 * 本票不做物理，所以这里**没有**动画帧循环：指针不动就没有任何计算。
 * 票 #41 接 Verlet 链时会在这里加固定步长累加器与停帧，门控与清理的形状不变。
 */
export function useCursorCharm(options: UseCursorCharmOptions = {}): CursorCharm {
  const getMediaQuery = options.matchMedia ?? defaultMatchMedia;
  const target = options.target === undefined ? defaultTarget() : options.target;
  const configEnabled = options.configEnabled ?? (() => false);

  const finePointerQuery = getMediaQuery(CHARM_FINE_POINTER_QUERY) ?? null;
  const reducedQuery = getMediaQuery(REDUCED_MOTION_QUERY) ?? null;

  const finePointerHover = ref(Boolean(finePointerQuery?.matches));
  const prefersReducedMotion = ref(Boolean(reducedQuery?.matches));
  const pointer = ref<CharmPoint>({ x: 0, y: 0 });
  const visible = ref(false);

  function handleFinePointerChange(event: { matches: boolean }) {
    finePointerHover.value = event.matches;
  }

  function handleReducedChange(event: { matches: boolean }) {
    prefersReducedMotion.value = event.matches;
  }

  // 总开关关着时**一个监听都不挂**（包括媒体查询监听）：后台关掉之后，
  // 这次页面里除了引导批里那一次很小的 JSON 读取，运行时代价为零。
  const switchOn = computed(() => Boolean(configEnabled()));

  function subscribeMedia() {
    finePointerQuery?.addEventListener?.("change", handleFinePointerChange);
    reducedQuery?.addEventListener?.("change", handleReducedChange);
  }

  function unsubscribeMedia() {
    finePointerQuery?.removeEventListener?.("change", handleFinePointerChange);
    reducedQuery?.removeEventListener?.("change", handleReducedChange);
  }

  const enabled = computed(() =>
    charmLayerEnabled({
      finePointerHover: finePointerHover.value,
      prefersReducedMotion: prefersReducedMotion.value,
      configEnabled: switchOn.value,
    }),
  );

  function handlePointerMove(event: CursorCharmPointerEvent) {
    pointer.value = { x: event.clientX, y: event.clientY };
    // 首次移动才淡入：页面刚打开时不该先冒出一个突兀的东西，
    // 触屏用户（永远匹配不上精确指针）也因此永远看不到它。
    if (!visible.value) visible.value = true;
  }

  // 监听随门控挂/摘，而不是"先绑上再在回调里 early return"：
  // 关掉总开关之后不该还留着一个每次移动都跑的回调。
  let bound = false;

  function bind() {
    if (bound || !enabled.value) return;
    target?.addEventListener?.("pointermove", handlePointerMove, { passive: true });
    bound = true;
  }

  function unbind() {
    if (!bound) return;
    target?.removeEventListener?.("pointermove", handlePointerMove);
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

  const stopWatch = watch(
    enabled,
    (value) => {
      if (value) bind();
      else unbind();
    },
    { immediate: true, flush: "sync" },
  );

  function dispose() {
    stopSwitchWatch();
    stopWatch();
    unbind();
    unsubscribeMedia();
  }

  // 组件内自动清理；裸调用（测试）由调用方手动 dispose。
  if (getCurrentScope()) onScopeDispose(dispose);

  return { enabled, visible, pointer, dispose };
}

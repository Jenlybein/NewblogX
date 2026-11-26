import { computed, getCurrentScope, onScopeDispose, ref, type Ref } from "vue";

import {
  REDUCED_MOTION_QUERY,
  defaultMatchMedia,
  type ReducedMotionMediaQuery,
} from "~/composables/useMotionPreference";
import { specularPercentage, type SpecularRect } from "~/utils/specular";

const MOBILE_QUERY = "(max-width: 768px)";

/** 指针事件里这里用到的元素接口（便于测试注入假元素）。 */
export interface PointerSpecularElement {
  getBoundingClientRect: () => SpecularRect;
  style: { setProperty: (name: string, value: string) => void };
}

export interface PointerSpecularEvent {
  clientX: number;
  clientY: number;
  /** 原生 PointerEvent 的 currentTarget 是 `EventTarget | null`，这里按结构再窄化。 */
  currentTarget: unknown;
}

export interface UsePointerSpecularOptions {
  /** 注入 matchMedia（测试与 SSR 用）。 */
  matchMedia?: (query: string) => ReducedMotionMediaQuery | null | undefined;
}

export interface PointerSpecular {
  /** 是否启用：桌面 + 未开启「减少动态」+ 有 matchMedia（SSR 关闭）。 */
  enabled: Ref<boolean>;
  /** 绑到玻璃元素的 `@pointermove`。 */
  handlePointerMove: (event: PointerSpecularEvent) => void;
  dispose: () => void;
}

/**
 * 指针镜面高光（写入 `--mx/--my`，由玻璃的 `::before` 消费）。
 *
 * 为什么要有这个 composable 而不是每处手写：这段跑在 pointermove 的热路径上，
 * 而它恰恰有两个必须关闭的场景 —— 移动端（每帧一次重绘，省一次就省一次）与
 * 系统「减少动态」（高光是被禁的增益效果）。把开关集中在这里，组件只绑一个 handler。
 * 只写 CSS 自定义属性，不碰布局（不读 width/height 之外的几何）。
 */
export function usePointerSpecular(options: UsePointerSpecularOptions = {}): PointerSpecular {
  const getMediaQuery = options.matchMedia ?? defaultMatchMedia;
  const reducedQuery = getMediaQuery(REDUCED_MOTION_QUERY) ?? null;
  const mobileQuery = getMediaQuery(MOBILE_QUERY) ?? null;

  const reducedMotion = ref(Boolean(reducedQuery?.matches));
  const mobile = ref(Boolean(mobileQuery?.matches));

  function handleReducedChange(event: { matches: boolean }) {
    reducedMotion.value = event.matches;
  }

  function handleMobileChange(event: { matches: boolean }) {
    mobile.value = event.matches;
  }

  reducedQuery?.addEventListener?.("change", handleReducedChange);
  mobileQuery?.addEventListener?.("change", handleMobileChange);

  const hasMediaQuery = Boolean(reducedQuery || mobileQuery);
  const enabled = computed(() => hasMediaQuery && !reducedMotion.value && !mobile.value);

  /**
   * 指针镜面高光：按 rAF 合并，每帧最多写一次。
   *
   * 原来每个 pointermove 都读一次 rect 并写两个自定义属性：`pointermove` 在
   * 120Hz 触控板/高刷鼠标上可以远超帧率，而这两次写入会让整棵 header 子树
   * 重新计算样式（变量是继承的）。合并到每帧后，输入事件只记录坐标。
   */
  let pendingFrame = 0;
  let pendingElement: PointerSpecularElement | null = null;
  let pendingX = 0;
  let pendingY = 0;

  function flush() {
    pendingFrame = 0;
    const element = pendingElement;
    if (!element) return;
    pendingElement = null;

    const rect = element.getBoundingClientRect();
    if (!rect || rect.width <= 0 || rect.height <= 0) return;

    const { mx, my } = specularPercentage(pendingX, pendingY, rect);
    element.style.setProperty("--mx", `${mx}%`);
    element.style.setProperty("--my", `${my}%`);
  }

  function handlePointerMove(event: PointerSpecularEvent) {
    if (!enabled.value) return;

    const element = event.currentTarget as PointerSpecularElement | null;
    if (!element || typeof element.getBoundingClientRect !== "function") return;

    pendingElement = element;
    pendingX = event.clientX;
    pendingY = event.clientY;
    if (!pendingFrame) pendingFrame = requestAnimationFrame(flush);
  }

  function dispose() {
    reducedQuery?.removeEventListener?.("change", handleReducedChange);
    mobileQuery?.removeEventListener?.("change", handleMobileChange);
    if (pendingFrame) cancelAnimationFrame(pendingFrame);
    pendingFrame = 0;
    pendingElement = null;
  }

  if (getCurrentScope()) onScopeDispose(dispose);

  return { enabled, handlePointerMove, dispose };
}

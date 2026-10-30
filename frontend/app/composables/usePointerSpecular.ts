import { computed, getCurrentScope, onScopeDispose, ref, type Ref } from "vue";

import type { ReducedMotionMediaQuery } from "~/composables/useMotionPreference";
import { specularPercentage, type SpecularRect } from "~/utils/specular";

const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";
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
  /** 移动端断点查询，缺省 `(max-width: 768px)`。 */
  mobileQuery?: string;
}

export interface PointerSpecular {
  /** 是否启用：桌面 + 未开启「减少动态」+ 有 matchMedia（SSR 关闭）。 */
  enabled: Ref<boolean>;
  /** 绑到玻璃元素的 `@pointermove`。 */
  handlePointerMove: (event: PointerSpecularEvent) => void;
  dispose: () => void;
}

function defaultMatchMedia(query: string): ReducedMotionMediaQuery | null {
  if (typeof globalThis.matchMedia !== "function") return null;
  return globalThis.matchMedia(query) as unknown as ReducedMotionMediaQuery;
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
  const mobileQuery = getMediaQuery(options.mobileQuery ?? MOBILE_QUERY) ?? null;

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

  function handlePointerMove(event: PointerSpecularEvent) {
    if (!enabled.value) return;

    const element = event.currentTarget as PointerSpecularElement | null;
    if (!element || typeof element.getBoundingClientRect !== "function") return;

    const rect = element.getBoundingClientRect();
    if (!rect || rect.width <= 0 || rect.height <= 0) return;

    const { mx, my } = specularPercentage(event.clientX, event.clientY, rect);
    element.style.setProperty("--mx", `${mx}%`);
    element.style.setProperty("--my", `${my}%`);
  }

  function dispose() {
    reducedQuery?.removeEventListener?.("change", handleReducedChange);
    mobileQuery?.removeEventListener?.("change", handleMobileChange);
  }

  if (getCurrentScope()) onScopeDispose(dispose);

  return { enabled, handlePointerMove, dispose };
}

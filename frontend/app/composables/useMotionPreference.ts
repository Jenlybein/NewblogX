import { getCurrentScope, onScopeDispose, ref, type Ref } from "vue";

import { prefersReducedMotionFrom } from "~/utils/motion";

/**
 * 「减少动态」的媒体查询串。
 *
 * 导出给需要自己再查一次 media 的组合式函数复用（比如 usePointerSpecular 还要查移动
 * 端断点），避免各处重抄同一个字符串。
 */
export const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

/** matchMedia 返回值里这里用到的最小接口（便于测试注入假的 MediaQueryList）。 */
export interface ReducedMotionMediaQuery {
  matches: boolean;
  addEventListener?: (type: "change", listener: (event: { matches: boolean }) => void) => void;
  removeEventListener?: (type: "change", listener: (event: { matches: boolean }) => void) => void;
}

export interface UseMotionPreferenceOptions {
  /** 注入 matchMedia（缺省读 `globalThis.matchMedia`）；测试与 SSR 用。 */
  matchMedia?: (query: string) => ReducedMotionMediaQuery | null | undefined;
}

export interface MotionPreference {
  prefersReducedMotion: Ref<boolean>;
  dispose: () => void;
}

/**
 * 默认的 matchMedia 取值器。
 *
 * 导出给同样需要读 media 的兄弟 composable（用同一个即可，不必各自再写一遍 SSR
 * 兜底）。SSR 没有 matchMedia：此时按"不减少动态"处理（动画仍由 CSS/客户端接管）。
 */
export function defaultMatchMedia(query: string): ReducedMotionMediaQuery | null {
  if (typeof globalThis.matchMedia !== "function") return null;
  return globalThis.matchMedia(query) as unknown as ReducedMotionMediaQuery;
}

/**
 * 「减少动态」的唯一取值来源。
 *
 * 为什么不直接用库里的 `useReducedMotion`：判定逻辑要能单测（纯函数在 utils/motion），
 * 接线要能注入假 matchMedia、要能在销毁时摘监听。组件一律消费这里，不要各自
 * `window.matchMedia`。
 */
export function useMotionPreference(options: UseMotionPreferenceOptions = {}): MotionPreference {
  const getMediaQuery = options.matchMedia ?? defaultMatchMedia;
  const mediaQuery = getMediaQuery(REDUCED_MOTION_QUERY);
  const prefersReducedMotion = ref(prefersReducedMotionFrom(mediaQuery));

  function handleChange(event: { matches: boolean }) {
    prefersReducedMotion.value = event.matches;
  }

  mediaQuery?.addEventListener?.("change", handleChange);

  function dispose() {
    mediaQuery?.removeEventListener?.("change", handleChange);
  }

  // 在组件/composable 作用域里自动清理；裸调用（比如测试）时由调用方手动 dispose。
  if (getCurrentScope()) onScopeDispose(dispose);

  return { prefersReducedMotion, dispose };
}

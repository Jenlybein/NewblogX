import { computed, getCurrentScope, onScopeDispose, ref, watch, type Ref } from "vue";

import {
  useMotionPreference,
  type UseMotionPreferenceOptions,
} from "~/composables/useMotionPreference";
import {
  SCROLL_CHROME_RESTING,
  nextScrollChromeState,
  type ScrollChromeState,
} from "~/utils/scrollChrome";

/** 页头滚动监听需要的最小目标接口（window 或测试注入的假对象）。 */
export interface ScrollChromeTarget {
  scrollY: number;
  addEventListener: (
    type: "scroll",
    listener: () => void,
    options?: { passive?: boolean },
  ) => void;
  removeEventListener: (type: "scroll", listener: () => void) => void;
}

export interface UseScrollChromeOptions {
  /** 滚动目标；缺省 window，SSR / 测试可注入（传 null 表示没有目标）。 */
  target?: ScrollChromeTarget | null;
  /** 转玻璃的阈值（px）。 */
  threshold?: number;
  /** 方向判定迟滞量（px）。 */
  delta?: number;
  /** 注入「减少动态」的 media（测试与 SSR 用）。 */
  motion?: UseMotionPreferenceOptions;
}

export interface ScrollChrome {
  /** 已滚动超过阈值：页头转玻璃。 */
  scrolled: Ref<boolean>;
  /** 下滚隐藏 / 上滚出现。 */
  hidden: Ref<boolean>;
  /** 是否启用（无滚动目标或「减少动态」时为 false）。 */
  enabled: Ref<boolean>;
  /** 手动同步一次（测试与首帧用）。 */
  sync: () => void;
  dispose: () => void;
}

function defaultTarget(): ScrollChromeTarget | null {
  if (typeof window === "undefined") return null;
  return window as unknown as ScrollChromeTarget;
}

/**
 * 吸顶页头的滚动方向 / 阈值接线（替代 ADR-0012 之前 layouts/default.vue 的
 * "页头不是 sticky"注释）。
 *
 * 判定逻辑在 utils/scrollChrome（纯函数 + 单测），这里只负责：监听 scroll、
 * 读 scrollY、把状态映射成 ref、在「减少动态」时整条链路不启用、销毁时摘监听。
 * 页头（AppHeader）是唯一的消费方；列表 / 卡片不许各自读 window.scrollY。
 */
export function useScrollChrome(options: UseScrollChromeOptions = {}): ScrollChrome {
  const target = options.target === undefined ? defaultTarget() : options.target;
  const { prefersReducedMotion, dispose: disposeMotion } = useMotionPreference(options.motion);

  const state = ref<ScrollChromeState>(SCROLL_CHROME_RESTING);
  const scrolled = computed(() => state.value.scrolled);
  const hidden = computed(() => state.value.hidden);

  /** 可写 ref：状态复位由下面的 sync watch 负责（系统切换减少动态时立刻归零）。 */
  const enabled = ref(Boolean(target) && !prefersReducedMotion.value);

  let lastScrollY = target ? Math.max(0, target.scrollY || 0) : 0;

  watch(
    prefersReducedMotion,
    (reduced) => {
      enabled.value = Boolean(target) && !reduced;
    },
    { flush: "sync" },
  );

  watch(
    enabled,
    (on) => {
      if (on) return;
      state.value = SCROLL_CHROME_RESTING;
      lastScrollY = target ? Math.max(0, target.scrollY || 0) : 0;
    },
    { flush: "sync" },
  );

  function sync() {
    if (!target || !enabled.value) {
      state.value = SCROLL_CHROME_RESTING;
      return;
    }

    const current = target.scrollY;
    state.value = nextScrollChromeState(state.value, lastScrollY, current, {
      threshold: options.threshold,
      delta: options.delta,
    });
    lastScrollY = Number.isFinite(current) && current > 0 ? current : 0;
  }

  target?.addEventListener("scroll", sync, { passive: true });
  // 首帧同步一次：刷新在页面中段/恢复滚动位置时，页头要立刻是"已滚过"的状态，
  // 而不是等到用户第一次滚动才转玻璃。
  sync();

  function dispose() {
    target?.removeEventListener("scroll", sync);
    disposeMotion();
  }

  if (getCurrentScope()) onScopeDispose(dispose);

  return { scrolled, hidden, enabled, sync, dispose };
}

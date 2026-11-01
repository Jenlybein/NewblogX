import { computed, getCurrentScope, onScopeDispose, ref, watch, type ComputedRef, type Ref } from "vue";
import { useScroll } from "motion-v";

import {
  useMotionPreference,
  type UseMotionPreferenceOptions,
} from "~/composables/useMotionPreference";
import { clampParallaxOffset } from "~/utils/motion";

/**
 * 滚动距离 → 位移的默认换算比例：滚动约 500px 用满 10px 预算。
 * 比例本身不是"效果强度"，上限永远由 `clampParallaxOffset` 卡住。
 */
const PARALLAX_STRENGTH = 0.02;

/**
 * 这里用到的 MotionValue 最小接口，按 motion-v / framer-motion 的**真实 API**收窄：
 *
 *   · `get()` 读当前值；
 *   · `on("change", cb)` 订阅变化并**返回取消订阅函数**——MotionValue 没有 `off`。
 *
 * 早先的实现把 MotionValue 强转成自带 `off(...)` 的假接口，于是组件卸载（路由切换）
 * 时 `dispose` 调用了不存在的 `off`，抛出未捕获异常并中断视图切换（#80 的 P0 崩溃）。
 * 收窄后的结构化接口同时是测试注入假滚动值的缝（与 `useScrollChrome` 的 `target` 同风格）。
 */
export interface ScrollNumberValue {
  get: () => number;
  on: (type: "change", callback: (value: number) => void) => () => void;
}

export interface UseParallaxOptions {
  /** 注入「减少动态」的 media（测试与 SSR 用）。 */
  motion?: UseMotionPreferenceOptions;
  /** 注入滚动值（缺省读 motion-v 的 `useScroll().scrollY`）；测试与 SSR 用。 */
  scroll?: ScrollNumberValue;
}

export interface Parallax {
  /** 当前视差位移（px，已钳到 ±`--parallax-max`）。 */
  offset: Ref<number>;
  /** 绑到元素的 `:style`（只含 transform）。 */
  parallaxStyle: ComputedRef<{ transform: string }>;
  dispose: () => void;
}

/**
 * 视差 ≤10px（ADR-0012 决策 7）。
 *
 * 用 motion-v 的 `useScroll`（与区块进场的 `useInView` 同一条依赖链，不引新库），
 * 只把滚动量映射成一次 `translateY`，并在两处钳位：换算后钳 ±10px、减少动态归零。
 * 只给布局级装饰元素用：正文段落 / 卡片面不接视差。
 */
export function useParallax(options: UseParallaxOptions = {}): Parallax {
  const { prefersReducedMotion } = useMotionPreference(options.motion);
  const scrollValue: ScrollNumberValue = options.scroll ?? useScroll().scrollY;

  const offset = ref(0);
  let lastScrollY = scrollValue.get();

  function apply(value: number) {
    const current = Number.isFinite(value) ? value : 0;
    lastScrollY = current;
    offset.value = clampParallaxOffset(current * PARALLAX_STRENGTH, prefersReducedMotion.value);
  }

  apply(lastScrollY);
  // 取消订阅是 `on()` 的返回值；卸载时调 `stopScroll()`（MotionValue 没有 `off`）。
  const stopScroll = scrollValue.on("change", apply);
  // 系统切换「减少动态」时立刻归零（不只是新滚动才归零）。
  watch(
    prefersReducedMotion,
    () => apply(lastScrollY),
    { flush: "sync" },
  );

  const parallaxStyle = computed(() => ({
    transform: offset.value === 0 ? "none" : `translateY(${offset.value}px)`,
  }));

  function dispose() {
    stopScroll();
  }

  if (getCurrentScope()) onScopeDispose(dispose);

  return { offset, parallaxStyle, dispose };
}

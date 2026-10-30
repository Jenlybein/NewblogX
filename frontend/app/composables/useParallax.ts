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

/** motion-v 的 MotionValue 里这里用到的最小接口（便于测试与类型收窄）。 */
interface MotionNumberValue {
  get: () => number;
  on: (type: "change", callback: (value: number) => void) => void;
  off: (type: "change", callback: (value: number) => void) => void;
}

export interface UseParallaxOptions {
  /** 滚动距离换算比例，缺省 0.02。 */
  strength?: number;
  /** 注入「减少动态」的 media（测试与 SSR 用）。 */
  motion?: UseMotionPreferenceOptions;
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
  const strength = options.strength ?? PARALLAX_STRENGTH;
  const { prefersReducedMotion } = useMotionPreference(options.motion);
  const { scrollY } = useScroll();
  const scrollValue = scrollY as unknown as MotionNumberValue;

  const offset = ref(0);
  let lastScrollY = scrollValue.get();

  function apply(value: number) {
    const current = Number.isFinite(value) ? value : 0;
    lastScrollY = current;
    offset.value = clampParallaxOffset(current * strength, prefersReducedMotion.value);
  }

  apply(lastScrollY);
  scrollValue.on("change", apply);
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
    scrollValue.off("change", apply);
  }

  if (getCurrentScope()) onScopeDispose(dispose);

  return { offset, parallaxStyle, dispose };
}

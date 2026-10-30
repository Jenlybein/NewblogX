import { staggerItemMotion } from "~/utils/motion";
import {
  useMotionPreference,
  type MotionPreference,
  type UseMotionPreferenceOptions,
} from "~/composables/useMotionPreference";

/**
 * 列表 stagger 入场（首个真实用例：首页「热门标签」）。
 *
 * 用法（后续票 #5–#9 直接复用）：
 *   const { entranceProps } = useStaggerEntrance();
 *   <Motion v-for="(item, i) in items" :key="item.id" v-bind="entranceProps(i)" class="motion-stagger-item">
 *
 * 只动 opacity 与 y；错峰 40ms；开启「减少动态」时时长与延迟归零（一次性出现）。
 * SSR 时 Motion 会把 initial 内联成 opacity:0，因此只用在**非关键内容**上，
 * 并用 app.vue 的 `<noscript>` 兜底样式保证无 JS 时内容可见。
 */
export function useStaggerEntrance(options: UseMotionPreferenceOptions = {}): {
  prefersReducedMotion: MotionPreference["prefersReducedMotion"];
  entranceProps: (index: number) => ReturnType<typeof staggerItemMotion>;
} {
  const { prefersReducedMotion } = useMotionPreference(options);

  return {
    prefersReducedMotion,
    entranceProps: (index: number) => staggerItemMotion(index, prefersReducedMotion.value),
  };
}

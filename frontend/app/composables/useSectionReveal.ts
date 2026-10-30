import { computed, getCurrentScope, onScopeDispose, ref, type ComputedRef, type Ref } from "vue";
import { useInView } from "motion-v";

import {
  useMotionPreference,
  type UseMotionPreferenceOptions,
} from "~/composables/useMotionPreference";
import { MOTION_REVEAL_OFFSET_PX } from "~/utils/motion";
import { sectionRevealStyle } from "~/utils/motion";

export interface UseSectionRevealOptions {
  /** 注入「减少动态」的 media（测试与 SSR 用）。 */
  motion?: UseMotionPreferenceOptions;
  /** 进场位移上限（px），缺省 `--motion-reveal-offset`。 */
  offset?: number;
  /** 触发进场需要露出的比例（0–1），缺省 0.2。 */
  amount?: number;
}

export interface SectionReveal {
  /** 绑到布局级容器的模板 ref（`ref="revealTarget"`）。 */
  target: Ref<HTMLElement | null>;
  /** 是否已经进过视野（`once: true` → 只进一次）。 */
  revealed: Ref<boolean>;
  /** 绑到容器的 `:style`（只含 transform / opacity / transition）。 */
  revealStyle: ComputedRef<ReturnType<typeof sectionRevealStyle>>;
}

/**
 * 区块级滚动进场（motion-v 的 `useInView` + 动效预算）。
 *
 * 用法（只给布局级容器，不给正文段落）：
 *   const { target, revealStyle } = useSectionReveal();
 *   <section ref="target" class="motion-reveal" :style="revealStyle">…</section>
 *
 * 三条硬约束都在这里落地：
 *   1. 位移 ≤12px 且只动 transform/opacity（样式来自 utils/motion 的纯函数）；
 *   2. `once: true` → 单区块只进一次（回滚不会再触发）；
 *   3. 「减少动态」直接可见、无过渡。
 * SSR 的 initial 内联成 opacity:0 由 app.vue 的 `<noscript>` 兜底（`.motion-reveal`）。
 */
export function useSectionReveal(options: UseSectionRevealOptions = {}): SectionReveal {
  const { prefersReducedMotion } = useMotionPreference(options.motion);
  const target = ref<HTMLElement | null>(null);
  const revealed = useInView(target, { once: true, amount: options.amount ?? 0.2 });
  const offset = options.offset ?? MOTION_REVEAL_OFFSET_PX;
  const revealStyle = computed(() =>
    sectionRevealStyle(revealed.value, prefersReducedMotion.value, offset),
  );

  return { target, revealed, revealStyle };
}

import { computed, onMounted, ref, type ComputedRef, type Ref } from "vue";
import { useInView } from "motion-v";

import {
  useMotionPreference,
  type UseMotionPreferenceOptions,
} from "~/composables/useMotionPreference";
import { MOTION_REVEAL_OFFSET_PX, sectionRevealStyle } from "~/utils/motion";

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
  /**
   * 是否已挂载完成。
   *
   * 首次渲染（水合阶段）恒为 false，此时样式强制取 SSR 基线，避免出现
   * "Hydration completed but contains mismatches"（#88）。
   */
  mounted: Ref<boolean>;
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
 *
 * # 为什么样式在挂载前不读真实偏好
 *
 * SSR 阶段没有 matchMedia，"减少动效"恒判为 false；客户端却能读到真实偏好。
 * 若首次渲染就直接用它，开了「减少动效」的用户会拿到与 SSR **不同**的内联
 * 样式，Vue 随即报水合不一致（#88）——这也是首页唯一一处与客户端环境相关
 * 的内联样式来源。
 *
 * 所以首次渲染一律取 SSR 基线（隐藏 + 位移），挂载完成后再切到真实偏好。
 * 代价是开启减少动效的用户会看到"先隐藏、随后立即无过渡出现"的一帧，
 * 视觉上仍是"立即出现"，且只发生一次。
 */
export function useSectionReveal(options: UseSectionRevealOptions = {}): SectionReveal {
  const { prefersReducedMotion } = useMotionPreference(options.motion);
  const target = ref<HTMLElement | null>(null);
  const revealed = useInView(target, { once: true, amount: options.amount ?? 0.2 });
  const offset = options.offset ?? MOTION_REVEAL_OFFSET_PX;

  const mounted = ref(false);
  onMounted(() => {
    mounted.value = true;
  });

  const revealStyle = computed(() =>
    // 挂载前两个布尔量都强制为 false —— 与 SSR 输出完全一致。
    sectionRevealStyle(
      mounted.value && revealed.value,
      mounted.value && prefersReducedMotion.value,
      offset,
    ),
  );

  return { target, revealed, mounted, revealStyle };
}

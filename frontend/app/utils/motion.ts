/**
 * 动效预算的唯一来源（spec 决策 3：统一三档时长 + 单一缓动 + stagger 节奏）。
 *
 * 两条消费路径：
 *   1. CSS 侧：tokens.css 的 `--motion-*` 变量由这里定义的取值镜像（design 测试交叉校验），
 *      路由过渡、hover 等一律 `var(--motion-base)`；
 *   2. motion-v 侧：它要的是秒为单位的 `duration` 和**贝塞尔数组**形式的 `ease`
 *      （CSS 里那种 `cubic-bezier(...)` 字符串不是 framer-motion 认可的写法），
 *      所以这里同时导出同一曲线的两种表示。
 *
 * 「减少动态」不是"放慢"，是**归零**：时长 0、延迟 0，条目一次性出现。
 * 这样 SSR 渲染的 initial 标记与客户端一致（不会因为 reduce 而改 markup 导致 hydration 抖动）。
 */

export const MOTION_DURATIONS_MS = { fast: 120, base: 200, slow: 320 } as const;

export type MotionToken = keyof typeof MOTION_DURATIONS_MS;

/** motion-v / framer-motion 的缓动表示：贝塞尔控制点数组。 */
export const MOTION_EASE: [number, number, number, number] = [0.2, 0.8, 0.2, 1];

/** CSS 的缓动表示：同一条曲线的 `cubic-bezier(...)` 字符串（tokens.css 里用）。 */
export const MOTION_EASE_CSS = `cubic-bezier(${MOTION_EASE.join(", ")})`;

/** 列表 stagger 节奏：相邻条目 40ms。 */
export const MOTION_STAGGER_MS = 40;

/** 列表入场位移量（px）：只动 transform，不做布局动画。 */
export const MOTION_ENTRANCE_OFFSET_PX = 8;

/** 毫秒 → 秒（motion-v 的 duration 单位）。 */
export function motionSeconds(ms: number): number {
  if (!Number.isFinite(ms) || ms <= 0) return 0;
  return Number((ms / 1000).toFixed(3));
}

/** 三档时长按「是否减少动态」解析成毫秒（reduce 时为 0 = 静态）。 */
export function motionDurationMs(token: MotionToken, prefersReducedMotion: boolean): number {
  return prefersReducedMotion ? 0 : MOTION_DURATIONS_MS[token];
}

/** 第 index 个条目的错峰延迟（毫秒）；非法索引钳到 0，避免负延迟/NaN 传到动画里。 */
export function staggerDelayMs(index: number, prefersReducedMotion: boolean): number {
  if (prefersReducedMotion) return 0;
  if (!Number.isFinite(index) || index <= 0) return 0;
  return index * MOTION_STAGGER_MS;
}

/** 单个过渡描述（motion-v 的 `transition` prop）。 */
export function motionTransition(
  token: MotionToken,
  prefersReducedMotion: boolean,
): { duration: number; delay: number; ease: [number, number, number, number] } {
  return {
    duration: motionSeconds(motionDurationMs(token, prefersReducedMotion)),
    delay: 0,
    ease: MOTION_EASE,
  };
}

/** 列表条目的过渡描述：时长取 base 档，delay = index × 40ms。 */
export function staggerTransition(
  index: number,
  prefersReducedMotion: boolean,
): { duration: number; delay: number; ease: [number, number, number, number] } {
  return {
    duration: motionSeconds(motionDurationMs("base", prefersReducedMotion)),
    delay: motionSeconds(staggerDelayMs(index, prefersReducedMotion)),
    ease: MOTION_EASE,
  };
}

export interface StaggerItemMotion {
  initial: { opacity: number; y: number };
  animate: { opacity: number; y: number };
  transition: { duration: number; delay: number; ease: [number, number, number, number] };
}

/**
 * 列表入场的 initial/animate。**只动 opacity 与 y（transform）**，不碰布局属性。
 * reduce 时 initial/animate 保持不变（SSR 与客户端 markup 一致），只把过渡归零。
 */
export function staggerItemMotion(index: number, prefersReducedMotion: boolean): StaggerItemMotion {
  return {
    initial: { opacity: 0, y: MOTION_ENTRANCE_OFFSET_PX },
    animate: { opacity: 1, y: 0 },
    transition: staggerTransition(index, prefersReducedMotion),
  };
}

/** 从 matchMedia 结果解析「是否减少动态」；SSR / 无 matchMedia 时按 false（不减少）兜底。 */
export function prefersReducedMotionFrom(
  mediaQuery: { matches: boolean } | null | undefined,
): boolean {
  return Boolean(mediaQuery?.matches);
}

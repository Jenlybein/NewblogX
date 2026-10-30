import type { CSSProperties } from "vue";

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

/**
 * 区块级滚动进场位移上限（px）：ADR-0012 决策 7 把它放宽到 ≤12px。
 * 与 tokens.css 的 `--motion-reveal-offset` 是同一份预算（design 测试交叉校验）。
 * 后续票用 motion-v 的 useInView 接进场时，一律从这里取上限，不写死数字。
 */
export const MOTION_REVEAL_OFFSET_PX = 12;

/**
 * 视差上限（px）：与 tokens.css 的 `--parallax-max` 同源。
 * 视差只允许出现在首屏封面等布局级容器上，正文段落不进场。
 */
export const PARALLAX_MAX_PX = 10;

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

/** 区块进场的样式（只含 transform / opacity / transition 三个键）。 */
export interface SectionRevealStyle extends CSSProperties {
  opacity: string;
  transform: string;
  transition: string;
}

/** 进场位移钳到 [0, `--motion-reveal-offset`]（负值 / NaN 归零，不做反向进场）。 */
export function clampRevealOffset(offsetPx: number): number {
  if (!Number.isFinite(offsetPx) || offsetPx <= 0) return 0;
  return Math.min(MOTION_REVEAL_OFFSET_PX, offsetPx);
}

/**
 * 区块滚动进场的可观察样式（ADR-0012 决策 7：位移 ≤12px、只动 transform/opacity）。
 *
 * 用内联样式而不是 `@keyframes`：motion-v 的 `useInView` 只告诉我们"进没进视野"，
 * 过渡交给 CSS 变量驱动，时长/缓动仍然只有 tokens.css 一个来源。
 * 「减少动态」下直接可见且没有过渡 —— 不是放慢，是整段归零。
 */
export function sectionRevealStyle(
  revealed: boolean,
  prefersReducedMotion: boolean,
  offsetPx: number = MOTION_REVEAL_OFFSET_PX,
): SectionRevealStyle {
  if (prefersReducedMotion) {
    return { opacity: "1", transform: "none", transition: "none" };
  }

  return {
    opacity: revealed ? "1" : "0",
    transform: revealed ? "none" : `translateY(${clampRevealOffset(offsetPx)}px)`,
    transition:
      "opacity var(--motion-base) var(--motion-ease), transform var(--motion-base) var(--motion-ease)",
  };
}

/**
 * 视差位移钳到 ±`--parallax-max`（ADR-0012 决策 7：视差 ≤10px）。
 * 「减少动态」归零（不做"慢速视差"）。
 */
export function clampParallaxOffset(offsetPx: number, prefersReducedMotion: boolean): number {
  if (prefersReducedMotion) return 0;
  if (!Number.isFinite(offsetPx)) return 0;
  return Math.max(-PARALLAX_MAX_PX, Math.min(PARALLAX_MAX_PX, offsetPx));
}

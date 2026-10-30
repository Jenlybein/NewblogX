/**
 * 吸顶页头的滚动状态机（纯函数）。
 *
 * 为什么单独拿出来：方向感知最容易写成"每次 scroll 事件都翻一次状态"，
 * 结果触控板/惯性滚动的微小抖动会让页头疯狂闪现。这里用一条**带迟滞的方向判定**：
 *   1. 滚动超过阈值 → 页头才转玻璃（顶部 8px 内的抖动不该让页头变形）；
 *   2. 下滚累计超过 `delta` → 隐藏；上滚累计超过 `delta` → 出现；
 *   3. 回到阈值以内 → 一律静止（页头必须可见）。
 *
 * 「减少动态」不是"慢一点"，是**根本不启用**：不转玻璃、不隐藏，静态页头。
 * 判定与 DOM 无关，接线在 composable 里（useScrollChrome）。
 */

/** 超过这个滚动距离才转玻璃（px）。 */
export const SCROLL_CHROME_THRESHOLD_PX = 8;

/** 方向判定需要的累计位移（px）：小于它的反向滚动视为抖动，不翻状态。 */
export const SCROLL_CHROME_DIRECTION_DELTA_PX = 6;

export interface ScrollChromeState {
  /** 已滚动超过阈值：页头转玻璃（吸顶条带）。 */
  scrolled: boolean;
  /** 下滚中：页头隐藏；上滚或回到顶部时出现。 */
  hidden: boolean;
}

/** 静止态：页头可见、不转玻璃。 */
export const SCROLL_CHROME_RESTING: ScrollChromeState = Object.freeze({
  scrolled: false,
  hidden: false,
});

export interface ScrollChromeDecisionOptions {
  /** 转玻璃的滚动阈值（px），缺省 `SCROLL_CHROME_THRESHOLD_PX`。 */
  threshold?: number;
  /** 方向判定迟滞量（px），缺省 `SCROLL_CHROME_DIRECTION_DELTA_PX`。 */
  delta?: number;
  /** 系统「减少动态」：整条链路不启用。 */
  reducedMotion?: boolean;
}

function nonNegative(value: number): number {
  return Number.isFinite(value) && value > 0 ? value : 0;
}

/**
 * 根据「上一次状态 + 上一点滚动位置 + 当前滚动位置」得到下一个状态。
 *
 * 输入被当作**一次滚动事件的两次采样**：`previousScrollY` 是上一次事件的位置，
 * 因此位移 = 当前 - 上一次。负的（iOS 回弹）位置按 0 处理。
 */
export function nextScrollChromeState(
  previous: ScrollChromeState,
  previousScrollY: number,
  scrollY: number,
  options: ScrollChromeDecisionOptions = {},
): ScrollChromeState {
  if (options.reducedMotion) {
    return SCROLL_CHROME_RESTING;
  }

  const threshold = options.threshold ?? SCROLL_CHROME_THRESHOLD_PX;
  const delta = options.delta ?? SCROLL_CHROME_DIRECTION_DELTA_PX;
  const current = nonNegative(scrollY);
  const before = nonNegative(previousScrollY);

  if (current <= threshold) {
    return SCROLL_CHROME_RESTING;
  }

  const movement = current - before;
  let hidden = previous.hidden;

  if (movement >= delta) {
    hidden = true;
  } else if (movement <= -delta) {
    hidden = false;
  }

  // 位移落在 (-delta, delta) 区间：保持原状态（迟滞）。
  return { scrolled: true, hidden };
}

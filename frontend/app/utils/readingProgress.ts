/**
 * 阅读进度/目录高亮的纯逻辑（从 useReadingProgress 抽出来，便于单测）。
 *
 * composable 只负责 DOM/滚动事件，判定规则放在这里：真实浏览器里"某个标题是否已
 * 读过去"依赖 getBoundingClientRect，没法在单测里稳定复现；把阈值与判定抽成纯函数后，
 * 边界（没标题/没测量到/负滚动/内容不足一屏）都能断言。
 */

/** 标题元素 top 小于该值视为"当前章节"（后台吸顶页头高约 88px + 余量）。 */
export const READING_HEADING_THRESHOLD = 164;

/** 取最后一个已越过阈值的标题；一个都没越过时停在第一个。 */
export function resolveActiveHeading(
  headingIds: string[],
  headingTops: Record<string, number>,
  threshold: number = READING_HEADING_THRESHOLD,
): string {
  let active = headingIds[0] ?? "";

  for (const id of headingIds) {
    const top = headingTops[id];
    if (typeof top !== "number") {
      continue;
    }
    if (top - threshold <= 0) {
      active = id;
      continue;
    }
    break;
  }

  return active;
}

/** 滚动进度百分比（0..100）；内容不足一屏时为 0。 */
export function resolveProgressPercent(
  scrollTop: number,
  scrollHeight: number,
  viewportHeight: number,
): number {
  const maxScrollable = scrollHeight - viewportHeight;
  if (!(maxScrollable > 0)) {
    return 0;
  }

  return Math.min(100, Math.max(0, (scrollTop / maxScrollable) * 100));
}

import type { HistoryArticleItem } from "~/types/api";

/**
 * 浏览历史的分组与游标（#241）。
 *
 * 两个判据都从页面里抽出来，因为它们都"错了也不报错"：
 *   - 分组按**本地日历日**算而不是"距今小时数"（今天 00:10 与昨天 23:50 只差
 *     20 分钟，却必须落在两个分组里）；
 *   - 游标直接取列表最后一条的 `updated_at + article_id`（服务端的第二排序键
 *     就是 article_id），前端不另造一个隐藏游标、也不重新格式化时间
 *     （时间戳一格式化就可能被服务端判成非法而返回参数错误）。
 */

/** 分组的显示文案。顺序即时间轴从上到下的顺序。 */
export const HISTORY_GROUP_LABELS = {
  today: "今天",
  yesterday: "昨天",
  recent: "近一周",
  older: "更早",
} as const;

/** 分组键。 */
export type HistoryGroupKey = keyof typeof HISTORY_GROUP_LABELS;

/** 一个分组：键 + 文案 + 该组的卡片（保持传入顺序，即浏览时间倒序）。 */
export interface HistoryGroup {
  key: HistoryGroupKey;
  label: string;
  items: HistoryArticleItem[];
}

/** 分组的固定输出顺序。 */
export const HISTORY_GROUP_ORDER: readonly HistoryGroupKey[] = [
  "today",
  "yesterday",
  "recent",
  "older",
] as const;

/**
 * 本地日历日差（忽略时刻）：今天 0、昨天 1、明天 -1。
 *
 * 用 `Math.round` 而不是截断：跨夏令时的一天可能是 23/25 小时，
 * 四舍五入后仍落在正确的整数日上。
 */
export function calendarDayDiff(now: Date, at: Date): number {
  const startOfDay = (value: Date) =>
    new Date(value.getFullYear(), value.getMonth(), value.getDate()).getTime();
  return Math.round((startOfDay(now) - startOfDay(at)) / 86_400_000);
}

/**
 * 某条浏览记录落在哪个分组。
 *
 * 边界口径：今天 / 昨天 / 最近 7 天（含今天，即日差 2–6）/ 更早（日差 ≥7）。
 * 未来的时间（本机时钟比服务端快）归入「今天」，不另开分组 —— 否则时间轴上
 * 会出现一个今天之前的分组。
 */
export function historyGroupKeyOf(viewedAt: Date, now: Date): HistoryGroupKey {
  const diff = calendarDayDiff(now, viewedAt);
  if (Number.isNaN(diff)) return "older";
  if (diff <= 0) return "today";
  if (diff === 1) return "yesterday";
  if (diff <= 6) return "recent";
  return "older";
}

/**
 * 把一页（或跨页累计的）记录按天分组。
 *
 * 入参已按浏览时间倒序；输出保持该顺序并按固定分组顺序排列。因为分组只由
 * 时间决定，追加下一页不会打断已有分组（同一组只会被 merge 进同一个桶）。
 */
export function groupHistoryItems(
  items: readonly HistoryArticleItem[],
  now: Date = new Date(),
): HistoryGroup[] {
  const buckets = new Map<HistoryGroupKey, HistoryArticleItem[]>();

  for (const item of items) {
    const key = historyGroupKeyOf(new Date(item.updated_at), now);
    const bucket = buckets.get(key);
    if (bucket) {
      bucket.push(item);
    } else {
      buckets.set(key, [item]);
    }
  }

  return HISTORY_GROUP_ORDER.filter((key) => buckets.has(key)).map((key) => ({
    key,
    label: HISTORY_GROUP_LABELS[key],
    items: buckets.get(key) ?? [],
  }));
}

/** 一页的续页游标。 */
export interface HistoryCursor {
  beforeUpdatedAt: string;
  beforeId: string;
}

/**
 * 从一页结果推出续页游标；空页返回 null。
 *
 * 空页返回 null 很重要：拿一个空时间戳去做"加载更多"会被服务端当成半游标
 * （参数错误），而前端若无其事地继续翻页只会得到同一份数据。
 */
export function cursorOfPage(items: readonly HistoryArticleItem[]): HistoryCursor | null {
  const last = items[items.length - 1];
  if (!last) return null;
  return { beforeUpdatedAt: last.updated_at, beforeId: last.article_id };
}

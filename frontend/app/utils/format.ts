import type { SearchArticleItem } from "~/types/api";

/**
 * 站点时区（票 #436，spec #423 #24）。
 *
 * 读者站面向中文读者，`/` 与 `/article/**` 是 SSR：服务端容器跑在 UTC、读者多半
 * 在 UTC+8。日期/时间格式化只要跟随运行环境时区，晚 8 点发布的文章就会在服务端
 * 被算成前一天——同一段文本服务端与浏览器各算一个值，既会水合不一致，也会让
 * "晚上发的文章显示成前一天"。
 *
 * 所以本模块里所有面向读者的日期/时间都显式带 `timeZone`，**不读运行环境时区**。
 * 值与后端 `internal/platform/timezone.Name` 相同（两端各自持有常量，不共享）。
 */
export const SITE_TIME_ZONE = "Asia/Shanghai";

export function formatCompactNumber(value?: number) {
  if (value == null) return "0";
  if (value < 1000) return String(value);
  if (value < 10000) return `${(value / 1000).toFixed(1)}k`;
  return `${(value / 10000).toFixed(1)}w`;
}

export const formatCount = formatCompactNumber;

/** 只取日期 `YYYY/MM/DD`（`zh-CN` 的数字日期就是斜杠式）。 */
const DATE_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  timeZone: SITE_TIME_ZONE,
});

/** `MM/DD HH:mm`，24 小时制。 */
const DATE_TIME_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hourCycle: "h23",
  timeZone: SITE_TIME_ZONE,
});

/**
 * 站点时区下的`YYYY-MM-DD HH:mm:ss` / 年 / 月 / 日 / 时 / 分 / 秒的零件。
 *
 * 用同一个 `formatToParts` 取全部零件，下面的日历日键、完整时间戳与分钟时间戳
 * 都从这一份来——各自再建一个 formatter 迟早会有一个漏掉 `timeZone`。
 */
const SITE_PARTS_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23",
  timeZone: SITE_TIME_ZONE,
});

interface SiteDateParts {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
}

/** 一天的毫秒数；站点时区的日历日只需要这个常量。 */
const DAY_MS = 86_400_000;

function siteDatePartsOf(date: Date): SiteDateParts {
  const parts: Record<string, number> = {};
  for (const part of SITE_PARTS_FORMATTER.formatToParts(date)) {
    if (part.type !== "literal") {
      parts[part.type] = Number(part.value);
    }
  }
  return parts as unknown as SiteDateParts;
}

function dateKeyOfParts(parts: SiteDateParts): string {
  return `${String(parts.year)}-${pad2(parts.month)}-${pad2(parts.day)}`;
}

/** 站点时区下的日历日 `YYYY-MM-DD`（日期分组、游标与日历日差用它）。 */
export function siteDateKey(date: Date): string {
  return dateKeyOfParts(siteDatePartsOf(date));
}

/**
 * 站点时区下的年月日数字（给「M月D日」这类中文拼接用）。
 *
 * 不是 `new Date` 的 `getFullYear()` 等本机投影：跨时区时那组值会差一天。
 */
export function siteDateParts(date: Date): { year: number; month: number; day: number } {
  const { year, month, day } = siteDatePartsOf(date);
  return { year, month, day };
}

/** 站点时区下的 `YYYY-MM-DD HH:mm:ss`（悬停完整时间戳）。 */
export function formatSiteTimestamp(date: Date): string {
  const parts = siteDatePartsOf(date);
  return `${dateKeyOfParts(parts)} ${pad2(parts.hour)}:${pad2(parts.minute)}:${pad2(parts.second)}`;
}

/** 站点时区下的 `YYYY-MM-DD HH:mm`（精确到分钟的提示文案）。 */
export function formatSiteMinuteTimestamp(date: Date): string {
  const parts = siteDatePartsOf(date);
  return `${dateKeyOfParts(parts)} ${pad2(parts.hour)}:${pad2(parts.minute)}`;
}

/**
 * 站点时区下的日历日差（忽略时刻）：今天 0、昨天 1、明天 -1。
 *
 * 两个日期各自落到站点日历日，再在 UTC 容器里相减——站点时区没有夏令时，
 * 每天都是 24 小时；用 UTC 只是避免再引入一份本机投影。
 */
export function siteCalendarDayDiff(now: Date, at: Date): number {
  const nowKey = siteDateKey(now);
  const atKey = siteDateKey(at);
  return Math.round((utcMsOfDayKey(nowKey) - utcMsOfDayKey(atKey)) / DAY_MS);
}

/** `YYYY-MM-DD` → UTC 零点毫秒（只用来做日历日算术）。 */
function utcMsOfDayKey(key: string): number {
  const [year, month, day] = key.split("-").map(Number);
  return Date.UTC(year!, (month ?? 1) - 1, day ?? 1);
}

function pad2(value: number) {
  return String(value).padStart(2, "0");
}

export function formatDateLabel(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return DATE_FORMATTER.format(date);
}

/**
 * 相对时间（"x 分钟前"）。
 *
 * **只给客户端路径用**：同一份文本在 SSR 与 hydration 之间会随时间漂移，把它放进
 * SSR 页面会直接造成水合文本不一致。当前读者站没有任何 SSR 页面（`/`、
 * `/article/**`）渲染它；将来若要上首屏，必须对这段文本显式声明
 * `suppressHydrationWarning` 并写明原因。`now` 可注入，测试里不摸真实时钟。
 */
export function formatRelativeLabel(value?: string, now: number = Date.now()) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const diff = now - date.getTime();
  const hour = 1000 * 60 * 60;
  const day = hour * 24;

  if (diff < hour) return `${Math.max(1, Math.floor(diff / (1000 * 60)))} 分钟前`;
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`;
  if (diff < day * 7) return `${Math.floor(diff / day)} 天前`;
  return formatDateLabel(value);
}

/** `MM/DD HH:mm`（列表行、评论、弹窗里的精确时间）。 */
export function formatDateTimeLabel(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return DATE_TIME_FORMATTER.format(date);
}

export function getArticleSummary(article: Pick<SearchArticleItem, "abstract" | "highlight">) {
  return article.highlight?.abstract || article.abstract || "暂无摘要";
}

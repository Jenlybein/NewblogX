import type { SearchArticleItem } from "~/types/api";

export function formatCompactNumber(value?: number) {
  if (value == null) return "0";
  if (value < 1000) return String(value);
  if (value < 10000) return `${(value / 1000).toFixed(1)}k`;
  return `${(value / 10000).toFixed(1)}w`;
}

export const formatCount = formatCompactNumber;

export function formatDateLabel(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(date);
}

/**
 * 只写日期的短格式：`2026/11/11`。
 *
 * 与 formatDateLabel 的区别是分隔符与"不补前缀"：文章卡、收藏夹、分类弹窗里的
 * 日期只回答"哪天"，不加"发布于 / 更新于"这类前缀（票 345 的口径决定）。
 * 用本地时区取年月日 —— 卡片上的日期是给人看的，不是给机器比对的。
 */
export function dateOnlyLabel(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}/${month}/${day}`;
}
export function formatRelativeLabel(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const diff = Date.now() - date.getTime();
  const hour = 1000 * 60 * 60;
  const day = hour * 24;

  if (diff < hour) return `${Math.max(1, Math.floor(diff / (1000 * 60)))} 分钟前`;
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`;
  if (diff < day * 7) return `${Math.floor(diff / day)} 天前`;
  return formatDateLabel(value);
}

export function formatDateTimeLabel(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function getArticleSummary(article: Pick<SearchArticleItem, "abstract" | "highlight">) {
  return article.highlight?.abstract || article.abstract || "暂无摘要";
}

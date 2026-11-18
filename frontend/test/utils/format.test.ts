import { afterEach, describe, expect, it, vi } from "vitest";
import {
  SITE_TIME_ZONE,
  formatCompactNumber,
  formatDateLabel,
  formatDateTimeLabel,
  formatRelativeLabel,
  getArticleSummary,
  siteCalendarDayDiff,
  siteDateKey,
} from "~/utils/format";

/**
 * 时间与站点时区（票 #436，spec #423 #24）。
 *
 * 读者站的 `/` 与 `/article/**` 是 SSR：服务端容器跑在 UTC、读者多半在 UTC+8。
 * 只要格式化跟着运行环境时区走，晚 8 点发布的文章在服务端就会被算成前一天，
 * 水合时同一段文本还会翻成另一天。所以下面每个期望值都是**站点时区
 * （Asia/Shanghai）下的固定输出**——把实现改回 `getFullYear()` 之类本机投影，
 * 在 UTC 的 CI 上会立刻红（这些用例在 `TZ=UTC` 下也应当全绿）。
 */
describe("format utils", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("formats compact counts with the project suffixes", () => {
    expect(formatCompactNumber()).toBe("0");
    expect(formatCompactNumber(999)).toBe("999");
    expect(formatCompactNumber(1500)).toBe("1.5k");
    expect(formatCompactNumber(12000)).toBe("1.2w");
  });

  it("keeps invalid dates readable instead of throwing", () => {
    expect(formatDateLabel("bad-date")).toBe("bad-date");
    expect(formatDateTimeLabel("bad-date")).toBe("bad-date");
  });

  it("formats relative labels from the current time", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-04-16T12:00:00.000Z"));

    expect(formatRelativeLabel("2026-04-16T11:45:00.000Z")).toBe("15 分钟前");
    expect(formatRelativeLabel("2026-04-16T09:00:00.000Z")).toBe("3 小时前");
    expect(formatRelativeLabel("2026-04-14T12:00:00.000Z")).toBe("2 天前");
  });

  it("formats relative labels against an injected clock", () => {
    const now = Date.parse("2026-04-16T12:00:00Z");

    expect(formatRelativeLabel("2026-04-16T11:45:00Z", now)).toBe("15 分钟前");
    expect(formatRelativeLabel("2026-04-16T09:00:00Z", now)).toBe("3 小时前");
    expect(formatRelativeLabel("2026-04-14T12:00:00Z", now)).toBe("2 天前");
  });

  it("prefers highlighted article summaries and falls back to an empty-state label", () => {
    expect(getArticleSummary({ abstract: "摘要", highlight: { abstract: "高亮摘要" } })).toBe("高亮摘要");
    expect(getArticleSummary({ abstract: "摘要", highlight: undefined })).toBe("摘要");
    expect(getArticleSummary({ abstract: "", highlight: undefined })).toBe("暂无摘要");
  });
});

describe("日期/时间显示固定站点时区（Asia/Shanghai）", () => {
  it("站点时区与后端 timezone.Name 同值（spec #423 #24 写死的口径）", () => {
    expect(SITE_TIME_ZONE).toBe("Asia/Shanghai");
  });

  it("UTC 深夜已进入站点次日：同一时刻不会算成前一天", () => {
    // 2026-11-15T16:30Z = 站点 2026-11-16 00:30；UTC 的 CI 本机投影会是 11-15。
    expect(formatDateLabel("2026-11-15T16:30:00Z")).toBe("2026/11/16");
    expect(formatDateTimeLabel("2026-11-15T16:30:00Z")).toBe("11/16 00:30");
  });

  it("站点凌晨发布的文章显示当天，不显示成前一天", () => {
    // 站点 2026-11-11 00:30 = UTC 2026-11-10 16:30。
    expect(formatDateLabel("2026-11-11T00:30:00+08:00")).toBe("2026/11/11");
    expect(formatDateTimeLabel("2026-11-11T00:30:00+08:00")).toBe("11/11 00:30");
  });

  it("晚 8 点发布的文章仍显示站点当天", () => {
    // 站点 21:30 = UTC 13:30：无论输入带 +08 还是 Z，都必须是同一个字符串。
    expect(formatDateLabel("2026-11-10T21:30:00+08:00")).toBe("2026/11/10");
    expect(formatDateLabel("2026-11-10T13:30:00Z")).toBe("2026/11/10");
    expect(formatDateTimeLabel("2026-11-10T21:30:00+08:00")).toBe("11/10 21:30");
    expect(formatDateTimeLabel("2026-11-10T13:30:00Z")).toBe("11/10 21:30");
  });

  it("相对时间超过一周时回落到站点时区日期", () => {
    const now = Date.parse("2026-04-16T12:00:00Z");
    // 2026-03-31T16:30Z = 站点 2026-04-01 00:30。
    expect(formatRelativeLabel("2026-03-31T16:30:00Z", now)).toBe("2026/04/01");
  });
});

describe("站点日历日：日期分组与日历日差只有这一个口径", () => {
  it("siteDateKey 按站点时区取日：UTC 串与 +08 串指向同一时刻时得到同一个键", () => {
    expect(siteDateKey(new Date("2026-11-15T16:30:00Z"))).toBe("2026-11-16");
    expect(siteDateKey(new Date("2026-11-16T00:30:00+08:00"))).toBe("2026-11-16");
  });

  it("siteCalendarDayDiff 以站点午夜为界：23:59 与次日 00:01 差 1 天", () => {
    // 站点 2026-11-05 00:01 = UTC 2026-11-04 16:01：本机（UTC）投影会把两者算成同一天。
    expect(
      siteCalendarDayDiff(
        new Date("2026-11-05T00:01:00+08:00"),
        new Date("2026-11-04T23:59:00+08:00"),
      ),
    ).toBe(1);
  });

  it("未来时间得到负数（客户端时钟快时「明天」不会被算成今天之后的新分组）", () => {
    expect(
      siteCalendarDayDiff(
        new Date("2026-11-05T12:00:00+08:00"),
        new Date("2026-11-06T08:00:00+08:00"),
      ),
    ).toBe(-1);
  });
});

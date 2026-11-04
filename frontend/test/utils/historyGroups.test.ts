import { describe, expect, it } from "vitest";

import type { HistoryArticleItem } from "~/types/api";
import {
  calendarDayDiff,
  cursorOfPage,
  groupHistoryItems,
  historyGroupKeyOf,
} from "~/utils/historyGroups";

/**
 * 历史记录按天分组（#241）。
 *
 * 分组的判据是**本地日历日**差，不是"距今多少小时"：今天 00:10 读的文章和昨天
 * 23:50 读的只差 20 分钟，却必须落在「今天」和「昨天」两个分组里。写错不会报错，
 * 只会让分组的边界在每天的凌晨到早上之间漂移 —— 所以用固定时刻钉住。
 */
function item(articleId: string, updatedAt: string): HistoryArticleItem {
  return {
    article_id: articleId,
    user_id: "u1",
    title: `文章${articleId}`,
    cover: "",
    nickname: "浏览者",
    avatar: "",
    updated_at: updatedAt,
  };
}

// 本地时间构造：`new Date(2026, 10, 4, 12)` 是 2026-11-04 12:00（本地时区）。
function local(year: number, month: number, day: number, hour = 12, minute = 0) {
  return new Date(year, month - 1, day, hour, minute);
}

const NOW = local(2026, 11, 4);

describe("历史分组：今天 / 昨天 / 近一周 / 更早", () => {
  it("按本地日历日差归类，顺序固定", () => {
    const groups = groupHistoryItems(
      [
        item("a1", local(2026, 11, 4, 9).toISOString()), // 今天 09:00
        item("a2", local(2026, 11, 3, 23, 50).toISOString()), // 昨天 23:50（只差 9 小时）
        item("a3", local(2026, 11, 1, 10).toISOString()), // 3 天前 → 近一周
        item("a4", local(2026, 10, 27, 8).toISOString()), // 8 天前 → 更早
      ],
      NOW,
    );

    expect(groups.map((group) => group.label)).toEqual(["今天", "昨天", "近一周", "更早"]);
    expect(groups.map((group) => group.items.map((entry) => entry.article_id))).toEqual([
      ["a1"],
      ["a2"],
      ["a3"],
      ["a4"],
    ]);
  });

  it("近一周的边界：6 天前仍在「近一周」，7 天前落到「更早」", () => {
    expect(historyGroupKeyOf(local(2026, 10, 29), NOW)).toBe("recent"); // 6 天前
    expect(historyGroupKeyOf(local(2026, 10, 28), NOW)).toBe("older"); // 7 天前
    expect(historyGroupKeyOf(local(2026, 11, 1), NOW)).toBe("recent"); // 3 天前
    expect(historyGroupKeyOf(local(2026, 11, 3), NOW)).toBe("yesterday");
    expect(historyGroupKeyOf(local(2026, 11, 4), NOW)).toBe("today");
  });

  it("未来时间（本机时钟偏差）归入「今天」，不新开分组", () => {
    expect(historyGroupKeyOf(local(2026, 11, 5, 8), NOW)).toBe("today");

    const groups = groupHistoryItems(
      [item("a1", local(2026, 11, 4, 9).toISOString()), item("a2", local(2026, 11, 5, 8).toISOString())],
      NOW,
    );
    expect(groups.map((group) => group.label)).toEqual(["今天"]);
    expect(groups[0]!.items).toHaveLength(2);
  });

  it("空列表没有分组；分组按固定顺序输出，不因输入顺序变化", () => {
    expect(groupHistoryItems([], NOW)).toEqual([]);

    const groups = groupHistoryItems(
      [item("a1", local(2026, 10, 1).toISOString()), item("a2", local(2026, 11, 4).toISOString())],
      NOW,
    );
    expect(groups.map((group) => group.key)).toEqual(["today", "older"]);
  });

  it("日历日差不含时分：同一天的任意时刻差都是 0", () => {
    expect(calendarDayDiff(NOW, local(2026, 11, 4, 0, 1))).toBe(0);
    expect(calendarDayDiff(NOW, local(2026, 11, 5, 23, 59))).toBe(-1); // "明天"的 23:59
    expect(calendarDayDiff(NOW, local(2026, 11, 3, 23, 59))).toBe(1);
  });
});

describe("跨页连续：追加下一页不打断分组", () => {
  it("两页拼起来后，同一天的文章合并进同一个分组而不是出现两个「今天」", () => {
    const page1 = [item("a1", local(2026, 11, 4, 11).toISOString()), item("a2", local(2026, 11, 3, 9).toISOString())];
    const page2 = [item("a3", local(2026, 11, 4, 8).toISOString()), item("a4", local(2026, 10, 20).toISOString())];

    const appended = groupHistoryItems([...page1, ...page2], NOW);

    expect(appended.map((group) => group.label)).toEqual(["今天", "昨天", "更早"]);
    // 分页按浏览时间倒序，所以第 2 页的今天（08:00）排在第 1 页的今天（11:00）之后。
    expect(appended[0]!.items.map((entry) => entry.article_id)).toEqual(["a1", "a3"]);
    expect(appended[1]!.items.map((entry) => entry.article_id)).toEqual(["a2"]);
  });

  it("第二排序键与游标同口径：同刻按 article_id 倒序，分组内顺序不被重排改变", () => {
    const halfHour = local(2026, 11, 4, 10).toISOString();
    const groups = groupHistoryItems([item("b", halfHour), item("c", halfHour), item("a", halfHour)], NOW);

    expect(groups[0]!.items.map((entry) => entry.article_id)).toEqual(["b", "c", "a"]);
  });
});

describe("游标：直接用列表最后一条的两个字段续页", () => {
  it("从一页结果推出游标", () => {
    const page = [
      item("a1", "2026-11-04T09:00:00+08:00"),
      item("a2", "2026-11-03T20:00:00+08:00"),
    ];

    expect(cursorOfPage(page)).toEqual({
      beforeUpdatedAt: "2026-11-03T20:00:00+08:00",
      beforeId: "a2",
    });
  });

  it("空页没有游标（避免用空值把服务端当第一页 / 或变成半游标参数错误）", () => {
    expect(cursorOfPage([])).toBeNull();
  });
});

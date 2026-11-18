import { describe, expect, it } from "vitest";
import type { ChatMessageItem } from "~/types/api";
import {
  buildChatTimeline,
  chatDateSeparatorLabel,
  fullChatTimestamp,
  isSameChatGroup,
  mergeChatMessages,
} from "~/utils/chatTimeline";

/**
 * 私信时间线的纯函数对账单。
 *
 * 日期口径固定站点时区（`Asia/Shanghai`，票 #436）：所有时间字面量都带 `+08:00`
 * 或 `Z` 显式偏移，「今天 / 昨天 / 跨午夜」这些边界不再依赖 `Date` 的本机投影——
 * CI 跑在 UTC 时旧的本地实现会在这里红。
 */

function message(id: string, sendTime: string, overrides: Partial<ChatMessageItem> = {}): ChatMessageItem {
  return {
    id,
    sender_id: "u2",
    receiver_id: "u1",
    session_id: "chat:u1:u2",
    content: `内容-${id}`,
    send_time: sendTime,
    msg_status: 1,
    msg_type: 1,
    is_self: false,
    is_read: false,
    ...overrides,
  };
}

const T1 = "2026-11-05T10:00:00.000000000+08:00";
const T2 = "2026-11-05T10:01:00+08:00";
const T3 = "2026-11-05T10:02:00+08:00";

describe("buildChatTimeline（唯一持有者交给面板的结果）", () => {
  it("按自然日切段：每天第一条之前插一个日期分隔条，同一天只有一个", () => {
    const day1a = message("a", "2026-11-04T23:58:00+08:00");
    const day1b = message("b", "2026-11-04T23:59:00+08:00");
    const day2 = message("c", "2026-11-05T00:01:00+08:00");
    const now = new Date("2026-11-05T12:00:00+08:00");

    const timeline = buildChatTimeline([day1a, day1b, day2], now);
    const kinds = timeline.map((entry) => entry.kind);

    expect(kinds).toEqual(["date", "group", "date", "group"]);
    const dates = timeline.filter((entry) => entry.kind === "date");
    expect(dates).toHaveLength(2);
    expect(dates[1]).toMatchObject({ label: "今天" });
    expect(dates[0]).toMatchObject({ label: "昨天" });
  });

  it("站点午夜切段：UTC 同一天、站点跨天的两条消息分成两段", () => {
    // UTC 2026-11-05 15:59 / 16:00 = 站点 11-05 23:59 / 11-06 00:00。
    const beforeMidnight = message("a", "2026-11-05T15:59:00Z");
    const afterMidnight = message("b", "2026-11-05T16:00:00Z");

    const timeline = buildChatTimeline([beforeMidnight, afterMidnight], new Date("2026-11-06T12:00:00+08:00"));
    const dates = timeline.filter((entry) => entry.kind === "date");

    expect(dates).toHaveLength(2);
    expect(dates[1]).toMatchObject({ label: "今天" });
    expect(dates[0]).toMatchObject({ label: "昨天" });
  });

  it("同一发送人同一天 2 分钟内合并成一组，跨人 / 超窗口拆组（不重排入参）", () => {
    const items = [
      message("a", T1),
      message("b", T2),
      message("c", T3, { sender_id: "u3" }),
      message("d", "2026-11-05T10:10:00+08:00"),
    ];

    const timeline = buildChatTimeline(items);
    const groups = timeline.filter((entry) => entry.kind === "group");

    expect(groups.map((group) => group.entries.map((bubble) => bubble.item.id))).toEqual([
      ["a", "b"],
      ["c"],
      ["d"],
    ]);
    expect(groups.map((group) => group.isSelf)).toEqual([false, false, false]);
  });

  it("回执只挂在最后一条自己发的消息上：已读 / 已送达 / 空", () => {
    const received = message("r", T1);
    const own1 = message("s1", T2, { sender_id: "u1", is_self: true, is_read: false });
    const own2 = message("s2", T3, { sender_id: "u1", is_self: true, is_read: true });

    const receipts = buildChatTimeline([received, own1, own2])
      .flatMap((entry) => (entry.kind === "group" ? entry.entries : []))
      .map((bubble) => bubble.receipt);

    // 收到的那条没有回执；更早的自己消息也没有 —— 只有最后一条自己发的才带。
    expect(receipts).toEqual(["", "", "已读"]);
  });

  it("空数组 → 空时间线", () => {
    expect(buildChatTimeline([])).toEqual([]);
  });
});

describe("mergeChatMessages", () => {
  it("已存在的行按 id 替换（保留原位置），新行追加到尾部", () => {
    const current = [message("a", T1), message("b", T2), message("c", T3)];
    const incoming = [
      message("c", T3, { is_read: true, msg_status: 3 }),
      message("d", "2026-11-05T10:03:00+08:00"),
    ];

    const merged = mergeChatMessages(current, incoming);

    expect(merged.map((item) => item.id)).toEqual(["a", "b", "c", "d"]);
    expect(merged[2]?.is_read).toBe(true);
    expect(merged[2]?.msg_status).toBe(3);
    expect(current[2]?.is_read).toBe(false); // 纯函数：不修改入参
  });

  it("绝不重排累积历史（id 是 FNV 字符串，不能按数字比大小）", () => {
    // 故意把 id 的数值顺序与时间顺序打乱。
    const current = [message("999", T1), message("1", T2), message("500", T3)];
    const incoming = [message("1", T2, { is_read: true })];

    expect(mergeChatMessages(current, incoming).map((item) => item.id)).toEqual(["999", "1", "500"]);
  });

  it("去重：current 内重复 id 只留第一条，incoming 内重复 id 后出现的生效", () => {
    const current = [message("a", T1), message("a", T1, { content: "重复" }), message("b", T2)];
    const incoming = [
      message("c", "2026-11-05T10:03:00+08:00", { content: "旧 c" }),
      message("c", "2026-11-05T10:03:00+08:00", { content: "新 c" }),
    ];

    const merged = mergeChatMessages(current, incoming);

    expect(merged.map((item) => item.id)).toEqual(["a", "b", "c"]);
    expect(merged[0]?.content).toBe("内容-a");
    expect(merged[2]?.content).toBe("新 c");
  });

  it("新追加的一批按 send_time 升序，时间相同或非法时保持 incoming 次序", () => {
    const current = [message("a", T1)];
    const incoming = [
      message("late", "2026-11-05T10:05:00+08:00"),
      message("early", "2026-11-05T10:02:00+08:00"),
      message("invalid", ""),
    ];

    expect(mergeChatMessages(current, incoming).map((item) => item.id)).toEqual([
      "a",
      "early",
      "late",
      "invalid",
    ]);
  });

  it("空入参边界", () => {
    const current = [message("a", T1)];
    expect(mergeChatMessages([], [])).toEqual([]);
    expect(mergeChatMessages(current, [])).toEqual(current);
    expect(mergeChatMessages([], current)).toEqual(current);
  });

  it("乐观 temp- 行不会被服务端真实行替换（由 composable 就地换）", () => {
    const current = [message("a", T1), message("temp-1", T2)];
    const merged = mergeChatMessages(current, [message("real-1", T2)]);

    expect(merged.map((item) => item.id)).toEqual(["a", "temp-1", "real-1"]);
  });
});

describe("isSameChatGroup", () => {
  it("同一发送者、同一天、恰好 2 分钟边界 → true", () => {
    const a = message("a", "2026-11-05T10:00:00+08:00");
    const b = message("b", "2026-11-05T10:02:00+08:00");

    expect(isSameChatGroup(a, b)).toBe(true);
  });

  it("超过 2 分钟 1 毫秒 → false", () => {
    const a = message("a", "2026-11-05T10:00:00.000+08:00");
    const b = message("b", "2026-11-05T10:02:00.001+08:00");

    expect(isSameChatGroup(a, b)).toBe(false);
  });

  it("不同发送者 → false", () => {
    const a = message("a", T1);
    const b = message("b", T2, { sender_id: "u3" });

    expect(isSameChatGroup(a, b)).toBe(false);
  });

  it("跨站点午夜即使只差 1 分钟也拆组", () => {
    // 站点 11-05 23:59 = UTC 11-05 15:59；站点 11-06 00:00 = UTC 11-05 16:00。
    // 按本机（UTC）投影这两条落在同一个日历日，旧口径会把它们粘成一组。
    const a = message("a", "2026-11-05T23:59:00+08:00");
    const b = message("b", "2026-11-06T00:00:00+08:00");
    const utcA = message("a", "2026-11-05T15:59:00Z");
    const utcB = message("b", "2026-11-05T16:00:00Z");

    expect(isSameChatGroup(a, b)).toBe(false);
    expect(isSameChatGroup(utcA, utcB)).toBe(false);
  });

  it("跨年、跨月、跨天都拆组", () => {
    expect(isSameChatGroup(
      message("a", "2025-12-31T23:59:00+08:00"),
      message("b", "2026-01-01T00:00:00+08:00"),
    )).toBe(false);
    expect(isSameChatGroup(
      message("a", "2026-10-31T12:00:00+08:00"),
      message("b", "2026-11-01T12:00:00+08:00"),
    )).toBe(false);
  });

  it("时间差取绝对值：传入顺序颠倒也不影响判定", () => {
    const a = message("a", T1);
    const b = message("b", T3);

    expect(isSameChatGroup(a, b)).toBe(true);
    expect(isSameChatGroup(b, a)).toBe(true);
  });

  it("空串 / 非法日期 → false", () => {
    const valid = message("a", T1);
    expect(isSameChatGroup(valid, message("b", ""))).toBe(false);
    expect(isSameChatGroup(valid, message("b", "not-a-date"))).toBe(false);
    expect(isSameChatGroup(message("a", ""), message("b", ""))).toBe(false);
  });
});

describe("chatDateSeparatorLabel", () => {
  // 站点 2026-11-05 15:30（UTC 07:30）。
  const now = new Date("2026-11-05T15:30:00+08:00");

  it("今天 / 昨天（按站点日历日）", () => {
    expect(chatDateSeparatorLabel("2026-11-05T08:00:00+08:00", now)).toBe("今天");
    expect(chatDateSeparatorLabel("2026-11-05T00:01:00+08:00", now)).toBe("今天");
    expect(chatDateSeparatorLabel("2026-11-04T23:59:00+08:00", now)).toBe("昨天");
  });

  it("UTC 同一天、站点已是次日的消息算「今天」", () => {
    // UTC 2026-11-04 17:30 = 站点 2026-11-05 01:30。
    expect(chatDateSeparatorLabel("2026-11-04T17:30:00Z", now)).toBe("今天");
  });

  it("同年更早的日期 → M月D日（不补零）", () => {
    expect(chatDateSeparatorLabel("2026-03-07T09:05:00+08:00", now)).toBe("3月7日");
    expect(chatDateSeparatorLabel("2026-11-03T09:05:00+08:00", now)).toBe("11月3日");
  });

  it("不同年份 → YYYY年M月D日", () => {
    expect(chatDateSeparatorLabel("2025-12-31T23:00:00+08:00", now)).toBe("2025年12月31日");
    expect(chatDateSeparatorLabel("2027-01-02T00:00:00+08:00", now)).toBe("2027年1月2日");
  });

  it("跨月 / 跨年的「昨天」仍按站点日历日判定", () => {
    const newYear = new Date("2026-01-01T12:00:00+08:00");
    expect(chatDateSeparatorLabel("2025-12-31T23:00:00+08:00", newYear)).toBe("昨天");
    const firstOfMarch = new Date("2026-03-01T12:00:00+08:00");
    expect(chatDateSeparatorLabel("2026-02-28T23:00:00+08:00", firstOfMarch)).toBe("昨天");
  });

  it("RFC3339Nano 的 9 位小数秒能解析", () => {
    expect(chatDateSeparatorLabel("2026-11-05T01:07:00.123456789Z", now)).toBe("今天");
  });

  it("非法 / 空日期 → 空串", () => {
    expect(chatDateSeparatorLabel("", now)).toBe("");
    expect(chatDateSeparatorLabel("invalid", now)).toBe("");
  });

  it("now 省略时用当前本地时间，不抛错", () => {
    expect(typeof chatDateSeparatorLabel(T1)).toBe("string");
  });
});

describe("fullChatTimestamp", () => {
  it("完整 YYYY-MM-DD HH:mm:ss，固定站点时区", () => {
    expect(fullChatTimestamp("2026-03-07T09:05:06+08:00")).toBe("2026-03-07 09:05:06");
  });

  it("RFC3339 带时区时转成站点时间（不是运行环境的投影）", () => {
    // 01:07 UTC = 09:07 站点；旧实现用本机投影，CI 在 UTC 时会得到 01:07。
    expect(fullChatTimestamp("2026-11-05T01:07:00.123456789Z")).toBe("2026-11-05 09:07:00");
    // 13:30 UTC = 21:30 站点（晚 8 点的消息）。
    expect(fullChatTimestamp("2026-11-10T13:30:00Z")).toBe("2026-11-10 21:30:00");
  });

  it("非法 / 空日期 → 空串", () => {
    expect(fullChatTimestamp("")).toBe("");
    expect(fullChatTimestamp("not-a-date")).toBe("");
  });
});

import { describe, expect, it } from "vitest";
import {
  formatMuteUntil,
  isMuted,
  muteNotice,
  muteRemainingDays,
  parseMuteUntil,
} from "~/utils/mute";

// 禁言展示口径（票 399 / ADR-0032 决策 8）。
//
// 关键边界：还剩几小时必须显示"1 天"（不能算成 0 天），到期那一刻起一律 0。
describe("mute utils", () => {
  const now = new Date("2026-11-17T12:00:00+08:00");

  it("parses RFC3339 mute_until and rejects invalid values", () => {
    expect(parseMuteUntil("2026-11-24T12:00:00+08:00")?.toISOString()).toBe(
      "2026-11-24T04:00:00.000Z",
    );
    expect(parseMuteUntil(null)).toBeNull();
    expect(parseMuteUntil("")).toBeNull();
    expect(parseMuteUntil("not-a-date")).toBeNull();
  });

  it("counts remaining days by rounding up", () => {
    expect(muteRemainingDays("2026-11-24T12:00:00+08:00", now)).toBe(7);
    expect(muteRemainingDays("2026-11-18T12:00:00+08:00", now)).toBe(1);
    // 只剩 1 小时：向上取整到 1 天，不能显示成 0。
    expect(muteRemainingDays("2026-11-18T11:00:00+08:00", now)).toBe(1);
    expect(muteRemainingDays("2026-11-17T12:00:00+08:00", now)).toBe(0);
    expect(muteRemainingDays("2026-11-16T12:00:00+08:00", now)).toBe(0);
    expect(muteRemainingDays(null, now)).toBe(0);
  });

  it("treats an expired or missing time as not muted", () => {
    expect(isMuted("2026-11-24T12:00:00+08:00", now)).toBe(true);
    expect(isMuted("2026-11-16T12:00:00+08:00", now)).toBe(false);
    expect(isMuted(null, now)).toBe(false);
  });

  it("formats the until time in site time (Asia/Shanghai) to the minute", () => {
    expect(formatMuteUntil("2026-11-24T08:05:00+08:00")).toBe("2026-11-24 08:05");
    // 2026-11-24T00:05Z = 站点 08:05：不跟运行环境时区走，CI 跑 UTC 也是同一个串。
    expect(formatMuteUntil("2026-11-24T00:05:00Z")).toBe("2026-11-24 08:05");
    expect(formatMuteUntil(null)).toBe("");
  });

  it("builds the reader-facing notice with days and until time", () => {
    const notice = muteNotice("2026-11-24T08:05:00+08:00", now);
    expect(notice).toContain("已禁言");
    expect(notice).toContain("还有 7 天");
    expect(notice).toContain("2026-11-24 08:05");
    expect(notice).toContain("不能发文");
    expect(muteNotice(null, now)).toBe("");
    expect(muteNotice("2026-11-16T08:05:00+08:00", now)).toBe("");
  });
});

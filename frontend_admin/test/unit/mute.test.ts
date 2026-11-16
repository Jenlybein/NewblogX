import { describe, expect, it } from "vitest";
import { formatMuteUntil, muteRemainingDays, muteStatusText, parseMuteUntil } from "~/utils/mute";

// 账号管理页的禁言展示口径（票 399 / ADR-0032 决策 8）。
describe("admin mute utils", () => {
  const now = new Date("2026-11-17T12:00:00+08:00");

  it("parses RFC3339 mute_until and rejects invalid values", () => {
    expect(parseMuteUntil("2026-11-24T12:00:00+08:00")?.toISOString()).toBe(
      "2026-11-24T04:00:00.000Z",
    );
    expect(parseMuteUntil(null)).toBeNull();
    expect(parseMuteUntil("oops")).toBeNull();
  });

  it("rounds remaining days up", () => {
    expect(muteRemainingDays("2026-11-24T12:00:00+08:00", now)).toBe(7);
    expect(muteRemainingDays("2026-11-18T11:00:00+08:00", now)).toBe(1);
    expect(muteRemainingDays("2026-11-17T12:00:00+08:00", now)).toBe(0);
    expect(muteRemainingDays(null, now)).toBe(0);
  });

  it("formats the until time to the minute", () => {
    expect(formatMuteUntil("2026-11-24T08:05:00+08:00")).toBe("2026-11-24 08:05");
    expect(formatMuteUntil(null)).toBe("");
  });

  it("builds the admin list label with until time and days", () => {
    expect(muteStatusText("2026-11-24T08:05:00+08:00", now)).toBe(
      "禁言至 2026-11-24 08:05（还有 7 天）",
    );
    expect(muteStatusText(null, now)).toBe("");
    expect(muteStatusText("2026-11-16T08:05:00+08:00", now)).toBe("");
  });
});

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

  // 下面两条**不做时区断言**：展示口径是「本地时区精确到分钟」（这是对的，
  // 用户看到的时间该跟着自己的时钟走），所以期望值必须由同一套本地格式化算出。
  // 硬编码 "08:05" 只在 +08:00 的机器上成立 —— CI 跑 UTC，那条用例会把
  // 门禁自检 job（跑全量测试）一起拖红。
  it("formats the until time to the minute (local timezone)", () => {
    const until = "2026-11-24T08:05:00+08:00";
    const expected = (() => {
      const at = new Date(until);
      const pad = (n: number) => String(n).padStart(2, "0");
      return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())} `
        + `${pad(at.getHours())}:${pad(at.getMinutes())}`;
    })();
    expect(formatMuteUntil(until)).toBe(expected);
    expect(formatMuteUntil(null)).toBe("");
  });

  it("builds the admin list label with until time and days", () => {
    const until = "2026-11-24T08:05:00+08:00";
    // 剩余天数是绝对值（7 天），与机器时区无关；时间部分用同一套格式化。
    expect(muteStatusText(until, now)).toBe(`禁言至 ${formatMuteUntil(until)}（还有 7 天）`);
    expect(muteStatusText(null, now)).toBe("");
    expect(muteStatusText("2026-11-16T08:05:00+08:00", now)).toBe("");
  });
});

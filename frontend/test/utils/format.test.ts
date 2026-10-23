import { afterEach, describe, expect, it, vi } from "vitest";
import {
  formatCompactNumber,
  formatDateLabel,
  formatDateTimeLabel,
  formatRelativeLabel,
  getArticleSummary,
} from "~/utils/format";

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

  it("prefers highlighted article summaries and falls back to an empty-state label", () => {
    expect(getArticleSummary({ abstract: "摘要", highlight: { abstract: "高亮摘要" } })).toBe("高亮摘要");
    expect(getArticleSummary({ abstract: "摘要", highlight: undefined })).toBe("摘要");
    expect(getArticleSummary({ abstract: "", highlight: undefined })).toBe("暂无摘要");
  });
});

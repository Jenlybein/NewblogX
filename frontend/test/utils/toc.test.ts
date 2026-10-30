import { describe, expect, it } from "vitest";

import { formatProgressPercent, tocIndentLevel } from "~/utils/toc";

describe("目录层级缩进（h1/h2 顶格，h3 一级，h4+ 两级）", () => {
  it("把标题级别压成 0/1/2 三档缩进", () => {
    expect(tocIndentLevel(1)).toBe(0);
    expect(tocIndentLevel(2)).toBe(0);
    expect(tocIndentLevel(3)).toBe(1);
    expect(tocIndentLevel(4)).toBe(2);
    expect(tocIndentLevel(5)).toBe(2);
  });

  it("对越界级别不抛错（后端正文可能出现异常级别）", () => {
    expect(tocIndentLevel(0)).toBe(0);
    expect(tocIndentLevel(9)).toBe(2);
  });
});

describe("目录进度文案", () => {
  it("四舍五入成整数百分比", () => {
    expect(formatProgressPercent(0)).toBe("0%");
    expect(formatProgressPercent(33.6)).toBe("34%");
    expect(formatProgressPercent(99.9)).toBe("100%");
  });

  it("越界值被钳制，不会出现 -3% / 103%", () => {
    expect(formatProgressPercent(-3)).toBe("0%");
    expect(formatProgressPercent(103)).toBe("100%");
  });
});

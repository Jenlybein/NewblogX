import { describe, expect, it } from "vitest";

import { formatProgressPercent, resolveTocFollow, tocIndentLevel } from "~/utils/toc";

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

// #100：跟随高亮时**绝不能滚动整页**。
//
// 原实现直接 scrollIntoView，它会滚动所有祖先容器（含 document）。宽屏侧栏
// sticky 时目录常驻视口、缺陷被掩盖；窄屏目录掉到文档底部就开始把整页往下拽，
// 并因为"滚动 → 高亮变化 → 再滚动"形成闭环。
describe("目录跟随的滚动决策", () => {
  const base = {
    visible: true,
    suppressed: false,
    offsetTop: 400,
    scrollTop: 0,
    clientHeight: 380,
    targetHeight: 28,
  };

  it("目标已在容器内可见时不滚动", () => {
    const result = resolveTocFollow({ ...base, offsetTop: 100 });

    expect(result.follow).toBe(false);
    expect(result.scrollTop).toBeNull();
  });

  it("目标在下方时只算容器自己的 scrollTop（不碰整页）", () => {
    const result = resolveTocFollow(base);

    expect(result.follow).toBe(true);
    // 目标底部贴住容器底部：400 + 28 - 380 = 48
    expect(result.scrollTop).toBe(48);
  });

  it("目标在上方时对齐到容器顶部", () => {
    const result = resolveTocFollow({ ...base, scrollTop: 600 });

    expect(result.follow).toBe(true);
    expect(result.scrollTop).toBe(400);
  });

  it("窄屏不渲染目录时完全不滚动", () => {
    // 这是"窄屏不再自己下跳"的判据：目录不可见 → 没有任何滚动。
    const result = resolveTocFollow({ ...base, visible: false });

    expect(result.follow).toBe(false);
    expect(result.scrollTop).toBeNull();
  });

  it("处在抑制窗口内时不滚动（断开回环）", () => {
    // 刚程序化滚动过就别再跟着滚，否则会形成"滚动 → 高亮 → 再滚动"闭环。
    const result = resolveTocFollow({ ...base, suppressed: true });

    expect(result.follow).toBe(false);
    expect(result.scrollTop).toBeNull();
  });

  it("scrollTop 不会为负", () => {
    const result = resolveTocFollow({ ...base, offsetTop: -50 });

    expect(result.scrollTop).toBe(0);
  });
});

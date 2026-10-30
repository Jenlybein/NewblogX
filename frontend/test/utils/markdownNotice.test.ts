import { describe, expect, it } from "vitest";

import { NOTICE_TONES, splitNoticeSegments } from "~/utils/markdownNotice";

describe("提示块（::: tip / note / warning / danger）", () => {
  it("没有提示块时整段都是普通 markdown", () => {
    expect(splitNoticeSegments("普通段落\n\n还有一段")).toEqual([
      { type: "markdown", source: "普通段落\n\n还有一段" },
    ]);
  });

  it("把提示块切成独立段，并保留标题与内容", () => {
    const segments = splitNoticeSegments("前言\n\n::: tip 小贴士\n内容 **重点**\n:::\n\n后记");

    expect(segments.map((segment) => segment.type)).toEqual(["markdown", "notice", "markdown"]);
    const notice = segments[1];
    expect(notice.type === "notice" && notice.tone).toBe("tip");
    expect(notice.type === "notice" && notice.title).toBe("小贴士");
    expect(notice.type === "notice" && notice.source).toBe("内容 **重点**");
    expect(segments[0]?.type === "markdown" && segments[0].source).toContain("前言");
    expect(segments[2]?.type === "markdown" && segments[2].source).toContain("后记");
  });

  it("四种语气都支持，标题可省略", () => {
    const segments = splitNoticeSegments(
      [
        "::: note\n备注\n:::",
        "::: warning\n注意\n:::",
        "::: danger\n危险\n:::",
      ].join("\n\n"),
    );

    expect(segments.every((segment) => segment.type === "notice")).toBe(true);
    expect(segments.map((segment) => (segment.type === "notice" ? segment.tone : null))).toEqual([
      ...NOTICE_TONES.filter((tone) => tone === "note" || tone === "warning" || tone === "danger"),
    ]);
    for (const segment of segments) {
      expect(segment.type === "notice" && segment.title).toBe("");
    }
  });

  it("未闭合的提示块退化成普通文本（不能吞掉后文）", () => {
    const segments = splitNoticeSegments("::: tip\n没有结束标记");

    expect(segments).toEqual([{ type: "markdown", source: "::: tip\n没有结束标记" }]);
  });

  it("未知语气不当作提示块（避免把代码里的 ::: 误判）", () => {
    const segments = splitNoticeSegments("::: foo\n内容\n::: ");

    expect(segments).toEqual([{ type: "markdown", source: "::: foo\n内容\n::: " }]);
    expect(segments[0]?.type === "markdown" && segments[0].source).toContain("::: foo");
  });

  it("同一段里多个提示块按顺序切分", () => {
    const segments = splitNoticeSegments("::: tip\n一\n:::\n\n::: warning\n二\n:::");

    expect(segments.map((segment) => segment.type)).toEqual(["notice", "notice"]);
    expect(
      segments.map((segment) => (segment.type === "notice" ? `${segment.tone}:${segment.source}` : "")),
    ).toEqual(["tip:一", "warning:二"]);
  });
});

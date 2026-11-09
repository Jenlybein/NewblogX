import { describe, expect, it } from "vitest";

import { normalizeRichTextLink } from "~/utils/richTextLink";

/**
 * 链接地址收敛（#329）。
 *
 * 这一层钉的是"能写进 href 的集合"：必须与读者站的白名单一致
 * （`frontend/app/utils/sanitizeHtml.ts` 的 `RICH_TEXT_ALLOWED_URI`）。
 * 编辑器比白名单宽就会骗人 —— 这边显示保存成功，读者站那边链接被剥掉。
 */

describe("normalizeRichTextLink：允许的地址原样保留", () => {
  it("http / https / mailto / tel", () => {
    expect(normalizeRichTextLink("https://gentlybeing.cn/article/1")).toEqual({
      ok: true,
      url: "https://gentlybeing.cn/article/1",
    });
    expect(normalizeRichTextLink("http://old.example.cn/a")).toEqual({
      ok: true,
      url: "http://old.example.cn/a",
    });
    expect(normalizeRichTextLink("mailto:hi@gentlybeing.cn")).toEqual({
      ok: true,
      url: "mailto:hi@gentlybeing.cn",
    });
    expect(normalizeRichTextLink("tel:+8613800000000")).toEqual({
      ok: true,
      url: "tel:+8613800000000",
    });
  });

  it("站内相对地址原样保留，不被补成外链", () => {
    expect(normalizeRichTextLink("/article/123")).toEqual({ ok: true, url: "/article/123" });
    expect(normalizeRichTextLink("#comments")).toEqual({ ok: true, url: "#comments" });
  });

  it("协议相对地址补上协议头，而不是当成站内路径", () => {
    expect(normalizeRichTextLink("//cdn.example.cn/a.png")).toEqual({
      ok: true,
      url: "https://cdn.example.cn/a.png",
    });
  });

  it("裸域名补 https://", () => {
    expect(normalizeRichTextLink("gentlybeing.cn/post")).toEqual({
      ok: true,
      url: "https://gentlybeing.cn/post",
    });
  });

  it("首尾空白剥掉", () => {
    expect(normalizeRichTextLink("  https://x.cn/a  ")).toEqual({ ok: true, url: "https://x.cn/a" });
  });
});

describe("normalizeRichTextLink：拒绝与报错", () => {
  it("空地址拒绝", () => {
    const result = normalizeRichTextLink("   ");
    expect(result.ok).toBe(false);
    expect(result.ok === false ? result.error : "").toContain("请填写");
  });

  it("javascript: / data: / vbscript: 一律拒绝（读者站反正会剥，早点说清）", () => {
    for (const bad of [
      "javascript:alert(1)",
      "JavaScript:alert(1)",
      "data:text/html;base64,PHNjcmlwdD4=",
      "vbscript:msgbox(1)",
    ]) {
      expect(normalizeRichTextLink(bad).ok, bad).toBe(false);
    }
  });

  it("拒绝时的文案说明允许什么，而不是只说「错了」", () => {
    const result = normalizeRichTextLink("javascript:alert(1)");
    expect(result.ok === false ? result.error : "").toContain("http");
  });
});

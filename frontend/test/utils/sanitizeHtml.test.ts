import { describe, expect, it } from "vitest";

import { sanitizeArticleHtml, sanitizeRichTextHtml } from "~/utils/sanitizeHtml";

/**
 * 富文本卡的清洗用例：先写"该被剥离的恶意输入"，再写"合法排版不能被误杀"。
 *
 * 右侧栏富文本卡由运营在后台编辑，内容会被读者站以 HTML 渲染 ——
 * 这是全站唯一一处"后台作者直接写 HTML、无需经过 markdown 渲染器"的入口，
 * 所以白名单必须比正文更窄：不放脚本、不放内联事件、不放 iframe，
 * 也不放 `javascript:` / `data:` 这类伪协议。
 */
describe("sanitizeRichTextHtml：恶意输入被剥离", () => {
  it("script 标签连同内容一起消失", () => {
    const html = sanitizeRichTextHtml('<p>正常</p><script>alert("x")</script>');

    expect(html).toContain("正常");
    expect(html).not.toContain("<script");
    expect(html).not.toContain("alert");
  });

  it("图片标签连同 onerror 一起被移除（富文本档不放行图片）", () => {
    const html = sanitizeRichTextHtml('<img src="https://cdn.example.com/a.png" onerror="alert(1)">');

    expect(html.toLowerCase()).not.toContain("onerror");
    expect(html).not.toContain("<img");
    expect(html).not.toContain("alert");
  });

  it("javascript: 伪协议链接被剥离", () => {
    const html = sanitizeRichTextHtml('<a href="javascript:alert(1)">点我</a>');

    expect(html).not.toContain("javascript:");
    expect(html).not.toContain("alert");
  });

  it("data: 伪协议链接被剥离（图片与链接都不放过）", () => {
    const html = sanitizeRichTextHtml(
      '<a href="data:text/html;base64,PHNjcmlwdD4=">点我</a><img src="data:image/png;base64,AAAA">',
    );

    expect(html).not.toContain("data:");
    expect(html).not.toContain("base64");
  });

  it("iframe 整块被移除，不留空壳", () => {
    const html = sanitizeRichTextHtml('<p>前</p><iframe src="https://evil.example.com"></iframe><p>后</p>');

    expect(html).toContain("前");
    expect(html).toContain("后");
    expect(html).not.toContain("iframe");
    expect(html).not.toContain("evil.example.com");
  });

  it("任意标签上的 on* 内联事件都被剥离", () => {
    const html = sanitizeRichTextHtml('<div onclick="alert(1)" onmouseover="alert(2)">文字</div>');

    expect(html).toContain("文字");
    expect(html.toLowerCase()).not.toContain("onclick");
    expect(html.toLowerCase()).not.toContain("onmouseover");
  });

  it("SVG 与脚本的组合攻击也被阻断", () => {
    const html = sanitizeRichTextHtml('<svg onload="alert(1)"><script>alert(1)</script></svg>');

    expect(html.toLowerCase()).not.toContain("onload");
    expect(html).not.toContain("<script");
    expect(html).not.toContain("<svg");
  });
});

describe("sanitizeRichTextHtml：合法排版内容不被误杀", () => {
  const LEGIT = [
    "<h2>更新日志</h2>",
    "<h3>本周改动</h3>",
    '<p>本周 <strong>重点</strong> 与 <em>说明</em></p>',
    "<ul><li>第一条</li><li>第二条</li></ul>",
    "<ol><li>第一</li></ol>",
    "<blockquote>引用一段</blockquote>",
    '<p><a href="https://example.com/post" target="_blank" rel="noopener">链接</a></p>',
    "<pre><code>npm run dev</code></pre>",
  ].join("");

  it("标题 / 粗斜体 / 列表 / 链接 / 引用 / 代码块全部保留", () => {
    const html = sanitizeRichTextHtml(LEGIT);

    for (const tag of ["<h2>", "<h3>", "<strong>", "<em>", "<ul>", "<li>", "<ol>", "<blockquote>", "<pre>", "<code>"]) {
      expect(html).toContain(tag);
    }
    expect(html).toContain("https://example.com/post");
    expect(html).toContain("重点");
    expect(html).toContain("第一条");
  });

  it("空字符串原样返回，不会渲染出占位内容", () => {
    expect(sanitizeRichTextHtml("")).toBe("");
    expect(sanitizeRichTextHtml("   ")).toBe("");
  });
});

describe("sanitizeArticleHtml：正文档位保持原行为（同一份白名单的两档）", () => {
  it("正文档仍放行内联 SVG（渲染档位不能互相污染）", () => {
    const html = sanitizeArticleHtml('<p>图形</p><svg viewBox="0 0 10 10"><circle cx="5" cy="5" r="4"></circle></svg>');

    expect(html).toContain("<svg");
    expect(html).not.toContain("<script");
  });
});

import { describe, expect, it } from "vitest";

import { renderArticleMarkdown } from "~/composables/useArticleMarkdown";

describe("正文渲染：标题锚点", () => {
  it("为 h1-h4 生成稳定 id 与目录项，h5 不进目录", () => {
    const { headings } = renderArticleMarkdown(
      "# 标题\n\n## 小节\n\n### 三级\n\n#### 四级\n\n##### 五级",
    );

    expect(headings.map((heading) => heading.title)).toEqual(["标题", "小节", "三级", "四级"]);
    expect(headings.map((heading) => heading.level)).toEqual([1, 2, 3, 4]);
    expect(headings.every((heading) => heading.href === `#${heading.id}`)).toBe(true);
  });

  it("重复标题自动加序号，锚点不冲突", () => {
    const { headings } = renderArticleMarkdown("## 重复\n\n## 重复\n\n## 重复");

    expect(headings.map((heading) => heading.id)).toEqual(["重复", "重复-2", "重复-3"]);
  });

  it("空正文给出明确的空态文案", () => {
    const { html, headings } = renderArticleMarkdown("   ");

    expect(headings).toEqual([]);
    expect(html).toContain("暂无正文内容");
  });
});

describe("正文渲染：代码块 / 表格 / 数学公式", () => {
  it("代码块带语言类名与 highlight.js 标记（由自有主题上色）", () => {
    const { html } = renderArticleMarkdown("```ts\nconst answer = 42\n```");

    expect(html).toContain('<pre><code class="hljs language-ts">');
    expect(html).toContain("answer");
  });

  it("表格保留 thead/tbody 结构，便于主题给表头与斑马纹上色", () => {
    const { html } = renderArticleMarkdown("| 列 A | 列 B |\n| --- | --- |\n| 1 | 2 |");

    expect(html).toContain("<table>");
    expect(html).toContain("<th>");
    expect(html).toContain("<td>");
  });

  it("数学公式输出 katex 结构（基线样式负责排版，主题负责配色）", () => {
    const { html } = renderArticleMarkdown("$E = mc^2$");

    expect(html).toContain("katex");
  });
});

describe("正文渲染：提示块", () => {
  it("::: tip 渲染为带语气类名的提示块，内部仍走 markdown", () => {
    const { html } = renderArticleMarkdown("::: tip 小贴士\n**重点**\n:::");

    expect(html).toContain('class="markdown-notice markdown-notice--tip"');
    expect(html).toContain("小贴士");
    expect(html).toContain("<strong>重点</strong>");
  });

  it("提示块里的标题也进目录（不会因为包在提示块里就丢锚点）", () => {
    const { headings } = renderArticleMarkdown("::: note\n## 提示块小节\n:::");

    expect(headings.map((heading) => heading.title)).toEqual(["提示块小节"]);
  });

  it("未闭合的提示块标记按原文输出，不吞正文", () => {
    const { html } = renderArticleMarkdown("::: tip\n后面的正文还在");

    expect(html).toContain("::: tip");
    expect(html).toContain("后面的正文还在");
  });
});

// 正文放行内联 SVG 的第二层（#136 / ADR-0021）。
//
// 后端已经能把绘图 SVG 存进库（第一层），但前端此前不解析内联 HTML，
// 于是即便库里有 SVG 也只会被转义成源码文本显示给读者。
describe("正文渲染：内联 SVG", () => {
  const figureSource = [
    '<div class="svg-block">',
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 820 502" role="img" style="width:100%;max-width:760px;height:auto">',
    "<title>Agent Loop 循环</title>",
    '<g><rect x="0" y="0" width="820" height="502" rx="16" fill="#fbfcfd"/><circle cx="50" cy="50" r="40" fill="red"/><path d="M330,124 L330,138" stroke="#3f6ea6" stroke-width="2"/><text x="330" y="87" font-size="15.5" fill="#1f2430">用户输入</text></g>',
    "</svg>",
    "<center>图 1　Agent Loop：推理 → 判定 → 执行 → 回注</center>",
    "</div>",
  ].join("\n");

  it("把内联 SVG 渲染成真实元素，而不是转义文本", () => {
    const { html } = renderArticleMarkdown(figureSource);

    // 必须出现真实的 <svg 标签；若只看到 &lt;svg 说明仍被当文本。
    expect(html).toContain("<svg");
    expect(html).not.toContain("&lt;svg");
    expect(html).toContain("<rect");
    expect(html).toContain("<circle");
    expect(html).toContain("<path");
  });

  it("保留排版包裹与居中图注", () => {
    const { html } = renderArticleMarkdown(figureSource);

    expect(html).toContain('class="svg-block"');
    expect(html).toContain("<center>");
    expect(html).toContain("图 1");
  });

  it("消毒后仍拦得住可执行内容", () => {
    const dangerous = [
      '<svg viewBox="0 0 10 10" onload="alert(1)">',
      "<script>alert(2)</script>",
      '<foreignObject><iframe src="javascript:alert(3)"></iframe></foreignObject>',
      '<rect width="10" height="10" fill="red" onclick="alert(4)"/>',
      "</svg>",
    ].join("\n");

    const { html } = renderArticleMarkdown(dangerous);

    expect(html).not.toContain("<script");
    expect(html).not.toContain("onload");
    expect(html).not.toContain("onclick");
    expect(html).not.toContain("javascript:");
    expect(html).not.toContain("foreignObject");
    // 同一片段里安全的绘图元素仍应保留。
    expect(html).toContain("<rect");
  });

  it("图片语法引入的 SVG 不受影响（本来就可用）", () => {
    const { html } = renderArticleMarkdown("![图](https://cdn.example.com/a.svg)");

    expect(html).toContain('<img src="https://cdn.example.com/a.svg"');
  });
});

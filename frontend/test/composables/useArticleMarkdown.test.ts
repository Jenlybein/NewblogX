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

import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { renderArticleMarkdown } from "~/composables/useArticleMarkdown";
import { MARKDOWN_THEME_CSS } from "~/utils/markdownThemeCss";

/**
 * 票 #244 的可对比预览途径：
 *
 *  `test/fixtures/markdown-palette-gallery.md` 是**一页可粘进预览的验收稿**，
 *  本文件保证"渲染出来的层级 / 内联形态 / 代码 token"与主题 CSS 的装置**一一对上**。
 *  纯肉眼看截图容易漏层，所以这里把"实际渲染"和"主题规则"对照着断言：
 *  改动任一侧（渲染器或主题）都会红，不会出现"CSS 有 H5 装置但渲染器不出 H5"。
 */
const projectDir = process.cwd();
const gallery = readFileSync(join(projectDir, "test/fixtures/markdown-palette-gallery.md"), "utf8");
const { html, headings } = renderArticleMarkdown(gallery);

/** 取主题 CSS 里某个选择器所在规则的声明体（与 utils 测试同款取法）。 */
function ruleBody(selector: string): string {
  const code = MARKDOWN_THEME_CSS.replace(/\/\*[\s\S]*?\*\//gu, "");
  let fallback: string | undefined;
  for (const match of code.matchAll(/([^{}]+)\{([^{}]*)\}/gu)) {
    const selectors = match[1]!.split(",").map((item) => item.trim());
    if (selectors.length === 1 && selectors[0] === selector) {
      return match[2]!;
    }
    if (selectors.includes(selector) && fallback === undefined) {
      fallback = match[2]!;
    }
  }
  if (fallback !== undefined) return fallback;
  throw new Error(`主题 CSS 里找不到规则：${selector}`);
}

describe("色板预览稿：六层标题与实际渲染一致", () => {
  it("六个标题层级都真的渲染出来（主题有装置 ≠ 渲染器会产出）", () => {
    for (let level = 1; level <= 6; level += 1) {
      expect(html, `h${level}`).toContain(`<h${level} `);
    }
  });

  it.each([1, 2, 3, 4, 5, 6])("H%i 的渲染层级与主题装置对得上（颜色 + 装置各不同）", (level) => {
    const rule = ruleBody(`.markdown-body h${level}`);

    // 颜色：每一层都消费自己那一档 "--md-heading-hN"。
    expect(rule, `h${level} 颜色`).toContain(`color: var(--md-heading-h${level});`);
  });

  // 装置语言已按 themes/graphite-island-spectrum 对齐：六层仍是六种不同装置，
  // 只是形态换成该主题的定标线 / 通栏色带 / 三角+发丝线 / 记号笔横条 / 贴纸块 / 前置引线。
  it("每层的装置彼此不同：定标线 / 通栏色带 / 三角+发丝线 / 记号笔横条 / 贴纸块 / 前置引线", () => {
    expect(ruleBody(".markdown-body h1")).toMatch(/border-bottom:/u);
    expect(ruleBody(".markdown-body h1::after")).toMatch(/background:\s*var\(--md-heading-h1\)/u);

    expect(ruleBody(".markdown-body h2")).toMatch(/border-left:\s*6px solid/u);
    expect(ruleBody(".markdown-body h2")).toMatch(/background-image:\s*linear-gradient\(/u);

    expect(ruleBody(".markdown-body h3::before")).toMatch(/clip-path:\s*polygon\(/u);
    expect(ruleBody(".markdown-body h3")).toMatch(/border-bottom:/u);

    expect(ruleBody(".markdown-body h4")).toMatch(/var\(--md-heading-h4\) 44px/u);

    const h5 = ruleBody(".markdown-body h5");
    expect(h5).toMatch(/display:\s*inline-block/u);
    expect(h5).toMatch(/border:\s*1px solid/u);

    const h6 = ruleBody(".markdown-body h6");
    expect(h6).toMatch(/text-transform:\s*uppercase/u);
    expect(h6).toMatch(/letter-spacing:/u);
    expect(ruleBody(".markdown-body h6::before")).toMatch(/height:\s*1px/u);
  });

  it("目录只收 h1–h4：预览稿里 h5/h6 不出现在 headings（与既有的锚点契约一致）", () => {
    expect(headings.map((heading) => heading.level)).toEqual([1, 2, 3, 4, 2, 2, 2]);
  });
});

describe("色板预览稿：四种内联形态渲染成四种不同元素", () => {
  it("加粗 / 斜体 / 删除线 / 下划线各出各的标签", () => {
    expect(html).toContain("<strong>");
    expect(html).toContain("<em>");
    // markdown-it 把 ~~文字~~ 产出 <s>（不是 <del>）；主题两种标签都覆盖。
    expect(html).toContain("<s>");
    expect(html).toContain("<ins>");
    expect(MARKDOWN_THEME_CSS).toContain(".markdown-body del");
    expect(MARKDOWN_THEME_CSS).toContain(".markdown-body s");
  });

  it("下划线是真 text-decoration，不是黄底冒充", () => {
    const ins = ruleBody(".markdown-body ins");

    expect(ins).toMatch(/text-decoration:\s*underline/u);
    expect(ins).toMatch(/text-decoration-color:\s*var\(--md-underline-line\)/u);
    expect(ins).toMatch(/text-underline-offset:\s*var\(--md-underline-offset\)/u);
    // 关键反向断言：不允许再用高亮底（包豪斯黄）冒充下划线。
    expect(ins).not.toMatch(/background:\s*var\(--color-highlight\)/u);
    expect(ins).toMatch(/background:\s*none/u);
  });

  it("四种内联各有自己的颜色 token，互不混淆", () => {
    expect(ruleBody(".markdown-body strong")).toContain("color: var(--md-bold-ink);");
    expect(ruleBody(".markdown-body em")).toContain("color: var(--md-italic-ink);");
    expect(ruleBody(".markdown-body del")).toContain("color: var(--md-strike-ink);");
    expect(ruleBody(".markdown-body ins")).toContain("color: inherit;");
  });
});

describe("色板预览稿：列表标记与正文有区分", () => {
  it("预览稿真的产出有序与无序两种列表", () => {
    expect(html).toContain("<ol>");
    expect(html).toContain("<ul>");
  });

  it("无序圆点苔绿、有序数字赤陶（各自一枚 ::marker 规则）", () => {
    expect(ruleBody(".markdown-body ul > li::marker")).toContain(
      "color: var(--md-list-unordered-marker);",
    );
    expect(ruleBody(".markdown-body ol > li::marker")).toContain(
      "color: var(--md-list-ordered-marker);",
    );
  });
});

describe("色板预览稿：代码七档 token 真被高亮器产出", () => {
  it.each([
    ["关键字", ".hljs-keyword", "hljs-keyword"],
    ["字符串", ".hljs-string", "hljs-string"],
    ["数字", ".hljs-number", "hljs-number"],
    ["函数", ".hljs-function", "hljs-function"],
    ["类型", ".hljs-type", "hljs-type"],
    ["注释", ".hljs-comment", "hljs-comment"],
    ["变量", ".hljs-params", "hljs-params"],
  ])("%s 既出现在渲染产物里（%s），主题也有对应规则", (_role, className, htmlClass) => {
    expect(html).toContain(htmlClass);
    expect(MARKDOWN_THEME_CSS).toContain(className);
  });

  it("代码块与行内代码都是「另一张纸」：消费 --md-code-paper", () => {
    expect(ruleBody(".markdown-body pre")).toContain("background: var(--md-code-paper);");
  });
});

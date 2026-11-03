import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  MARKDOWN_THEME_ROLES,
  buildMarkdownThemeCss,
  parseTokenBlocks,
  resolveMarkdownTheme,
  resolveTokenValue,
} from "~/utils/markdownTheme";
import { MARKDOWN_THEME_CSS } from "~/utils/markdownThemeCss";

// vitest 以 project root（frontend/）为 cwd。
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
const themeCss = MARKDOWN_THEME_CSS;

describe("token 解析：明暗是同一组名字的两份取值", () => {
  it("从 tokens.css 的 :root / .dark 两份块里读出取值", () => {
    const blocks = parseTokenBlocks(tokensCss);

    expect(blocks.light["--color-ink"]).toBe("#1c1b19");
    expect(blocks.light["--color-bg-page"]).toBe("var(--jelly-warm-100)");
    expect(blocks.dark["--color-ink"]).toBe("#f5f5f7");
    expect(blocks.dark["--color-bg-page"]).toBe("#161617");
  });

  it("var() 间接引用会被解析成真实色值（用于对比度/截图断言）", () => {
    const blocks = parseTokenBlocks(tokensCss);

    expect(resolveTokenValue(blocks, "light", "--color-ink")).toBe("#1c1b19");
    expect(resolveTokenValue(blocks, "dark", "--color-ink")).toBe("#f5f5f7");
    expect(resolveTokenValue(blocks, "light", "--color-surface")).toBe("#fdfbf6");
    expect(resolveTokenValue(blocks, "dark", "--color-surface")).toBe("#1d1d1f");
    expect(resolveTokenValue(blocks, "light", "--color-line")).toBe("rgba(28, 27, 25, 0.16)");
    expect(resolveTokenValue(blocks, "dark", "--color-line")).toBe("rgba(245, 245, 247, 0.14)");
  });

  it("未知 token 与循环引用直接抛错（避免静默回落到默认色）", () => {
    const blocks = parseTokenBlocks(tokensCss);

    expect(() => resolveTokenValue(blocks, "light", "--color-does-not-exist")).toThrow(/未知 token/u);
    expect(() =>
      resolveTokenValue(
        { light: { "--a": "var(--b)", "--b": "var(--a)" }, dark: {} },
        "light",
        "--a",
      ),
    ).toThrow(/循环引用/u);
  });
});

describe("markdown 主题取值：明暗两套都来自语义层", () => {
  const light = resolveMarkdownTheme(tokensCss, "light");
  const dark = resolveMarkdownTheme(tokensCss, "dark");

  it("所有角色都能解析成具体颜色（不允许把 var() 漏给渲染层）", () => {
    for (const mode of [light, dark]) {
      for (const value of Object.values(mode)) {
        expect(value).not.toContain("var(");
        expect(value).toMatch(/^#[0-9a-f]{6}$|^rgba\(/u);
      }
    }
  });

  it("角色映射覆盖正文/纸面/结构线/代码/提示块/危险态", () => {
    for (const role of Object.keys(MARKDOWN_THEME_ROLES)) {
      expect(light[role as keyof typeof light]).toBeTruthy();
      expect(dark[role as keyof typeof light]).toBeTruthy();
    }
  });

  it("明暗取值确实翻转：正文/纸面/危险文字在两种模式下不同", () => {
    expect(light.body).toBe("#1c1b19");
    expect(dark.body).toBe("#f5f5f7");
    expect(light.paper).toBe("#fdfbf6");
    expect(dark.paper).toBe("#1d1d1f");
    expect(light.dangerInk).toBe("#c2401f");
    expect(dark.dangerInk).toBe("#ff9b85");
    // 反白层：亮色是白字配墨水实心 CTA，暗色反转为深字配浅墨填充。
    expect(light.onStrong).toBe("#ffffff");
    expect(dark.onStrong).toBe("#1d1d1f");
  });
});

/** 取主题 CSS 里某个选择器所在规则的声明体（静态规则门禁专用；分组/媒体查询也能命中）。
 *  优先命中"只含该选择器"的规则，避免把共用基线的分组规则当成单档规则。 */
function ruleBody(selector: string): string {
  const code = themeCss.replace(/\/\*[\s\S]*?\*\//gu, "");
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
  if (fallback !== undefined) {
    return fallback;
  }
  throw new Error(`主题 CSS 里找不到规则：${selector}`);
}

describe("正文节奏：16px / 行高 1.85，行宽继承页面容器", () => {
  it("正文统一 16px、行高 1.85（参考主题的节奏）", () => {
    const body = ruleBody(".markdown-body");

    expect(body).toMatch(/font-size:\s*16px/u);
    expect(body).toMatch(/line-height:\s*1\.85/u);
    expect(body).toMatch(/font-family:\s*var\(--font-sans\)/u);
    expect(body).toMatch(/color:\s*var\(--color-ink\)/u);
  });

  it("shadow root 里的正文不再另设行宽（行长仍由 .article-body 的 68ch 决定）", () => {
    expect(ruleBody(".markdown-body")).not.toMatch(/max-width/u);
  });
});

describe("六层标题装置：颜色 + 装置逐层不同（正文用 --md-* 色板）", () => {
  it.each([1, 2, 3, 4, 5, 6])("H%i 有自己的字色 token", (level) => {
    expect(ruleBody(`.markdown-body h${level}`)).toContain(`color: var(--md-heading-h${level});`);
  });

  it("H1 墨：居中 + 定标线 + 居中记号块", () => {
    const rule = ruleBody(".markdown-body h1");

    expect(rule).toMatch(/text-align:\s*center/u);
    expect(rule).toContain("border-bottom: 1px solid var(--md-heading-h1);");

    const mark = ruleBody(".markdown-body h1::after");
    expect(mark).toContain("background: var(--md-heading-h1);");
    expect(mark).toMatch(/margin:\s*[^;]*auto/u);
  });

  it("H2 赤陶：左侧 3px 色带", () => {
    expect(ruleBody(".markdown-body h2")).toContain("border-left: 3px solid var(--md-heading-h2);");
  });

  it("H3 苔绿：行首三角", () => {
    const mark = ruleBody(".markdown-body h3::before");

    expect(mark).toMatch(/clip-path:\s*polygon\(/u);
    expect(mark).toContain("background: var(--md-heading-h3);");
  });

  it("H4 靛青：约 40% 宽的短下划线", () => {
    const mark = ruleBody(".markdown-body h4::after");

    expect(mark).toMatch(/width:\s*40%/u);
    expect(mark).toMatch(/height:\s*[23]px/u);
    expect(mark).toContain("background: var(--md-heading-h4);");
  });

  it("H5 梅紫：上下双发丝线", () => {
    const rule = ruleBody(".markdown-body h5");

    expect(rule).toContain("border-top: 1px solid var(--md-heading-h5);");
    expect(rule).toContain("border-bottom: 1px solid var(--md-heading-h5);");
  });

  it("H6 赭黄：大写字距 + 顶横线", () => {
    const h6 = ruleBody(".markdown-body h6");

    expect(h6).toMatch(/text-transform:\s*uppercase/u);
    expect(h6).toMatch(/letter-spacing:\s*0\.18em/u);
    expect(h6).toContain("border-top: 1px solid var(--md-heading-h6);");
  });

  it("字号阶梯是 em（随 16px 正文同步缩放）", () => {
    const ladder = new Map([
      [1, "2.15em"],
      [2, "1.58em"],
      [3, "1.33em"],
      [4, "1.18em"],
      [6, "0.8em"],
    ]);

    for (const [level, size] of ladder) {
      expect(ruleBody(`.markdown-body h${level}`), `h${level}`).toContain(`font-size: ${size};`);
    }
  });
});

describe("四种内联样式：颜色与形态互不混淆", () => {
  it("加粗：深墨 + 字重 650", () => {
    const rule = ruleBody(".markdown-body strong");

    expect(rule).toContain("color: var(--md-bold-ink);");
    expect(rule).toContain("font-weight: var(--md-bold-weight);");
  });

  it("斜体：暖灰 + 真斜体", () => {
    const rule = ruleBody(".markdown-body em");

    expect(rule).toContain("color: var(--md-italic-ink);");
    expect(rule).toMatch(/font-style:\s*italic/u);
  });

  it("删除线：赤陶线 / 暖灰字 / 1.5px 线宽", () => {
    const rule = ruleBody(".markdown-body del");

    expect(rule).toContain("color: var(--md-strike-ink);");
    expect(rule).toMatch(/text-decoration:\s*line-through/u);
    expect(rule).toContain("text-decoration-color: var(--md-strike-line);");
    expect(rule).toContain("text-decoration-thickness: var(--md-strike-width);");
  });

  it("下划线：真 text-decoration + 靛青线 + offset token（不再用黄底冒充）", () => {
    const rule = ruleBody(".markdown-body ins");

    expect(rule).toContain("color: inherit;");
    expect(rule).toMatch(/background:\s*none/u);
    expect(rule).toMatch(/text-decoration:\s*underline/u);
    expect(rule).toContain("text-decoration-color: var(--md-underline-line);");
    expect(rule).toContain("text-underline-offset: var(--md-underline-offset);");
    // 反向断言：黄底（--color-highlight）不得再出现在下划线规则里。
    expect(rule).not.toContain("--color-highlight");

    expect(ruleBody(".markdown-body ins:hover")).toContain("font-weight: var(--md-bold-weight);");
  });

  it("mark 仍是记号笔底（==文字==），与下划线是两回事", () => {
    expect(ruleBody(".markdown-body mark")).toContain("background: var(--color-highlight);");
  });
});

describe("列表标记：有序赤陶 / 无序苔绿", () => {
  it("无序圆点苔绿", () => {
    expect(ruleBody(".markdown-body ul > li::marker")).toContain(
      "color: var(--md-list-unordered-marker);",
    );
  });

  it("有序数字赤陶", () => {
    expect(ruleBody(".markdown-body ol > li::marker")).toContain(
      "color: var(--md-list-ordered-marker);",
    );
  });
});

describe("代码高亮：七档 token 各归其位", () => {
  it.each([
    [".markdown-body .hljs-keyword", "--md-code-keyword"],
    [".markdown-body .hljs-string", "--md-code-string"],
    [".markdown-body .hljs-number", "--md-code-number"],
    [".markdown-body .hljs-function", "--md-code-function"],
    [".markdown-body .hljs-type", "--md-code-type"],
    [".markdown-body .hljs-comment", "--md-code-comment"],
    [".markdown-body .hljs-params", "--md-code-variable"],
  ])("%s 消费 %s", (selector, token) => {
    expect(ruleBody(selector)).toContain(`color: var(${token});`);
  });
});

describe("代码：代码块是另一层纸面，行内代码可辨识", () => {
  it("代码块有独立纸面层级：背景 + 边框 + 平投影 + token 圆角", () => {
    const pre = ruleBody(".markdown-body pre");

    // 代码纸面走正文色板（--md-code-paper），其余结构仍走 --color-* / --radius-*。
    expect(pre).toMatch(/background:\s*var\(--(?:color|md)-[a-z-]+\)/u);
    expect(pre).toMatch(/border:\s*1px solid var\(--color-[a-z-]+\)/u);
    expect(pre).toMatch(/box-shadow:\s*[^;]*var\(--color-shadow\)/u);
    expect(pre).toMatch(/border-radius:\s*var\(--radius-(?:sm|md|lg|xl|pill)\)/u);
    expect(pre).toMatch(/font-family:\s*var\(--font-mono\)/u);
  });

  it("行内代码有底色与发丝边，和正文一眼可分", () => {
    const code = ruleBody(".markdown-body code");

    expect(code).toMatch(/background:\s*var\(--color-[a-z-]+\)/u);
    expect(code).toMatch(/border:\s*1px solid var\(--color-[a-z-]+\)/u);
    expect(code).toMatch(/border-radius:\s*var\(--radius-/u);
    expect(code).toMatch(/font-family:\s*var\(--font-mono\)/u);
  });

  it("代码块里的 code 回到透明底（纸面只由 pre 画一次）", () => {
    const inner = ruleBody(".markdown-body pre code");

    expect(inner).toMatch(/background:\s*transparent/u);
    expect(inner).toMatch(/border:\s*0/u);
  });
});

describe("引用与表格：引用块是强调面，表格用发丝线", () => {
  it("引用块用强调面（标记底 + 边线 + token 圆角），字回到正文墨色", () => {
    const quote = ruleBody(".markdown-body blockquote");

    expect(quote).toMatch(/background:\s*var\(--color-[a-z-]+\)/u);
    expect(quote).toMatch(/border:.*var\(--color-[a-z-]+\)/u);
    expect(quote).toMatch(/border-radius:\s*var\(--radius-/u);
    expect(quote).toMatch(/color:\s*var\(--color-ink\)/u);
  });

  it("引用块里的行内代码换回纸面（彩底上再压彩贴片会脏）", () => {
    const inner = ruleBody(".markdown-body blockquote code");

    expect(inner).toMatch(/background:\s*var\(--color-surface\)/u);
  });

  it("表格走发丝线：表头 1px、单元格 line-faint，不再是 2px 结构线", () => {
    expect(ruleBody(".markdown-body thead")).toMatch(
      /border-bottom:\s*1px solid var\(--color-[a-z-]+\)/u,
    );
    expect(ruleBody(".markdown-body td")).toMatch(
      /border:\s*1px solid var\(--color-line-faint\)/u,
    );
  });
});

/** 相对亮度（WCAG 2.x），只用于本文件的对比度断言。 */
function relativeLuminance(color: string): number {
  const hex = /^#([0-9a-f]{6})$/iu.exec(color.trim())?.[1];
  if (!hex) {
    throw new Error(`对比度断言只接受十六进制色值，收到：${color}`);
  }
  const channels = [0, 2, 4].map((offset) => Number.parseInt(hex.slice(offset, offset + 2), 16) / 255);
  const linear = channels.map((channel) =>
    channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4,
  );
  return 0.2126 * linear[0]! + 0.7152 * linear[1]! + 0.0722 * linear[2]!;
}

function contrastRatio(foreground: string, background: string): number {
  const [high, low] = [relativeLuminance(foreground), relativeLuminance(background)].sort(
    (left, right) => right - left,
  ) as [number, number];
  return (high + 0.05) / (low + 0.05);
}

describe("链接：无下划线、1px 底边，暗色下仍满足对比度", () => {
  it("默认态去掉 text-decoration，改用 1px 语义色底边", () => {
    const link = ruleBody(".markdown-body a");

    expect(link).toMatch(/text-decoration:\s*none/u);
    expect(link).toMatch(/border-bottom:\s*1px solid var\(--color-[a-z-]+\)/u);
  });

  it("明暗两态的链接对纸面都 ≥ 4.5:1（AA 正文对比度）", () => {
    const light = resolveMarkdownTheme(tokensCss, "light");
    const dark = resolveMarkdownTheme(tokensCss, "dark");

    for (const [mode, values] of [
      ["light", light],
      ["dark", dark],
    ] as const) {
      expect(contrastRatio(values.link, values.paper), mode).toBeGreaterThanOrEqual(4.5);
    }
  });

  it("主题 CSS 不写明暗分叉：换肤完全靠同一组 token 的第二份取值", () => {
    const declarations = themeCss.replace(/\/\*[\s\S]*?\*\//gu, "");

    expect(declarations).not.toMatch(/\.theme-dark|\.dark\b|prefers-color-scheme/u);
  });
});

describe("markdown 主题 CSS：自有主题，不再加载 github-markdown-css", () => {
  it("构建函数把主题源包成可注入 shadow root 的字符串", () => {
    const css = buildMarkdownThemeCss(".markdown-body { color: var(--color-ink); }");

    expect(css).toContain("markdown 主题（JellyBin 自有）");
    expect(css).toContain(".markdown-body { color: var(--color-ink); }");
    expect(css.trim().endsWith("}")).toBe(true);
  });

  it("主题源覆盖代码块 / 表格 / 数学公式 / 提示块（缺一不可）", () => {
    for (const selector of [
      ".markdown-body",
      ".markdown-body pre",
      ".markdown-body code",
      ".hljs-keyword",
      ".hljs-string",
      ".hljs-comment",
      ".markdown-body table",
      ".markdown-body th",
      ".markdown-body td",
      ".markdown-body blockquote",
      ".katex",
      ".markdown-notice",
      ".markdown-notice--tip",
      ".markdown-notice--warning",
      ".markdown-notice--danger",
    ]) {
      expect(themeCss).toContain(selector);
    }
  });

  it("颜色只来自语义 token：没有裸 hex / rgba 字面量（漂移门禁同款规则）", () => {
    const declarations = themeCss.replace(/\/\*[\s\S]*?\*\//gu, "");

    expect(declarations).not.toMatch(/#[0-9a-fA-F]{3,8}(?![0-9a-zA-Z_-])/u);
    expect(declarations).not.toMatch(/(?:rgba?|hsla?)\(/u);
    // 必须真的消费语义层与排版层，而不是自己造颜色。
    expect(declarations).toMatch(/var\(--color-/u);
    // 正文色板单独治理：标题/内联/列表/代码另有一组 --md-*（ADR-0012 后续修订）。
    expect(declarations).toMatch(/var\(--md-/u);
    expect(declarations).toMatch(/var\(--type-/u);
    expect(declarations).toMatch(/var\(--font-/u);
    // 只消费语义层，不绕过它去摸原始层（那等于给主题开了一个新色相入口）。
    expect(declarations).not.toMatch(/--jelly-/u);
  });

  it("动效只用 token（颜色过渡短、无滚动编排/关键帧）", () => {
    const declarations = themeCss.replace(/\/\*[\s\S]*?\*\//gu, "");

    expect(declarations).toMatch(/transition[\s\S]*?var\(--motion-fast\)/u);
    expect(declarations).not.toMatch(/@keyframes|animation-timeline|scroll-timeline/u);
  });
});

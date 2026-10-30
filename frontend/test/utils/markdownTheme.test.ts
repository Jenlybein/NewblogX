import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  MARKDOWN_THEME_CSS,
  MARKDOWN_THEME_ROLES,
  buildMarkdownThemeCss,
  parseTokenBlocks,
  resolveMarkdownTheme,
  resolveTokenValue,
} from "~/utils/markdownTheme";

// vitest 以 project root（frontend/）为 cwd。
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
const themeCss = MARKDOWN_THEME_CSS;

describe("token 解析：明暗是同一组名字的两份取值", () => {
  it("从 tokens.css 的 :root / .dark 两份块里读出取值", () => {
    const blocks = parseTokenBlocks(tokensCss);

    expect(blocks.light["--color-ink"]).toBe("var(--jelly-blue-900)");
    expect(blocks.light["--color-bg-page"]).toBe("var(--jelly-cream)");
    expect(blocks.dark["--color-ink"]).toBe("var(--jelly-cream)");
    expect(blocks.dark["--color-bg-page"]).toBe("var(--jelly-blue-900)");
  });

  it("var() 间接引用会被解析成真实色值（用于对比度/截图断言）", () => {
    const blocks = parseTokenBlocks(tokensCss);

    expect(resolveTokenValue(blocks, "light", "--color-ink")).toBe("#001b63");
    expect(resolveTokenValue(blocks, "dark", "--color-ink")).toBe("#fdfbf6");
    expect(resolveTokenValue(blocks, "light", "--color-surface")).toBe("#fdfbf6");
    expect(resolveTokenValue(blocks, "dark", "--color-surface")).toBe("#001f70");
    expect(resolveTokenValue(blocks, "light", "--color-line")).toBe("rgba(0, 47, 167, 0.32)");
    expect(resolveTokenValue(blocks, "dark", "--color-line")).toBe("rgba(253, 251, 246, 0.22)");
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
    expect(light.body).toBe("#001b63");
    expect(dark.body).toBe("#fdfbf6");
    expect(light.paper).toBe("#fdfbf6");
    expect(dark.paper).toBe("#001f70");
    expect(light.dangerInk).toBe("#c2410c");
    expect(dark.dangerInk).toBe("#ff9b85");
    // 反白层（蓝底上的字）两种模式都是奶油白。
    expect(light.onStrong).toBe("#fdfbf6");
    expect(dark.onStrong).toBe("#fdfbf6");
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
    expect(declarations).toMatch(/var\(--type-/u);
    expect(declarations).toMatch(/var\(--font-/u);
  });

  it("动效只用 token（颜色过渡短、无滚动编排/关键帧）", () => {
    const declarations = themeCss.replace(/\/\*[\s\S]*?\*\//gu, "");

    expect(declarations).toMatch(/transition[\s\S]*?var\(--motion-fast\)/u);
    expect(declarations).not.toMatch(/@keyframes|animation-timeline|scroll-timeline/u);
  });
});

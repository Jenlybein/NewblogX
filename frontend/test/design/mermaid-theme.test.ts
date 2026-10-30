import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { buildMermaidConfig, resolveMarkdownTheme, resolveMermaidTheme } from "~/utils/markdownTheme";

/**
 * mermaid 图表接语义 token（ADR-0012 决策 #11 与 #17 spec 的硬要求）。
 *
 * 为什么值得测：mermaid 的 `themeVariables` 不传就是**第三方默认配色**（浅紫 flowchart
 * 节点底），图表因此成为全站唯一不受 token 约束的绘制面 —— 换肤不跟随、对比度门禁也
 * 扫不到 SVG 内联样式。这里钉住三件事：
 *   1. 变量确实由 tokens.css 的语义层派生（不是写死十六进制）；
 *   2. 明暗两套取值不同（暗色是真的一组第二份取值，不是复制浅色）；
 *   3. 退回第三方默认配色会红（默认浅紫主色被显式排除）。
 */
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");

/** mermaid `theme: "base"` 默认主色 / 节点底的那支浅紫：留着它就说明没接 token。 */
const MERMAID_DEFAULT_PRIMARY = "#ECECFF";

describe("mermaid themeVariables 由语义 token 派生", () => {
  it("每个变量都解析成真实色值，且不含 var() 引用", () => {
    for (const mode of ["light", "dark"] as const) {
      const variables = resolveMermaidTheme(tokensCss, mode);

      for (const [name, value] of Object.entries(variables)) {
        expect(value, `${mode} 的 ${name} 必须是真实色值`).not.toContain("var(");
        expect(value, `${mode} 的 ${name} 不能为空`).toMatch(/^(#|rgba?\()/u);
      }
    }
  });

  it("取值来自 token 层：与 markdown 主题共用同一份解析器给出的色值", () => {
    const light = resolveMermaidTheme(tokensCss, "light");
    const theme = resolveMarkdownTheme(tokensCss, "light");

    // mainBkg 来自 --color-surface（纸面）、textColor 来自 --color-ink（正文墨）、
    // lineColor 来自 --color-line（发丝线）——三条都由 resolveTokenValue 解析出来。
    expect(light.mainBkg).toBe(theme.paper);
    expect(light.textColor).toBe(theme.body);
    expect(light.lineColor).toBe(theme.line);
  });

  it("改 token 会改掉取值（不是写死十六进制常量）", () => {
    const baseline = resolveMermaidTheme(tokensCss, "light");
    const mutated = tokensCss.replace(
      "--color-surface: var(--jelly-cream);",
      "--color-surface: #123456;",
    );

    expect(resolveMermaidTheme(mutated, "light").mainBkg).toBe("#123456");
    expect(baseline.mainBkg).not.toBe("#123456");
  });

  it("明暗两套取值不同（暗色是第二份取值，不是复制浅色）", () => {
    const light = resolveMermaidTheme(tokensCss, "light");
    const dark = resolveMermaidTheme(tokensCss, "dark");

    expect(dark.background).not.toBe(light.background);
    expect(dark.mainBkg).not.toBe(light.mainBkg);
    expect(dark.textColor).not.toBe(light.textColor);
    expect(dark.lineColor).not.toBe(light.lineColor);
  });

  it("不再是 mermaid 第三方默认配色（主色不是它默认的浅紫）", () => {
    for (const mode of ["light", "dark"] as const) {
      const variables = resolveMermaidTheme(tokensCss, mode);

      expect(variables.primaryColor).not.toBe(MERMAID_DEFAULT_PRIMARY);
      expect(JSON.stringify(variables).toLowerCase()).not.toContain(
        MERMAID_DEFAULT_PRIMARY.toLowerCase(),
      );
    }
  });

  it("明暗两套覆盖同一组键（没有单侧的漏配键）", () => {
    const light = resolveMermaidTheme(tokensCss, "light");

    expect(Object.keys(resolveMermaidTheme(tokensCss, "dark")).sort()).toEqual(
      Object.keys(light).sort(),
    );
  });
});

describe("mermaid initialize 配置：带上 token 派生的主题", () => {
  it("配置固定为 theme:base，并把 themeVariables 透传给 mermaid", () => {
    const config = buildMermaidConfig(tokensCss, "dark");

    expect(config.startOnLoad).toBe(false);
    expect(config.securityLevel).toBe("strict");
    expect(config.theme).toBe("base");
    expect(config.themeVariables).toEqual(resolveMermaidTheme(tokensCss, "dark"));
  });

  it("明/暗两份配置取到的主题不同", () => {
    const light = buildMermaidConfig(tokensCss, "light");
    const dark = buildMermaidConfig(tokensCss, "dark");

    expect(dark.themeVariables).not.toEqual(light.themeVariables);
  });
});

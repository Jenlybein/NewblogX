import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import tailwindConfig from "../../tailwind.config";
import { compileTailwindProbe } from "../../../scripts/tailwind-probe.mjs";

// vitest 以 project root（frontend/）为 cwd（不能依赖 import.meta.url，见漂移门禁用例的注释）。
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");

/** 取出某个选择器块里声明的自定义属性名（只要名字，不要值）。 */
function declaredNames(css: string, selectorPattern: RegExp): string[] {
  const block = selectorPattern.exec(css);
  if (!block) throw new Error(`tokens.css 里找不到 ${selectorPattern}`);
  return [...block[1].matchAll(/(--[a-z0-9-]+)\s*:/giu)].map((match) => match[1]);
}

describe("原始层：五色锁死", () => {
  it("五个锁定色相取到 spec 给定的值", () => {
    expect(tokensCss).toContain("--jelly-blue: #002fa7");
    expect(tokensCss).toContain("--jelly-cream: #fdfbf6");
    expect(tokensCss).toContain("--jelly-yellow: #f5c518");
    expect(tokensCss).toContain("--jelly-coral: #ff6b4a");
    expect(tokensCss).toContain("--jelly-warm: #f2eee6");
  });

  it("派生色只动同色相明度，不新增第六个色相", () => {
    const hues = new Set(
      [...tokensCss.matchAll(/--jelly-([a-z]+)[a-z0-9-]*\s*:/giu)].map((match) => match[1].toLowerCase()),
    );

    expect([...hues].sort()).toEqual(["blue", "coral", "cream", "warm", "yellow"]);
  });
});

describe("语义层：组件唯一消费对象", () => {
  it("明暗两份映射是同一组名字（暗色只是第二份取值）", () => {
    const light = declaredNames(tokensCss, /:root\s*\{([\s\S]*?)\n\s*\}/u).filter((name) =>
      name.startsWith("--color-"),
    );
    const dark = declaredNames(tokensCss, /(?:\.dark|\.theme-dark)[^\n{]*\{([\s\S]*?)\n\s*\}/u).filter((name) =>
      name.startsWith("--color-"),
    );

    expect(light.length).toBeGreaterThanOrEqual(13);
    expect([...dark].sort()).toEqual([...light].sort());
  });

  it("三档结构线 / 高亮底 / 危险与危险文字 / 焦点环 / 阴影都在语义层", () => {
    for (const name of [
      "--color-bg-page",
      "--color-bg-subtle",
      "--color-surface",
      "--color-ink",
      "--color-ink-muted",
      "--color-line",
      "--color-line-strong",
      "--color-line-faint",
      "--color-highlight",
      "--color-danger",
      "--color-danger-ink",
      "--color-focus",
      "--color-shadow",
    ]) {
      expect(tokensCss).toContain(`${name}:`);
    }
  });

  it("黄永不作为字色：高亮 token 只做底，唯一黄色语义名是 --color-highlight", () => {
    const yellowSemanticNames = new Set(
      [...tokensCss.matchAll(/(--color-[a-z-]+)\s*:[^;]*--jelly-yellow/gu)].map((match) => match[1]),
    );

    expect([...yellowSemanticNames]).toEqual(["--color-highlight"]);
  });
});

describe("排版层：中文尺度 token 化", () => {
  it("行宽 68ch、正文行高 1.75、标题字重与轻微负字距", () => {
    expect(tokensCss).toContain("--type-measure: 68ch");
    expect(tokensCss).toContain("--type-leading-prose: 1.75");
    expect(tokensCss).toContain("--type-weight-title: 700");
    expect(tokensCss).toMatch(/--type-tracking-title:\s*-0\.0[0-9]em/u);
  });

  it("字体族拆成拉丁/中日韩两段，中文排在系统中文回退之前", () => {
    expect(tokensCss).toContain('--font-latin: "Sora", "Sora Fallback"');
    expect(tokensCss).toContain('--font-cjk: "Noto Sans SC", "Noto Sans SC Fallback"');

    const stack = tokensCss.slice(tokensCss.indexOf("--font-sans:"), tokensCss.indexOf("--font-sans:") + 260);
    // --font-sans 通过 var(--font-cjk) 引用中文组：只要它在系统中文回退之前，
    // 中文就会优先命中自托管的 Noto Sans SC，而不是掉到 PingFang / YaHei。
    expect(stack.indexOf("var(--font-cjk)")).toBeGreaterThan(-1);
    expect(stack.indexOf("var(--font-cjk)")).toBeLessThan(stack.indexOf("PingFang SC"));
  });
});

describe("Tailwind 配置：色板与暗色策略", () => {
  const extend = tailwindConfig.theme?.extend as Record<string, Record<string, unknown>>;

  it("暗色策略是 class（站内开关才管得住 dark: 工具类）", () => {
    expect(tailwindConfig.darkMode).toBe("class");
  });

  it("语义色类名映射到 CSS 变量，配置里没有颜色字面量（颜色只有一个来源）", () => {
    expect(extend.colors.ink).toEqual({ DEFAULT: "var(--color-ink)", muted: "var(--color-ink-muted)" });
    expect(extend.colors.surface).toBe("var(--color-surface)");
    expect(extend.colors.line).toEqual({
      DEFAULT: "var(--color-line)",
      strong: "var(--color-line-strong)",
      faint: "var(--color-line-faint)",
    });
    expect(extend.colors.danger).toEqual({
      DEFAULT: "var(--color-danger)",
      ink: "var(--color-danger-ink)",
    });
    expect(JSON.stringify(tailwindConfig)).not.toMatch(/#[0-9a-f]{3,8}/iu);
  });

  it("排版尺度进 Tailwind：行宽/行高/字重/字体都引用 token 变量", () => {
    expect(extend.maxWidth.measure).toBe("var(--type-measure)");
    expect(extend.lineHeight.prose).toBe("var(--type-leading-prose)");
    expect(extend.fontWeight.title).toBe("var(--type-weight-title)");
    expect(extend.fontFamily.body).toBe("var(--font-sans)");
    expect(extend.fontFamily.cjk).toBe("var(--font-cjk)");
  });

  it("可编译：语义色/排版类真的生成了对应 CSS 声明", async () => {
    const css = await compileTailwindProbe({
      projectDir,
      html: '<div class="bg-surface text-ink-muted border-line max-w-measure leading-prose font-cjk font-title"></div>',
    });

    expect(css).toContain("background-color: var(--color-surface)");
    expect(css).toContain("color: var(--color-ink-muted)");
    expect(css).toContain("border-color: var(--color-line)");
    expect(css).toContain("max-width: var(--type-measure)");
    expect(css).toContain("line-height: var(--type-leading-prose)");
    expect(css).toContain("font-family: var(--font-cjk)");
    expect(css).toContain("font-weight: var(--type-weight-title)");
  });

  it("可编译：dark: 工具类挂在 .dark 祖先上，而不是跟系统偏好走（修掉历史 bug）", async () => {
    const css = await compileTailwindProbe({
      projectDir,
      html: '<div class="dark:bg-surface dark:text-ink"></div>',
    });

    // Tailwind 3.4 的 class 策略输出形如 `.dark\:bg-surface:is(.dark *)`。
    expect(css).toMatch(/\.dark\\:bg-surface:is\(\.dark \*\)/u);
    expect(css).not.toContain("prefers-color-scheme: dark");
  });
});

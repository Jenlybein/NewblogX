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

describe("原始层：五色锁死（克莱因蓝退役）", () => {
  it("五个锁定色相取到 spec 给定的值", () => {
    expect(tokensCss).toContain("--jelly-cream: #fdfbf6");
    expect(tokensCss).toContain("--jelly-warm: #f2eee6");
    expect(tokensCss).toContain("--jelly-warm-100: #f5f3ee");
    expect(tokensCss).toContain("--jelly-yellow: #f5c518");
    expect(tokensCss).toContain("--jelly-coral: #ff6b4a");
    expect(tokensCss).toContain("--jelly-coral-700: #c2401f");
    expect(tokensCss).toContain("--jelly-interactive: #0066cc");
  });

  it("克莱因蓝整个退役：不再有任何 --jelly-blue* / #002fa7 取值", () => {
    expect(tokensCss).not.toMatch(/--jelly-blue/iu);
    expect(tokensCss).not.toMatch(/#002fa7/iu);
    expect(tokensCss).not.toMatch(/rgb\(0\s*47\s*167/iu);
  });

  it("色相集合 = 奶油白 / 暖灰 / 黄 / 珊瑚橙 / 功能蓝（不新增第六个色相）", () => {
    const hues = new Set(
      [...tokensCss.matchAll(/--jelly-([a-z0-9-]+)\s*:/giu)]
        .map((match) => match[1]!.toLowerCase())
        .filter((name) => !name.endsWith("-rgb"))
        .map((name) => name.split("-")[0]!),
    );

    expect([...hues].sort()).toEqual(["coral", "cream", "interactive", "warm", "yellow"]);
  });

  it("石墨阶与功能蓝的三元组基底只定义两套派生源（亮色在 :root、暗色在 .dark）", () => {
    const root = /:root\s*\{([\s\S]*?)\n\}/u.exec(tokensCss)?.[1] ?? "";
    const dark = /(?:\.dark|\.theme-dark)[^\n{]*\{([\s\S]*?)\n\}/u.exec(tokensCss)?.[1] ?? "";

    expect(root).toMatch(/--graphite-rgb:\s*28\s+27\s+25;?/u);
    expect(root).toMatch(/--interactive-rgb:\s*0\s+102\s+204;?/u);
    expect(dark).toMatch(/--graphite-rgb:\s*245\s+245\s+247;?/u);
    expect(dark).toMatch(/--interactive-rgb:\s*41\s+151\s+255;?/u);
    expect(dark).toMatch(/--jelly-interactive:\s*#?2997ff;?/u);
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

    expect(light.length).toBeGreaterThanOrEqual(20);
    expect([...dark].sort()).toEqual([...light].sort());
  });

  it("中性石墨阶 / 功能蓝 / 三档结构线 / 高亮底 / 危险 / 反白层都在语义层", () => {
    for (const name of [
      "--color-bg-page",
      "--color-bg-subtle",
      "--color-surface",
      "--color-surface-raised",
      "--color-ink",
      "--color-ink-muted",
      "--color-ink-faint",
      "--color-line",
      "--color-line-strong",
      "--color-line-faint",
      "--color-link",
      "--color-focus",
      "--color-highlight",
      "--color-danger",
      "--color-danger-ink",
      "--color-danger-soft",
      "--color-danger-line",
      "--color-on-strong",
      "--color-on-danger",
      "--color-on-inverse",
      "--color-on-inverse-muted",
      "--color-surface-inverse",
      "--color-shadow",
    ]) {
      expect(tokensCss).toContain(`${name}:`);
    }
  });

  it("结构线三档全部由墨色三元组按 0.10 / 0.16 / 0.32 派生", () => {
    expect(tokensCss).toContain("--color-line-faint: rgb(var(--graphite-rgb) / 0.10)");
    expect(tokensCss).toContain("--color-line: rgb(var(--graphite-rgb) / 0.16)");
    expect(tokensCss).toContain("--color-line-strong: rgb(var(--graphite-rgb) / 0.32)");
  });

  it("黄永不作为字色：高亮 token 只做底，唯一黄色语义名是 --color-highlight", () => {
    const yellowSemanticNames = new Set(
      [...tokensCss.matchAll(/(--color-[a-z-]+)\s*:[^;]*--jelly-yellow/gu)].map((match) => match[1]),
    );

    expect([...yellowSemanticNames]).toEqual(["--color-highlight"]);
  });

  it("功能蓝只承担「文字链接」与「焦点环」两个语义角色", () => {
    const blueSemanticNames = new Set(
      [...tokensCss.matchAll(/(--color-[a-z-]+)\s*:[^;]*(?:--jelly-interactive|--interactive-rgb)/gu)].map(
        (match) => match[1],
      ),
    );

    expect([...blueSemanticNames].sort()).toEqual(["--color-focus", "--color-focus-soft", "--color-link"]);
  });
});

describe("排版层：中文尺度 token 化", () => {
  it("行宽 68ch、正文行高 1.75、标题字重与 -0.03em 大标题字距", () => {
    expect(tokensCss).toContain("--type-measure: 68ch");
    expect(tokensCss).toContain("--type-leading-prose: 1.75");
    expect(tokensCss).toContain("--type-weight-title: 700");
    expect(tokensCss).toMatch(/--type-tracking-title:\s*-0\.03em/u);
  });

  it("字体族拆成拉丁/中文两段：中文改走系统字体栈（不再自托管、不再压过系统中文）", () => {
    expect(tokensCss).toContain('--font-latin: "Inter", "Inter Fallback"');
    expect(tokensCss).toContain(
      '--font-cjk: -apple-system, BlinkMacSystemFont, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans SC", sans-serif',
    );

    const cjk = tokensCss.slice(
      tokensCss.indexOf("--font-cjk:"),
      tokensCss.indexOf("--font-cjk:") + 200,
    );
    // 中文组内部顺序：苹方 → 冬青黑 → 雅黑 → 末位 Noto（系统真装了才命中）→ sans-serif。
    expect(cjk.indexOf('"PingFang SC"')).toBeGreaterThan(-1);
    expect(cjk.indexOf('"PingFang SC"')).toBeLessThan(cjk.indexOf('"Microsoft YaHei"'));
    expect(cjk.indexOf('"Microsoft YaHei"')).toBeLessThan(cjk.indexOf('"Noto Sans SC"'));
    // 反向断言：自托管的中文 fallback 名与 @font-face 整包都不在 token 层。
    expect(tokensCss).not.toContain("Noto Sans SC Fallback");
    expect(tokensCss).not.toMatch(/noto-sans-sc/iu);

    // 中文组是唯一的中文来源：--font-sans 不再自己再排一遍系统中文。
    const stack = tokensCss.slice(tokensCss.indexOf("--font-sans:"), tokensCss.indexOf("--font-sans:") + 80);
    expect(stack).toContain("var(--font-latin), var(--font-cjk)");
    expect(stack).not.toContain("PingFang SC");
  });
});

describe("Tailwind 配置：色板与暗色策略", () => {
  const extend = tailwindConfig.theme?.extend as Record<string, Record<string, unknown>>;

  it("暗色策略是 class（站内开关才管得住 dark: 工具类）", () => {
    expect(tailwindConfig.darkMode).toBe("class");
  });

  it("语义色类名映射到 CSS 变量，配置里没有颜色字面量（颜色只有一个来源）", () => {
    expect(extend.colors.ink).toEqual({
      DEFAULT: "var(--color-ink)",
      muted: "var(--color-ink-muted)",
      faint: "var(--color-ink-faint)",
    });
    expect(extend.colors.surface).toBe("var(--color-surface)");
    expect(extend.colors["surface-inverse"]).toBe("var(--color-surface-inverse)");
    expect(extend.colors.link).toBe("var(--color-link)");
    expect(extend.colors.line).toEqual({
      DEFAULT: "var(--color-line)",
      strong: "var(--color-line-strong)",
      faint: "var(--color-line-faint)",
    });
    expect(extend.colors.danger).toEqual({
      DEFAULT: "var(--color-danger)",
      ink: "var(--color-danger-ink)",
      soft: "var(--color-danger-soft)",
      line: "var(--color-danger-line)",
    });
    expect(extend.colors.inverse).toEqual({
      DEFAULT: "var(--color-on-inverse)",
      muted: "var(--color-on-inverse-muted)",
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
      html: '<div class="bg-surface text-ink-muted text-ink-faint border-line text-link bg-surface-inverse text-inverse max-w-measure leading-prose font-cjk font-title"></div>',
    });

    expect(css).toContain("background-color: var(--color-surface)");
    expect(css).toContain("color: var(--color-ink-muted)");
    expect(css).toContain("color: var(--color-ink-faint)");
    expect(css).toContain("color: var(--color-link)");
    expect(css).toContain("color: var(--color-on-inverse)");
    expect(css).toContain("background-color: var(--color-surface-inverse)");
    expect(css).toContain("border-color: var(--color-line)");
    expect(css).toContain("max-width: var(--type-measure)");
    expect(css).toContain("line-height: var(--type-leading-prose)");
    expect(css).toContain("font-family: var(--font-cjk)");
    expect(css).toContain("font-weight: var(--type-weight-title)");
  }, 15000);

  it("可编译：dark: 工具类挂在 .dark 祖先上，而不是跟系统偏好走（修掉历史 bug）", async () => {
    const css = await compileTailwindProbe({
      projectDir,
      html: '<div class="dark:bg-surface dark:text-ink"></div>',
    });

    // Tailwind 3.4 的 class 策略输出形如 `.dark\:bg-surface:is(.dark *)`。
    expect(css).toMatch(/\.dark\\:bg-surface:is\(\.dark \*\)/u);
    expect(css).not.toContain("prefers-color-scheme: dark");
  }, 15000);
});

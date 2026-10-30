import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import tailwindConfig from "../../tailwind.config";
import { compileTailwindProbe } from "../../../scripts/tailwind-probe.mjs";
import { readProjectEntries } from "../../../scripts/check-design-tokens.mjs";
import { MOTION_REVEAL_OFFSET_PX, PARALLAX_MAX_PX } from "~/utils/motion";

/**
 * #18 地基票的结构断言：三元组派生、新 token 种子与真实接线、旧层删除、门禁前提。
 *
 * 这一层是"数据面"测试：直接解析 tokens.css 与 Tailwind 编译产物，断言**可观察的
 * 取值约定**（不测组件怎么用）。redeem 之后由后续票逐面消费这些种子。
 */
const projectDir = process.cwd();
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
const adminTokensCss = readFileSync(
  join(projectDir, "../frontend_admin/app/assets/css/tokens.css"),
  "utf8",
);
const indexVue = readFileSync(join(projectDir, "app/pages/index.vue"), "utf8");
const markdownThemeTs = readFileSync(join(projectDir, "app/utils/markdownTheme.ts"), "utf8");


/** 去掉 CSS 注释：注释里可以提到旧 token 名/旧写法（可读性需要），不算"还在用"。 */
function stripComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//gu, "");
}

/** 某个 token 是否真的被声明（注释里提到不算）。 */
function isDeclared(css: string, name: string): boolean {
  return new RegExp(`${name}\\s*:`, "u").test(stripComments(css));
}

/** 取 `:root` 块（第一份；减少动态块里那个在它之后，不会被选中）。 */
function rootBlock(css: string): string {
  const match = /:root\s*\{([\s\S]*?)\n\}/u.exec(css);
  if (!match) throw new Error("tokens.css 里找不到 :root 块");
  return match[1];
}

/** 取暗色映射块（`.dark` / `.theme-dark`）。 */
function darkBlock(css: string): string {
  const match = /(?:\.dark|\.theme-dark)[^\n{]*\{([\s\S]*?)\n\}/u.exec(css);
  if (!match) throw new Error("tokens.css 里找不到暗色映射块");
  return match[1];
}

/** 取某个媒体查询块的内容（按花括号配对，够用即可）。 */
function mediaBlock(css: string, query: string): string {
  const start = css.indexOf(`@media (${query})`);
  if (start < 0) throw new Error(`tokens.css 里找不到 @media (${query})`);
  const open = css.indexOf("{", start);
  let depth = 0;
  for (let index = open; index < css.length; index += 1) {
    if (css[index] === "{") depth += 1;
    if (css[index] === "}") {
      depth -= 1;
      if (depth === 0) return css.slice(open + 1, index);
    }
  }
  throw new Error("媒体查询块没有闭合");
}

/** 某个块里某条自定义属性的取值（去注释、去首尾空白）。 */
function declaration(block: string, name: string): string | undefined {
  const match = new RegExp(`${name}\\s*:\\s*([^;\\n]+?)\\s*(?:;|\\/\\*)`, "u").exec(block);
  return match?.[1]?.trim();
}

/**
 * 必须由三元组派生的语义色：结构线三档、悬停/强调底、危险底与危险线、
 * 强底次级字、阴影、高亮底。值形如 `rgb(var(--jelly-x-rgb) / α)`。
 */
const ALPHA_SEMANTIC_TOKENS = [
  "--color-line",
  "--color-line-strong",
  "--color-line-faint",
  "--color-focus-soft",
  "--color-danger-soft",
  "--color-danger-line",
  "--color-on-strong-muted",
  "--color-shadow",
  "--color-highlight",
];

const TRIPLET_VALUE_RE = /^rgb\(var\(--[a-z0-9-]+-rgb\) \/ [0-9.]+\)$/u;

describe("三元组派生：带透明度的语义色不再写死 rgba(r,g,b,α)", () => {
  it.each(ALPHA_SEMANTIC_TOKENS)("%s 的明暗两份取值都是 rgb(var(--*-rgb) / α)", (name) => {
    const light = declaration(rootBlock(tokensCss), name);
    const dark = declaration(darkBlock(tokensCss), name);

    expect(light, `亮色 ${name}`).toMatch(TRIPLET_VALUE_RE);
    expect(dark, `暗色 ${name}`).toMatch(TRIPLET_VALUE_RE);
  });

  it("tokens.css 里没有 rgba() 常量，也没有逗号分隔的 rgb() 历史写法", () => {
    const code = stripComments(tokensCss);

    expect(code).not.toMatch(/\brgba\(/u);
    expect(code).not.toMatch(/\brgb\(\s*\d+\s*,/u);
  });

  it("样式与组件里也没有 rgba(r,g,b,α) 常量（仓库内归零）", () => {
    // .vue / .css 全量扫描（tokens.css 本身也已在上一条断言过）。
    const offenders = readProjectEntries(projectDir).filter(
      (entry) => entry.file.endsWith(".vue") || entry.file.endsWith(".css"),
    );

    for (const entry of offenders) {
      expect(entry.text, `${entry.file} 不应出现 rgba() 常量`).not.toMatch(/\brgba\(/u);
    }
  });

  it.each([
    ["读者站", tokensCss],
    ["后台", adminTokensCss],
  ])("%s的三元组基底与原始层 hex 同值（两份定义不会漂移）", (_label, css) => {
    const root = rootBlock(css);
    const triplets = [
      ...root.matchAll(/--jelly-([a-z0-9-]+)-rgb\s*:\s*(\d+)\s+(\d+)\s+(\d+);/gu),
    ].map((match) => ({
      base: match[1]!,
      channels: [Number(match[2]), Number(match[3]), Number(match[4])],
    }));

    expect(triplets.length).toBeGreaterThanOrEqual(3);

    for (const { base, channels } of triplets) {
      // 三元组不得引入第六个色相：基底名必须以锁定色相之一开头。
      expect(["blue", "cream", "yellow", "coral", "warm"]).toContain(base.split("-")[0]);

      const hex = declaration(root, `--jelly-${base}`);
      if (!hex) continue; // 暗色阴影专用基底（-1000）本来就没有对应的 hex。
      const raw = hex.replace("#", "");
      const hexChannels = [0, 2, 4].map((offset) => Number.parseInt(raw.slice(offset, offset + 2), 16));

      expect(channels).toEqual(hexChannels);
    }
  });

  it("读者站的石墨阶 / 功能蓝三元组不引入新色相，暗色换第二份取值", () => {
    const root = rootBlock(tokensCss);
    const dark = darkBlock(tokensCss);

    expect(declaration(root, "--graphite-rgb")).toMatch(/^28\s+27\s+25$/u);
    expect(declaration(root, "--interactive-rgb")).toMatch(/^0\s+102\s+204$/u);
    expect(declaration(dark, "--graphite-rgb")).toMatch(/^245\s+245\s+247$/u);
    expect(declaration(dark, "--interactive-rgb")).toMatch(/^41\s+151\s+255$/u);
  });
});

describe("圆角阶梯：五档种子（控件 8/14、卡片 20、浮层 28、胶囊 pill）", () => {
  const RADIUS_TIERS = new Map([
    ["--radius-sm", "8px"],
    ["--radius-md", "14px"],
    ["--radius-lg", "20px"],
    ["--radius-xl", "28px"],
    ["--radius-pill", "999px"],
  ]);

  it.each([...RADIUS_TIERS])("%s 取到规定值", (name, value) => {
    expect(declaration(rootBlock(tokensCss), name)).toBe(value);
    expect(declaration(rootBlock(adminTokensCss), name)).toBe(value);
  });

  it("Tailwind 有对应类名且真的编译成 token 变量（不是只写了个数字）", async () => {
    const extend = tailwindConfig.theme?.extend as Record<string, Record<string, unknown>>;

    expect(extend.borderRadius).toEqual({
      control: "var(--radius-sm)",
      field: "var(--radius-md)",
      card: "var(--radius-lg)",
      overlay: "var(--radius-xl)",
      pill: "var(--radius-pill)",
    });

    const css = await compileTailwindProbe({
      projectDir,
      html: '<div class="rounded-control rounded-field rounded-card rounded-overlay rounded-pill"></div>',
    });

    expect(css).toContain("border-radius: var(--radius-sm)");
    expect(css).toContain("border-radius: var(--radius-md)");
    expect(css).toContain("border-radius: var(--radius-lg)");
    expect(css).toContain("border-radius: var(--radius-xl)");
    expect(css).toContain("border-radius: var(--radius-pill)");
  }, 15000);
});

describe("液态玻璃组：两档的填充/模糊/饱和度 + 边缘/噪点/移动端档", () => {
  const LIGHT_GLASS = new Map([
    ["--glass-thin-fill", "rgb(255 255 255 / 0.72)"],
    ["--glass-thin-blur", "20px"],
    ["--glass-thin-saturate", "180%"],
    ["--glass-thick-fill", "rgb(253 251 246 / 0.80)"],
    ["--glass-thick-blur", "28px"],
    ["--glass-thick-saturate", "160%"],
    ["--glass-rim", "rgb(255 255 255 / 0.62)"],
    ["--glass-rim-warm", "rgb(255 253 246 / 0.50)"],
    ["--glass-grain-opacity", "0.055"],
    ["--glass-blur-mobile", "12px"],
  ]);

  const DARK_GLASS = new Map([
    ["--glass-thin-fill", "rgb(22 22 23 / 0.72)"],
    ["--glass-thick-fill", "rgb(29 29 31 / 0.82)"],
    ["--glass-rim", "rgb(255 255 255 / 0.30)"],
    ["--glass-rim-warm", "rgb(255 253 246 / 0.22)"],
    ["--glass-grain-opacity", "0.045"],
  ]);

  it.each([...LIGHT_GLASS])("亮色 %s 取到规定值", (name, value) => {
    expect(declaration(rootBlock(tokensCss), name)).toBe(value);
  });

  it.each([...DARK_GLASS])("暗色 %s 有第二份取值", (name, value) => {
    expect(declaration(darkBlock(tokensCss), name)).toBe(value);
  });

  it("玻璃参数被 Tailwind 类名真实消费（编译产物里能看到 var）", async () => {
    const extend = tailwindConfig.theme?.extend as Record<string, Record<string, unknown>>;

    expect(extend.colors.glass).toEqual({
      thin: "var(--glass-thin-fill)",
      thick: "var(--glass-thick-fill)",
      rim: "var(--glass-rim)",
      "rim-warm": "var(--glass-rim-warm)",
    });
    expect(extend.backdropBlur).toEqual({
      "glass-thin": "var(--glass-thin-blur)",
      "glass-thick": "var(--glass-thick-blur)",
      "glass-mobile": "var(--glass-blur-mobile)",
    });
    expect(extend.backdropSaturate).toEqual({
      "glass-thin": "var(--glass-thin-saturate)",
      "glass-thick": "var(--glass-thick-saturate)",
    });
    expect(extend.opacity).toEqual({ grain: "var(--glass-grain-opacity)" });

    const css = await compileTailwindProbe({
      projectDir,
      html: '<div class="bg-glass-thin border-glass-rim border-glass-rim-warm backdrop-blur-glass-thin backdrop-saturate-glass-thin backdrop-blur-glass-mobile opacity-grain"></div>',
    });

    expect(css).toContain("background-color: var(--glass-thin-fill)");
    expect(css).toContain("border-color: var(--glass-rim)");
    expect(css).toContain("border-color: var(--glass-rim-warm)");
    expect(css).toContain("blur(var(--glass-thin-blur))");
    expect(css).toContain("saturate(var(--glass-thin-saturate))");
    expect(css).toContain("blur(var(--glass-blur-mobile))");
    expect(css).toContain("opacity: var(--glass-grain-opacity)");
  }, 15000);
});

describe("动效补充档位：滚动进场 12px / 视差 10px（与 motion.ts 同一份预算）", () => {
  it("motion.ts 常量就是档位上限", () => {
    expect(MOTION_REVEAL_OFFSET_PX).toBe(12);
    expect(PARALLAX_MAX_PX).toBe(10);
  });

  it("tokens.css 与 motion.ts 同源（改一处必须两处同改）", () => {
    expect(declaration(rootBlock(tokensCss), "--motion-reveal-offset")).toBe(
      `${MOTION_REVEAL_OFFSET_PX}px`,
    );
    expect(declaration(rootBlock(tokensCss), "--parallax-max")).toBe(`${PARALLAX_MAX_PX}px`);
  });

  it("减少动态时位移归零（不是放慢）", () => {
    const reduced = mediaBlock(tokensCss, "prefers-reduced-motion: reduce");

    expect(reduced).toContain("--motion-reveal-offset: 0px");
    expect(reduced).toContain("--parallax-max: 0px");
  });
});

describe("字体分工键：display / title / mono 都有定义与真实消费点", () => {
  it.each([
    ["读者站", tokensCss],
    ["后台", adminTokensCss],
  ])("%s定义 --font-display / --font-title / --font-mono", (_label, css) => {
    const root = rootBlock(css);

    expect(declaration(root, "--font-display")).toBe('"Sora", "Sora Fallback", var(--font-cjk)');
    expect(declaration(root, "--font-title")).toBe("var(--font-display)");
    expect(declaration(root, "--font-mono")).toContain('"Cascadia Code"');
  });

  it("标题字体键被首屏/字标真实消费（此前是未定义变量）", () => {
    expect(indexVue).toContain("var(--font-title)");
  });

  it("阅读站等宽字体键被 markdown 代码块真实消费（此前是未定义变量）", () => {
    expect(markdownThemeTs).toContain("font-family: var(--font-mono)");
  });

  it("Tailwind 暴露 display / mono / title 类名（接线，不改变既有字体取值归属）", async () => {
    const extend = tailwindConfig.theme?.extend as Record<string, Record<string, unknown>>;

    expect(extend.fontFamily.display).toBe("var(--font-display)");
    expect(extend.fontFamily.mono).toBe("var(--font-mono)");
    expect(extend.fontFamily.title).toBe("var(--font-title)");

    const css = await compileTailwindProbe({
      projectDir,
      html: '<div class="font-display font-mono font-title"></div>',
    });

    expect(css).toContain("font-family: var(--font-display)");
    expect(css).toContain("font-family: var(--font-mono)");
    expect(css).toContain("font-family: var(--font-title)");
  }, 15000);

  it("#21 把拉丁正文/UI 交给 Inter（display 仍归 Sora）", () => {
    expect(declaration(rootBlock(tokensCss), "--font-latin")).toBe('"Inter", "Inter Fallback"');
    expect(declaration(rootBlock(adminTokensCss), "--font-latin")).toBe(
      '"Inter", "Inter Fallback"',
    );
  });
});

describe("无消费者的旧 token 已删除（含 v1 青绿与渐变）", () => {
  it.each([
    ["--bg-page", "过渡层页面渐变"],
    ["--bg-page-dark", "过渡层暗色渐变"],
    ["--surface-strong", "过渡层纸面"],
    ["--surface-dark", "过渡层纸面"],
    ["--line-dark", "过渡层结构线"],
    ["--text-main", "过渡层正文色"],
    ["--text-soft", "过渡层次级色"],
    ["--accent", "v1 青绿主色"],
    ["--accent-soft", "v1 青绿悬停底"],
    ["--shadow-soft", "过渡层阴影"],
    ["--shadow-strong", "过渡层阴影"],
    ["#0f766e", "v1 青绿主色字面量"],
    ["--jelly-blue-200", "无引用派生色"],
    ["--jelly-blue-100", "无引用派生色"],
    ["--color-on-danger-pressed", "无消费者的语义派生色"],
  ])("%s（%s）不在 tokens.css 里", (token) => {
    const exact = new RegExp(`${token}(?![a-z0-9-])`, "u");

    expect(isDeclared(tokensCss, token)).toBe(false);
    expect(isDeclared(adminTokensCss, token)).toBe(false);
    expect(stripComments(tokensCss)).not.toMatch(exact);
    expect(stripComments(adminTokensCss)).not.toMatch(exact);
  });

  it("读者站不再有克莱因蓝族（#20 视觉语言翻转后整个删除）", () => {
    for (const token of [
      "--jelly-blue",
      "--jelly-blue-950",
      "--jelly-blue-900",
      "--jelly-blue-800",
      "--jelly-blue-700",
      "--jelly-blue-600",
      "--jelly-blue-400",
      "--jelly-cream-700",
    ]) {
      expect(isDeclared(tokensCss, token)).toBe(false);
      expect(stripComments(tokensCss)).not.toMatch(new RegExp(`${token}(?![a-z0-9-])`, "u"));
    }
  });

  it("全仓样式/组件/TS 都不再消费这些旧变量", () => {
    const deadRefs = [
      "var(--bg-page)",
      "var(--bg-page-dark)",
      "var(--surface)",
      "var(--surface-strong)",
      "var(--surface-dark)",
      "var(--line)",
      "var(--line-dark)",
      "var(--text-main)",
      "var(--text-soft)",
      "var(--accent)",
      "var(--accent-soft)",
      "var(--shadow-soft)",
      "var(--shadow-strong)",
    ];
    const sources = readProjectEntries(projectDir);

    expect(sources.length).toBeGreaterThan(0);
    for (const entry of sources) {
      for (const dead of deadRefs) {
        expect(entry.text, `${entry.file} 不应消费已删除的过渡层变量 ${dead}`).not.toContain(dead);
      }
    }
  });
});

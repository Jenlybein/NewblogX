import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 静态扫描：暗色下不许出现"硬编码白 / 浅色块"。
 *
 * 为什么需要它：漂移门禁只挡"裸色值"，但 `bg-white/80`、`text-white`、`border-slate-200`
 * 这些**类名**不含颜色字面量，门禁看不见；可在克莱因蓝暗色里它们就是一块块白补丁。
 *
 * 扫描器是纯函数（输入 file → content 的映射），先跑一遍"人造违规 canary"证明它真的会报，
 * 再扫真实组件，避免写出一条永远绿的假测试。
 */

export interface DarkSurfaceRule {
  id: string;
  description: string;
  pattern: RegExp;
}

export const DARK_SURFACE_RULES: DarkSurfaceRule[] = [
  {
    id: "pure-white-hex",
    description: "纯白 hex：暗色下必然是一块白",
    pattern: /#(?:fff|ffffff)(?![0-9a-fA-F])/gu,
  },
  {
    id: "white-alpha",
    description: "白色半透明叠加：暗色下变成浅色雾面",
    pattern: /rgba?\(\s*255\s*,\s*255\s*,\s*255/gu,
  },
  {
    id: "white-utility",
    description: "白底/白字/白描边工具类：不吃 token，明暗都不翻转",
    pattern: /\b(?:bg|text|border|ring|fill|stroke)-white(?:\/[0-9]{1,3})?\b/gu,
  },
  {
    id: "light-gradient-stop",
    description: "渐变里的浅色停靠点（整块发亮的卡片）",
    pattern: /(?:linear|radial)-gradient\([^;]*rgba\(\s*2[4-9]\d\s*,/gu,
  },
  {
    id: "light-slate-surface",
    description: "浅灰底/浅灰描边：暗色下是浅块",
    pattern: /\b(?:bg|border|ring)-(?:slate|gray|zinc|stone)-(?:50|100|200)(?:\/[0-9]{1,3})?\b/gu,
  },
];

export interface DarkSurfaceViolation {
  file: string;
  line: number;
  rule: string;
  match: string;
}

export interface DarkSurfaceExemption {
  file: string;
  rule: string;
  reason: string;
}

/**
 * 纯扫描函数。`exemptions` 必须写清理由（白名单不是垃圾桶）。
 */
export function scanDarkSurfaces(
  files: Record<string, string>,
  exemptions: DarkSurfaceExemption[] = [],
): DarkSurfaceViolation[] {
  const violations: DarkSurfaceViolation[] = [];

  for (const [file, content] of Object.entries(files)) {
    const lines = content.split(/\r?\n/u);
    for (const rule of DARK_SURFACE_RULES) {
      const isExempt = exemptions.some((item) => item.file === file && item.rule === rule.id);
      if (isExempt) {
        continue;
      }
      for (const [index, line] of lines.entries()) {
        // 行内注释里的示例代码不算违规：先去掉 // 与 <!-- --> 之后的部分。
        const code = line.replace(/\/\/.*$/u, "").replace(/<!--[\s\S]*?-->/gu, "");
        rule.pattern.lastIndex = 0;
        const match = rule.pattern.exec(code);
        if (match) {
          violations.push({ file, line: index + 1, rule: rule.id, match: match[0] });
        }
      }
    }
  }

  return violations;
}

function collectVueFiles(directory: string, cwd: string): Record<string, string> {
  const files: Record<string, string> = {};
  for (const entry of readdirSync(directory)) {
    const absolute = join(directory, entry);
    if (statSync(absolute).isDirectory()) {
      Object.assign(files, collectVueFiles(absolute, cwd));
      continue;
    }
    if (!entry.endsWith(".vue")) {
      continue;
    }
    files[relative(cwd, absolute).replace(/\\/gu, "/")] = readFileSync(absolute, "utf8");
  }
  return files;
}

const projectDir = process.cwd();
const componentFiles = {
  ...collectVueFiles(join(projectDir, "app/components"), projectDir),
  ...collectVueFiles(join(projectDir, "app/pages"), projectDir),
  ...collectVueFiles(join(projectDir, "app/layouts"), projectDir),
};

const EXEMPTIONS: DarkSurfaceExemption[] = [];

describe("扫描器本身：人造违规 canary 必须被抓住", () => {
  it("四种规则都能命中（防止扫描器写成永远绿）", () => {
    const canary = [
      "<template><div class=\"bg-white/80 border-slate-200 text-white\" /></template>",
      "<style>",
      ".bad { background: #ffffff; box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.4); }",
      ".worse { background: linear-gradient(135deg, rgba(240, 253, 250, 0.94), transparent); }",
      "</style>",
    ].join("\n");

    const hits = scanDarkSurfaces({ "app/components/Canary.vue": canary });

    expect(hits.map((item) => item.rule)).toEqual(
      expect.arrayContaining([
        "pure-white-hex",
        "white-alpha",
        "white-utility",
        "light-slate-surface",
        "light-gradient-stop",
      ]),
    );
    expect(hits.length).toBeGreaterThanOrEqual(5);
  });

  it("同一条规则能在同一行/不同行分别报出，并带行号", () => {
    const hits = scanDarkSurfaces({
      "app/components/Canary.vue": "<style>\n.a { color: #fff; }\n.b { color: #ffffff; }\n</style>",
    });

    expect(hits.map((item) => item.line)).toEqual([2, 3]);
  });

  it("豁免必须 file + rule 精确匹配；没写理由的白名单不生效", () => {
    const files = { "app/components/Canary.vue": ".a { background: #ffffff; }" };
    const exemptions: DarkSurfaceExemption[] = [
      { file: "app/components/Canary.vue", rule: "pure-white-hex", reason: "测试用" },
    ];

    expect(scanDarkSurfaces(files, exemptions)).toHaveLength(0);
    expect(scanDarkSurfaces(files, [])).toHaveLength(1);
  });
});

describe("真实组件：暗色下没有硬编码白/浅色块", () => {
  it("app/components + app/pages + app/layouts 下的 .vue 全部干净（豁免项除外）", () => {
    expect(scanDarkSurfaces(componentFiles, EXEMPTIONS)).toEqual([]);
  });

  it("豁免清单为空：所有读者站组件都吃语义 token，#20 之后没有白名单", () => {
    expect(EXEMPTIONS).toEqual([]);
  });
});

describe("AI 面板 / Markdown 渲染面：不再注入外部 light 主题", () => {
  const read = (path: string) => readFileSync(join(projectDir, path), "utf8");

  it("InboxChatPanel / JellyAiPanel 不再引用 github-markdown-light 与 highlight 浅色主题", () => {
    for (const path of ["app/components/inbox/InboxChatPanel.vue", "app/components/jelly/JellyAiPanel.vue"]) {
      const source = read(path);

      expect(source).not.toMatch(/github-markdown-light/u);
      expect(source).not.toMatch(/highlight\.js\/styles\/(?:github|github\.min)\.css/u);
      // 仅保留 katex 的数学字体样式（无底色，跟随正文墨色）。
      expect(source).toContain("katex");
    }
  });

  it("MarkdownRenderSurface 的默认主题为空：走 #8 的自有 token 主题，明暗自动跟随", () => {
    const source = read("app/components/common/MarkdownRenderSurface.vue");

    expect(source).toContain('themeHref: ""');
    expect(source).toContain("StudioMarkdownShadowPreview");
  });

  it("MarkdownShadowPreview 始终注入自有主题样式（外部 href 只是可选叠加）", () => {
    const source = read("app/components/studio/MarkdownShadowPreview.vue");

    expect(source).toContain("MARKDOWN_THEME_STYLE");
    expect(source).toContain("buildMarkdownThemeCss");
  });
});

describe("切换动效：走 motion token，减少动态时归零", () => {
  const tailwindCss = readFileSync(join(projectDir, "app/assets/css/tailwind.css"), "utf8");
  const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");

  it("body 的颜色/背景过渡用 --motion-base + --motion-ease，不再写死 0.28s", () => {
    const bodyRule = /body\s*\{[\s\S]*?\}/u.exec(tailwindCss)?.[0] ?? "";

    expect(bodyRule).toContain("var(--motion-base)");
    expect(bodyRule).toContain("var(--motion-ease)");
    expect(bodyRule).not.toMatch(/0\.28s/u);
  });

  it("减少动态时现存动效 token 归零：切换不产生过渡", () => {
    const reduceBlock =
      /@media \(prefers-reduced-motion: reduce\)\s*\{[\s\S]*?\n\}/u.exec(tokensCss)?.[0] ?? "";

    // --motion-slow / --motion-stagger 已因零 var() 引用被删（spec A2），这里只断言现存的。
    for (const token of ["--motion-fast", "--motion-base", "--motion-reveal-offset", "--parallax-max"]) {
      expect(reduceBlock).toContain(`${token}: 0`);
    }
  });
});

describe("暗色铺色：中性石墨阶 + 浅石墨字（不是深蓝/暖黑）", () => {
  const tailwindCss = readFileSync(join(projectDir, "app/assets/css/tailwind.css"), "utf8");

  it("body 的暗色取值改吃语义 token，并覆盖水合前的 html.theme-dark", () => {
    expect(tailwindCss).toContain("html.theme-dark body");
    expect(tailwindCss).toContain("var(--color-bg-page)");
    expect(tailwindCss).toContain("var(--color-ink)");
  });

  it("旧辅助类的暗色覆盖不再用深灰蓝/接近黑的旧变量", () => {
    expect(tailwindCss).not.toContain("var(--surface-dark)");
    expect(tailwindCss).not.toContain("var(--line-dark)");
    expect(tailwindCss).not.toContain("var(--bg-page-dark)");
    expect(tailwindCss).not.toContain("rgba(12, 20, 31");
    expect(tailwindCss).not.toContain("rgba(15, 23, 42");
  });
});

describe("切换不跳版：暗色映射只换颜色，不动字号/间距", () => {
  const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
  const darkBlock = /(?:.dark|.theme-dark)[^\n{]*\u007b([\s\S]*?)\n\s*\u007d/u.exec(tokensCss)?.[1] ?? "";

  it("暗色块里没有 font-size / line-height / padding / margin 之类的布局声明", () => {
    expect(darkBlock).not.toMatch(/(?:font-size|line-height|letter-spacing|padding|margin|gap|width|height)\s*:/u);
  });

  it("原生控件跟随：:root 是 light，暗色块是 dark（color-scheme 两份）", () => {
    expect(tokensCss).toContain("color-scheme: light;");
    expect(darkBlock).toContain("color-scheme: dark;");
  });
});

describe("用户选择后根类跟着走：htmlAttrs 也由 store 驱动", () => {
  const appSource = readFileSync(join(projectDir, "app/app.vue"), "utf8");

  it("app.vue 用 themeHtmlClass(uiStore.theme) 绑定 html 类（避免切换后 html 残留旧类）", () => {
    expect(appSource).toContain("htmlAttrs");
    expect(appSource).toContain("themeHtmlClass(uiStore.theme)");
  });

  it("首屏脚本 + htmlAttrs 都在：前者防闪白，后者保证切换生效", () => {
    expect(appSource).toContain("buildThemeInitScript()");
    expect(appSource).toContain("bodyAttrs");
  });
});

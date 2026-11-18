import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

import {
  buildReaderThemeOverrides,
  parseReaderThemePalette,
  parseReaderThemeScale,
} from "~/utils/readerTheme";

// vitest 以 project root（frontend/）为 cwd。
const projectDir = process.cwd();
const repoRoot = join(projectDir, "..");
const fontsCss = readFileSync(join(projectDir, "app/assets/css/fonts.css"), "utf8");
const tailwindCss = readFileSync(join(projectDir, "app/assets/css/tailwind.css"), "utf8");
const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
const adminFontsCss = readFileSync(
  join(repoRoot, "frontend_admin/app/assets/css/fonts.css"),
  "utf8",
);
const adminTokensCss = readFileSync(
  join(repoRoot, "frontend_admin/app/assets/css/tokens.css"),
  "utf8",
);
const appVue = readFileSync(join(projectDir, "app/app.vue"), "utf8");
const adminAppVue = readFileSync(join(repoRoot, "frontend_admin/app/app.vue"), "utf8");
const indexVue = readFileSync(join(projectDir, "app/pages/index.vue"), "utf8");
// 样式源已外移到 markdownThemeCss.ts（受「单文件 ≤ 800 行」约束）：
// 这里把两份都读进来，字体键被代码块真实消费这件事不因文件拆分而失效。
const markdownThemeTs = [
  readFileSync(join(projectDir, "app/utils/markdownTheme.ts"), "utf8"),
  readFileSync(join(projectDir, "app/utils/markdownThemeCss.ts"), "utf8"),
].join("\n");
const wordmarkVue = readFileSync(join(projectDir, "app/components/brand/Wordmark.vue"), "utf8");
const adminWordmarkVue = readFileSync(
  join(repoRoot, "frontend_admin/app/components/admin/BrandWordmark.vue"),
  "utf8",
);

const faces = [...fontsCss.matchAll(/@font-face\s*\{([^}]*)\}/gu)].map((match) => match[1].trim());

function facesOfFamily(css: string, family: string): string[] {
  const pattern = new RegExp(`font-family:\\s*["']${family}["']`, "u");
  return [...css.matchAll(/@font-face\s*\{([^}]*)\}/gu)]
    .map((match) => match[1].trim())
    .filter((face) => pattern.test(face));
}

/** 取 `:root` 块的声明值（去掉注释与多余空白）。 */
function declaration(css: string, name: string): string {
  const root = /:root\s*\{([\s\S]*?)\n\}/u.exec(css)?.[1] ?? "";
  const match = new RegExp(`${name}\\s*:\\s*([^;]+);`, "u").exec(root);
  return (match?.[1] ?? "").replace(/\s+/gu, " ").trim();
}

/** 取某条 CSS 规则体（只用于结构断言）。 */
function ruleBody(css: string, selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");
  const match = new RegExp(`${escaped}\\s*\\{([^}]*)\\}`, "u").exec(css);
  if (!match) throw new Error(`找不到 CSS 规则 ${selector}`);
  return match[1]!;
}

function stripComments(source: string): string {
  return source.replace(/\/\*[\s\S]*?\*\//gu, "");
}

/**
 * 票 430：字体角色收口的数据面。
 *
 * 扫读者站 `.vue` 的每个 `<style>` 块与独立 `.css`（跳过 tokens.css / fonts.css ——
 * 前者只声明 token、后者的 `@font-face` 不是"消费"），把 CSS 规则拆成
 * `{ file, selector, body }`。字号从 `@apply` 里的 `text-*` 工具类、`text-[18px]`
 * 与显式 `font-size` 取，clamp 只取下限（下限才是"会不会出现在小字号"的判据）。
 */
function styleSources(): { file: string; css: string }[] {
  const sources: { file: string; css: string }[] = [];
  const skip = new Set(["node_modules", ".nuxt", ".output", "coverage"]);
  const walk = (dir: string) => {
    for (const name of readdirSync(dir)) {
      if (skip.has(name)) continue;
      const full = join(dir, name);
      if (statSync(full).isDirectory()) {
        walk(full);
        continue;
      }
      const file = relative(repoRoot, full).split("\\").join("/");
      if (name.endsWith("tokens.css") || name.endsWith("fonts.css")) continue;
      if (name.endsWith(".vue")) {
        const text = stripComments(readFileSync(full, "utf8"));
        for (const block of text.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/gu)) {
          sources.push({ file, css: block[1]! });
        }
        continue;
      }
      if (name.endsWith(".css")) {
        sources.push({ file, css: stripComments(readFileSync(full, "utf8")) });
      }
    }
  };

  walk(join(projectDir, "app"));
  return sources;
}

interface CssRule {
  file: string;
  selector: string;
  body: string;
}

function cssRules(): CssRule[] {
  const rules: CssRule[] = [];
  for (const source of styleSources()) {
    for (const match of source.css.matchAll(/([^{}]+)\{([^{}]*)\}/gu)) {
      const selector = match[1]!.trim().replace(/\s+/gu, " ");
      if (!selector || selector.startsWith("@")) continue;
      rules.push({ file: source.file, selector, body: match[2]! });
    }
  }
  return rules;
}

const TW_FONT_SIZE_REM: Record<string, number> = {
  xs: 0.75,
  sm: 0.875,
  base: 1,
  lg: 1.125,
  xl: 1.25,
  "2xl": 1.5,
  "3xl": 1.875,
  "4xl": 2.25,
  "5xl": 3,
  "6xl": 3.75,
};

function remOf(value: string, unit: string): number {
  return unit === "rem" ? Number(value) : Number(value) / 16;
}

/** 规则里声明的最小字号（rem）；没有声明字号时返回 null。 */
function minDeclaredFontSizeRem(body: string): number | null {
  const sizes: number[] = [];
  for (const match of body.matchAll(/\btext-(xs|sm|base|lg|xl|2xl|3xl|4xl|5xl|6xl)\b/gu)) {
    sizes.push(TW_FONT_SIZE_REM[match[1]!]!);
  }
  for (const match of body.matchAll(/text-\[([\d.]+)px\]/gu)) {
    sizes.push(Number(match[1]) / 16);
  }
  for (const match of body.matchAll(/font-size:\s*([\d.]+)(rem|px)/gu)) {
    sizes.push(remOf(match[1]!, match[2]!));
  }
  for (const match of body.matchAll(/font-size:\s*clamp\(\s*([\d.]+)(rem|px)/gu)) {
    sizes.push(remOf(match[1]!, match[2]!));
  }
  return sizes.length > 0 ? Math.min(...sizes) : null;
}

const SORA_RE = /font-title|font-display|var\(--font-title\)|var\(--font-display\)/u;
const INTER_RE = /font-body|font-latin|var\(--font-sans\)|var\(--font-latin\)/u;

/**
 * Sora 的登记清单（字标 + 页面级大标题）。
 *
 * "登记"不是文档，是门禁：新增一个 Sora 消费点就会在这里红，逼着作者想清楚
 * "这张卡片的标题真的需要 display 字体吗"。票 430 的答案是：18–20px 的卡片标题
 * 与计数不需要 —— 它们改用 Inter，层级靠字号与字重表达，不靠换字体硬撑。
 * Wordmark 的字体写在类名字符串里（`font-display`），不是 CSS 规则，所以它由
 * 下面"字标仍然用 display 族"那条单独登记。
 */
const SORA_CONSUMER_REGISTRY = [
  "frontend/app/assets/css/tailwind.css:.article-hero__title",
  "frontend/app/assets/css/tailwind.css:.article-state__title",
  "frontend/app/assets/css/tailwind.css:.page-title",
  "frontend/app/assets/css/tailwind.css:.profile-hero-name",
  "frontend/app/pages/collections/[id].vue:.collection-detail__title",
  "frontend/app/pages/collections/index.vue:.collection-page__title",
  "frontend/app/pages/index.vue:.home-cover__bracket",
  "frontend/app/pages/index.vue:.home-cover__title",
  "frontend/app/pages/projects/[id].vue:.project-detail__title",
  "frontend/app/pages/projects/index.vue:.project-page__title",
  "frontend/app/pages/studio/dashboard.vue:.overview__title",
];

/** 收集两端 `app/**` 与 Tailwind 配置里的字体 token 引用（全仓口径）。 */
function fontTokenReferences(): { file: string; token: string }[] {
  const references: { file: string; token: string }[] = [];
  const scan = (file: string) => {
    const text = stripComments(readFileSync(file, "utf8"));
    for (const match of text.matchAll(/var\((--font-[a-z0-9-]+)\)/gu)) {
      references.push({ file, token: match[1]! });
    }
  };
  const walk = (dir: string) => {
    for (const name of readdirSync(dir)) {
      if (name === "node_modules" || name === ".nuxt" || name === ".output") continue;
      const full = join(dir, name);
      const info = statSync(full);
      if (info.isDirectory()) {
        walk(full);
        continue;
      }
      if (/\.(vue|css|ts)$/u.test(name) && !name.endsWith(".d.ts")) scan(full);
    }
  };

  walk(join(repoRoot, "frontend/app"));
  walk(join(repoRoot, "frontend_admin/app"));
  scan(join(repoRoot, "frontend/tailwind.config.ts"));
  scan(join(repoRoot, "frontend_admin/tailwind.config.ts"));

  return references;
}

/** 收集全部 `font-family` 声明值（含 `fonts.css` 的 @font-face）。 */
function fontFamilyDeclarations(): { file: string; family: string }[] {
  const declarations: { file: string; family: string }[] = [];
  const scan = (file: string) => {
    const text = stripComments(readFileSync(file, "utf8"));
    for (const match of text.matchAll(/font-family:\s*([^;}]+)[;}]/gu)) {
      declarations.push({ file, family: match[1]!.replace(/\s+/gu, " ").trim() });
    }
  };
  const walk = (dir: string) => {
    for (const name of readdirSync(dir)) {
      if (name === "node_modules" || name === ".nuxt" || name === ".output") continue;
      const full = join(dir, name);
      const info = statSync(full);
      if (info.isDirectory()) {
        walk(full);
        continue;
      }
      if (/\.(vue|css|ts)$/u.test(name) && !name.endsWith(".d.ts")) scan(full);
    }
  };

  scan(join(projectDir, "app/assets/css/fonts.css"));
  scan(join(repoRoot, "frontend_admin/app/assets/css/fonts.css"));
  walk(join(projectDir, "app"));
  walk(join(repoRoot, "frontend_admin/app"));

  return declarations;
}

describe("中文字体不再自托管：改走系统字体栈（ADR-0016 决策 2 / 约束 5）", () => {
  it("fonts.css 里没有任何 CJK 字族的 @font-face（Noto 两个整包已删除）", () => {
    // 反向断言：把 Noto 两档 @font-face 加回来（首屏 2.3 MB）就会红。
    expect(facesOfFamily(fontsCss, "Noto Sans SC")).toHaveLength(0);
    expect(facesOfFamily(fontsCss, "Noto Sans SC Fallback")).toHaveLength(0);
    // 不只看族名：任何一条 @font-face 的 src 都不许指向 noto-sans-sc 的整包文件。
    expect(fontsCss).not.toMatch(/noto-sans-sc/iu);
  });

  it("每一条 src 的 woff2 都真实存在于 node_modules（剩下的 Sora / Inter 继续自托管）", () => {
    for (const face of faces) {
      const urls = [...face.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/gu)].map((match) => match[1]);

      if (urls.length === 0) continue;
      for (const url of urls) {
        expect(url).toMatch(/^@fontsource(-variable)?\//u);
        expect(existsSync(join(projectDir, "node_modules", url!))).toBe(true);
      }
    }
  });

  it("Sora / Inter 保留自托管：Sora 多档 + 度量匹配 fallback，Inter 可变字体仍在", () => {
    expect(facesOfFamily(fontsCss, "Sora").length).toBeGreaterThanOrEqual(3);
    expect(facesOfFamily(fontsCss, "Sora Fallback").length).toBe(1);
    expect(facesOfFamily(fontsCss, "Inter").length).toBe(1);
    expect(facesOfFamily(fontsCss, "Inter Fallback").length).toBe(1);
  });

  it("首屏字体预算 < 100 KB：读者站 preload 的字体文件合计 ≤ 102400 B", () => {
    // 首屏真正被预加载的字体 = app.vue 里 import 且在 preload 链接里引用的 woff2。
    const preloaded = [...appVue.matchAll(/import\s+(\w+)\s+from\s+"(@fontsource[^"]+\.woff2)"/gu)]
      .map(([, name, file]) => ({ name: name!, file: file! }))
      .filter(({ name }) => new RegExp(`href:\\s*${name}`, "u").test(appVue));

    expect(preloaded.length).toBeGreaterThanOrEqual(4);
    const bytes = preloaded.reduce(
      (sum, { file }) => sum + statSync(join(projectDir, "node_modules", file)).size,
      0,
    );

    expect(bytes).toBeGreaterThan(0);
    expect(bytes).toBeLessThan(102400);
  });

  it("首屏 preload 里没有任何中文字体文件（中文不再产生下载）", () => {
    expect(appVue).not.toMatch(/noto-sans-sc/iu);
    expect(appVue).not.toMatch(/Noto Sans SC/iu);
  });
});

describe("Inter 自托管：正文 / UI 的拉丁字体", () => {
  it("Inter 只有一条可变字体 @font-face（100–900 全覆盖，不用逐个权重加载）", () => {
    const interFaces = facesOfFamily(fontsCss, "Inter");

    expect(interFaces).toHaveLength(1);
    expect(interFaces[0]).toMatch(/font-weight:\s*100 900/u);
    expect(interFaces[0]).toMatch(/font-display:\s*swap/u);
    expect(interFaces[0]).toMatch(/format\(\s*["']woff2(?:-variations)?["']\s*\)/u);
    expect(interFaces[0]).toContain(
      '@fontsource-variable/inter/files/inter-latin-wght-normal.woff2',
    );
  });

  it("Naive UI 的 themeOverrides 字体栈先 Inter、后系统中文（不再有自托管中文 fallback）", () => {
    // 主题栈由 app/utils/readerTheme.ts 从 tokens.css 解析后组装（见 readerTheme.test.ts）。
    const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
    const stack = String(
      buildReaderThemeOverrides(
        parseReaderThemePalette(tokensCss),
        parseReaderThemeScale(tokensCss),
      ).common?.fontFamily,
    );

    expect(stack.startsWith('"Inter", "Inter Fallback"')).toBe(true);
    expect(stack).not.toContain('"Noto Sans SC Fallback"');
    expect(stack).toContain('"PingFang SC"');
    expect(stack.indexOf('"PingFang SC"')).toBeLessThan(stack.indexOf('"Hiragino Sans GB"'));
    expect(stack.indexOf('"Hiragino Sans GB"')).toBeLessThan(stack.indexOf('"Microsoft YaHei"'));
    expect(stack.indexOf('"Microsoft YaHei"')).toBeLessThan(stack.indexOf('"Noto Sans SC"'));
  });

  it("Inter 的度量匹配 fallback 用 Arial 系替身 + 与 Inter 度量对齐的覆盖值", () => {
    const fallback = facesOfFamily(fontsCss, "Inter Fallback")[0];

    expect(fallback).toBeDefined();
    expect(fallback).toMatch(/src:\s*local\(/u);
    // 数值来自 Inter v4 的 head/hhea（upem 2048 / ascent 1984 / descent 494，
    // 至 2048 的比例 96.87% / 24.12%）与 xWidthAvg 比例 978/913 = 107.12%。
    expect(Number(/ascent-override:\s*([\d.]+)%/u.exec(fallback!)?.[1])).toBeCloseTo(96.88, 2);
    expect(Number(/descent-override:\s*([\d.]+)%/u.exec(fallback!)?.[1])).toBeCloseTo(24.12, 2);
    expect(fallback).toMatch(/line-gap-override:\s*0%/u);
    expect(Number(/size-adjust:\s*([\d.]+)%/u.exec(fallback!)?.[1])).toBeCloseTo(107.12, 2);
  });

  it("后台自托管同一份 Inter（后台不存在另一套字体加载路径）", () => {
    const adminFaces = facesOfFamily(adminFontsCss, "Inter");

    expect(adminFaces.length).toBe(1);
    expect(adminFaces[0]).toContain(
      '@fontsource-variable/inter/files/inter-latin-wght-normal.woff2',
    );
    expect(facesOfFamily(adminFontsCss, "Inter Fallback").length).toBe(1);
  });

  it("首屏 preload 覆盖关键权重：Sora 三档 + Inter 可变字体（两端都有）", () => {
    for (const href of [
      "sora-latin-400-normal.woff2",
      "sora-latin-600-normal.woff2",
      "sora-latin-700-normal.woff2",
      "inter-latin-wght-normal.woff2",
    ]) {
      expect(appVue).toContain(href);
    }
    expect(appVue.match(/rel:\s*"preload"/gu)?.length).toBeGreaterThanOrEqual(4);

    expect(adminAppVue).toContain("inter-latin-wght-normal.woff2");
    expect(adminAppVue).toMatch(/rel:\s*"preload"/u);
    expect(adminAppVue).toContain('crossorigin: "anonymous"');
  });
});

describe("字体分工：Sora = 字标/大标题，Inter = 正文/UI，中文 = 系统字体栈", () => {
  const SYSTEM_CJK_STACK =
    '-apple-system, BlinkMacSystemFont, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans SC", sans-serif';

  it.each([
    ["读者站", tokensCss],
    ["后台", adminTokensCss],
  ])("%s 的字体 token：display=Sora（含系统中文）、latin=Inter、cjk=系统字体栈", (_label, css) => {
    const display = declaration(css, "--font-display");

    expect(display.startsWith('"Sora", "Sora Fallback"')).toBe(true);
    expect(display).toContain("var(--font-cjk)");
    expect(declaration(css, "--font-latin")).toBe('"Inter", "Inter Fallback"');
    expect(declaration(css, "--font-title")).toBe("var(--font-display)");
    // 中文组就是系统字体栈：末位 "Noto Sans SC" 只是"系统装了就有"的名字，不再自托管。
    expect(declaration(css, "--font-cjk")).toBe(SYSTEM_CJK_STACK);
  });

  it("--font-sans 由拉丁组 + 中文系统组拼成，中文不再指向任何自托管族", () => {
    const sans = declaration(tokensCss, "--font-sans");

    expect(sans).toBe("var(--font-latin), var(--font-cjk)");
  });

  it("标题字距/行高按三档取 token（两端同值；旧单值只是后台消费的别名）", () => {
    for (const token of [
      "--type-tracking-lg",
      "--type-leading-lg",
      "--type-tracking-md",
      "--type-leading-md",
      "--type-tracking-sm",
      "--type-leading-sm",
    ]) {
      expect(declaration(adminTokensCss, token), token).toBe(declaration(tokensCss, token));
    }
  });

  it("首屏标题 / 页面标题仍是 display 族并走大档；区块/卡片标题改 Inter（票 430）", () => {
    expect(ruleBody(indexVue, ".home-cover__title")).toContain("font-family: var(--font-title)");
    for (const selector of [".page-title", ".article-hero__title", ".article-state__title", ".profile-hero-name"]) {
      const body = ruleBody(tailwindCss, selector);

      expect(body, selector).toContain("font-title");
      expect(body, selector).toContain("tracking-title-lg");
      expect(body, selector).toContain("leading-title-lg");
    }
    for (const selector of [
      ".section-title",
      ".article-toc__title",
      ".feed-entry__title",
      ".empty-state__title",
      ".comment-panel__title",
    ]) {
      const body = ruleBody(tailwindCss, selector);

      expect(body, selector).toMatch(/font-body|font-latin|var\(--font-sans\)|var\(--font-latin\)/u);
      expect(body, selector).not.toContain("font-title");
    }
  });

  it("字标用 display 族（不再挂 font-latin，否则正文换 Inter 会顺带换掉字标）", () => {
    expect(wordmarkVue).toMatch(/font-display|font-title/u);
    expect(wordmarkVue).not.toContain("font-latin font-title");
    expect(adminWordmarkVue).toMatch(/font-display|font-title/u);
    expect(adminWordmarkVue).not.toContain("font-latin font-title");
  });

  it("Naive 主题的字体栈也切成 Inter + 系统中文（按钮/表单/弹层不再继承 Sora）", () => {
    const stack = String(
      buildReaderThemeOverrides(
        parseReaderThemePalette(tokensCss),
        parseReaderThemeScale(tokensCss),
      ).common?.fontFamily,
    );

    expect(stack.startsWith('"Inter"')).toBe(true);
    expect(stack.indexOf('"Inter"')).toBeLessThan(stack.indexOf('"PingFang SC"'));
    expect(stack).not.toContain('"Sora"');
    expect(stack).not.toContain('"Noto Sans SC Fallback"');
  });
});

describe("字体角色收口：Sora 只给字标与页面级大标题（票 430）", () => {
  const rules = cssRules();
  const soraRules = rules.filter((rule) => SORA_RE.test(rule.body));

  it("Sora 消费点全部登记在册（新增一个就得来这里登记）", () => {
    expect(new Set(soraRules.map((rule) => `${rule.file}:${rule.selector}`))).toEqual(
      new Set(SORA_CONSUMER_REGISTRY),
    );
  });

  it("Sora 不再出现在小字号：字号 ≤1.375rem 的规则一律不许用 display 族", () => {
    const offenders = rules
      .map((rule) => ({ rule, size: minDeclaredFontSizeRem(rule.body) }))
      .filter(({ rule, size }) => size !== null && size <= 1.375 && SORA_RE.test(rule.body))
      .map(({ rule, size }) => `${rule.file}:${rule.selector}（${size}rem）`);

    expect(offenders).toEqual([]);
  });

  it("Sora 只出现在 ≥1.75rem（30px，display 档）的规则里", () => {
    const offenders = soraRules
      .map((rule) => ({ rule, size: minDeclaredFontSizeRem(rule.body) }))
      .filter(({ size }) => size !== null && size < 1.75)
      .map(({ rule, size }) => `${rule.file}:${rule.selector}（${size}rem）`);

    expect(offenders).toEqual([]);
  });

  it("18–20px 的目录/条目/空态/评论标题与计数改 Inter（验收第 4 条）", () => {
    for (const selector of [
      ".article-toc__title",
      ".feed-entry__title",
      ".empty-state__title",
      ".comment-panel__title",
    ]) {
      const rule = rules.find(
        (candidate) =>
          candidate.file === "frontend/app/assets/css/tailwind.css" &&
          candidate.selector === selector,
      );

      expect(rule, selector).toBeDefined();
      expect(rule!.body, selector).not.toMatch(SORA_RE);
      expect(rule!.body, selector).toMatch(INTER_RE);
    }
    // 计数：作者卡的数字与仪表盘的 KPI 值同样不能是 Sora。
    for (const selector of [".article-author__stat dd", ".profile-hero-metric__value"]) {
      const rule = rules.find(
        (candidate) =>
          candidate.file === "frontend/app/assets/css/tailwind.css" &&
          candidate.selector === selector,
      );

      expect(rule, selector).toBeDefined();
      expect(rule!.body, selector).not.toMatch(SORA_RE);
      expect(rule!.body, selector).toMatch(INTER_RE);
    }
  });

  it("字标仍然用 display 族（不因卡片标题改 Inter 而顺带换掉）", () => {
    expect(wordmarkVue).toMatch(/font-display|font-title/u);
  });
});

describe("等宽字体键真正生效（prose 代码块 / 行内代码不再掉回 sans）", () => {
  it("--font-mono 有定义且不走 --font-sans", () => {
    const mono = declaration(tokensCss, "--font-mono");

    expect(mono).toContain("ui-monospace, monospace");
    expect(mono).not.toContain("var(--font-sans)");
  });

  it("markdown 的 code / pre 消费 --font-mono（两端 UI 等宽文本同样如此）", () => {
    expect(markdownThemeTs).toContain("font-family: var(--font-mono)");

    const adminCss = readFileSync(
      join(repoRoot, "frontend_admin/app/assets/css/admin.css"),
      "utf8",
    );
    expect(ruleBody(adminCss, ".admin-mono")).toContain("font-family: var(--font-mono)");
  });
});

describe("无衬线主体 + 唯一的衬线例外（首屏字标）、无 CDN、无未定义的字体 token", () => {
  it("两端字体全部本地自托管：src 只指向 node_modules 里的 @fontsource 文件", () => {
    for (const css of [fontsCss, adminFontsCss]) {
      expect(css).not.toMatch(/https?:/u);
      expect(css).not.toMatch(/@import/u);
      for (const match of css.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/gu)) {
        expect(match[1]).toMatch(/^@fontsource(-variable)?\//u);
      }
    }
  });

  it("除首屏字标外，没有任何衬线体声明（正文 / UI / 标题仍然全无衬线）", () => {
    const declarations = fontFamilyDeclarations();

    expect(declarations.length).toBeGreaterThan(0);
    for (const { file, family } of declarations) {
      expect(family, file).not.toMatch(/(?<!sans-)serif/iu);
      expect(family, file).not.toMatch(/Georgia|Times New Roman|SimSun|Songti/iu);
    }
  });
  /**
   * 例外登记（唯一的衬线字）。
   *
   * 首屏大字要"碎玻璃 + 高对比衬线斜体"（首屏封面例外区的延伸），所以允许
   * Playfair Display 自托管一条 700 italic。例外被三件事夹住，避免它长成全站第二套字体：
   *   1. 只有 `--font-wordmark` 这个键能指到衬线族（两端 tokens 逐字节一致，后台也声明但不用）；
   *   2. 全仓只有 GlassHeroTitle.vue 消费这个键；
   *   3. 组件里写的仍是 token（`font-family: var(--font-wordmark)`），不是族名。
   * 这把"有没有人偷偷在别处写 serif"从约定变成断言。
   */
  it("衬线例外只属于 --font-wordmark，且只被首屏字标组件消费", () => {
    // 1. token 指向自托管衬线；两端同源（后台声明了同一个键但不消费）
    for (const [label, css] of [
      ["读者站", tokensCss],
      ["后台", adminTokensCss],
    ] as const) {
      expect(css, label).toContain('--font-wordmark: "Playfair Display", "Playfair Display Fallback"');
    }

    // 2. 只有字标组件消费它（注释里的提及不算，扫描器会抹掉注释）
    const consumers = fontTokenReferences()
      .filter((reference) => reference.token === "--font-wordmark")
      .map((reference) => relative(repoRoot, reference.file).split("\\").join("/"));

    expect(new Set(consumers)).toEqual(
      new Set(["frontend/app/components/brand/GlassHeroTitle.vue"]),
    );

    // 3. 组件写的是 token，不是族名
    expect(
      readFileSync(join(projectDir, "app/components/brand/GlassHeroTitle.vue"), "utf8"),
    ).toContain("font-family: var(--font-wordmark)");
  });

  it("除 fonts.css 外，所有 font-family 都消费 token（没有第二套字体来源）", () => {
    for (const { file, family } of fontFamilyDeclarations()) {
      if (file.endsWith("fonts.css")) continue;
      expect(family, file).toMatch(/var\(--font-/u);
    }
  });

  it("全仓不再引用未定义的字体 token", () => {
    const declared = new Set(
      [...tokensCss.matchAll(/--font-[a-z0-9-]+\s*:/gu)].map((match) => match[0].replace(/\s*:/u, "")),
    );
    const references = fontTokenReferences();

    expect(references.length).toBeGreaterThan(0);
    for (const { file, token } of references) {
      expect(declared, `${file} 引用了 ${token}`).toContain(token);
    }
  });
});

import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

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
const markdownThemeTs = readFileSync(join(projectDir, "app/utils/markdownTheme.ts"), "utf8");
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

describe("中文自托管字体：声明真的指向可加载的文件", () => {
  it("Noto Sans SC 的 400 / 700 两档都有 @font-face（依赖早就在，之前只是从未加载）", () => {
    const weights = facesOfFamily(fontsCss, "Noto Sans SC").map(
      (face) => /\bfont-weight:\s*(\d+)/u.exec(face)?.[1] ?? "?",
    );

    expect(weights.sort()).toEqual(["400", "700"]);
  });

  it("每一条 src 的 woff2 都真实存在于 node_modules（拼错路径 = 字体静默不加载）", () => {
    for (const face of faces) {
      const urls = [...face.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/gu)].map((match) => match[1]);

      if (urls.length === 0) continue;
      for (const url of urls) {
        expect(url).toMatch(/^@fontsource(-variable)?\//u);
        expect(existsSync(join(projectDir, "node_modules", url!))).toBe(true);
      }
    }
  });

  it("用 font-display: swap（首屏先出字，不白屏）", () => {
    for (const face of facesOfFamily(fontsCss, "Noto Sans SC")) {
      expect(face).toMatch(/font-display:\s*swap/u);
    }
  });

  it("配有度量匹配 fallback：本地中文族 + ascent/descent/size-adjust 覆盖（避免换字跳版）", () => {
    const fallback = facesOfFamily(fontsCss, "Noto Sans SC Fallback")[0];

    expect(fallback).toBeDefined();
    expect(fallback).toMatch(/src:\s*local\(/u);
    expect(fallback).toMatch(/ascent-override:\s*[\d.]+%/u);
    expect(fallback).toMatch(/descent-override:\s*[\d.]+%/u);
    expect(fallback).toMatch(/line-gap-override:\s*0%/u);
    expect(fallback).toMatch(/size-adjust:\s*100%/u);
  });

  it("既有拉丁字体 Sora 没有被中文改造挤掉", () => {
    expect(facesOfFamily(fontsCss, "Sora").length).toBeGreaterThanOrEqual(3);
    expect(facesOfFamily(fontsCss, "Sora Fallback").length).toBe(1);
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

  it("Naive UI 的 themeOverrides 字体栈把 Noto Sans SC 排在中文字体之前", () => {
    // 主题栈由 app/utils/readerTheme.ts 从 tokens.css 解析后组装（见 readerTheme.test.ts）。
    const tokensCss = readFileSync(join(projectDir, "app/assets/css/tokens.css"), "utf8");
    const stack = String(
      buildReaderThemeOverrides(
        parseReaderThemePalette(tokensCss),
        parseReaderThemeScale(tokensCss),
      ).common?.fontFamily,
    );

    expect(stack).toContain('"Noto Sans SC"');
    expect(stack).toContain('"Noto Sans SC Fallback"');
    expect(stack.indexOf('"Noto Sans SC"')).toBeLessThan(stack.indexOf('"PingFang SC"'));
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

describe("字体分工：Sora = 字标/大标题，Inter = 正文/UI，中文 = Noto Sans SC", () => {
  it.each([
    ["读者站", tokensCss],
    ["后台", adminTokensCss],
  ])("%s 的字体 token：display=Sora（含中文回退）、latin=Inter、cijk=Noto", (_label, css) => {
    const display = declaration(css, "--font-display");

    expect(display.startsWith('"Sora", "Sora Fallback"')).toBe(true);
    expect(display).toContain("var(--font-cjk)");
    expect(declaration(css, "--font-latin")).toBe('"Inter", "Inter Fallback"');
    expect(declaration(css, "--font-title")).toBe("var(--font-display)");
    expect(declaration(css, "--font-cjk")).toBe(
      '"Noto Sans SC", "Noto Sans SC Fallback"',
    );
  });

  it("--font-sans 先走 Inter、再走 Noto，中文不会掉到系统 PingFang/YaHei", () => {
    const sans = declaration(tokensCss, "--font-sans");

    expect(sans.startsWith("var(--font-latin), var(--font-cjk)")).toBe(true);
    expect(sans.indexOf("var(--font-cjk)")).toBeLessThan(sans.indexOf("PingFang SC"));
  });

  it("大标题字距收到 -0.03em 一档（两端同值）", () => {
    expect(declaration(tokensCss, "--type-tracking-title")).toBe("-0.03em");
    expect(declaration(adminTokensCss, "--type-tracking-title")).toBe("-0.03em");
  });

  it("首屏标题 / 页面标题 / 区块标题 / 文章标题都取 display 族并走标题字距", () => {
    expect(ruleBody(indexVue, ".home-hero__title")).toContain("font-family: var(--font-title)");
    for (const selector of [".page-title", ".section-title", ".article-hero__title"]) {
      const body = ruleBody(tailwindCss, selector);

      expect(body, selector).toContain("font-title");
      expect(body, selector).toContain("tracking-title");
    }
  });

  it("字标用 display 族（不再挂 font-latin，否则正文换 Inter 会顺带换掉字标）", () => {
    expect(wordmarkVue).toMatch(/font-display|font-title/u);
    expect(wordmarkVue).not.toContain("font-latin font-title");
    expect(adminWordmarkVue).toMatch(/font-display|font-title/u);
    expect(adminWordmarkVue).not.toContain("font-latin font-title");
  });

  it("Naive 主题的字体栈也切成 Inter（按钮/表单/弹层不再继承 Sora）", () => {
    const stack = String(
      buildReaderThemeOverrides(
        parseReaderThemePalette(tokensCss),
        parseReaderThemeScale(tokensCss),
      ).common?.fontFamily,
    );

    expect(stack.startsWith('"Inter"')).toBe(true);
    expect(stack.indexOf('"Inter"')).toBeLessThan(stack.indexOf('"Noto Sans SC"'));
    expect(stack).not.toContain('"Sora"');
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

describe("全站无衬线、无 CDN、无未定义的字体 token", () => {
  it("两端字体全部本地自托管：src 只指向 node_modules 里的 @fontsource 文件", () => {
    for (const css of [fontsCss, adminFontsCss]) {
      expect(css).not.toMatch(/https?:/u);
      expect(css).not.toMatch(/@import/u);
      for (const match of css.matchAll(/url\(\s*["']?([^"')]+)["']?\s*\)/gu)) {
        expect(match[1]).toMatch(/^@fontsource(-variable)?\//u);
      }
    }
  });

  it("没有任何衬线体声明（全站无衬线）", () => {
    const declarations = fontFamilyDeclarations();

    expect(declarations.length).toBeGreaterThan(0);
    for (const { file, family } of declarations) {
      expect(family, file).not.toMatch(/(?<!sans-)serif/iu);
      expect(family, file).not.toMatch(/Georgia|Times New Roman|SimSun|Songti/iu);
    }
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

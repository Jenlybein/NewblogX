import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";

import { describe, expect, it } from "vitest";

// vitest 以 project root（frontend_admin/）为 cwd。
const projectDir = process.cwd();
const appDir = join(projectDir, "app");

// 旧青绿族：重构前事实上的品牌色（#0f766e）及其渐变搭档、Tailwind 默认色板。
// 这些值只允许出现在「tokens.css」之外的历史注释里 —— 页面与组件里出现即视为残留。
const LEGACY_TEAL_HEXES = [
  "#0f766e",
  "#115e59",
  "#134e4a",
  "#155e75",
  "#99f6e4",
  "#123f3c",
  "#2d746c",
  "#d97706",
];
const LEGACY_TEAL_RGB_TRIPLES = ["15, 118, 110", "6, 12, 23", "226, 232, 240"];

// Tailwind 默认色板类名（bg-teal-700 / text-slate-500 …）。后台颜色只允许来自语义
// token 的类名（text-ink / border-line / bg-surface …），所以出现默认色板即漂移。
const PALETTE_NAMES = [
  "slate",
  "gray",
  "zinc",
  "neutral",
  "stone",
  "teal",
  "emerald",
  "green",
  "cyan",
  "sky",
  "blue",
  "indigo",
  "violet",
  "purple",
  "fuchsia",
  "pink",
  "rose",
  "amber",
  "yellow",
  "orange",
  "red",
  "lime",
];
const PALETTE_CLASS_RE = new RegExp(
  String.raw`(?:^|[^a-z-])(?:bg|text|border|ring|from|to|via|divide|outline|fill|stroke|placeholder|shadow)-(${PALETTE_NAMES.join("|")})(?:-[0-9]{2,3})?\b`,
  "giu",
);

type Hit = { line: number; match: string };

function lineOf(source: string, index: number): number {
  return source.slice(0, index).split(/\r?\n/u).length;
}

/** 找出源码里的旧青绿色值（hex 与 rgb 三元组），返回行号。 */
export function findLegacyTeal(source: string): Hit[] {
  const hits: Hit[] = [];
  for (const literal of LEGACY_TEAL_HEXES) {
    const re = new RegExp(literal, "giu");
    for (const match of source.matchAll(re)) {
      hits.push({ line: lineOf(source, match.index), match: literal });
    }
  }
  for (const triple of LEGACY_TEAL_RGB_TRIPLES) {
    const re = new RegExp(String.raw`rgba?\(\s*${triple}`, "giu");
    for (const match of source.matchAll(re)) {
      hits.push({ line: lineOf(source, match.index), match: match[0].replace(/\s+/gu, " ") });
    }
  }
  return hits.sort((a, b) => a.line - b.line);
}

/** 找出 Tailwind 默认色板类名（不属于语义 token 的颜色来源）。 */
export function findLegacyPaletteClasses(source: string): Hit[] {
  const hits: Hit[] = [];
  for (const match of source.matchAll(PALETTE_CLASS_RE)) {
    hits.push({ line: lineOf(source, match.index), match: match[0].trim() });
  }
  return hits;
}

function walk(dir: string, files: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      walk(full, files);
      continue;
    }
    files.push(full);
  }
  return files;
}

function readAppSources(): { file: string; text: string }[] {
  return walk(appDir)
    .filter((file) => file.endsWith(".vue") || file.endsWith(".css"))
    .map((file) => ({
      file: relative(projectDir, file).split(sep).join("/"),
      text: readFileSync(file, "utf8"),
    }))
    .sort((a, b) => a.file.localeCompare(b.file));
}

const pageFiles = walk(join(appDir, "pages"))
  .filter((file) => file.endsWith(".vue"))
  .map((file) => relative(projectDir, file).split(sep).join("/"));

describe("静态断言：后台不得残留旧青绿色（canary 保证扫描器不是永远绿）", () => {
  it("抓旧青绿 hex、rgb 三元组与 Tailwind 默认色板类名", () => {
    const source = [
      "<template>",
      '  <NuxtLink class="text-sm text-teal-700">返回</NuxtLink>',
      "</template>",
      "<style>",
      "  .legacy { color: #0f766e; background: rgba(15, 118, 110, 0.12); }",
      "</style>",
    ].join("\n");

    const teal = findLegacyTeal(source);
    expect(teal.map((hit) => hit.line)).toEqual([5, 5]);
    expect(findLegacyPaletteClasses(source)).toEqual([{ line: 2, match: "text-teal-700" }]);
  });

  it("消费语义 token 的类名与样式算合规", () => {
    const source =
      '<div class="text-ink-muted border-line-faint bg-surface"><span class="text-focus">ok</span></div>\n' +
      ".x { color: var(--color-ink); border-color: var(--color-danger); }\n";

    expect(findLegacyTeal(source)).toEqual([]);
    expect(findLegacyPaletteClasses(source)).toEqual([]);
  });

  it("11 个路由页面全部无旧青色、不碰 Tailwind 默认色板", () => {
    // 页面数量本身就是验收项（11 个路由页面）。
    expect(pageFiles.length).toBe(11);

    const sources = readAppSources();
    const problems = sources.flatMap(({ file, text }) => [
      ...findLegacyTeal(text).map((hit) => ({ file, ...hit })),
      ...findLegacyPaletteClasses(text).map((hit) => ({ file, ...hit })),
    ]);

    // 扫描器必须真的扫到东西，否则"目录空了 → 全绿"这种失效模式会漏过。
    expect(sources.length).toBeGreaterThanOrEqual(30);
    expect(problems).toEqual([]);
  });

  it("admin.css 不再消费过渡层旧变量（颜色只剩语义层一条来源）", () => {
    const adminCss = readFileSync(join(appDir, "assets/css/admin.css"), "utf8");

    for (const legacyVar of [
      "--bg-page",
      "--surface:",
      "--surface-strong",
      "--line:",
      "--text-main",
      "--text-soft",
      "--accent:",
      "--accent-soft",
      "--shadow-soft",
    ]) {
      expect(adminCss).not.toContain(legacyVar);
    }
  });

  it("tokens.css 里也清掉了旧青绿（过渡层整个删除）", () => {
    const tokensCss = readFileSync(join(appDir, "assets/css/tokens.css"), "utf8");

    expect(findLegacyTeal(tokensCss)).toEqual([]);
  });
});

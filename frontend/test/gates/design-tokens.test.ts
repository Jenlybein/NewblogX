import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

import { afterAll, describe, expect, it } from "vitest";

import {
  checkProject,
  evaluateDrift,
  findColorLiterals,
  isExemptPath,
  loadBaseline,
  readProjectEntries,
} from "../../../scripts/check-design-tokens.mjs";

// vitest 以 project root（frontend/）为 cwd；不用 import.meta.url —— vitest 会把
// 它改写成非 file 协议的地址，fileURLToPath 会抛 "must be of scheme file"。
const projectDir = process.cwd();
const canaryDirs: string[] = [];

/** 造一个"只有 app/ 目录"的临时项目，喂给扫描器用（人造违规的载体）。 */
function makeCanaryProject(files: Record<string, string>): string {
  const dir = mkdtempSync(join(tmpdir(), "jelly-token-canary-"));
  canaryDirs.push(dir);
  for (const [relPath, content] of Object.entries(files)) {
    const full = join(dir, relPath);
    mkdirSync(resolve(full, ".."), { recursive: true });
    writeFileSync(full, content, "utf8");
  }
  return dir;
}

afterAll(() => {
  for (const dir of canaryDirs) {
    rmSync(dir, { recursive: true, force: true });
  }
});

describe("设计 token 漂移门禁：扫描规则", () => {
  it("抓裸十六进制与 rgba() 字面量，并给出准确行号", () => {
    // 这人造违规用例故意混入三种写法：hex、rgba()、Tailwind 任意值。
    const source = [
      "<template>",
      '  <div class="text-[#ff0000]" />',
      "</template>",
      "<style>",
      "  .x { color: #0f766e; background: rgba(15, 118, 110, 0.12); }",
      "</style>",
    ].join("\n");

    expect(findColorLiterals(source)).toEqual([
      { line: 2, match: "#ff0000" },
      { line: 5, match: "#0f766e" },
      { line: 5, match: "rgba(15, 118, 110, 0.12)" },
    ]);
  });

  it("只消费 token 变量的样式算合规（门禁不能永远为红）", () => {
    const source = [
      "<style>",
      "  .x {",
      "    color: var(--color-ink);",
      "    background: var(--color-surface);",
      "    border-color: var(--color-line);",
      "  }",
      "</style>",
    ].join("\n");

    expect(findColorLiterals(source)).toEqual([]);
  });

  it("不把 CSS ID 选择器与页面锚点当成颜色（避免误红）", () => {
    const source = [
      "<template>",
      '  <a href="#top">回到顶部</a>',
      '  <a href="/articles#comments">评论</a>',
      "</template>",
      "<style>",
      "  #__nuxt, #app { min-height: 100%; }",
      "</style>",
    ].join("\n");

    expect(findColorLiterals(source)).toEqual([]);
  });

  it("Tailwind 任意值里用 `_` 转义的十六进制也要抓到（历史漏检）", () => {
    // 旧正则把 `_` 当成标识符字符，于是 `#fefcf8_0%` 这种写法整类溜过扫描。
    const source = [
      "<template>",
      '  <div class="bg-[linear-gradient(180deg,_#fefcf8_0%,_#f7fbff_100%)]" />',
      "</template>",
    ].join("\n");

    expect(findColorLiterals(source)).toEqual([
      { line: 2, match: "#fefcf8" },
      { line: 2, match: "#f7fbff" },
    ]);
  });

  it("token 文件与品牌矢量/OG 生成物目录豁免，普通组件不豁免", () => {
    expect(isExemptPath("app/assets/css/tokens.css")).toBe(true);
    expect(isExemptPath("app/assets/brand/JellyMark.vue")).toBe(true);
    expect(isExemptPath("app/assets/og/preview.vue")).toBe(true);
    expect(isExemptPath("app/components/home/HomeSidebar.vue")).toBe(false);
    expect(isExemptPath("app/assets/css/tailwind.css")).toBe(false);
  });
});

describe("设计 token 漂移门禁：baseline 棘轮", () => {
  const canaryViolation = { file: "app/components/__canary.vue", line: 3, match: "#ff0000" };

  it("人造违规文件会让门禁失败（canary：门禁必须能红）", () => {
    const result = evaluateDrift({
      violations: [canaryViolation],
      baselineFiles: [],
      scannedFiles: ["app/components/__canary.vue"],
    });

    expect(result.ok).toBe(false);
    expect(result.failures).toContainEqual({
      kind: "new-color-literal",
      file: "app/components/__canary.vue",
      line: 3,
      match: "#ff0000",
    });
  });

  it("baseline 里的文件被清干净后必须从清单删掉（清单只许收缩）", () => {
    const result = evaluateDrift({
      violations: [],
      baselineFiles: ["app/components/OldDirty.vue"],
      scannedFiles: ["app/components/OldDirty.vue"],
    });

    expect(result.ok).toBe(false);
    expect(result.failures).toContainEqual({
      kind: "baseline-should-shrink",
      file: "app/components/OldDirty.vue",
    });
  });

  it("baseline 允许清单内的文件仍然裸色值时不报错；清单外的文件裸色值必须报错", () => {
    const result = evaluateDrift({
      violations: [
        { file: "app/assets/css/tailwind.css", line: 12, match: "rgba(" },
        { file: "app/pages/NewPage.vue", line: 9, match: "#123456" },
      ],
      baselineFiles: ["app/assets/css/tailwind.css"],
      scannedFiles: ["app/assets/css/tailwind.css", "app/pages/NewPage.vue"],
    });

    expect(result.ok).toBe(false);
    expect(result.failures.map((failure) => failure.file)).toEqual(["app/pages/NewPage.vue"]);
  });

  it("baseline 里登记了不存在的文件也算失效（防止清单腐化）", () => {
    const result = evaluateDrift({
      violations: [],
      baselineFiles: ["app/components/Deleted.vue"],
      scannedFiles: ["app/components/Other.vue"],
    });

    expect(result.ok).toBe(false);
    expect(result.failures).toContainEqual({
      kind: "baseline-file-missing",
      file: "app/components/Deleted.vue",
    });
  });
});

describe("设计 token 漂移门禁：在真实文件系统上跑", () => {
  it("真实项目已归零：baseline 为空且 0 处裸色值（门禁为绿）", () => {
    const baselineFiles = loadBaseline(projectDir);
    const result = checkProject({ projectDir, baselineFiles });

    // #13 收口：允许清单必须为空，且项目里真的没有裸色值（不是靠清单藏起来）。
    expect(baselineFiles).toEqual([]);
    expect(result.violations).toEqual([]);
    expect(result.ok).toBe(true);
  });

  it("临时目录里放真实违规文件时，扫描器必须报出它（证明门禁不是永远为绿）", () => {
    const canaryProject = makeCanaryProject({
      "app/components/CanaryWidget.vue": [
        "<template>",
        '  <div class="canary" />',
        "</template>",
        "<style>",
        "  .canary { color: #ff0000; }",
        "</style>",
      ].join("\n"),
      // 非 CSS 的 TS 字面量（普通 JS 字符串 / 颜色构造函数）不在扫描范围内；
      // .ts 里只有「CSS 字符串」才按样式判定（见下一条用例）。
      "app/utils/canary.ts": 'export const leaked = "#ff0000";\n',
      "README.md": "颜色 #ff0000\n",
    });

    const result = checkProject({ projectDir: canaryProject, baselineFiles: [] });

    expect(result.violations).toEqual([
      { file: "app/components/CanaryWidget.vue", line: 5, match: "#ff0000" },
    ]);
    expect(result.ok).toBe(false);
  });

  it("藏在 .ts 里的 CSS 字符串会被抓到（此前是整块盲区）", () => {
    const canaryProject = makeCanaryProject({
      // 样式表写在 TS 模板字符串里：`.ts` 文件也必须按样式扫描。
      "app/utils/CanaryStyleSheet.ts": [
        "export const sheet = `",
        ".canary { color: #ff0000; }",
        "`;",
        "",
        // 颜色构造函数不是 CSS 字符串，不该误报（否则后台 Naive 主题层会被全量标红）。
        "export function withAlpha(red: number, green: number, blue: number, alpha: number) {",
        "  return `rgba(${red}, ${green}, ${blue}, ${alpha})`;",
        "}",
      ].join("\n"),
    });

    const result = checkProject({ projectDir: canaryProject, baselineFiles: [] });

    expect(result.violations).toEqual([
      { file: "app/utils/CanaryStyleSheet.ts", line: 2, match: "#ff0000" },
    ]);
    expect(result.ok).toBe(false);
  });

  it("token 文件里的颜色字面量不算违规（唯一合法定义处）", () => {
    const canaryProject = makeCanaryProject({
      "app/assets/css/tokens.css": ":root { --color-ink: #1f2937; }\n",
      "app/assets/brand/JellyMark.vue": "<svg><path fill='#002FA7' /></svg>\n",
    });

    // 豁免文件在扫描阶段就被剔除，不会进入判定。
    expect(readProjectEntries(canaryProject)).toEqual([]);
    expect(checkProject({ projectDir: canaryProject, baselineFiles: [] })).toEqual(
      expect.objectContaining({ ok: true, violations: [] }),
    );
  });
});

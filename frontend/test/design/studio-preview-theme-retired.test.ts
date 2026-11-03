import { existsSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

import { collectFiles, extensionFilter, stripComments } from "../helpers/designFiles";

// 票 #212 / spec C3 的「退场断言」：创作区的预览主题必须真的从代码里消失，
// 而不是把下拉藏起来、把注入通道留着。
//
// 为什么用源码扫描：正确状态是"引用根本不存在"（13 套外链 CSS、主题 composable、
// `themeHref` 注入通道、指示条文案、`markdown-theme` 依赖）。渲染断言只能证明当前页面
// 没画出来，挡不住下一个页面又把通道接回来 —— 门禁式静态断言才能挡住回潮。
// 只扫 app 源码（测试文件本身会提到这些字符串）。

const projectDir = process.cwd();
const appDir = join(projectDir, "app");

function source(relativePath: string): string {
  return readFileSync(join(projectDir, relativePath), "utf8");
}

const appSourceFiles = collectFiles(appDir, extensionFilter("vue", "ts", "css", "mjs"));

/** 去注释后仍包含 `phrase` 的源码文件（相对 frontend/）。 */
function offendingFiles(phrase: string): string[] {
  return appSourceFiles
    .filter((file) => stripComments(readFileSync(file, "utf8")).includes(phrase))
    .map((file) => relative(projectDir, file));
}

describe("创作区预览主题已退场（#212）", () => {
  it("13 套外链 CSS 与主题 composable 已删除", () => {
    expect(offendingFiles("markdown-theme/themes/")).toEqual([]);
    expect(offendingFiles("useStudioMarkdownTheme")).toEqual([]);
    expect(existsSync(join(appDir, "composables/useStudioMarkdownTheme.ts"))).toBe(false);
  });

  it("全站源码不再有外部主题注入通道（themeHref 属性与 shadow root 外链）", () => {
    expect(offendingFiles("themeHref")).toEqual([]);
    expect(offendingFiles("theme-href")).toEqual([]);
    expect(offendingFiles("external-theme")).toEqual([]);
  });

  it("页面上不再出现「预览样式」指示条，工具栏也不再提供主题下拉", () => {
    expect(offendingFiles("预览样式")).toEqual([]);
    expect(offendingFiles("write-page__theme-indicator")).toEqual([]);
    expect(offendingFiles("selectedMarkdownTheme")).toEqual([]);
    expect(offendingFiles("markdownThemeOptions")).toEqual([]);
    expect(offendingFiles("IconPalette")).toEqual([]);
  });

  it("只服务主题下拉的「选项选中态」通道一并退场", () => {
    expect(offendingFiles("isOptionActive")).toEqual([]);
    expect(offendingFiles("isToolOptionActive")).toEqual([]);
  });

  it("markdown-theme 依赖从 package.json 与锁文件里消失", () => {
    const manifest = JSON.parse(source("package.json")) as {
      dependencies?: Record<string, string>;
      devDependencies?: Record<string, string>;
    };

    expect(manifest.dependencies?.["markdown-theme"]).toBeUndefined();
    expect(manifest.devDependencies?.["markdown-theme"]).toBeUndefined();
    expect(source("pnpm-lock.yaml")).not.toContain("markdown-theme");
  });

  it("创作预览与阅读页共用同一套主题：自有主题之外只保留 katex 基线样式", () => {
    const writePage = source("app/pages/studio/write.vue");
    const articlePage = source("app/pages/article/[id].vue");

    for (const [file, text] of [
      ["app/pages/studio/write.vue", writePage],
      ["app/pages/article/[id].vue", articlePage],
    ] as const) {
      expect(text, file).toContain("katex/dist/katex.min.css?url");
      expect(text, file).toContain("extra-style-hrefs");
    }
  });
});

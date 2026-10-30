import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 收口票（#25）：复核项为 0 的断言与降级链的静态核验。
 *
 * 为什么值得测：前面 7 票各自在自己的 worktree 里为绿，合并之后"全仓是否真的没有
 * 残留"没人再验过；而降级（不支持背景模糊 / 减少透明度 / 减少动态 / 移动端）是
 * 最容易在合并时被覆盖掉的一类规则。这里把它们钉成断言。
 */
const projectDir = process.cwd();

/** 两端 app 目录下所有源文件（只读源码，不看依赖与构建产物）。 */
function sourceFiles(appDir: string): string[] {
  const root = join(projectDir, "..", appDir, "app");
  const files: string[] = [];

  const walk = (dir: string) => {
    for (const entry of readdirSync(dir)) {
      const full = join(dir, entry);
      if (statSync(full).isDirectory()) {
        walk(full);
        continue;
      }
      if (/\.(vue|css|ts)$/u.test(entry)) files.push(full);
    }
  };

  walk(root);
  return files.map((file) => relative(projectDir, file).replace(/\\/gu, "/"));
}

function read(relPath: string): string {
  return readFileSync(join(projectDir, relPath), "utf8");
}

/** 品牌资产例外（ADR-0012 约束、GLOSSARY「品牌资产例外」）。 */
const brandAssetExempt = (file: string) => file.includes("assets/brand/") || file.includes("assets/og/");

const readerFiles = sourceFiles("frontend");
const adminFiles = sourceFiles("frontend_admin");
const allFiles = [...readerFiles, ...adminFiles];

describe("收口：复核项全为 0", () => {
  it("旧克莱因蓝与青绿不再出现（品牌资产除外）", () => {
    const offenders = allFiles.filter((file) => {
      if (brandAssetExempt(file)) return false;
      const text = read(file);
      return /#002fa7|#002FA7|rgb\(\s*0\s+47\s+167|0f766e/iu.test(text);
    });

    expect(offenders).toEqual([]);
  });

  it("白底 Logo 变体与资产键已退场", () => {
    for (const text of allFiles.map(read)) {
      expect(text).not.toMatch(/logoWhite/iu);
      expect(text).not.toMatch(/logo-512-white/iu);
    }
  });

  it("主旨句「一个会呼吸的个人博客」不再出现在任何源码里", () => {
    const offenders = allFiles.filter((file) => read(file).includes("一个会呼吸的个人博客"));
    expect(offenders).toEqual([]);
  });

  it("三格事实的纯函数已退场", () => {
    for (const text of allFiles.map(read)) {
      expect(text).not.toMatch(/homeHeroFacts/iu);
    }
  });

  it("私自圆角字面量归零（圆角只来自五档 token）", () => {
    const offenders = allFiles.filter((file) => /rounded-\[\s*\d+px\s*\]|border-radius:\s*\d+px/iu.test(read(file)));
    expect(offenders).toEqual([]);
  });
});

describe("收口：玻璃的四类降级链都在", () => {
  const tailwind = read("app/assets/css/tailwind.css");

  it("不支持 backdrop-filter 时退化为不透明面", () => {
    // 实际写法是 `@supports not ((backdrop-filter: …) or (-webkit-backdrop-filter: …))`，
    // 两组括号是为了把 -webkit- 前缀一并覆盖。
    expect(tailwind).toMatch(/@supports\s+not\s*\(\s*\(\s*backdrop-filter/iu);
  });

  it("系统「减少透明度」时退化为不透明面", () => {
    expect(tailwind).toMatch(/prefers-reduced-transparency/iu);
  });

  it("系统「减少动态」时关掉指针镜面高光", () => {
    expect(tailwind).toMatch(/prefers-reduced-motion/iu);
  });

  it("移动端把模糊降到 12px", () => {
    expect(tailwind).toMatch(/max-width:\s*768px/iu);
    expect(tailwind).toMatch(/--glass-blur-mobile/iu);
  });
});

/**
 * design 测试的共用辅助函数。
 *
 * 为什么抽这里：吸顶样式的断言要靠"取某个选择器之后的第一个花括号块"和"遍历 app
 * 目录找源文件"两件小事，它们此前在多个 test 里各抄一遍 —— 抄的时候往往顺手改了
 * 过滤条件，于是同名函数在不同文件里行为不同，很难一眼看出某个 topic 到底扫了哪些文件。
 *
 * 收录原则：**只收逐字节相同的实现**。过滤条件不同（比如只扫 .vue 还是 .vue/.css/.ts）
 * 的 walk 不合并 —— 合并等于悄悄放大/缩小某个测试的扫描面，正是这类重构最容易
 * 引入的假绿。所以本模块只提供参数化的通用版本，让差异显式出现在调用点。
 */
import { readdirSync, statSync } from "node:fs";
import { join } from "node:path";

import { expect } from "vitest";

/**
 * 遍历目录收集文件。
 *
 * @param dir 起始目录
 * @param accept 文件过滤器：返回 true 才收录。**必传** —— 这个参数就是以前各份抄写
 *   唯一真正有差异的地方，显式传出来才能看清每个测试扫的是什么。
 * @param skipDirs 跳过的目录名（默认避开依赖与构建产物）
 */
export function collectFiles(
  dir: string,
  accept: (fileName: string, absolutePath: string) => boolean,
  skipDirs: string[] = ["node_modules", ".nuxt", ".output", "coverage", ".git"],
  files: string[] = [],
): string[] {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) {
      if (skipDirs.includes(name)) continue;
      collectFiles(full, accept, skipDirs, files);
      continue;
    }
    if (accept(name, full)) files.push(full);
  }
  return files;
}

/** 常见的源码扩展名集合（walk 的过滤器按测试需要挑其中几种）。 */
export const SOURCE_EXTENSIONS = ["vue", "css", "ts"] as const;

/** 生成"只收录指定扩展名"的过滤器。 */
export function extensionFilter(...extensions: string[]): (fileName: string) => boolean {
  const pattern = new RegExp(`\\.(?:${extensions.join("|")})$`, "u");
  return (fileName: string) => pattern.test(fileName);
}

/**
 * 取出某个选择器 / at-rule 之后的第一个 `{...}` 块内容。
 *
 * 够用即可，不做完整 CSS 解析：配平一层花括号就能拿到 declared 块。
 *
 * @param source CSS / SCSS 源码
 * @param marker 选择器或 at-rule（含它后面的空格与 `{`，如 `".site-header {"`）
 * @param last 找最后一次出现而不是第一次
 */
export function blockAfter(source: string, marker: string, last = false): string {
  const start = last ? source.lastIndexOf(marker) : source.indexOf(marker);
  expect(start, `找不到 ${marker}`).toBeGreaterThan(-1);
  const open = source.indexOf("{", start);
  let depth = 0;
  for (let index = open; index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    if (source[index] === "}") {
      depth -= 1;
      if (depth === 0) return source.slice(open + 1, index);
    }
  }
  throw new Error(`${marker} 的块没有闭合`);
}

/**
 * 去掉 CSS / TS / Vue 源码里的注释。
 *
 * 注释里提到旧 token 名或旧写法（为了可读性）不该被当成"还在消费"，所有扫描类断言
 * 都应先过一遍这里。
 */
export function stripComments(text: string): string {
  return text
    .replace(/<!--[\s\S]*?-->/gu, "")
    .replace(/\/\*[\s\S]*?\*\//gu, "")
    .replace(/^[ \t]*\/\/.*$/gmu, "");
}

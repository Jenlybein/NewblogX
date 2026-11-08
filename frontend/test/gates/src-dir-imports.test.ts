import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 门禁：`app/`（srcDir）里的代码**不许用相对路径跨出 srcDir**。
 *
 * 为什么需要它（真实事故）：#314 的 `app/utils/seo.ts` 写了
 * `import { siteOrigin } from "../../shared/siteUrl"`。typecheck 过、单测过
 * （vitest 的 alias 能解析任何路径），但 `nuxt build` 在 nitro 阶段炸了：
 *
 *     RollupError: Could not resolve "../shared/siteUrl.ts"
 *     from node_modules/.cache/nuxt/.nuxt/dist/server/_nuxt/history-*.js
 *
 * 因为 server chunk 被搬进 `_nuxt/` 之后，相对路径的基准变了。这类错误**只有
 * 真正跑构建才暴露**，而那一步不在任何门禁里——于是它一路合并进 main、进了
 * 生产部署流程。
 *
 * 修法与规则：跨出 `app/` 的导入一律走别名（`#shared/*` 等，见 nuxt.config）。
 * 别名在构建期由 resolver 展开，不会被原样搬进 chunk。这条门禁用静态扫描，
 * 毫秒级，不用跑构建就能拦住同一类错误。
 */

const appDir = resolve(process.cwd(), "app");

/** 收集 app/ 下所有 .ts / .vue 文件。 */
function collectSourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      out.push(...collectSourceFiles(full));
      continue;
    }
    if (/\.(ts|vue)$/u.test(entry)) out.push(full);
  }
  return out;
}

/** 抓出所有相对导入的模块说明符。 */
function relativeImportsOf(source: string): string[] {
  const specifiers: string[] = [];
  // import ... from "..." / export ... from "..." / import("...")
  const patterns = [
    /(?:^|[\s;])import\s[^;]*?from\s*["']([^"']+)["']/gu,
    /(?:^|[\s;])export\s[^;]*?from\s*["']([^"']+)["']/gu,
    /\bimport\s*\(\s*["']([^"']+)["']\s*\)/gu,
  ];
  for (const pattern of patterns) {
    for (const match of source.matchAll(pattern)) {
      const specifier = match[1];
      if (specifier?.startsWith(".")) specifiers.push(specifier);
    }
  }
  return specifiers;
}

describe("门禁：app/ 内不得用相对路径跨出 srcDir", () => {
  it("扫到的文件数非零（否则这条门禁是空转）", () => {
    expect(collectSourceFiles(appDir).length).toBeGreaterThan(50);
  });

  it("没有 import ../../ 这类逃出 app/ 的相对导入", () => {
    const violations: string[] = [];

    for (const file of collectSourceFiles(appDir)) {
      const source = readFileSync(file, "utf8");
      for (const specifier of relativeImportsOf(source)) {
        const target = resolve(file, "..", specifier);
        // 只要落点还在 app/ 里就没问题；逃出 srcDir 就必须改走别名。
        if (!relative(appDir, target).startsWith("..")) continue;
        violations.push(`${relative(process.cwd(), file)} -> ${specifier}`);
      }
    }

    expect(violations).toEqual([]);
  });

  it("共享目录仍然通过 #shared 别名被引用（别名没被误删）", () => {
    const seo = readFileSync(resolve(appDir, "utils/seo.ts"), "utf8");
    expect(seo).toContain("#shared/");
  });
});

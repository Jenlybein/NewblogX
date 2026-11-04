import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

import { errorMessage } from "~/utils/errorMessage";

/**
 * 错误文案的唯一取值点（评审 #248 / P1-2）。
 *
 * 修复前 `reason instanceof Error ? reason.message : "…"` 在 app/ 里复制了五十多处，
 * 连 `describeError` / `errorMessageOf` 这类"看起来像工具"的本地副本都有两份。
 * 这里钉住共用实现，并加一道防回归扫描：app/ 下不许再出现原地三元式。
 */

describe("errorMessage：从 unknown 取一句可展示的话", () => {
  it("Error 取它的 message", () => {
    expect(errorMessage(new Error("收藏操作失败"), "兜底")).toBe("收藏操作失败");
  });

  it("非 Error 一律回落（字符串 / 普通对象 / undefined / null）", () => {
    expect(errorMessage("字符串不会被拿来展示", "兜底")).toBe("兜底");
    expect(errorMessage({ message: "看起来像错误但类型不是" }, "兜底")).toBe("兜底");
    expect(errorMessage(undefined, "兜底")).toBe("兜底");
    expect(errorMessage(null, "兜底")).toBe("兜底");
  });

  it("空 message 也回落：toast 里不能出现空气泡", () => {
    expect(errorMessage(new Error(""), "兜底")).toBe("兜底");
  });
});

describe("防回归：app/ 源码里不再原地复制 Error 取值三元式", () => {
  const APP_DIR = join(process.cwd(), "app");
  // errorMessage.ts 的文档注释里保留了这个写法的引用（说明"它以前长什么样"），不算违规。
  const ALLOWED = new Set([join("app", "utils", "errorMessage.ts")]);

  function walk(dir: string, out: string[] = []): string[] {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const full = join(dir, entry.name);
      if (entry.isDirectory()) walk(full, out);
      else if (/\.(ts|vue)$/u.test(entry.name)) out.push(full);
    }
    return out;
  }

  it("实例化写法 `X instanceof Error ? X.message : …` 只允许出现在 errorMessage.ts 的实现与注释里", () => {
    const offenders = walk(APP_DIR)
      .filter((file) => !ALLOWED.has(relative(process.cwd(), file)))
      .filter((file) => /instanceof Error \?/u.test(readFileSync(file, "utf8")))
      .map((file) => relative(process.cwd(), file));

    expect(offenders).toEqual([]);
  });
});

import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  TOKEN_FILES,
  checkTokenParity,
  evaluateTokenParity,
  findFirstDifference,
} from "../../../scripts/check-token-parity.mjs";

/**
 * 「两端 tokens.css 逐字节一致」同步门禁（#22）。
 *
 * 为什么是逐字节而不是"解析后比语义"：两份文件是**同源义务**，不是两份可漂移的
 * 配置。语义比对会放过注释/顺序/空白差异，而任何一种差异都意味着有人只改了一端 ——
 * 恰恰是这门禁要拦的事故（改了读者站忘记改后台，后台就静默停在旧色）。
 *
 * canary 必须是"故意只改一端"：这条用例若永远绿，门禁等于不存在。
 *
 * vitest 以 project root（frontend/）为 cwd；仓库根是上一级目录。
 */
const projectDir = process.cwd();
const repoRoot = join(projectDir, "..");
const readerCss = readFileSync(join(repoRoot, TOKEN_FILES.reader), "utf8");
const adminCss = readFileSync(join(repoRoot, TOKEN_FILES.admin), "utf8");

/** 同一段内容里"第几次出现该标记"所在的行号（1-based）。 */
function lineOfToken(source: string, marker: string): number {
  return source.split(/\r?\n/u).findIndex((line) => line.includes(marker)) + 1;
}

describe("两端 token 逐字节同源门禁（真实仓库为绿）", () => {
  it("两份 tokens.css 逐字节一致，且真的读到了内容", () => {
    const result = checkTokenParity({ repoRoot });

    expect(result.ok).toBe(true);
    expect(result.failures).toEqual([]);
    // 扫描器必须真的读到文件：两端都非空且长度相等。
    expect(result.bytes).toBeGreaterThan(2048);
    expect(result.files.reader).toBe(result.bytes);
    expect(result.files.admin).toBe(result.bytes);
    expect(findFirstDifference(readerCss, adminCss)).toBeNull();
  });
});

describe("canary：故意只改一端时必须红", () => {
  it("改一个字符（长度不变）→ 失败，并指出第一个差异的字节偏移与两端行号", () => {
    const marker = "--jelly-cream: #fdfbf6";
    const brokenAdmin = adminCss.replace(marker, "--jelly-cream: #fdfbf0");
    const expectedLine = lineOfToken(readerCss, marker);

    const result = evaluateTokenParity({
      left: readerCss,
      right: brokenAdmin,
      leftPath: TOKEN_FILES.reader,
      rightPath: TOKEN_FILES.admin,
    });

    expect(result.ok).toBe(false);
    expect(result.failures.map((failure: { kind: string }) => failure.kind)).toEqual([
      "byte-mismatch",
    ]);
    expect(result.difference?.offset).toBeGreaterThan(0);
    expect(result.difference?.leftLine).toBe(expectedLine);
    expect(result.difference?.rightLine).toBe(expectedLine);
    // 长度相同也算不一致：只比文件大小会漏掉这类"改了一半"的事故。
    expect(result.difference?.leftSize).toBe(result.difference?.rightSize);
    expect(result.failures[0].message).toContain(TOKEN_FILES.reader);
    expect(result.failures[0].message).toContain(TOKEN_FILES.admin);
  });

  it("只多一个结尾换行也算不一致（不做 diff 归一化）", () => {
    const result = evaluateTokenParity({
      left: readerCss,
      right: `${adminCss}\n`,
      leftPath: TOKEN_FILES.reader,
      rightPath: TOKEN_FILES.admin,
    });

    expect(result.ok).toBe(false);
    expect(result.difference?.rightSize ?? 0).toBe((result.difference?.leftSize ?? 0) + 1);
  });

  it("只改注释也算不一致（同源义务覆盖整份文件）", () => {
    const result = evaluateTokenParity({
      left: `${readerCss.replace("/* ---- 1.", "/* ---- 改过的 1.")}`,
      right: adminCss,
      leftPath: TOKEN_FILES.reader,
      rightPath: TOKEN_FILES.admin,
    });

    expect(result.ok).toBe(false);
  });

  it("文件不可读（缺失）时失败，而不是当成「两端都空」通过", () => {
    const result = checkTokenParity({ repoRoot: join(projectDir, "test/gates/__missing") });

    expect(result.ok).toBe(false);
    expect(result.failures.map((failure: { kind: string }) => failure.kind)).toEqual([
      "token-file-missing",
      "token-file-missing",
    ]);
  });
});

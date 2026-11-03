import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  IMAGE_SHAPING_FILES,
  checkImageShapingParity,
  describeDrift,
  evaluateImageShapingParity,
  findFirstDifference,
} from "../../../scripts/check-image-shaping-parity.mjs";

/**
 * 「两端 imageShaping.ts 逐字节一致」同步门禁（#200；#195 明确交办）。
 *
 * 后台不能 import 读者站模块（#176：生产构建会失败），所以三种封面与头像共用的
 * 定形实现只能在两端各放一份**逐字一致**的文件。只靠纪律的话，改一端忘另一端时
 * 两端各自的测试都绿（每份内部自洽），后台静默停在旧裁切尺寸。
 *
 * canary 必须是"故意只改一端"：这条用例若永远绿，门禁等于不存在。
 * vitest 以 project root（frontend/）为 cwd；仓库根是上一级目录。
 */
const projectDir = process.cwd();
const repoRoot = join(projectDir, "..");
const readerShaping = readFileSync(join(repoRoot, IMAGE_SHAPING_FILES.reader), "utf8");
const adminShaping = readFileSync(join(repoRoot, IMAGE_SHAPING_FILES.admin), "utf8");

describe("两端定形工具逐字节同源门禁（真实仓库为绿）", () => {
  it("两份 imageShaping.ts 逐字节一致，且真的读到了内容", () => {
    const result = checkImageShapingParity({ repoRoot });

    expect(result.ok).toBe(true);
    expect(result.failures).toEqual([]);
    // 扫描器必须真的读到文件：两端都非空且长度相等。
    expect(result.bytes).toBeGreaterThan(1024);
    expect(result.files.reader).toBe(result.bytes);
    expect(result.files.admin).toBe(result.bytes);
    expect(findFirstDifference(readerShaping, adminShaping)).toBeNull();
  });
});

describe("canary：只改一端时必须红，并指出是哪一份漂移", () => {
  it("只改后台副本的一个常量（长度相同）→ 失败，报偏移、两端行号与权威方向", () => {
    const marker = "{ width: 1600, height: 900 }";
    const brokenAdmin = adminShaping.replace(marker, "{ width: 1500, height: 900 }");

    const result = evaluateImageShapingParity({
      left: readerShaping,
      right: brokenAdmin,
      leftPath: IMAGE_SHAPING_FILES.reader,
      rightPath: IMAGE_SHAPING_FILES.admin,
    });

    expect(result.ok).toBe(false);
    expect(result.failures.map((failure: { kind: string }) => failure.kind)).toEqual(["byte-mismatch"]);
    expect(result.difference?.offset).toBeGreaterThan(0);
    // 长度相同也算不一致：只比文件大小会漏掉"改了一个字符"这类最常见的事故。
    expect(result.difference?.leftSize).toBe(result.difference?.rightSize);
    expect(result.failures[0].message).toContain(IMAGE_SHAPING_FILES.reader);
    expect(result.failures[0].message).toContain(IMAGE_SHAPING_FILES.admin);
    expect(result.failures[0].message).toContain("两份长度相同");
    expect(result.failures[0].message).toContain("权威方向");
  });

  it("后台副本多出内容时，明确指出多出多少字节（漂移方向）", () => {
    const result = evaluateImageShapingParity({
      left: readerShaping,
      right: `${adminShaping}\n// 只改了后台，忘了同步读者站\n`,
      leftPath: IMAGE_SHAPING_FILES.reader,
      rightPath: IMAGE_SHAPING_FILES.admin,
    });

    expect(result.ok).toBe(false);
    expect(result.failures[0].message).toContain("后台那份比读者站多出");
    expect(describeDrift(result.difference, IMAGE_SHAPING_FILES.reader, IMAGE_SHAPING_FILES.admin)).toContain(
      "读者站还没同步",
    );
  });

  it("只改读者站那份也算不一致（同源义务是双向的）", () => {
    const brokenReader = `${readerShaping}\n// 只在读者站改了，忘了复制到后台\n`;

    const result = evaluateImageShapingParity({
      left: brokenReader,
      right: adminShaping,
      leftPath: IMAGE_SHAPING_FILES.reader,
      rightPath: IMAGE_SHAPING_FILES.admin,
    });

    expect(result.ok).toBe(false);
    expect(result.failures[0].message).toContain("读者站那份比后台多出");
  });

  it("文件不可读（缺失）时失败，而不是当成「两端都空」通过", () => {
    const result = checkImageShapingParity({ repoRoot: join(projectDir, "test/utils/__missing") });

    expect(result.ok).toBe(false);
    expect(result.failures.map((failure: { kind: string }) => failure.kind)).toEqual([
      "image-shaping-file-missing",
      "image-shaping-file-missing",
    ]);
  });
});

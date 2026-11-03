import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import {
  IMAGE_CROP_FILES,
  IMAGE_SHAPING_FILES,
  checkImageCropParity,
  checkImageShapingParity,
  describeDrift,
  evaluateImageCropParity,
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

/**
 * 裁剪窗四份组件同源门禁（#225）。
 *
 * 与 imageShaping.ts 同一个物理原因：后台不能 import 读者站模块，于是
 * `ImageCropView.vue` / `ImageCropDialog.vue` 在两端各放一份**逐字节一致**的副本。
 * 改动前的状态就是没有门禁：谁改了一端的取景交互，另一端的组件测试照样绿，
 * 用户在后台与读者站看到两套裁剪窗——而且没有任何一处会报出来。
 *
 * canary 同样是"故意只改一端"：只改后台那份的一个属性值，门禁必须红并点名
 * 是哪一对、哪一份漂移。
 */
const cropView = readFileSync(join(repoRoot, IMAGE_CROP_FILES.view.reader), "utf8");
const cropDialog = readFileSync(join(repoRoot, IMAGE_CROP_FILES.dialog.reader), "utf8");

describe("四份裁剪组件逐字节同源门禁（真实仓库为绿）", () => {
  it("View / Dialog 两对都逐字节一致，且四份文件都真的读到了内容", () => {
    const result = checkImageCropParity({ repoRoot });

    expect(result.ok).toBe(true);
    expect(result.failures).toEqual([]);
    expect(result.files.view.reader).toBeGreaterThan(1024);
    expect(result.files.view.reader).toBe(result.files.view.admin);
    expect(result.files.dialog.reader).toBeGreaterThan(1024);
    expect(result.files.dialog.reader).toBe(result.files.dialog.admin);
    expect(findFirstDifference(cropView, readFileSync(join(repoRoot, IMAGE_CROP_FILES.view.admin), "utf8"))).toBeNull();
  });
});

describe("canary：四份组件里只改一端必须红，并点名是哪一对", () => {
  it("只改后台 ImageCropView 的一个字符（长度相同）→ 失败，消息里带文件名与「两份长度相同」", () => {
    const result = evaluateImageCropParity({
      pairs: [
        {
          label: "ImageCropView",
          left: cropView,
          right: cropView.replace("defineProps", "definePropz"),
          leftPath: IMAGE_CROP_FILES.view.reader,
          rightPath: IMAGE_CROP_FILES.view.admin,
        },
      ],
    });

    expect(result.ok).toBe(false);
    expect(result.failures.map((failure: { kind: string }) => failure.kind)).toEqual(["byte-mismatch"]);
    expect(result.failures[0].message).toContain("ImageCropView");
    expect(result.failures[0].message).toContain(IMAGE_CROP_FILES.view.reader);
    expect(result.failures[0].message).toContain(IMAGE_CROP_FILES.view.admin);
    expect(result.failures[0].message).toContain("两份长度相同");
  });

  it("只改后台 ImageCropDialog（多出一行）→ 指出多出字节的那一侧", () => {
    const result = evaluateImageCropParity({
      pairs: [
        {
          label: "ImageCropDialog",
          left: cropDialog,
          right: `${cropDialog}\n<!-- 只改了后台，忘了同步读者站 -->\n`,
          leftPath: IMAGE_CROP_FILES.dialog.reader,
          rightPath: IMAGE_CROP_FILES.dialog.admin,
        },
      ],
    });

    expect(result.ok).toBe(false);
    expect(result.failures[0].message).toContain("ImageCropDialog");
    expect(result.failures[0].message).toContain("后台那份比读者站多出");
  });

  it("四份里缺文件时失败，而不是当成「两端都空」通过", () => {
    const result = checkImageCropParity({ repoRoot: join(projectDir, "test/utils/__missing") });

    expect(result.ok).toBe(false);
    expect(result.failures.map((failure: { kind: string }) => failure.kind)).toEqual([
      "image-crop-file-missing",
      "image-crop-file-missing",
      "image-crop-file-missing",
      "image-crop-file-missing",
    ]);
  });
});

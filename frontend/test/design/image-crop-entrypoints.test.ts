import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

/**
 * 裁剪窗的五个入口必须共用同一份编排（#215 / ADR-0013 后续修订）。
 *
 * 为什么是静态门禁：五个入口分布在两个应用、五个文件里，行为测试只能逐个挂载
 * （读者站写作页 / 设置页、后台三个页面），回归时最容易出的不是几何算错，而是
 * **某个入口又自己写了一段"先上传再问"或漏了取消路径**。这类事故每个入口的
 * 界面都看不出来，只能从源码约束：必须引用 `useImageCropUpload` 与 `ImageCropDialog`，
 * 且取消/确认是两条独立出口。
 *
 * 几何与比例判定的正确性由 `imageShaping` 的纯函数用例覆盖；这里只锁接线。
 */
const projectDir = process.cwd();
const repoRoot = join(projectDir, "..");

/** 五个入口（读者站文章封面由 composable 编排、页面渲染裁剪窗，所以是一对文件）。 */
const ENTRYPOINTS = [
  {
    name: "读者站文章封面",
    files: ["frontend/app/composables/useStudioArticle.ts", "frontend/app/pages/studio/write.vue"],
  },
  {
    name: "读者站头像",
    files: ["frontend/app/pages/studio/settings.vue"],
  },
  {
    // 票 330 起轮播的新增 / 编辑都在弹窗组件里：封面编排与裁剪窗随之从页面搬进组件，
    // 页面只负责打开弹窗。入口换了文件，不变量没变。
    name: "后台轮播封面",
    files: ["frontend_admin/app/components/media/BannerFormModal.vue"],
  },
  {
    // 2026-11-04 起合集 / 项目的封面都在编辑弹窗里：两个入口共用同一个组件，
    // 所以各自指向同一个文件（弹窗按 `kind` 决定调哪套接口）。
    name: "后台合集封面",
    files: ["frontend_admin/app/components/articles/CollectionEditorOverlay.vue"],
  },
  {
    name: "后台项目封面",
    files: ["frontend_admin/app/components/articles/CollectionEditorOverlay.vue"],
  },
] as const;

function read(relativePath: string): string {
  return readFileSync(join(repoRoot, relativePath), "utf8");
}

/** 一个入口的全部源码（多文件时按顺序拼起来，断言只关心"这个入口整体有没有"）。 */
function readEntrypoint(files: readonly string[]): string {
  return files.map(read).join("\n");
}

describe("五个封面/头像入口共用同一份裁剪编排", () => {
  it.each(ENTRYPOINTS)("$name 走 useImageCropUpload + ImageCropDialog", ({ files }) => {
    const source = readEntrypoint(files);

    // 编排：比例判定、定形、上传顺序只有一份实现。
    expect(source).toContain("useImageCropUpload");
    // 界面：裁剪窗只有一个组件（两端各一份逐字节副本）。
    expect(source).toContain("ImageCropDialog");
    // 不合规时"只请求裁剪"，确认与取消是两个独立出口。
    expect(source).toMatch(/requestFraming/u);
    expect(source).toMatch(/confirm\w*Crop/u);
    expect(source).toMatch(/cancel\w*Crop/u);
    // 目标比例来自设计常量（封面 16:9 / 头像 1:1），入口不写自己的比例。
    expect(source).toMatch(/AVATAR_SHAPE_TARGET|COVER_SHAPE_TARGET/u);
  });
});

describe("两份 useImageCropUpload 逐字节一致（两端同一个顺序）", () => {
  const readerPath = "frontend/app/composables/useImageCropUpload.ts";
  const adminPath = "frontend_admin/app/composables/useImageCropUpload.ts";

  it("同一个文件内容、同一个判定出口", () => {
    const reader = read(readerPath);
    const admin = read(adminPath);

    // 逐字节一致：与 imageShaping 同一条"改一端必须复制到另一端"的纪律。
    expect(admin).toBe(reader);
    // 不合规 → 只请求裁剪（这里没有上传）；合规 → 先定形再上传。
    expect(reader).toContain("needsFraming(");
    expect(reader).toContain("requestFraming(");
    expect(reader).toMatch(/framing[\s\S]*shapeForUpload|shapeForUpload/u);
  });
});

describe("裁剪窗没有旋转 / 镜像 / 滤镜，比例框由目标派生", () => {
  const views = [
    "frontend/app/components/studio/ImageCropView.vue",
    "frontend_admin/app/components/media/ImageCropView.vue",
  ];

  it.each(views)("%s 只有固定比例框 + 拖动 + 缩放", (path) => {
    const source = read(path);

    expect(source).toMatch(/aspectRatio/u);
    expect(source).not.toMatch(/rotate|scaleX|scaleY|mirror|filter:/u);
    // 1–3× 缩放的上限来自共享常量，组件里不写自己的端点。
    expect(source).toContain("SHAPE_FRAMING_ZOOM_MIN");
    expect(source).toContain("SHAPE_FRAMING_ZOOM_MAX");
    expect(source).toContain("framingImageBox");
  });
});

describe("取消即中止：入口不把「取消」接成第二条上传路径", () => {
  it.each(ENTRYPOINTS)("$name 的取消处理器只清状态、不调用上传", ({ files }) => {
    const source = readEntrypoint(files);
    const cancelHandler = (/(?:function|const)\s+(?:cancel\w*Crop)[^\n]*\{([\s\S]*?)\n\}/u.exec(source)?.[1] ?? "").trim();

    expect(cancelHandler.length).toBeGreaterThan(0);
    // 取消之后不能再有"留下半成品封面"的动作。
    expect(cancelHandler).not.toMatch(/upload|shape|createCollection|createBanner/u);
  });
});

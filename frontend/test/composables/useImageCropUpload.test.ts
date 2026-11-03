import { beforeEach, describe, expect, it, vi } from "vitest";

import { useImageCropUpload } from "~/composables/useImageCropUpload";
import {
  COVER_SHAPE_OPTIONS,
  COVER_SHAPE_TARGET,
  type ShapedUploadFile,
} from "~/utils/imageShaping";

/**
 * 裁剪窗的编排（#215）：所有封面/头像入口都经过这一层。
 *
 * 这一层只有两个出口，二者都影响用户看到的结果：
 *   1. **合规（相差 ≤1%）**：直接定形并交给宿主上传，不弹窗；
 *   2. **不合规**：只把"原图 + 尺寸"交给宿主去弹窗——**不产出文件、不上传**。
 * 确认后再走"按选定区域定形 → 上传"。
 *
 * 判据放在这里测，是因为五个入口（读者站文章封面/头像、后台轮播/合集/项目封面）
 * 都只差一个"上传闭包"，顺序错了每个入口都会静默上传一张自动居中的图。
 */

const measureImageMock = vi.hoisted(() => vi.fn());
const shapeForUploadMock = vi.hoisted(() => vi.fn());

vi.mock("~/utils/imageShaping", async (importOriginal) => {
  const actual = await importOriginal<typeof import("~/utils/imageShaping")>();
  return { ...actual, measureImage: measureImageMock, shapeForUpload: shapeForUploadMock };
});

function shapedCover(): ShapedUploadFile {
  return {
    file: new File(["shaped"], "cover.jpg", { type: "image/jpeg" }),
    previewUrl: "blob:shaped",
    size: { width: 1600, height: 900 },
  };
}

function harness(overrides: Record<string, unknown> = {}) {
  const upload = vi.fn(async () => {});
  const requestFraming = vi.fn();
  const flow = useImageCropUpload({
    options: COVER_SHAPE_OPTIONS,
    upload,
    requestFraming,
    ...overrides,
  });
  return { flow, upload, requestFraming };
}

const image = () => new File(["raw"], "selected.png", { type: "image/png" });

beforeEach(() => {
  measureImageMock.mockReset();
  shapeForUploadMock.mockReset();
  measureImageMock.mockResolvedValue({ width: 1920, height: 1080 });
  shapeForUploadMock.mockResolvedValue(shapedCover());
});

describe("useImageCropUpload · 合规图不弹窗（相差 ≤1%）", () => {
  it("先定形再上传：上传拿到的是定形产物，不是用户选的原图", async () => {
    const { flow, upload, requestFraming } = harness();
    const original = image();

    await flow.select(original);

    expect(measureImageMock).toHaveBeenCalledWith(original);
    expect(shapeForUploadMock).toHaveBeenCalledWith(original, COVER_SHAPE_OPTIONS);
    expect(requestFraming).not.toHaveBeenCalled();
    expect(upload).toHaveBeenCalledTimes(1);
    const uploaded = upload.mock.calls[0]?.[0] as ShapedUploadFile;
    expect(uploaded.file).not.toBe(original);
    expect(uploaded.file.name).toBe("cover.jpg");
  });
});

describe("useImageCropUpload · 不合规图先弹窗、不产出也不上传", () => {
  it("只把原图与像素尺寸交给宿主，形状留待确认", async () => {
    measureImageMock.mockResolvedValue({ width: 1000, height: 1000 });
    const { flow, upload, requestFraming } = harness();
    const original = image();

    await flow.select(original);

    expect(requestFraming).toHaveBeenCalledWith({ file: original, width: 1000, height: 1000 });
    // 关键：候选图在确认之前不能留下任何"已定形"的产物，也不能上传。
    expect(shapeForUploadMock).not.toHaveBeenCalled();
    expect(upload).not.toHaveBeenCalled();
  });
});

describe("useImageCropUpload · 确认裁剪后按选区定形再上传", () => {
  it("把裁剪状态带进定形参数（沿用目标尺寸），然后上传新文件", async () => {
    const { flow, upload } = harness();
    const original = image();
    const source = { file: original, width: 1000, height: 1000 };
    const framing = { zoom: 2, centerX: 0.5, centerY: 0.5 };

    await flow.confirm(source, framing);

    expect(shapeForUploadMock).toHaveBeenCalledWith(original, {
      ...COVER_SHAPE_OPTIONS,
      framing,
    });
    expect(upload).toHaveBeenCalledTimes(1);
    expect((upload.mock.calls[0]?.[0] as ShapedUploadFile).file.name).toBe("cover.jpg");
  });

  it("确认不会碰自动居中那条路径：没有 framing 的定形参数不会被复用", async () => {
    const { flow } = harness();
    const framing = { zoom: 3, centerX: 0.2, centerY: 0.8 };
    await flow.confirm({ file: image(), width: 900, height: 1200 }, framing);

    const options = shapeForUploadMock.mock.calls[0]?.[1] as { framing?: unknown; target: unknown };
    expect(options.framing).toEqual(framing);
    expect(options.target).toEqual(COVER_SHAPE_TARGET);
  });
});

describe("useImageCropUpload · 失败路径", () => {
  it("量尺寸失败原样上抛，不弹窗、不定形、不上传", async () => {
    measureImageMock.mockRejectedValue(new Error("无法读取这张图片，请换一张"));
    const { flow, upload, requestFraming } = harness();

    await expect(flow.select(image())).rejects.toThrow("无法读取这张图片，请换一张");

    expect(requestFraming).not.toHaveBeenCalled();
    expect(shapeForUploadMock).not.toHaveBeenCalled();
    expect(upload).not.toHaveBeenCalled();
  });

  it("定形失败原样上抛，不上传（宿主据此把失败留在面板上）", async () => {
    shapeForUploadMock.mockRejectedValue(new Error("无法生成处理后的图片"));
    const { flow, upload } = harness();

    await expect(flow.select(image())).rejects.toThrow("无法生成处理后的图片");
    expect(upload).not.toHaveBeenCalled();
  });

  it("上传失败原样上抛，由宿主决定如何呈现", async () => {
    const { flow } = harness({
      upload: async () => {
        throw new Error("网络断开");
      },
    });

    await expect(flow.select(image())).rejects.toThrow("网络断开");
  });
});

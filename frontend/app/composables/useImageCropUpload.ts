import {
  measureImage,
  needsFraming,
  shapeForUpload,
  type ShapeFramingState,
  type ShapeImageOptions,
  type ShapeImageTarget,
  type ShapedUploadFile,
} from "~/utils/imageShaping";

/**
 * 裁剪窗的编排（票 215）：五个封面/头像入口只差一个"上传闭包"。
 *
 * 它的全部职责是保证顺序只有一条：
 *   1. 量原图尺寸 → 判比例；
 *   2. 合规（相差 ≤1%）→ 定形 → 交给宿主上传（不弹窗）；
 *   3. 不合规 → **只**把原图与尺寸交给宿主打开裁剪窗，不产出文件、不上传；
 *   4. 确认裁剪 → 按选定区域定形成新文件 → 上传。
 *
 * 为什么单独抽一层：判据错一处（比如把"不合规"当成"先上传再问"），五个入口会
 * 同时静默上传一张自动居中的图，而每个入口的界面都看不出来。把顺序钉在这里，
 * 入口页面只剩"打开窗、传回调"。
 *
 * `measure` / `shape` 可注入：单测不需要真实画布或图片解码（ADR-0016）。
 */
export interface ImageFramingRequest {
  /** 用户选的原图（确认前不上传、不产生任何文件）。 */
  file: File;
  /** 原图像素尺寸：裁剪窗据此决定可拖动的余量。 */
  width: number;
  height: number;
}

/** 量原图尺寸（默认浏览器解码；测试注入假实现）。 */
export type CropMeasure = (file: File) => Promise<ShapeImageTarget>;

/** 定形（默认 `shapeForUpload`；测试注入假实现）。 */
export type CropShaper = (file: File, options: ShapeImageOptions) => Promise<ShapedUploadFile>;

export interface ImageCropUploadDeps {
  /** 该入口的目标比例/尺寸/输出格式（封面 16:9 或头像 1:1）。 */
  options: ShapeImageOptions;
  /** 定形产物就绪：宿主上传它（原图永不进入这一步）。 */
  upload: (shaped: ShapedUploadFile) => Promise<void> | void;
  /** 比例不合规：宿主保存来源并打开裁剪窗。这里不会产出任何文件。 */
  requestFraming: (request: ImageFramingRequest) => void;
  measure?: CropMeasure;
  shape?: CropShaper;
}

export function useImageCropUpload(deps: ImageCropUploadDeps) {
  const measure = deps.measure ?? ((file: File) => measureImage(file));
  const shape = deps.shape ?? ((file: File, options: ShapeImageOptions) => shapeForUpload(file, options));

  /** 定形并上传；`framing` 存在时按作者选定的区域取景。 */
  async function uploadShaped(file: File, framing?: ShapeFramingState): Promise<void> {
    const options = framing ? { ...deps.options, framing } : deps.options;
    await deps.upload(await shape(file, options));
  }

  /** 选完图后的唯一入口。 */
  async function select(file: File): Promise<void> {
    const size = await measure(file);
    if (needsFraming(size, deps.options.target)) {
      deps.requestFraming({ file, width: size.width, height: size.height });
      return;
    }
    await uploadShaped(file);
  }

  /** 裁剪窗确认：把选区烤进新文件再上传。 */
  async function confirm(request: ImageFramingRequest, framing: ShapeFramingState): Promise<void> {
    await uploadShaped(request.file, framing);
  }

  return { select, confirm };
}

/**
 * 定形工具：给定文件 + 目标尺寸 + 输出类型 → 新 Blob。
 *
 * 「定形」是文件级改动：把任意比例的图片**居中**裁成目标比例，
 * 再降采样到目标像素尺寸，原图不参与上传；显示侧不再有第二套几何。
 *
 * 为什么把画布与图像构造做成注入点：测试环境没有 canvas 实现（ADR-0016），
 * 定形又必须能在 vitest / SSR 里安全 import。所以本模块只做**纯计算 + 编排**，
 * 所有浏览器 API（decode / createCanvas / encode）都在 `deps` 后面，默认实现
 * 也只在真正调用时才碰 `document` / `Image`，模块加载期没有任何全局副作用。
 *
 * 工具保持单文件、零 import：后台（frontend_admin）按 #176 的「自洽」约束持有
 * 一份**逐字一致**的副本（`frontend_admin/app/utils/imageShaping.ts`），由
 * `scripts/check-image-shaping-parity.mjs` 逐字节比对。因此这里不能依赖读者站
 * 任何其他模块——这份文件就是两端共享面（头像、三种封面都从这里取）。
 */

/** 定形目标：产物的像素宽高（也是裁切后的比例）。 */
export interface ShapeImageTarget {
  width: number;
  height: number;
}

/** 居中裁切的源矩形（drawImage 的 9 参形式前 4 个）。 */
export interface ShapeImageCropRect {
  sx: number;
  sy: number;
  sw: number;
  sh: number;
}

/**
 * 裁剪窗的取景状态（票 215）：缩放倍数 + 取景区域中心在源图上的归一化坐标。
 * 这是**内存里的临时状态**，确认后只会被换算成源图矩形烤进新文件；不存任何参数。
 */
export interface ShapeFramingState {
  /** 1 = 目标比例下能取到的最大区域（等于自动居中裁的那块），3 = 放大三倍。 */
  zoom: number;
  /** 取景区域中心在源图上的相对位置（0–1）。 */
  centerX: number;
  centerY: number;
}

/** 裁剪窗的缩放范围（票 215 的交互规格：1–3×，不做 0.x 的外扩）。 */
export const SHAPE_FRAMING_ZOOM_MIN = 1;
export const SHAPE_FRAMING_ZOOM_MAX = 3;
/** 一次键盘/按钮缩放的步长；滑杆也用它当步进。 */
export const SHAPE_FRAMING_ZOOM_STEP = 0.1;

/** 打开裁剪窗时的默认取景：居中、1×（与自动居中裁切完全同一块区域）。 */
export const DEFAULT_SHAPE_FRAMING: ShapeFramingState = { zoom: 1, centerX: 0.5, centerY: 0.5 };

/**
 * 与目标比例的相对偏差容差：±1%（票 215 / ADR-0015 后续修订）。
 * 偏差按目标比例归一化，因此 16:9 与 1:1 两个目标共用同一个阈值。
 */
export const SHAPE_ASPECT_TOLERANCE = 0.01;

/** 定形产物类型：只允许位图格式，不允许 SVG（canvas 无法安全栅格化外部 SVG）。 */
export type ShapeImageOutputType = "image/png" | "image/jpeg" | "image/webp";

/**
 * 定形参数：目标尺寸 + 输出类型（JPEG/WebP 可给质量）。
 * `framing` 存在时按作者在裁剪窗里选定的区域取景；缺省＝自动居中裁切。
 */
export interface ShapeImageOptions {
  target: ShapeImageTarget;
  outputType: ShapeImageOutputType;
  quality?: number;
  framing?: ShapeFramingState;
}

/** 解码后的源：原始尺寸 + 能直接交给 drawImage 的对象。 */
export interface ShapeImageSource {
  source: CanvasImageSource;
  width: number;
  height: number;
}

/**
 * 定形依赖的三个注入点。测试里全部注入假实现，因此不需要真实画布；
 * 默认实现见 `browserShapeImageDeps`。
 */
export interface ShapeImageDeps {
  /** 把图片文件解码成可绘制的源与原始像素尺寸。 */
  decode: (file: Blob) => Promise<ShapeImageSource>;
  /** 创建目标尺寸的画布。 */
  createCanvas: (width: number, height: number) => HTMLCanvasElement;
  /** 把画布编码成 Blob；编码失败返回 null。 */
  encode: (
    canvas: HTMLCanvasElement,
    outputType: ShapeImageOutputType,
    quality?: number,
  ) => Promise<Blob | null>;
}

/** 定形失败的类型；调用方按码判断，按 message 直接给用户看。 */
export type ImageShapingErrorCode =
  | "not_image"
  | "invalid_target"
  | "decode_failed"
  | "canvas_unavailable"
  | "encode_failed";

export class ImageShapingError extends Error {
  readonly code: ImageShapingErrorCode;

  constructor(code: ImageShapingErrorCode, message: string) {
    super(message);
    this.name = "ImageShapingError";
    this.code = code;
  }
}

/**
 * 封面定形的目标尺寸：**16:9，1600×900**（#200）。三种封面上传（后台轮播、读者站
 * 文章封面、后台合集封面）与两端的用户提示都只消费这一份取值，模板与页面里不再
 * 各写一遍 16:9 / 1600 / 900。
 *
 * 为什么放在这份文件里而不是各 app 的 utils：后台不能 import 读者站模块（#176 的
 * 「自洽」约束），共享只能靠「两端逐字节一致的同一份文件」；把取值放进这份文件，
 * 两端拿到的常量就必然同一个，漂移会被 scripts/check-image-shaping-parity.mjs 拦住。
 */
export const COVER_SHAPE_TARGET: ShapeImageTarget = { width: 1600, height: 900 };

/** 封面的固定比例口径（16:9，与 `COVER_SHAPE_TARGET` 的 1600×900 同一比例）。 */
export const COVER_SHAPE_RATIO_TEXT = "16:9";

/** 封面产物尺寸的可读形式（界面提示与元信息都用它，免得散落魔数）。 */
export const COVER_SHAPE_SIZE_TEXT = `${COVER_SHAPE_TARGET.width} × ${COVER_SHAPE_TARGET.height}`;

/**
 * 封面定形参数：居中裁成 16:9、降采样到 1600×900、输出 JPEG（封面是照片类内容，
 * JPEG 比 PNG 小一个量级；上传链路只接受位图，SVG 已在类型上被排除）。
 */
export const COVER_SHAPE_OPTIONS: ShapeImageOptions = {
  target: COVER_SHAPE_TARGET,
  outputType: "image/jpeg",
  quality: 0.9,
};

/** 只看 MIME 前缀，不看文件后缀（后缀可伪造，MIME 由浏览器按内容给出）。 */
export function isImageFile(file: Blob): boolean {
  return typeof file.type === "string" && file.type.toLowerCase().startsWith("image/");
}

function normalizeShapeTarget(target: ShapeImageTarget): ShapeImageTarget {
  const { width, height } = target;
  if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0) {
    throw new ImageShapingError("invalid_target", "目标尺寸必须是正整数");
  }
  return { width, height };
}

/**
 * 居中裁切计算：把源图按目标比例取最大居中矩形，不缩放、只取子集。
 * 纯函数，独立于画布，便于封面等场景直接复用同一套比例口径。
 */
export function centerCropRect(
  source: ShapeImageTarget,
  target: ShapeImageTarget,
): ShapeImageCropRect {
  const targetRatio = target.width / target.height;
  const sourceRatio = source.width / source.height;

  if (sourceRatio > targetRatio) {
    // 源更宽：左右各裁掉一部分，高度取满。
    const sw = source.height * targetRatio;
    return { sx: (source.width - sw) / 2, sy: 0, sw, sh: source.height };
  }

  // 源更高（或比例相同）：上下各裁掉一部分，宽度取满。
  const sh = source.width / targetRatio;
  return { sx: 0, sy: (source.height - sh) / 2, sw: source.width, sh };
}

/**
 * 源图比例与目标比例的**相对偏差**（0.01 = 1%）。
 * 按目标比例归一化，所以"差 1%"在 16:9 与 1:1 下是同一个口径。
 * 尺寸非法时返回 Infinity（当作"必须让作者选"），不让脏数据走进自动居中。
 */
export function aspectRatioDeviation(source: ShapeImageTarget, target: ShapeImageTarget): number {
  if (
    !(source.width > 0) ||
    !(source.height > 0) ||
    !(target.width > 0) ||
    !(target.height > 0)
  ) {
    return Number.POSITIVE_INFINITY;
  }
  const targetRatio = target.width / target.height;
  return Math.abs(source.width / source.height - targetRatio) / targetRatio;
}

/** 比例合规（相差 ≤ tolerance）→ 直接自动居中裁，不弹裁剪窗。 */
export function isAspectRatioCompliant(
  source: ShapeImageTarget,
  target: ShapeImageTarget,
  tolerance: number = SHAPE_ASPECT_TOLERANCE,
): boolean {
  const deviation = aspectRatioDeviation(source, target);
  return Number.isFinite(deviation) && deviation <= tolerance;
}

/** 比例不合规（相差 > 1%）→ 必须由作者在裁剪窗里选区域。 */
export function needsFraming(
  source: ShapeImageTarget,
  target: ShapeImageTarget,
  tolerance: number = SHAPE_ASPECT_TOLERANCE,
): boolean {
  return !isAspectRatioCompliant(source, target, tolerance);
}

/** 把值夹在 [min, max]；区间非法（源图比取景框还小）时取 0。 */
function clampRange(value: number, min: number, max: number): number {
  if (max <= min) return min;
  return Math.min(max, Math.max(min, value));
}

/**
 * 取景状态钳位：缩放夹在 1–3×，中心夹到"取景框不越过源图边界"的范围。
 * 非法输入回落到默认值（居中 / 1×），不让 NaN 传进 drawImage。
 */
export function clampFraming(
  framing: ShapeFramingState,
  source: ShapeImageTarget,
  target: ShapeImageTarget,
): ShapeFramingState {
  const rawZoom = Number.isFinite(framing.zoom) ? framing.zoom : DEFAULT_SHAPE_FRAMING.zoom;
  const zoom = Math.min(SHAPE_FRAMING_ZOOM_MAX, Math.max(SHAPE_FRAMING_ZOOM_MIN, rawZoom));

  // 1× 的取景框 = 自动居中裁的那块；放大只会让它变小，所以永远塞得进源图。
  const base = centerCropRect(source, target);
  const halfWidth = (base.sw / zoom) / 2 / source.width;
  const halfHeight = (base.sh / zoom) / 2 / source.height;

  const rawCenterX = Number.isFinite(framing.centerX) ? framing.centerX : DEFAULT_SHAPE_FRAMING.centerX;
  const rawCenterY = Number.isFinite(framing.centerY) ? framing.centerY : DEFAULT_SHAPE_FRAMING.centerY;

  return {
    zoom,
    centerX: clampRange(rawCenterX, halfWidth, 1 - halfWidth),
    centerY: clampRange(rawCenterY, halfHeight, 1 - halfHeight),
  };
}

/**
 * 把裁剪窗里的取景状态换算成**源图上的矩形**（drawImage 的 9 参形式前 4 个）。
 *
 * 这是"预览与最终文件一致"的唯一换算出口：预览组件用它画画面，`shapeImage`
 * 用它烤文件——不存在第二套几何。
 * 默认取景（居中、1×）与 `centerCropRect` 逐值相同。
 */
export function framingSourceRect(
  source: ShapeImageTarget,
  target: ShapeImageTarget,
  framing: ShapeFramingState = DEFAULT_SHAPE_FRAMING,
): ShapeImageCropRect {
  const clamped = clampFraming(framing, source, target);
  const base = centerCropRect(source, target);
  const sw = base.sw / clamped.zoom;
  const sh = base.sh / clamped.zoom;

  return {
    sx: clampRange(clamped.centerX * source.width - sw / 2, 0, source.width - sw),
    sy: clampRange(clamped.centerY * source.height - sh / 2, 0, source.height - sh),
    sw,
    sh,
  };
}

/**
 * 图片在取景框里的布局（相对框的分数）：四边偏移与放大倍数。
 * 与 `framingSourceRect` 一一对应——预览把源矩形的左上角对到框的左上角。
 */
export interface ShapeFramingImageBox {
  left: number;
  top: number;
  width: number;
  height: number;
}

export function framingImageBox(
  source: ShapeImageTarget,
  target: ShapeImageTarget,
  framing: ShapeFramingState = DEFAULT_SHAPE_FRAMING,
): ShapeFramingImageBox {
  const rect = framingSourceRect(source, target, framing);
  // 源矩形贴边时偏移是 0 而不是 -0（-0 在对象比较与 CSS 序列化里都是噪声）。
  return {
    left: rect.sx === 0 ? 0 : -rect.sx / rect.sw,
    top: rect.sy === 0 ? 0 : -rect.sy / rect.sh,
    width: source.width / rect.sw,
    height: source.height / rect.sh,
  };
}

/**
 * 拖动画面：dx/dy 是被拖动的距离占取景框宽/高的**比例**（向右为正）。
 * 拖动画面向右＝取景范围向左，所以中心反向移动；结果始终重新钳位。
 */
export function panFramingByFraction(
  framing: ShapeFramingState,
  dx: number,
  dy: number,
  source: ShapeImageTarget,
  target: ShapeImageTarget,
): ShapeFramingState {
  const rect = framingSourceRect(source, target, framing);
  return clampFraming(
    {
      zoom: framing.zoom,
      centerX: framing.centerX - (dx * rect.sw) / source.width,
      centerY: framing.centerY - (dy * rect.sh) / source.height,
    },
    source,
    target,
  );
}

/**
 * 方向键移动**取景范围**（右箭头看右边）：与拖动方向相反，两个动词各自讲得清。
 * dx/dy 是取景框宽/高的比例。
 */
export function nudgeFraming(
  framing: ShapeFramingState,
  dx: number,
  dy: number,
  source: ShapeImageTarget,
  target: ShapeImageTarget,
): ShapeFramingState {
  return panFramingByFraction(framing, -dx, -dy, source, target);
}

/** 按步长缩放（direction = 1 放大 / -1 缩小），并在 1–3× 端点停住。 */
export function stepFramingZoom(
  framing: ShapeFramingState,
  direction: number,
  source: ShapeImageTarget,
  target: ShapeImageTarget,
): ShapeFramingState {
  return clampFraming(
    { ...framing, zoom: framing.zoom + direction * SHAPE_FRAMING_ZOOM_STEP },
    source,
    target,
  );
}

/** 量原图像素尺寸（选图后先判比例）；不建画布、不裁切。失败按定形失败上报。 */
export async function measureImage(
  file: Blob,
  deps: ShapeImageDeps = browserShapeImageDeps,
): Promise<ShapeImageTarget> {
  if (!isImageFile(file)) {
    throw new ImageShapingError("not_image", "只能处理图片文件（JPG / PNG / WebP / GIF）");
  }
  const decoded = await decodeSource(deps, file);
  return { width: decoded.width, height: decoded.height };
}

/**
 * 定形主入口：居中裁成目标比例 → 降采样到目标像素 → 编码成目标类型。
 * 纯编排：所有浏览器副作用都在注入点里。
 */
export async function shapeImage(
  file: Blob,
  options: ShapeImageOptions,
  deps: ShapeImageDeps = browserShapeImageDeps,
): Promise<Blob> {
  if (!isImageFile(file)) {
    throw new ImageShapingError("not_image", "只能处理图片文件（JPG / PNG / WebP / GIF）");
  }

  // 先校验目标再去解码：参数错了不该浪费一次图片解码，更不该进入上传链路。
  const target = normalizeShapeTarget(options.target);
  const decoded = await decodeSource(deps, file);
  const canvas = deps.createCanvas(target.width, target.height);
  const context = canvas.getContext("2d");
  if (!context) {
    throw new ImageShapingError("canvas_unavailable", "当前环境无法处理图片");
  }

  // 降采样时要高质量平滑，否则缩到 256 会有明显锯齿。
  context.imageSmoothingEnabled = true;
  context.imageSmoothingQuality = "high";

  // 比例合规走自动居中；裁剪窗确认时走同一份换算（选区即 drawImage 的源矩形）。
  const rect = options.framing
    ? framingSourceRect(decoded, target, options.framing)
    : centerCropRect(decoded, target);
  context.drawImage(
    decoded.source,
    rect.sx,
    rect.sy,
    rect.sw,
    rect.sh,
    0,
    0,
    target.width,
    target.height,
  );

  const blob = await encodeCanvas(deps, canvas, options);
  if (!blob) {
    throw new ImageShapingError("encode_failed", "无法生成处理后的图片");
  }
  return blob;
}

/** 定形后可直接上传的产物：新文件 + 定形结果的本地预览地址 + 产物尺寸。 */
export interface ShapedUploadFile {
  /** 定形后的新文件（文件名与输出格式一致）；上传只发它，原图不进入链路。 */
  file: File;
  /** 定形产物的 object URL：上传前就能看到结果；调用方用完负责 revoke。 */
  previewUrl: string;
  /** 产物像素尺寸（封面即 1600×900）。 */
  size: ShapeImageTarget;
}

/**
 * 定形的三个浏览器/环境副作用（测试各自注入替身，因此不需要真实画布或 object URL）。
 * `shape` 默认就是 `shapeImage`：本模块不另写一套裁切数学。
 */
export interface ShapedUploadDeps {
  /** 定形实现：默认 `shapeImage`。 */
  shape: (file: Blob, options: ShapeImageOptions) => Promise<Blob>;
  /** 把定形产物包成可上传的文件（带上与输出格式一致的名字）。 */
  createFile: (blob: Blob, name: string) => File;
  /** 定形产物的本地预览地址；环境不支持时返回空串。 */
  createObjectURL: (blob: Blob) => string;
}

/** 输出类型 → 文件后缀（上传任务按后缀判类型，产物必须带一个对得上格式的名字）。 */
function extensionForOutputType(outputType: ShapeImageOutputType): string {
  if (outputType === "image/jpeg") return "jpg";
  if (outputType === "image/png") return "png";
  return "webp";
}

/** 定形产物的文件名：沿用原图的名字、换成产物后缀；原图没有名字时用 "cover"。 */
function shapedFileName(source: Blob, outputType: ShapeImageOutputType): string {
  const original = typeof (source as File).name === "string" ? (source as File).name : "";
  const withoutExtension = original.replace(/\.\w+$/u, "").trim();
  return `${withoutExtension || "cover"}.${extensionForOutputType(outputType)}`;
}

/**
 * 上传前定形（#200）：把选中的文件定形成目标尺寸/比例，产出**新文件**与它的本地预览。
 *
 * 调用方拿到结果后只上传 `result.file`——原图不进入上传链路（GLOSSARY「定形」）。
 * 预览地址由调用方持有并负责 revoke（切换/移除封面时）。
 */
export async function shapeForUpload(
  file: Blob,
  options: ShapeImageOptions,
  deps: ShapedUploadDeps = browserShapedUploadDeps,
): Promise<ShapedUploadFile> {
  const target = normalizeShapeTarget(options.target);
  // 定形失败直接上抛：不做半成品（既是"没产出图片"，也是"没产出预览"）。
  const shaped = await deps.shape(file, { ...options, target });
  const name = shapedFileName(file, options.outputType);
  return {
    file: deps.createFile(shaped, name),
    previewUrl: deps.createObjectURL(shaped),
    size: target,
  };
}

async function decodeSource(deps: ShapeImageDeps, file: Blob): Promise<ShapeImageSource> {
  try {
    const decoded = await deps.decode(file);
    if (decoded.width <= 0 || decoded.height <= 0) {
      throw new Error("图片尺寸无效");
    }
    return decoded;
  } catch {
    throw new ImageShapingError("decode_failed", "无法读取这张图片，请换一张");
  }
}

async function encodeCanvas(
  deps: ShapeImageDeps,
  canvas: HTMLCanvasElement,
  options: ShapeImageOptions,
): Promise<Blob | null> {
  try {
    return await deps.encode(canvas, options.outputType, options.quality);
  } catch {
    throw new ImageShapingError("encode_failed", "无法生成处理后的图片");
  }
}
/** 用 object URL 喂给 Image，拿回原始尺寸与可绘制的源。 */
function decodeInBrowser(file: Blob): Promise<ShapeImageSource> {
  return new Promise((resolve, reject) => {
    const url = typeof URL.createObjectURL === "function" ? URL.createObjectURL(file) : "";
    if (!url) {
      reject(new Error("当前环境无法读取图片"));
      return;
    }

    const image = new Image();
    image.onload = () => {
      URL.revokeObjectURL(url);
      resolve({ source: image, width: image.naturalWidth, height: image.naturalHeight });
    };
    image.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error("无法解码这张图片"));
    };
    image.src = url;
  });
}

function createBrowserCanvas(width: number, height: number): HTMLCanvasElement {
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  return canvas;
}

function encodeInBrowser(
  canvas: HTMLCanvasElement,
  outputType: ShapeImageOutputType,
  quality?: number,
): Promise<Blob | null> {
  return new Promise((resolve, reject) => {
    try {
      // PNG 不消费 quality（传了也会被忽略），其余格式交给浏览器编码器。
      canvas.toBlob((blob) => resolve(blob), outputType, quality);
    } catch (error) {
      reject(error);
    }
  });
}

/** 浏览器默认实现：只在被调用时才创建 Image / canvas。 */
export const browserShapeImageDeps: ShapeImageDeps = {
  decode: decodeInBrowser,
  createCanvas: createBrowserCanvas,
  encode: encodeInBrowser,
};

/** 定形成可上传文件的默认实现：只在真正调用时才碰 `File` / `URL`。 */
export const browserShapedUploadDeps: ShapedUploadDeps = {
  shape: (file, options) => shapeImage(file, options, browserShapeImageDeps),
  createFile: (blob, name) => new File([blob], name, { type: blob.type }),
  createObjectURL: (blob) => {
    try {
      // 非浏览器环境（SSR / 无 object URL 的宿主）没有预览地址：留空不影响上传。
      return typeof URL.createObjectURL === "function" ? URL.createObjectURL(blob) : "";
    } catch {
      return "";
    }
  },
};

import { describe, expect, it, vi } from "vitest";
import { AVATAR_SHAPE_OPTIONS, shapeAvatar } from "~/utils/avatar";
import {
  COVER_SHAPE_OPTIONS,
  COVER_SHAPE_RATIO_TEXT,
  COVER_SHAPE_SIZE_TEXT,
  COVER_SHAPE_TARGET,
  DEFAULT_SHAPE_FRAMING,
  SHAPE_ASPECT_TOLERANCE,
  SHAPE_FRAMING_ZOOM_MAX,
  SHAPE_FRAMING_ZOOM_MIN,
  aspectRatioDeviation,
  centerCropRect,
  clampFraming,
  framingImageBox,
  framingSourceRect,
  isAspectRatioCompliant,
  measureImage,
  needsFraming,
  nudgeFraming,
  panFramingByFraction,
  shapeForUpload,
  shapeImage,
  stepFramingZoom,
  type ShapeFramingState,
  type ShapeImageDeps,
  type ShapeImageOptions,
  type ShapedUploadDeps,
} from "~/utils/imageShaping";

interface FakeShapeDeps {
  deps: ShapeImageDeps;
  drawImage: ReturnType<typeof vi.fn>;
  createCanvas: ReturnType<typeof vi.fn>;
  encode: ReturnType<typeof vi.fn>;
}

/** 假画布：只记录 drawImage 的 9 个参数，不依赖真实 canvas（ADR-0016）。 */
function createFakeDeps(decoded = { width: 800, height: 400 }): FakeShapeDeps {
  const drawImage = vi.fn();
  const encode = vi.fn(async (_canvas: HTMLCanvasElement, type: string) => new Blob([], { type }));
  const createCanvas = vi.fn((width: number, height: number) => {
    const context = {
      imageSmoothingEnabled: false,
      imageSmoothingQuality: "low",
      drawImage,
    };
    return {
      width,
      height,
      getContext: (kind: string) => (kind === "2d" ? context : null),
    } as unknown as HTMLCanvasElement;
  });

  return {
    deps: {
      decode: async () => ({ source: {} as CanvasImageSource, ...decoded }),
      createCanvas: createCanvas as unknown as ShapeImageDeps["createCanvas"],
      encode: encode as unknown as ShapeImageDeps["encode"],
    },
    drawImage,
    createCanvas,
    encode,
  };
}

const PNG_OPTIONS: ShapeImageOptions = {
  target: { width: 256, height: 256 },
  outputType: "image/png",
};

describe("居中裁切矩形", () => {
  it("把更宽的图左右各裁一半，保持目标比例", () => {
    expect(centerCropRect({ width: 800, height: 400 }, { width: 256, height: 256 })).toEqual({
      sx: 200,
      sy: 0,
      sw: 400,
      sh: 400,
    });
  });

  it("把更高的图上下各裁一半，保持目标比例", () => {
    expect(centerCropRect({ width: 400, height: 800 }, { width: 256, height: 256 })).toEqual({
      sx: 0,
      sy: 200,
      sw: 400,
      sh: 400,
    });
  });

  it("按目标比例而不是固定 1:1 裁切，供封面复用", () => {
    expect(centerCropRect({ width: 1000, height: 1000 }, { width: 800, height: 450 })).toEqual({
      sx: 0,
      sy: 218.75,
      sw: 1000,
      sh: 562.5,
    });
  });
});

describe("图片定形", () => {
  it("居中裁切后按目标尺寸重绘，产出的是新图而不是原图", async () => {
    const fake = createFakeDeps();
    const source = new File(["raw"], "photo.jpg", { type: "image/jpeg" });

    const shaped = await shapeImage(source, PNG_OPTIONS, fake.deps);

    expect(fake.createCanvas).toHaveBeenCalledWith(256, 256);
    expect(fake.drawImage).toHaveBeenCalledWith(
      expect.anything(),
      200,
      0,
      400,
      400,
      0,
      0,
      256,
      256,
    );
    expect(fake.encode).toHaveBeenCalledWith(expect.anything(), "image/png", undefined);
    expect(shaped).not.toBe(source);
    expect(shaped.type).toBe("image/png");
  });

  it("头像定形固定为 1:1、256×256、PNG 输出", async () => {
    const fake = createFakeDeps({ width: 400, height: 400 });
    const source = new File(["raw"], "avatar.webp", { type: "image/webp" });

    const shaped = await shapeAvatar(source, fake.deps);

    expect(AVATAR_SHAPE_OPTIONS.target).toEqual({ width: 256, height: 256 });
    expect(AVATAR_SHAPE_OPTIONS.outputType).toBe("image/png");
    expect(fake.createCanvas).toHaveBeenCalledWith(256, 256);
    expect(fake.encode).toHaveBeenCalledWith(expect.anything(), "image/png", undefined);
    expect(shaped.type).toBe("image/png");
  });
});

describe("封面定形参数：三种封面共用同一份取值（#200）", () => {
  it("封面统一 16:9、1600×900、输出 JPEG，并与全站唯一比例口径一致", () => {
    expect(COVER_SHAPE_TARGET).toEqual({ width: 1600, height: 900 });
    // 独立来源：规格写的是 16:9（不是拿 1600/900 自己验自己，那种断言永远为真）。
    expect(COVER_SHAPE_TARGET.width / COVER_SHAPE_TARGET.height).toBe(16 / 9);
    expect(COVER_SHAPE_OPTIONS.target).toEqual(COVER_SHAPE_TARGET);
    expect(COVER_SHAPE_OPTIONS.outputType).toBe("image/jpeg");
    // 界面文案也从同一份取值派生，模板里不再手写 1600 / 900 / 16:9。
    expect(COVER_SHAPE_RATIO_TEXT).toBe("16:9");
    expect(COVER_SHAPE_SIZE_TEXT).toBe("1600 × 900");
  });
});

describe("shapeForUpload：上传前定形，原图不进上传链路（#200）", () => {
  /** 只替换定形这一步：不碰画布，受控地产出"定形后的新图"。 */
  function fakeDeps(overrides: Partial<ShapedUploadDeps> = {}) {
    const shaped = new Blob(["shaped-bytes"], { type: "image/jpeg" });
    const shape = vi.fn(async () => shaped);
    const createObjectURL = vi.fn(() => "blob:shaped-preview");
    const deps: ShapedUploadDeps = {
      shape: shape as unknown as ShapedUploadDeps["shape"],
      createFile: (blob, name) => new File([blob], name, { type: blob.type }),
      createObjectURL,
      ...overrides,
    };
    return { deps, shape, createObjectURL, shaped };
  }

  it("产出可上传的新文件（同格式后缀）+ 定形产物的预览地址 + 产物尺寸", async () => {
    const { deps, shape, createObjectURL, shaped } = fakeDeps();
    const source = new File(["raw"], "my-cover.png", { type: "image/png" });

    const result = await shapeForUpload(source, COVER_SHAPE_OPTIONS, deps);

    expect(shape).toHaveBeenCalledWith(source, COVER_SHAPE_OPTIONS);
    // 预览必须是定形产物，不能拿原图当预览（否则用户看到的是原图，不是结果）。
    expect(createObjectURL).toHaveBeenCalledWith(shaped);
    expect(result.previewUrl).toBe("blob:shaped-preview");
    expect(result.file).not.toBe(source);
    expect(result.file.name).toBe("my-cover.jpg");
    expect(result.file.type).toBe("image/jpeg");
    expect(result.size).toEqual({ width: 1600, height: 900 });
  });

  it("原图没有文件名时给一个确定的名字，不产生空后缀", async () => {
    const { deps } = fakeDeps();

    const result = await shapeForUpload(new Blob(["raw"], { type: "image/png" }), COVER_SHAPE_OPTIONS, deps);

    expect(result.file.name).toBe("cover.jpg");
  });

  it("定形失败原样上抛，不产出文件与预览", async () => {
    const { deps, createObjectURL } = fakeDeps({
      shape: async () => {
        throw new Error("无法读取这张图片，请换一张");
      },
    });

    await expect(shapeForUpload(new File(["raw"], "cover.png", { type: "image/png" }), COVER_SHAPE_OPTIONS, deps)).rejects.toThrow(
      "无法读取这张图片，请换一张",
    );
    expect(createObjectURL).not.toHaveBeenCalled();
  });

  it("非图片文件在定形阶段就被拒（不建画布、不产出预览）", async () => {
    const fake = createFakeDeps();
    const createObjectURL = vi.fn(() => "blob:never");

    await expect(
      shapeForUpload(new File(["text"], "note.txt", { type: "text/plain" }), COVER_SHAPE_OPTIONS, {
        shape: (file, options) => shapeImage(file, options, fake.deps),
        createFile: (blob, name) => new File([blob], name, { type: blob.type }),
        createObjectURL,
      }),
    ).rejects.toMatchObject({ code: "not_image" });

    expect(fake.createCanvas).not.toHaveBeenCalled();
    expect(createObjectURL).not.toHaveBeenCalled();
  });

  it("环境没有 object URL 时预览留空，文件与尺寸照常返回（不因预览失败丢掉定形结果）", async () => {
    const { deps } = fakeDeps({ createObjectURL: () => "" });

    const result = await shapeForUpload(new File(["raw"], "cover.png", { type: "image/png" }), COVER_SHAPE_OPTIONS, deps);

    expect(result.previewUrl).toBe("");
    expect(result.file.name).toBe("cover.jpg");
  });
});

describe("比例判定：与目标比例相差 ≤1% 才不弹裁剪窗（#215）", () => {
  it("阈值就是 ±1%，是设计常量而不是某一处写死的数字", () => {
    expect(SHAPE_ASPECT_TOLERANCE).toBe(0.01);
  });

  it("16:9 的封面图与 1:1 的头像图在各自目标下都合规", () => {
    // 独立来源：规格写的是 16:9 / 1:1，不是拿 1600/900 自己验自己。
    expect(isAspectRatioCompliant({ width: 1920, height: 1080 }, COVER_SHAPE_TARGET)).toBe(true);
    expect(isAspectRatioCompliant({ width: 400, height: 400 }, { width: 256, height: 256 })).toBe(true);
    expect(needsFraming({ width: 1920, height: 1080 }, COVER_SHAPE_TARGET)).toBe(false);
  });

  it("比例相同但像素尺寸不同不算不合规（比的是比例，不是尺寸）", () => {
    expect(isAspectRatioCompliant({ width: 320, height: 180 }, COVER_SHAPE_TARGET)).toBe(true);
  });

  it("1:1 的图当封面用不合规：需要作者选区域（头像入口里反过来同理）", () => {
    expect(needsFraming({ width: 1000, height: 1000 }, COVER_SHAPE_TARGET)).toBe(true);
    expect(needsFraming({ width: 400, height: 300 }, { width: 256, height: 256 })).toBe(true);
    // 偏差约 43.75%：明显超过 1%，不是边界抖动造成的误判。
    expect(aspectRatioDeviation({ width: 1000, height: 1000 }, COVER_SHAPE_TARGET)).toBeCloseTo(0.4375, 4);
  });

  it("偏差是相对目标比例的归一化值：刚好 1% 以内算合规，超过就不合规", () => {
    // 1800×1000 ≈ 1.8：相对 16:9 偏差约 1.25% → 不合规。
    expect(aspectRatioDeviation({ width: 1800, height: 1000 }, COVER_SHAPE_TARGET)).toBeGreaterThan(0.01);
    expect(needsFraming({ width: 1800, height: 1000 }, COVER_SHAPE_TARGET)).toBe(true);

    // 1778×1000 ≈ 1.778：偏差约 0.01% → 合规（不被 1% 边界误判）。
    expect(aspectRatioDeviation({ width: 1778, height: 1000 }, COVER_SHAPE_TARGET)).toBeLessThan(0.01);
    expect(isAspectRatioCompliant({ width: 1778, height: 1000 }, COVER_SHAPE_TARGET)).toBe(true);
  });

  it("容差可以显式收紧（数组/参数化的调用方不必复制阈值）", () => {
    expect(isAspectRatioCompliant({ width: 1600, height: 901 }, COVER_SHAPE_TARGET)).toBe(true);
    expect(isAspectRatioCompliant({ width: 1600, height: 901 }, COVER_SHAPE_TARGET, 0.0001)).toBe(false);
  });
});

describe("取景状态：缩放钳位与归一化中心（#215）", () => {
  it("缩放被钳在 1–3×：0 与负数回落到 1，超过上限回落到 3", () => {
    expect(SHAPE_FRAMING_ZOOM_MIN).toBe(1);
    expect(SHAPE_FRAMING_ZOOM_MAX).toBe(3);
    expect(clampFraming({ ...DEFAULT_SHAPE_FRAMING, zoom: 0.2 }, COVER_SHAPE_TARGET, COVER_SHAPE_TARGET).zoom).toBe(1);
    expect(clampFraming({ ...DEFAULT_SHAPE_FRAMING, zoom: -4 }, COVER_SHAPE_TARGET, COVER_SHAPE_TARGET).zoom).toBe(1);
    expect(clampFraming({ ...DEFAULT_SHAPE_FRAMING, zoom: 5 }, COVER_SHAPE_TARGET, COVER_SHAPE_TARGET).zoom).toBe(3);
    expect(clampFraming({ ...DEFAULT_SHAPE_FRAMING, zoom: Number.NaN }, COVER_SHAPE_TARGET, COVER_SHAPE_TARGET).zoom).toBe(1);
  });

  it("中心被夹住，取景区域永远不越过源图边界（不会露出底色）", () => {
    const source = { width: 2000, height: 900 };
    const atCenter = clampFraming({ zoom: 1, centerX: 0.5, centerY: 0.5 }, source, COVER_SHAPE_TARGET);
    expect(atCenter).toEqual({ zoom: 1, centerX: 0.5, centerY: 0.5 });

    const farLeft = clampFraming({ zoom: 1, centerX: -0.5, centerY: -0.5 }, source, COVER_SHAPE_TARGET);
    expect(farLeft.centerX).toBeCloseTo(0.4, 6);
    expect(farLeft.centerY).toBeCloseTo(0.5, 6);
    expect(framingSourceRect(source, COVER_SHAPE_TARGET, farLeft)).toEqual({
      sx: 0,
      sy: 0,
      sw: 1600,
      sh: 900,
    });

    const farRight = clampFraming({ zoom: 1, centerX: 2, centerY: 2 }, source, COVER_SHAPE_TARGET);
    expect(framingSourceRect(source, COVER_SHAPE_TARGET, farRight)).toEqual({
      sx: 400,
      sy: 0,
      sw: 1600,
      sh: 900,
    });
  });

  it("默认取景（居中、1×）与自动居中裁切完全同一块区域", () => {
    const sources = [
      { width: 800, height: 400 },
      { width: 400, height: 800 },
      { width: 1000, height: 1000 },
      { width: 3200, height: 1800 },
    ];
    for (const source of sources) {
      expect(framingSourceRect(source, COVER_SHAPE_TARGET)).toEqual(centerCropRect(source, COVER_SHAPE_TARGET));
      expect(framingSourceRect(source, { width: 256, height: 256 })).toEqual(
        centerCropRect(source, { width: 256, height: 256 }),
      );
    }
  });
});

describe("视口→源图矩形换算：拖动、方向键、缩放（#215）", () => {
  const source = { width: 2000, height: 900 };

  it("放大到 2× 时取景框缩到自动裁切区域的一半，仍居中", () => {
    const framing: ShapeFramingState = { zoom: 2, centerX: 0.5, centerY: 0.5 };
    expect(framingSourceRect(source, COVER_SHAPE_TARGET, framing)).toEqual({
      sx: 600,
      sy: 225,
      sw: 800,
      sh: 450,
    });
  });

  it("中心偏移按源图归一化：0.2 的中心把 2× 的取景框贴到左边", () => {
    expect(framingSourceRect(source, COVER_SHAPE_TARGET, { zoom: 2, centerX: 0.2, centerY: 0.5 })).toEqual({
      sx: 0,
      sy: 225,
      sw: 800,
      sh: 450,
    });
  });

  it("拖动画面：向右拖 10% 视口宽，取景区域在源图上左移 160px", () => {
    // 1× 时取景框宽 1600px，等于整个视口宽；10% 视口 = 160 源图像素。
    const moved = panFramingByFraction(DEFAULT_SHAPE_FRAMING, 0.1, 0, source, COVER_SHAPE_TARGET);
    expect(framingSourceRect(source, COVER_SHAPE_TARGET, moved).sx).toBeCloseTo(40, 6);
  });

  it("拖到边界就被钳住：再拖也只会停在源图边缘", () => {
    const dragged = panFramingByFraction(DEFAULT_SHAPE_FRAMING, 0.5, 0, source, COVER_SHAPE_TARGET);
    expect(framingSourceRect(source, COVER_SHAPE_TARGET, dragged)).toEqual({
      sx: 0,
      sy: 0,
      sw: 1600,
      sh: 900,
    });
  });

  it("方向键移的是取景范围：右箭头让视口向右，源图矩形跟着右移", () => {
    const nudged = nudgeFraming(DEFAULT_SHAPE_FRAMING, 0.05, 0, source, COVER_SHAPE_TARGET);
    expect(framingSourceRect(source, COVER_SHAPE_TARGET, nudged).sx).toBeCloseTo(280, 6);

    const up = nudgeFraming({ ...DEFAULT_SHAPE_FRAMING, zoom: 2 }, 0, -0.05, source, COVER_SHAPE_TARGET);
    expect(framingSourceRect(source, COVER_SHAPE_TARGET, up).sy).toBeCloseTo(225 - 22.5, 6);
  });

  it("缩放步进在两端停住：1× 不能再缩小、3× 不能再放大", () => {
    expect(stepFramingZoom(DEFAULT_SHAPE_FRAMING, 1, source, COVER_SHAPE_TARGET).zoom).toBeCloseTo(1.1, 6);
    expect(stepFramingZoom({ zoom: 3, centerX: 0.5, centerY: 0.5 }, 1, source, COVER_SHAPE_TARGET).zoom).toBe(3);
    expect(stepFramingZoom({ zoom: 3, centerX: 0.5, centerY: 0.5 }, -1, source, COVER_SHAPE_TARGET).zoom).toBeCloseTo(2.9, 6);
    expect(stepFramingZoom({ zoom: 1, centerX: 0.5, centerY: 0.5 }, -1, source, COVER_SHAPE_TARGET).zoom).toBe(1);
  });

  it("放大后中心仍然被重新夹住（不会因为缩放把边界留在框外）", () => {
    const shifted = clampFraming({ zoom: 3, centerX: 0, centerY: 1 }, source, COVER_SHAPE_TARGET);
    expect(framingSourceRect(source, COVER_SHAPE_TARGET, shifted)).toEqual({
      sx: 0,
      sy: 600,
      sw: 533.3333333333334,
      sh: 300,
    });
  });
});

describe("裁剪窗预览几何：与烤进文件的源矩形同源（#215）", () => {
  it("图片相对取景框的偏移与放大，由源矩形反推（不是另一套 CSS 算法）", () => {
    const box = framingImageBox({ width: 800, height: 400 }, { width: 256, height: 256 });
    const rect = framingSourceRect({ width: 800, height: 400 }, { width: 256, height: 256 });

    expect(box).toEqual({ left: -0.5, top: 0, width: 2, height: 1 });
    // 预览把源矩形的左上角对到框的左上角：四条数与 drawImage 用的是同一组。
    expect(box.left).toBeCloseTo(-rect.sx / rect.sw, 6);
    expect(box.top).toBeCloseTo(-rect.sy / rect.sh, 6);
    expect(box.width).toBeCloseTo(800 / rect.sw, 6);
    expect(box.height).toBeCloseTo(400 / rect.sh, 6);
  });

  it("放大与平移同样落到预览几何上", () => {
    const source = { width: 2000, height: 900 };
    const framing: ShapeFramingState = { zoom: 2, centerX: 0.2, centerY: 0.5 };
    const box = framingImageBox(source, COVER_SHAPE_TARGET, framing);
    const rect = framingSourceRect(source, COVER_SHAPE_TARGET, framing);

    expect(box).toEqual({ left: 0, top: -0.5, width: 2.5, height: 2 });
    expect(box.top).toBeCloseTo(-rect.sy / rect.sh, 6);
  });
});

describe("确认裁剪：按选定区域定形成新文件（#215）", () => {
  it("定形用的是取景矩形（确认的选区），不是自动居中，并沿用目标尺寸", async () => {
    const fake = createFakeDeps({ width: 2000, height: 900 });
    const source = new File(["raw"], "wide.jpg", { type: "image/jpeg" });
    const framing: ShapeFramingState = { zoom: 2, centerX: 0.2, centerY: 0.5 };

    const shaped = await shapeImage(source, { ...COVER_SHAPE_OPTIONS, framing }, fake.deps);

    expect(fake.createCanvas).toHaveBeenCalledWith(1600, 900);
    expect(fake.drawImage).toHaveBeenCalledWith(expect.anything(), 0, 225, 800, 450, 0, 0, 1600, 900);
    expect(fake.encode).toHaveBeenCalledWith(expect.anything(), "image/jpeg", 0.9);
    expect(shaped).not.toBe(source);
  });

  it("确认后的产物仍是新文件、预览是产物、原图不上传", async () => {
    const shaped = new Blob(["shaped-bytes"], { type: "image/jpeg" });
    const shape = vi.fn(async () => shaped);
    const deps: ShapedUploadDeps = {
      shape: shape as unknown as ShapedUploadDeps["shape"],
      createFile: (blob, name) => new File([blob], name, { type: blob.type }),
      createObjectURL: vi.fn(() => "blob:framed-preview"),
    };
    const framing: ShapeFramingState = { zoom: 2, centerX: 0.5, centerY: 0.5 };

    const result = await shapeForUpload(
      new File(["raw"], "photo.png", { type: "image/png" }),
      { ...COVER_SHAPE_OPTIONS, framing },
      deps,
    );

    expect(shape).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ target: { width: 1600, height: 900 }, framing }),
    );
    expect(result.previewUrl).toBe("blob:framed-preview");
    expect(result.file.name).toBe("photo.jpg");
    expect(result.size).toEqual({ width: 1600, height: 900 });
  });
});

describe("量原图尺寸（选图后先判比例）", () => {
  it("返回解码出的像素尺寸，不裁切、不建画布", async () => {
    const fake = createFakeDeps({ width: 1778, height: 1000 });

    const size = await measureImage(new File(["raw"], "photo.jpg", { type: "image/jpeg" }), fake.deps);

    expect(size).toEqual({ width: 1778, height: 1000 });
    expect(fake.createCanvas).not.toHaveBeenCalled();
    expect(fake.encode).not.toHaveBeenCalled();
  });

  it("非图片在解码前就被拒；解码失败按定形失败上报", async () => {
    const fake = createFakeDeps();

    await expect(
      measureImage(new File(["text"], "note.txt", { type: "text/plain" }), fake.deps),
    ).rejects.toMatchObject({ code: "not_image" });

    fake.deps.decode = async () => {
      throw new Error("坏文件");
    };
    await expect(measureImage(new File(["raw"], "photo.jpg", { type: "image/jpeg" }), fake.deps)).rejects.toMatchObject({
      code: "decode_failed",
    });
  });
});

describe("定形失败路径", () => {
  const IMAGE_FILE = new File(["raw"], "photo.jpg", { type: "image/jpeg" });

  it("非图片文件在解码前就被拒绝，不产生任何画布或上传材料", async () => {
    const fake = createFakeDeps();
    const decode = vi.fn(fake.deps.decode);
    fake.deps.decode = decode;

    await expect(
      shapeImage(new File(["text"], "note.txt", { type: "text/plain" }), PNG_OPTIONS, fake.deps),
    ).rejects.toMatchObject({ code: "not_image", message: expect.stringContaining("图片") });

    expect(decode).not.toHaveBeenCalled();
    expect(fake.createCanvas).not.toHaveBeenCalled();
    expect(fake.encode).not.toHaveBeenCalled();
  });

  it("解码失败上报为定形失败，且不建画布", async () => {
    const fake = createFakeDeps();
    fake.deps.decode = async () => {
      throw new Error("坏文件");
    };

    await expect(shapeImage(IMAGE_FILE, PNG_OPTIONS, fake.deps)).rejects.toMatchObject({
      code: "decode_failed",
    });
    expect(fake.createCanvas).not.toHaveBeenCalled();
  });

  it("编码拿不到结果时上报为定形失败", async () => {
    const fake = createFakeDeps();
    fake.deps.encode = async () => null;

    await expect(shapeImage(IMAGE_FILE, PNG_OPTIONS, fake.deps)).rejects.toMatchObject({
      code: "encode_failed",
    });
  });

  it("目标尺寸非法时直接拒绝，不进入解码", async () => {
    const fake = createFakeDeps();
    const decode = vi.fn(fake.deps.decode);
    fake.deps.decode = decode;

    await expect(
      shapeImage(IMAGE_FILE, { target: { width: 0, height: 256 }, outputType: "image/png" }, fake.deps),
    ).rejects.toMatchObject({ code: "invalid_target" });
    expect(decode).not.toHaveBeenCalled();
  });

  it("画布拿不到 2D 上下文时报错", async () => {
    const fake = createFakeDeps();
    fake.deps.createCanvas = () => ({ getContext: () => null }) as unknown as HTMLCanvasElement;

    await expect(shapeImage(IMAGE_FILE, PNG_OPTIONS, fake.deps)).rejects.toMatchObject({
      code: "canvas_unavailable",
    });
  });
});

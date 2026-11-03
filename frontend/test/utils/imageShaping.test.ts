import { describe, expect, it, vi } from "vitest";
import { AVATAR_SHAPE_OPTIONS, shapeAvatar } from "~/utils/avatar";
import { COVER_ASPECT } from "~/utils/coverFraming";
import {
  COVER_SHAPE_OPTIONS,
  COVER_SHAPE_RATIO_TEXT,
  COVER_SHAPE_SIZE_TEXT,
  COVER_SHAPE_TARGET,
  centerCropRect,
  shapeForUpload,
  shapeImage,
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
    // 交给独立来源（coverFraming 的 16/9）验证：常量写错比例时这里会红，
    // 而不是拿 1600/900 自己验自己（那种断言永远为真）。
    expect(COVER_SHAPE_TARGET.width / COVER_SHAPE_TARGET.height).toBe(COVER_ASPECT);
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

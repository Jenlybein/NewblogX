import { describe, expect, it, vi } from "vitest";
import { AVATAR_SHAPE_OPTIONS, shapeAvatar } from "~/utils/avatar";
import {
  centerCropRect,
  shapeImage,
  type ShapeImageDeps,
  type ShapeImageOptions,
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

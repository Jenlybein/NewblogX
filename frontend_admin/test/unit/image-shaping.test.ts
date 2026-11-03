import { describe, expect, it, vi } from 'vitest'

import {
  COVER_SHAPE_OPTIONS,
  COVER_SHAPE_RATIO_TEXT,
  COVER_SHAPE_SIZE_TEXT,
  COVER_SHAPE_TARGET,
  browserShapeImageDeps,
  browserShapedUploadDeps,
  centerCropRect,
  isImageFile,
  shapeForUpload,
  shapeImage,
  type ShapeImageDeps,
  type ShapeImageOptions,
  type ShapedUploadDeps,
} from '~/utils/imageShaping'
import { BANNER_ASPECT } from '~/utils/framing'

/**
 * 后台这一份定形工具（票 200）。
 *
 * 为什么后台还要自己测一遍：`frontend_admin` 必须自洽（#176 的教训——相对路径 import
 * 读者站模块会让后台生产构建失败），所以后台拿的是**自己仓库里的那份逐字副本**。
 * 字节一致由 scripts/check-image-shaping-parity.mjs 守住，但"这份副本真的能定形"
 * 只能靠后台自己的用例证明；顺带它也是后台覆盖率门槛的分母（utils 门槛 95%）。
 *
 * 画布 / 图片解码 / object URL 全部可注入（ADR-0016：测试环境没有 canvas 实现），
 * 这里连浏览器默认实现也一并覆盖：那些函数只在真正调用时才碰 `Image` / `canvas`。
 */

interface FakeShapeDeps {
  deps: ShapeImageDeps
  drawImage: ReturnType<typeof vi.fn>
  createCanvas: ReturnType<typeof vi.fn>
  encode: ReturnType<typeof vi.fn>
}

/** 假画布：只记录 drawImage 的 9 个参数，不依赖真实 canvas。 */
function createFakeDeps(decoded = { width: 800, height: 400 }): FakeShapeDeps {
  const drawImage = vi.fn()
  const encode = vi.fn(async (_canvas: HTMLCanvasElement, type: string) => new Blob([], { type }))
  const createCanvas = vi.fn((width: number, height: number) => {
    const context = {
      imageSmoothingEnabled: false,
      imageSmoothingQuality: 'low',
      drawImage,
    }
    return {
      width,
      height,
      getContext: (kind: string) => (kind === '2d' ? context : null),
    } as unknown as HTMLCanvasElement
  })

  return {
    deps: {
      decode: async () => ({ source: {} as CanvasImageSource, ...decoded }),
      createCanvas: createCanvas as unknown as ShapeImageDeps['createCanvas'],
      encode: encode as unknown as ShapeImageDeps['encode'],
    },
    drawImage,
    createCanvas,
    encode,
  }
}

const PNG_OPTIONS: ShapeImageOptions = {
  target: { width: 256, height: 256 },
  outputType: 'image/png',
}

const IMAGE_FILE = new File(['raw'], 'photo.jpg', { type: 'image/jpeg' })

describe('封面定形参数：三种封面共用同一份取值（票 200）', () => {
  it('封面统一 16:9、1600×900、输出 JPEG，并与后台自己的 16:9 口径一致', () => {
    expect(COVER_SHAPE_TARGET).toEqual({ width: 1600, height: 900 })
    // 交给独立来源（framing 的 BANNER_ASPECT = 16/9）验证比例：常量写错时这里会红，
    // 而不是拿 1600/900 自己验自己。
    expect(COVER_SHAPE_TARGET.width / COVER_SHAPE_TARGET.height).toBe(BANNER_ASPECT)
    expect(COVER_SHAPE_OPTIONS.target).toEqual(COVER_SHAPE_TARGET)
    expect(COVER_SHAPE_OPTIONS.outputType).toBe('image/jpeg')
    // 界面文案从同一份取值派生，模板里不再手写 1600 / 900 / 16:9。
    expect(COVER_SHAPE_RATIO_TEXT).toBe('16:9')
    expect(COVER_SHAPE_SIZE_TEXT).toBe('1600 × 900')
  })

  it('isImageFile 只看 MIME 前缀（后缀可伪造）', () => {
    expect(isImageFile(new File(['x'], 'fake.txt', { type: 'image/png' }))).toBe(true)
    expect(isImageFile(new File(['x'], 'fake.png', { type: 'text/plain' }))).toBe(false)
  })
})

describe('居中裁切矩形', () => {
  it('把更宽的图左右各裁一半，保持目标比例', () => {
    expect(centerCropRect({ width: 800, height: 400 }, { width: 256, height: 256 })).toEqual({
      sx: 200,
      sy: 0,
      sw: 400,
      sh: 400,
    })
  })

  it('把更高的图上下各裁一半，保持目标比例', () => {
    expect(centerCropRect({ width: 400, height: 800 }, { width: 256, height: 256 })).toEqual({
      sx: 0,
      sy: 200,
      sw: 400,
      sh: 400,
    })
  })

  it('按目标比例而不是固定 1:1 裁切，供封面复用', () => {
    expect(centerCropRect({ width: 1000, height: 1000 }, { width: 800, height: 450 })).toEqual({
      sx: 0,
      sy: 218.75,
      sw: 1000,
      sh: 562.5,
    })
  })
})

describe('图片定形', () => {
  it('居中裁切后按目标尺寸重绘，产出的是新图而不是原图', async () => {
    const fake = createFakeDeps({ width: 3200, height: 1800 })

    const shaped = await shapeImage(IMAGE_FILE, COVER_SHAPE_OPTIONS, fake.deps)

    expect(fake.createCanvas).toHaveBeenCalledWith(1600, 900)
    expect(fake.drawImage).toHaveBeenCalledWith(expect.anything(), 0, 0, 3200, 1800, 0, 0, 1600, 900)
    expect(fake.encode).toHaveBeenCalledWith(expect.anything(), 'image/jpeg', 0.9)
    expect(shaped).not.toBe(IMAGE_FILE)
    expect(shaped.type).toBe('image/jpeg')
  })
})

describe('定形失败路径', () => {
  it('非图片文件在解码前就被拒绝，不产生任何画布或上传材料', async () => {
    const fake = createFakeDeps()
    const decode = vi.fn(fake.deps.decode)
    fake.deps.decode = decode

    await expect(
      shapeImage(new File(['text'], 'note.txt', { type: 'text/plain' }), PNG_OPTIONS, fake.deps),
    ).rejects.toMatchObject({ code: 'not_image', message: expect.stringContaining('图片') })

    expect(decode).not.toHaveBeenCalled()
    expect(fake.createCanvas).not.toHaveBeenCalled()
    expect(fake.encode).not.toHaveBeenCalled()
  })

  it('解码失败上报为定形失败，且不建画布', async () => {
    const fake = createFakeDeps()
    fake.deps.decode = async () => {
      throw new Error('坏文件')
    }

    await expect(shapeImage(IMAGE_FILE, PNG_OPTIONS, fake.deps)).rejects.toMatchObject({
      code: 'decode_failed',
    })
    expect(fake.createCanvas).not.toHaveBeenCalled()
  })

  it('解码出来的尺寸为 0 也按解码失败处理', async () => {
    const fake = createFakeDeps({ width: 0, height: 0 })

    await expect(shapeImage(IMAGE_FILE, PNG_OPTIONS, fake.deps)).rejects.toMatchObject({
      code: 'decode_failed',
    })
  })

  it('编码拿不到结果时上报为定形失败', async () => {
    const fake = createFakeDeps()
    fake.deps.encode = async () => null

    await expect(shapeImage(IMAGE_FILE, PNG_OPTIONS, fake.deps)).rejects.toMatchObject({
      code: 'encode_failed',
    })
  })

  it('编码抛异常时也上报为定形失败（不把异常原样漏给页面）', async () => {
    const fake = createFakeDeps()
    fake.deps.encode = async () => {
      throw new Error('编码器炸了')
    }

    await expect(shapeImage(IMAGE_FILE, PNG_OPTIONS, fake.deps)).rejects.toMatchObject({
      code: 'encode_failed',
    })
  })

  it('目标尺寸非法时直接拒绝，不进入解码', async () => {
    const fake = createFakeDeps()
    const decode = vi.fn(fake.deps.decode)
    fake.deps.decode = decode

    await expect(
      shapeImage(IMAGE_FILE, { target: { width: 0, height: 256 }, outputType: 'image/png' }, fake.deps),
    ).rejects.toMatchObject({ code: 'invalid_target' })
    expect(decode).not.toHaveBeenCalled()
  })

  it('画布拿不到 2D 上下文时报错', async () => {
    const fake = createFakeDeps()
    fake.deps.createCanvas = () => ({ getContext: () => null }) as unknown as HTMLCanvasElement

    await expect(shapeImage(IMAGE_FILE, PNG_OPTIONS, fake.deps)).rejects.toMatchObject({
      code: 'canvas_unavailable',
    })
  })
})

describe('shapeForUpload：上传前定形，原图不进上传链路（票 200）', () => {
  /** 只替换定形这一步：不碰画布，受控地产出"定形后的新图"。 */
  function fakeDeps(overrides: Partial<ShapedUploadDeps> = {}) {
    const shaped = new Blob(['shaped-bytes'], { type: 'image/jpeg' })
    const shape = vi.fn(async () => shaped)
    const createObjectURL = vi.fn(() => 'blob:shaped-preview')
    const deps: ShapedUploadDeps = {
      shape: shape as unknown as ShapedUploadDeps['shape'],
      createFile: (blob, name) => new File([blob], name, { type: blob.type }),
      createObjectURL,
      ...overrides,
    }
    return { deps, shape, createObjectURL, shaped }
  }

  it('产出可上传的新文件（同格式后缀）+ 定形产物的预览地址 + 产物尺寸', async () => {
    const { deps, shape, createObjectURL, shaped } = fakeDeps()
    const source = new File(['raw'], 'my-cover.png', { type: 'image/png' })

    const result = await shapeForUpload(source, COVER_SHAPE_OPTIONS, deps)

    expect(shape).toHaveBeenCalledWith(source, COVER_SHAPE_OPTIONS)
    // 预览必须是定形产物，不能拿原图当预览（否则运营看到的是原图，不是结果）。
    expect(createObjectURL).toHaveBeenCalledWith(shaped)
    expect(result.file).not.toBe(source)
    expect(result.file.name).toBe('my-cover.jpg')
    expect(result.size).toEqual({ width: 1600, height: 900 })
  })

  it('原图没有文件名时给一个确定的名字，不产生空后缀', async () => {
    const { deps } = fakeDeps()

    const result = await shapeForUpload(new Blob(['raw'], { type: 'image/png' }), COVER_SHAPE_OPTIONS, deps)

    expect(result.file.name).toBe('cover.jpg')
  })

  it('定形失败原样上抛，不产出文件与预览', async () => {
    const { deps, createObjectURL } = fakeDeps({
      shape: async () => {
        throw new Error('无法读取这张图片，请换一张')
      },
    })

    await expect(
      shapeForUpload(new File(['raw'], 'cover.png', { type: 'image/png' }), COVER_SHAPE_OPTIONS, deps),
    ).rejects.toThrow('无法读取这张图片，请换一张')
    expect(createObjectURL).not.toHaveBeenCalled()
  })

  it('非图片文件在定形阶段就被拒（不建画布、不产出预览）', async () => {
    const fake = createFakeDeps()
    const createObjectURL = vi.fn(() => 'blob:never')

    await expect(
      shapeForUpload(new File(['text'], 'note.txt', { type: 'text/plain' }), COVER_SHAPE_OPTIONS, {
        shape: (file, options) => shapeImage(file, options, fake.deps),
        createFile: (blob, name) => new File([blob], name, { type: blob.type }),
        createObjectURL,
      }),
    ).rejects.toMatchObject({ code: 'not_image' })

    expect(fake.createCanvas).not.toHaveBeenCalled()
    expect(createObjectURL).not.toHaveBeenCalled()
  })

  it('环境没有 object URL 时预览留空，文件与尺寸照常返回', async () => {
    const { deps } = fakeDeps({ createObjectURL: () => '' })

    const result = await shapeForUpload(
      new File(['raw'], 'cover.png', { type: 'image/png' }),
      COVER_SHAPE_OPTIONS,
      deps,
    )

    expect(result.previewUrl).toBe('')
    expect(result.file.name).toBe('cover.jpg')
  })

  it('默认实现直接可用：包文件与生成预览都走浏览器那份 deps', async () => {
    const shaped = new Blob(['shaped-bytes'], { type: 'image/jpeg' })
    const deps: ShapedUploadDeps = {
      ...browserShapedUploadDeps,
      shape: async () => shaped,
    }

    const result = await shapeForUpload(new File(['raw'], 'cover.png', { type: 'image/png' }), COVER_SHAPE_OPTIONS, deps)

    // happy-dom 有 object URL 时拿到的就是真的 blob 地址；没有时为空串——两种都算成功。
    expect(typeof result.previewUrl).toBe('string')
    expect(result.file.name).toBe('cover.jpg')
    expect(result.file.type).toBe('image/jpeg')
  })
})

describe('浏览器默认实现（只在被调用时才碰 Image / canvas）', () => {
  it('createCanvas 按目标尺寸建画布', () => {
    const canvas = browserShapeImageDeps.createCanvas(1600, 900)

    expect(canvas.width).toBe(1600)
    expect(canvas.height).toBe(900)
  })

  it('encode 走 canvas.toBlob；toBlob 抛异常时把失败转成 rejection', async () => {
    const toBlob = vi.fn((callback: (blob: Blob | null) => void) => callback(new Blob([], { type: 'image/png' })))
    const okCanvas = { toBlob } as unknown as HTMLCanvasElement

    await expect(browserShapeImageDeps.encode(okCanvas, 'image/png', 0.9)).resolves.toBeInstanceOf(Blob)

    const brokenCanvas = {
      toBlob: () => {
        throw new Error('画布已失效')
      },
    } as unknown as HTMLCanvasElement
    await expect(browserShapeImageDeps.encode(brokenCanvas, 'image/png')).rejects.toThrow('画布已失效')
  })

  it('decode 用 object URL 喂 Image：加载成功读自然宽高，失败与无 URL 都 reject', async () => {
    class FakeImage {
      naturalWidth = 3200
      naturalHeight = 1800
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      private source = ''
      set src(value: string) {
        this.source = value
        if (value) this.onload?.()
        else this.onerror?.()
      }
      get src() {
        return this.source
      }
    }
    vi.stubGlobal('Image', FakeImage)

    await expect(browserShapeImageDeps.decode(new Blob(['raw'], { type: 'image/png' }))).resolves.toEqual({
      source: expect.anything(),
      width: 3200,
      height: 1800,
    })

    // 没有 object URL 时不进入 Image：直接 reject（"当前环境无法读取图片"）。
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('')
    await expect(browserShapeImageDeps.decode(new Blob(['raw']))).rejects.toThrow('当前环境无法读取图片')
    createObjectURL.mockRestore()

    vi.unstubAllGlobals()
  })
})

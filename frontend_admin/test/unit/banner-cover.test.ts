import { describe, expect, it, vi } from 'vitest'

import {
  buildBannerCreatePayload,
  readImageSize,
  requiresFraming,
  uploadBannerCover,
} from '~/utils/bannerCover'
import type { ImageSizeProbe } from '~/utils/bannerCover'

/**
 * 后台上传即取景的编排单元测试（#49）。
 *
 * 断言的是**可观察的决定与副作用顺序**：合规/不合规怎么判、创建请求里带了什么、
 * 上传失败时会不会继续创建。页面交给 Vue，这里只锁业务规则。
 */

describe('requiresFraming', () => {
  it('16:9 与 ±1% 边界直接上传，不弹取景窗', () => {
    expect(requiresFraming({ width: 1920, height: 1080 })).toBe(false)
    expect(requiresFraming({ width: 1280, height: 720 })).toBe(false)
    // 1616×900 恰好 +1%，仍然是合规的一侧。
    expect(requiresFraming({ width: 1616, height: 900 })).toBe(false)
  })

  it('比例差超过 1% 必须先取景（含竖图与 3:2 横图）', () => {
    expect(requiresFraming({ width: 1617, height: 900 })).toBe(true)
    expect(requiresFraming({ width: 1200, height: 800 })).toBe(true)
    expect(requiresFraming({ width: 1080, height: 1920 })).toBe(true)
  })

  it('尺寸非法时按"需要取景"处理（这种图本来就没法直接上传）', () => {
    expect(requiresFraming({ width: 0, height: 0 })).toBe(true)
    expect(requiresFraming({ width: Number.NaN, height: 1080 })).toBe(true)
  })
})

describe('buildBannerCreatePayload', () => {
  it('没有取景记录时带缺省取景（居中、缩放 1），不是省略 crop', () => {
    const payload = buildBannerCreatePayload({
      coverImageId: '42',
      href: 'https://example.com',
      show: true,
      framing: null,
    })

    expect(payload).toEqual({
      cover_image_id: '42',
      href: 'https://example.com',
      show: true,
      crop: { focus_x: 0.5, focus_y: 0.5, zoom: 1 },
    })
  })

  it('取景参数先钳进合法范围再落进请求体', () => {
    const payload = buildBannerCreatePayload({
      coverImageId: '42',
      href: '',
      show: false,
      framing: { focus_x: -0.2, focus_y: 1.4, zoom: 2.4 },
    })

    expect(payload.crop).toEqual({ focus_x: 0, focus_y: 1, zoom: 2 })
  })

  it('href / show / 图片 ID 原样透传（创建轮播不只提交取景）', () => {
    const payload = buildBannerCreatePayload({
      coverImageId: '99',
      href: '',
      show: false,
      framing: { focus_x: 0.25, focus_y: 0.75, zoom: 1.5 },
    })

    expect(payload.cover_image_id).toBe('99')
    expect(payload.href).toBe('')
    expect(payload.show).toBe(false)
    expect(payload.crop).toEqual({ focus_x: 0.25, focus_y: 0.75, zoom: 1.5 })
  })
})

describe('uploadBannerCover', () => {
  const file = { name: 'cover.png', type: 'image/png' } as File

  it('上传成功后把图片 ID 与地址收成确定的两个字段', async () => {
    const uploader = vi.fn(async () => ({ image_id: '7', url: 'https://cdn.example.com/cover.png' }))

    const uploaded = await uploadBannerCover(file, uploader)

    expect(uploaded).toEqual({ imageId: '7', url: 'https://cdn.example.com/cover.png' })
    expect(uploader).toHaveBeenCalledWith(file)
  })

  it('缺少图片 ID 或地址就失败——绝不创建一条没有封面的轮播', async () => {
    await expect(uploadBannerCover(file, async () => ({ url: 'https://cdn.example.com/cover.png' }))).rejects.toThrow(
      '上传成功但缺少图片标识',
    )
    await expect(uploadBannerCover(file, async () => ({ image_id: '7', url: '' }))).rejects.toThrow(
      '上传成功但缺少图片标识',
    )
  })

  it('上传失败原样上抛（页面据此提示并保持未创建状态）', async () => {
    const uploader = vi.fn(async () => {
      throw new Error('网络中断')
    })

    await expect(uploadBannerCover(file, uploader)).rejects.toThrow('网络中断')
  })
})

interface ProbeStub extends ImageSizeProbe {
  requestedUrl: string
}

/**
 * 假的 `Image`：不真的发请求，只按脚本触发 load / error。
 *
 * 这不是在替浏览器实现图片解码，而是把"读到了 / 读到 0×0 / 加载失败"三种
 * 真实可能发生的结果喂给 `readImageSize`，看它怎么收敛。
 */
function stubProbe(width: number, height: number, outcome: 'load' | 'error'): ProbeStub {
  const listeners: { load: Array<() => void>; error: Array<() => void> } = { load: [], error: [] }
  const probe: ProbeStub = {
    naturalWidth: width,
    naturalHeight: height,
    requestedUrl: '',
    addEventListener(type, listener) {
      listeners[type].push(listener)
    },
    get src() {
      return probe.requestedUrl
    },
    set src(value: string) {
      probe.requestedUrl = value
      const target = outcome === 'error' ? listeners.error : listeners.load
      for (const listener of target) listener()
    },
  }
  return probe
}

describe('readImageSize', () => {
  it('在浏览器本地读到原始像素尺寸后才继续（不先上传）', async () => {
    const probe = stubProbe(3200, 1800, 'load')

    await expect(readImageSize('blob:cover', () => probe)).resolves.toEqual({ width: 3200, height: 1800 })
    expect(probe.requestedUrl).toBe('blob:cover')
  })

  it('加载成功但宽高为 0 时返回 null（不能拿 0×0 去判比例）', async () => {
    await expect(readImageSize('blob:broken', () => stubProbe(0, 0, 'load'))).resolves.toBeNull()
    await expect(readImageSize('blob:flat', () => stubProbe(3200, 0, 'load'))).resolves.toBeNull()
  })

  it('宽高不是有穷数时返回 null', async () => {
    await expect(readImageSize('blob:nan', () => stubProbe(Number.NaN, 1080, 'load'))).resolves.toBeNull()
    await expect(
      readImageSize('blob:infinite', () => stubProbe(1920, Number.POSITIVE_INFINITY, 'load')),
    ).resolves.toBeNull()
  })

  it('图片加载失败返回 null（页面据此报错并中止本次上传）', async () => {
    await expect(readImageSize('blob:damaged', () => stubProbe(0, 0, 'error'))).resolves.toBeNull()
  })
})

import { describe, expect, it, vi } from 'vitest'

import {
  buildBannerCreatePayload,
  describeBannerCoverSize,
  readImageSize,
  uploadBannerCover,
} from '~/utils/bannerCover'
import type { ImageSizeProbe } from '~/utils/bannerCover'

/**
 * 后台上传前定形的编排单元测试（#49；#200 起上传前先定形；#204 取景退场）。
 *
 * 断言的是**可观察的决定与副作用顺序**：创建请求里带了什么、上传失败时会不会继续
 * 创建、封面尺寸读数怎么收敛。取景窗与 `crop` 载荷已在 #204 全量退场：
 * 选中的图先在客户端定形成 16:9 再上传，请求体里不再有任何取景参数。
 */

describe('buildBannerCreatePayload', () => {
  it('请求体只带封面 ID / 标题 / 跳转地址 / 展示开关，没有 crop 字段', () => {
    const payload = buildBannerCreatePayload({
      coverImageId: '42',
      title: '春季上新',
      href: 'https://example.com',
      show: true,
    })

    expect(payload).toEqual({
      cover_image_id: '42',
      title: '春季上新',
      href: 'https://example.com',
      show: true,
    })
    expect('crop' in payload).toBe(false)
  })

  it('缺省标题收敛成空串（创建路径标题可空，但字段不能缺）', () => {
    const payload = buildBannerCreatePayload({
      coverImageId: '42',
      href: '',
      show: false,
    })

    expect(payload.title).toBe('')
  })

  it('纯空白标题收敛成空串（不落一条只有空格的标题）', () => {
    const payload = buildBannerCreatePayload({
      coverImageId: '42',
      title: '   ',
      href: '',
      show: false,
    })

    expect(payload.title).toBe('')
  })

  it('href / show / 图片 ID 原样透传', () => {
    const payload = buildBannerCreatePayload({
      coverImageId: '99',
      title: ' 标题 ',
      href: 'https://example.com/a',
      show: false,
    })

    expect(payload.cover_image_id).toBe('99')
    expect(payload.title).toBe('标题')
    expect(payload.href).toBe('https://example.com/a')
    expect(payload.show).toBe(false)
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
  it('在浏览器本地读到像素尺寸后才继续（不先上传）', async () => {
    const probe = stubProbe(3200, 1800, 'load')

    await expect(readImageSize('blob:cover', () => probe)).resolves.toEqual({ width: 3200, height: 1800 })
    expect(probe.requestedUrl).toBe('blob:cover')
  })

  it('加载成功但宽高为 0 时返回 null（不能拿 0×0 冒充尺寸）', async () => {
    await expect(readImageSize('blob:broken', () => stubProbe(0, 0, 'load'))).resolves.toBeNull()
    await expect(readImageSize('blob:flat', () => stubProbe(3200, 0, 'load'))).resolves.toBeNull()
  })

  it('宽高不是有穷数时返回 null', async () => {
    await expect(readImageSize('blob:nan', () => stubProbe(Number.NaN, 1080, 'load'))).resolves.toBeNull()
    await expect(
      readImageSize('blob:infinite', () => stubProbe(1920, Number.POSITIVE_INFINITY, 'load')),
    ).resolves.toBeNull()
  })

  it('图片加载失败返回 null（列表显示"尺寸未知"）', async () => {
    await expect(readImageSize('blob:damaged', () => stubProbe(0, 0, 'error'))).resolves.toBeNull()
  })
})

describe('describeBannerCoverSize', () => {
  it('尺寸未知时返回 null（列表显示占位，不冒充 0×0）', () => {
    expect(describeBannerCoverSize(null)).toBeNull()
    expect(describeBannerCoverSize(undefined)).toBeNull()
  })

  it('宽高非法时返回 null', () => {
    expect(describeBannerCoverSize({ width: Number.NaN, height: 1080 })).toBeNull()
    expect(describeBannerCoverSize({ width: 1920, height: Number.POSITIVE_INFINITY })).toBeNull()
    expect(describeBannerCoverSize({ width: 0, height: 0 })).toBeNull()
    expect(describeBannerCoverSize({ width: 100, height: 0 })).toBeNull()
  })

  it('只回传"宽 × 高"（#204 起不再判比例合规）', () => {
    expect(describeBannerCoverSize({ width: 1600, height: 900 })).toEqual({
      width: 1600,
      height: 900,
      label: '1600 × 900',
    })
    expect('compliant' in describeBannerCoverSize({ width: 1200, height: 800 })!).toBe(false)
  })
})

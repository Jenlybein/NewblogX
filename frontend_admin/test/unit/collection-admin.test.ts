import { describe, expect, it, vi } from 'vitest'

import {
  DESCRIPTION_MAX,
  TITLE_MAX,
  applyDisplayResult,
  buildCollectionCreatePayload,
  buildCollectionUpdatePayload,
  coverUploadGate,
  resolveCollectionCover,
  selectCollectionCover,
  validateCollectionForm,
} from '~/utils/collectionAdmin'
import type { CollectionAdminCard } from '~/types/api'

/**
 * 后台合集管理页的编排单元测试。
 *
 * 断言的是**可观察的决定**：表单什么算合法、请求体里带了什么、16:9 闸门
 * 在什么情况下直接上传 / 什么时候先取景、封面回退到哪一张、展示开关如何
 * 落到列表。页面交给组件测试，这里只锁规则。
 */

function card(overrides: Partial<CollectionAdminCard> = {}): CollectionAdminCard {
  return {
    id: '1',
    title: '主题合集',
    description: '',
    cover: '',
    fallback_cover: '',
    displayed: true,
    article_count: 0,
    sort_order: 1,
    ...overrides,
  }
}

describe('validateCollectionForm', () => {
  it('标题去掉首尾空白后不能为空', () => {
    const errors = validateCollectionForm({ title: '   ', description: '', cover: '' })
    expect(errors.title).toBe('合集标题不能为空')
    expect(validateCollectionForm({ title: '  Nuxt 部署实战  ', description: '', cover: '' }).title).toBeNull()
  })

  it('标题按字符数（不是字节数）卡上限', () => {
    expect(validateCollectionForm({ title: '合'.repeat(TITLE_MAX), description: '', cover: '' }).title).toBeNull()
    expect(validateCollectionForm({ title: '合'.repeat(TITLE_MAX + 1), description: '', cover: '' }).title).toContain(
      String(TITLE_MAX),
    )
    // 中文一个字算一个字符：128 个汉字合法，而不是 128 字节。
    expect(validateCollectionForm({ title: '中'.repeat(TITLE_MAX), description: '', cover: '' }).title).toBeNull()
  })

  it('描述可以为空，超过上限才报错', () => {
    expect(validateCollectionForm({ title: '合集', description: '', cover: '' }).description).toBeNull()
    expect(
      validateCollectionForm({ title: '合集', description: '字'.repeat(DESCRIPTION_MAX + 1), cover: '' }).description,
    ).toContain(String(DESCRIPTION_MAX))
  })
})

describe('buildCollectionCreatePayload', () => {
  it('去掉标题空白，并带上展示开关与封面', () => {
    const payload = buildCollectionCreatePayload({
      title: '  Nuxt 部署实战  ',
      description: ' 从零到上线 ',
      cover: ' https://cdn.example.com/cover.png ',
      displayed: false,
    })

    expect(payload).toEqual({
      title: 'Nuxt 部署实战',
      description: '从零到上线',
      cover: 'https://cdn.example.com/cover.png',
      displayed: false,
    })
  })

  it('没选封面时 cover 是空串（交给读模型回退首篇），不是省略或 null', () => {
    const payload = buildCollectionCreatePayload({
      title: '合集',
      description: '',
      cover: '',
      displayed: true,
    })

    expect(payload.cover).toBe('')
    expect(payload.displayed).toBe(true)
  })
})

describe('buildCollectionUpdatePayload', () => {
  it('改合集只提交标题 / 描述 / 封面，不带展示开关', () => {
    const payload = buildCollectionUpdatePayload({
      title: ' 新标题 ',
      description: '新描述',
      cover: '',
    })

    expect(payload).toEqual({ title: '新标题', description: '新描述', cover: '' })
    expect('displayed' in payload).toBe(false)
  })
})

describe('coverUploadGate', () => {
  it('16:9 且在 ±1% 噪声内：直接上传，不弹取景窗', () => {
    expect(coverUploadGate({ width: 1920, height: 1080 })).toBe('direct')
    expect(coverUploadGate({ width: 1280, height: 720 })).toBe('direct')
    // 1919×1079 这类像素噪声仍算合规。
    expect(coverUploadGate({ width: 1919, height: 1079 })).toBe('direct')
  })

  it('比例差超过 1% 必须先取景（竖图、3:2 横图）', () => {
    expect(coverUploadGate({ width: 1200, height: 800 })).toBe('framing')
    expect(coverUploadGate({ width: 1080, height: 1920 })).toBe('framing')
  })

  it('读不到尺寸时既不上传也不开窗，而是报错', () => {
    expect(coverUploadGate(null)).toBe('unreadable')
    expect(coverUploadGate(undefined)).toBe('unreadable')
    expect(coverUploadGate({ width: 0, height: 0 })).toBe('unreadable')
    expect(coverUploadGate({ width: Number.NaN, height: 1080 })).toBe('unreadable')
  })
})

describe('resolveCollectionCover', () => {
  it('上传过封面就用上传的那张，不标回退', () => {
    expect(resolveCollectionCover('https://cdn.example.com/mine.png', 'https://cdn.example.com/first.png')).toEqual({
      url: 'https://cdn.example.com/mine.png',
      fallback: false,
    })
  })

  it('没上传封面时回退到首篇文章的封面，并标记来源', () => {
    expect(resolveCollectionCover('', 'https://cdn.example.com/first.png')).toEqual({
      url: 'https://cdn.example.com/first.png',
      fallback: true,
    })
    expect(resolveCollectionCover('   ', '  https://cdn.example.com/first.png  ')).toEqual({
      url: 'https://cdn.example.com/first.png',
      fallback: true,
    })
  })

  it('两张都没有时返回 null——页面据此渲染显式占位，绝不发一个空的 img', () => {
    expect(resolveCollectionCover('', '')).toBeNull()
    expect(resolveCollectionCover(undefined, null)).toBeNull()
  })
})

describe('applyDisplayResult', () => {
  it('只改目标行，其余行原样保留', () => {
    const rows = [card({ id: '1', displayed: true }), card({ id: '2', displayed: true })]

    const next = applyDisplayResult(rows, '2', false)

    expect(next[0]).toEqual(rows[0])
    expect(next[1]!.displayed).toBe(false)
    // 原数组不被就地改写（渲染层据此拿到新引用）。
    expect(rows[1]!.displayed).toBe(true)
  })

  it('找不到目标 ID 时原样返回（不凭空造行）', () => {
    const rows = [card({ id: '1' })]
    expect(applyDisplayResult(rows, 'missing', false)).toEqual(rows)
  })
})

describe('selectCollectionCover', () => {
  const file = { name: 'cover.png', type: 'image/png' } as File

  function deps(size: { width: number; height: number } | null) {
    return {
      readSize: vi.fn(async () => size),
      createPreviewUrl: vi.fn(() => 'blob:cover'),
      revokePreviewUrl: vi.fn(),
      upload: vi.fn(async () => ({ imageId: '7', url: 'https://cdn.example.com/cover.png' })),
    }
  }

  it('合规图直接上传，不等取景确认', async () => {
    const d = deps({ width: 1920, height: 1080 })

    const result = await selectCollectionCover(file, d)

    expect(result).toMatchObject({ kind: 'uploaded', cover: 'https://cdn.example.com/cover.png', imageId: '7' })
    expect(d.upload).toHaveBeenCalledWith(file)
    // 上传完成后本地预览地址要释放，避免内存泄漏。
    expect(d.revokePreviewUrl).toHaveBeenCalledWith('blob:cover')
  })

  it('不合规的图不先上传，而是进取景窗等确认', async () => {
    const d = deps({ width: 1080, height: 1920 })

    const result = await selectCollectionCover(file, d)

    expect(result).toMatchObject({ kind: 'framing', file, previewUrl: 'blob:cover' })
    expect(d.upload).not.toHaveBeenCalled()
    // 取景窗还要用这个预览地址，不能提前释放。
    expect(d.revokePreviewUrl).not.toHaveBeenCalled()
  })

  it('读不到尺寸时不产生任何上传，并释放预览地址', async () => {
    const d = deps(null)

    await expect(selectCollectionCover(file, d)).resolves.toEqual({ kind: 'unreadable' })
    expect(d.upload).not.toHaveBeenCalled()
    expect(d.revokePreviewUrl).toHaveBeenCalledWith('blob:cover')
  })

  it('上传失败时原样上抛，并且不留下没人释放的预览地址', async () => {
    const d = deps({ width: 1920, height: 1080 })
    d.upload.mockRejectedValueOnce(new Error('网络中断'))

    await expect(selectCollectionCover(file, d)).rejects.toThrow('网络中断')
    expect(d.revokePreviewUrl).toHaveBeenCalledWith('blob:cover')
  })
})

import { describe, expect, it } from 'vitest'

import {
  DESCRIPTION_MAX,
  TITLE_MAX,
  applyDisplayResult,
  buildCollectionCreatePayload,
  buildCollectionUpdatePayload,
  resolveCollectionCover,
  validateCollectionForm,
} from '~/utils/collectionAdmin'
import type { CollectionAdminCard } from '~/types/api'

/**
 * 后台合集管理页的编排单元测试。
 *
 * 断言的是**可观察的决定**：表单什么算合法、请求体里带了什么、封面回退到哪一张、
 * 展示开关如何落到列表。页面交给组件测试，这里只锁规则。
 *
 * #200 起封面上传改成"上传前定形"（与轮播、文章封面同一份实现），所以原来的
 * 16:9 闸门（`coverUploadGate` / `selectCollectionCover`）连同它的用例一起退场：
 * 定形之后每张封面都已经是 16:9，不再有"合规直传 / 不合规先取景"的分叉。
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

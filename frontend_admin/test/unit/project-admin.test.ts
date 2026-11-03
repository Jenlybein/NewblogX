import { describe, expect, it } from 'vitest'

import {
  PROJECT_DESCRIPTION_MAX,
  PROJECT_TITLE_MAX,
  applyProjectDisplayResult,
  buildProjectCreatePayload,
  buildProjectUpdatePayload,
  resolveProjectCover,
  validateProjectForm,
} from '~/utils/projectAdmin'
import type { ProjectAdminCard } from '~/types/api'

/**
 * 后台项目管理页的编排单元测试（与合集管理页镜像）。
 *
 * 断言的是**可观察的决定**：表单什么算合法、请求体里带了什么、封面在上传前
 * 定形成 16:9（#204 起没有取景步骤）、封面回退到哪一张、展示开关如何落到列表。
 * 页面交给组件测试，这里只锁规则。
 */

function card(overrides: Partial<ProjectAdminCard> = {}): ProjectAdminCard {
  return {
    id: '1',
    title: '重构实战',
    description: '',
    cover: '',
    fallback_cover: '',
    displayed: true,
    article_count: 0,
    sort_order: 1,
    ...overrides,
  }
}

describe('validateProjectForm', () => {
  it('标题去掉首尾空白后不能为空', () => {
    expect(validateProjectForm({ title: '   ', description: '', cover: '' }).title).toBe('项目标题不能为空')
    expect(validateProjectForm({ title: '  重构实战  ', description: '', cover: '' }).title).toBeNull()
  })

  it('标题按字符数（不是字节数）卡上限', () => {
    const atLimit = '项'.repeat(PROJECT_TITLE_MAX)
    const overLimit = '项'.repeat(PROJECT_TITLE_MAX + 1)

    expect(validateProjectForm({ title: atLimit, description: '', cover: '' }).title).toBeNull()
    expect(validateProjectForm({ title: overLimit, description: '', cover: '' }).title).toContain(
      String(PROJECT_TITLE_MAX),
    )
  })

  it('描述可以为空，超过上限才报错', () => {
    expect(validateProjectForm({ title: '项目', description: '', cover: '' }).description).toBeNull()
    expect(
      validateProjectForm({
        title: '项目',
        description: '字'.repeat(PROJECT_DESCRIPTION_MAX + 1),
        cover: '',
      }).description,
    ).toContain(String(PROJECT_DESCRIPTION_MAX))
  })
})

describe('buildProjectCreatePayload', () => {
  it('去掉标题空白，并带上展示开关与封面', () => {
    const payload = buildProjectCreatePayload({
      title: '  重构实战  ',
      description: ' 从零到上线 ',
      cover: ' https://cdn.example.com/cover.png ',
      displayed: false,
    })

    expect(payload).toEqual({
      title: '重构实战',
      description: '从零到上线',
      cover: 'https://cdn.example.com/cover.png',
      displayed: false,
    })
  })

  it('没选封面时 cover 是空串（交给读模型回退首篇），不是省略或 null', () => {
    const payload = buildProjectCreatePayload({ title: '项目', description: '', cover: '', displayed: true })

    expect(payload.cover).toBe('')
    expect(payload.displayed).toBe(true)
  })
})

describe('buildProjectUpdatePayload', () => {
  it('改项目只提交标题 / 描述 / 封面，不带展示开关', () => {
    const payload = buildProjectUpdatePayload({ title: ' 新标题 ', description: '新描述', cover: '' })

    expect(payload).toEqual({ title: '新标题', description: '新描述', cover: '' })
    expect('displayed' in payload).toBe(false)
  })
})

describe('resolveProjectCover', () => {
  it('上传过封面就用上传的那张，不标回退', () => {
    expect(resolveProjectCover('https://cdn.example.com/mine.png', 'https://cdn.example.com/first.png')).toEqual({
      url: 'https://cdn.example.com/mine.png',
      fallback: false,
    })
  })

  it('没上传封面时回退到首篇公开文章的封面，并标记来源', () => {
    expect(resolveProjectCover('', 'https://cdn.example.com/first.png')).toEqual({
      url: 'https://cdn.example.com/first.png',
      fallback: true,
    })
  })

  it('两张都没有时返回 null——页面据此渲染显式占位，绝不发一个空的 img', () => {
    expect(resolveProjectCover('', '')).toBeNull()
    expect(resolveProjectCover(undefined, null)).toBeNull()
  })
})

describe('applyProjectDisplayResult', () => {
  it('只改目标行，其余行原样保留，且不就地改写入参', () => {
    const rows = [card({ id: '1', displayed: true }), card({ id: '2', displayed: true })]

    const next = applyProjectDisplayResult(rows, '2', false)

    expect(next[0]).toEqual(rows[0])
    expect(next[1]!.displayed).toBe(false)
    expect(rows[1]!.displayed).toBe(true)
  })

  it('找不到目标 ID 时原样返回（不凭空造行）', () => {
    const rows = [card({ id: '1' })]
    expect(applyProjectDisplayResult(rows, 'missing', false)).toEqual(rows)
  })
})


import { describe, expect, it } from 'vitest'

import {
  PUBLISH_DRAFT,
  PUBLISH_PENDING,
  PUBLISH_PUBLISHED,
  PUBLISH_REJECTED,
  isAdminHidden,
  publishLabel,
  visibilityLabel,
} from '~/utils/articleLabels'

describe('publishLabel', () => {
  it('四个状态的中文文案与后端枚举一致', () => {
    expect(PUBLISH_DRAFT).toBe(1)
    expect(PUBLISH_PENDING).toBe(2)
    expect(PUBLISH_PUBLISHED).toBe(3)
    expect(PUBLISH_REJECTED).toBe(4)
    expect(publishLabel(PUBLISH_DRAFT)).toBe('草稿')
    expect(publishLabel(PUBLISH_PENDING)).toBe('待审核')
    expect(publishLabel(PUBLISH_PUBLISHED)).toBe('已发布')
    expect(publishLabel(PUBLISH_REJECTED)).toBe('已驳回')
  })

  it('空值显示占位符', () => {
    expect(publishLabel(undefined)).toBe('-')
    expect(publishLabel(null)).toBe('-')
  })

  it('未知值原样显示，便于发现契约新增了枚举', () => {
    expect(publishLabel(99)).toBe('99')
  })
})

describe('visibilityLabel', () => {
  it('三种可见性的文案', () => {
    expect(visibilityLabel('visible')).toBe('公开')
    expect(visibilityLabel('user_hidden')).toBe('作者隐藏')
    expect(visibilityLabel('admin_hidden')).toBe('后台隐藏')
  })

  it('缺省按公开处理（与后端默认值一致）', () => {
    expect(visibilityLabel(undefined)).toBe('公开')
  })
})

describe('isAdminHidden', () => {
  it('只有 admin_hidden 才算被后台隐藏', () => {
    expect(isAdminHidden('admin_hidden')).toBe(true)
    expect(isAdminHidden('user_hidden')).toBe(false)
    expect(isAdminHidden('visible')).toBe(false)
    expect(isAdminHidden(undefined)).toBe(false)
  })
})

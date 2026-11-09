import { describe, expect, it } from 'vitest'

import {
  PUBLISH_DRAFT,
  PUBLISH_PENDING,
  PUBLISH_PUBLISHED,
  PUBLISH_REJECTED,
  isAdminHidden,
  publishLabel,
  isAdminHidden,
  isPendingReview,
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
  it('四种可见性的文案', () => {
    expect(visibilityLabel('visible')).toBe('公开')
    expect(visibilityLabel('user_hidden')).toBe('作者隐藏')
    expect(visibilityLabel('admin_hidden')).toBe('后台隐藏')
    // 票 302 的中间态：已发布、但对读者还不可见。漏掉这一档会回落成「公开」，
    // 于是"作者以为发了、读者看不到"这种状态在后台完全看不见。
    expect(visibilityLabel('pending_review')).toBe('待可见')
  })

  it('缺省按公开处理（与后端默认值一致）', () => {
    expect(visibilityLabel(undefined)).toBe('公开')
  })
})

describe('isPendingReview', () => {
  it('只有 pending_review 算"等合规判定"', () => {
    expect(isPendingReview('pending_review')).toBe(true)
    // 这三档都能用隐藏/恢复按钮动，不能被当成待可见。
    expect(isPendingReview('visible')).toBe(false)
    expect(isPendingReview('admin_hidden')).toBe(false)
    expect(isPendingReview('user_hidden')).toBe(false)
    expect(isPendingReview(undefined)).toBe(false)
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

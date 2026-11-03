import { describe, expect, it } from 'vitest'

import {
  projectStaleReasonLabel,
  staleProjectItemCount,
  staleProjectItemIds,
} from '~/utils/projectItems'

/**
 * 后台项目收录页失效行的纯逻辑用例（与合集收录页镜像）。
 *
 * 失效判定不由前端重算（行上的 stale 来自后端共用领域函数），这里只锁
 * "清理挑哪些行 / 计数 / 文案"这三处——挑错会删掉展示中的文章。
 */

function item(articleId: string, stale: boolean, reason = ''): {
  article_id: string
  stale: boolean
  stale_reason: string
} {
  return { article_id: articleId, stale, stale_reason: reason }
}

describe('staleProjectItemIds', () => {
  it('只挑 stale 为真的行，保持收录顺序', () => {
    const items = [item('a1', false), item('a2', true, 'deleted'), item('a3', true, 'not_visible')]

    expect(staleProjectItemIds(items)).toEqual(['a2', 'a3'])
  })

  it('没有失效行时返回空数组（清理按钮据此禁用）', () => {
    expect(staleProjectItemIds([item('a1', false)])).toEqual([])
    expect(staleProjectItemCount([])).toBe(0)
  })
})

describe('staleProjectItemCount', () => {
  it('与 ID 列表同一份事实', () => {
    const items = [item('a1', true), item('a2', false), item('a3', true)]
    expect(staleProjectItemCount(items)).toBe(staleProjectItemIds(items).length)
  })
})

describe('projectStaleReasonLabel', () => {
  it('把后端枚举翻成运营看得懂的原因', () => {
    expect(projectStaleReasonLabel('')).toBe('展示中')
    expect(projectStaleReasonLabel('deleted')).toBe('文章已删除')
    expect(projectStaleReasonLabel('missing')).toBe('文章已删除')
    expect(projectStaleReasonLabel('not_published')).toBe('文章已不是已发布状态')
    expect(projectStaleReasonLabel('not_visible')).toBe('文章已不可见')
  })

  it('未知枚举与空值都给兜底而不是空白', () => {
    expect(projectStaleReasonLabel(undefined)).toBe('展示中')
    expect(projectStaleReasonLabel('who_knows')).toBe('已失效')
  })
})

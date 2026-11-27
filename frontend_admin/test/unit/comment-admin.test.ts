import { describe, expect, it } from 'vitest'

import { buildCommentAdminQuery, dayOf, splitUserInput } from '~/utils/commentAdmin'
import type { CommentAdminFilters } from '~/utils/commentAdmin'

/**
 * 评论管理的筛选拼装（2026-11-27）。
 *
 * 这几条规则错了不会报错：只会静默查到"看起来空"的列表或者扫全表，
 * 所以每条都钉住。断言的是**发给后端的参数**，不关心组件内部状态。
 */

function filters(overrides: Partial<CommentAdminFilters> = {}): CommentAdminFilters {
  return {
    keyword: '',
    userInput: '',
    articleInput: '',
    status: 'all',
    range: null,
    ...overrides,
  }
}

describe('splitUserInput', () => {
  it('全数字当用户 ID，其余当昵称片段', () => {
    expect(splitUserInput('384186503196053504')).toEqual({ user_id: '384186503196053504' })
    expect(splitUserInput('小明')).toEqual({ user_keyword: '小明' })
    // 昵称里带数字但整串不是纯数字 → 昵称。
    expect(splitUserInput('user123')).toEqual({ user_keyword: 'user123' })
  })

  it('空 / 纯空白不产生任何筛选参数', () => {
    expect(splitUserInput('')).toEqual({})
    expect(splitUserInput('   ')).toEqual({})
  })

  it('前后空白不影响判据', () => {
    expect(splitUserInput('  384186503196053504  ')).toEqual({ user_id: '384186503196053504' })
  })
})

describe('dayOf', () => {
  it('毫秒时间戳切成 YYYY-MM-DD', () => {
    expect(dayOf(Date.UTC(2026, 10, 27, 12, 0, 0))).toBe('2026-11-27')
  })

  it('同一天的任意时刻都落在同一天', () => {
    const start = Date.UTC(2026, 10, 27, 0, 0, 0)
    const end = Date.UTC(2026, 10, 27, 23, 59, 59)
    expect(dayOf(start)).toBe(dayOf(end))
  })
})

describe('buildCommentAdminQuery', () => {
  it('空筛选只传分页与状态，不传空串', () => {
    const query = buildCommentAdminQuery(filters(), 1, 20)
    expect(query).toEqual({ page: 1, limit: 20, status: 'all' })
    // 空串会被后端当成 LIKE '%%'，必须真的缺席。
    expect(query.keyword).toBeUndefined()
    expect(query.user_id).toBeUndefined()
    expect(query.article_id).toBeUndefined()
  })

  it('内容关键词与文章 ID 走 trim 后的值', () => {
    const query = buildCommentAdminQuery(filters({ keyword: '  广告  ', articleInput: ' 378268359633932289 ' }), 2, 20)
    expect(query.keyword).toBe('广告')
    expect(query.article_id).toBe('378268359633932289')
    expect(query.page).toBe(2)
  })

  it('用户输入按 ID / 昵称分别落到不同参数上，且互斥', () => {
    const byID = buildCommentAdminQuery(filters({ userInput: '384186503196053504' }), 1, 20)
    expect(byID.user_id).toBe('384186503196053504')
    expect(byID.user_keyword).toBeUndefined()

    const byName = buildCommentAdminQuery(filters({ userInput: '小明' }), 1, 20)
    expect(byName.user_keyword).toBe('小明')
    expect(byName.user_id).toBeUndefined()
  })

  it('时间区间两端一起传；不传区间时都不传', () => {
    const ranged = buildCommentAdminQuery(
      filters({ range: [Date.UTC(2026, 10, 1, 8), Date.UTC(2026, 10, 27, 9)] }),
      1,
      20,
    )
    expect(ranged.from).toBe('2026-11-01')
    expect(ranged.to).toBe('2026-11-27')

    const unlimited = buildCommentAdminQuery(filters(), 1, 20)
    expect(unlimited.from).toBeUndefined()
    expect(unlimited.to).toBeUndefined()
  })

  it('状态三态原样透传', () => {
    for (const status of ['all', 'published', 'deleted'] as const) {
      expect(buildCommentAdminQuery(filters({ status }), 1, 20).status).toBe(status)
    }
  })
})

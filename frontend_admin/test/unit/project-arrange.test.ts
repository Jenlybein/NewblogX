import { describe, expect, it } from 'vitest'

import {
  canMoveProjectArrangeItem,
  dedupeProjectArticleIds,
  moveProjectArrangeItem,
  projectBatchOutcomeLabel,
  projectItemIds,
  projectOrderIds,
  summarizeProjectBatchInclude,
} from '~/utils/projectArrange'

/**
 * 项目编排页的纯逻辑用例（与合集编排页镜像）。
 *
 * 两级排序都是全量替换：漏传一条会让保存失败，顺序里少一条会在服务端整单
 * 拒绝。批量收录的提示要能分清"进去了几篇"与"几篇重复/不可收录"。
 */

describe('两级排序的请求体顺序', () => {
  it('项目之间按列表顺序给出完整 ID 列表', () => {
    expect(projectOrderIds([{ id: 'p1' }, { id: 'p2' }, { id: 'p3' }])).toEqual(['p1', 'p2', 'p3'])
  })

  it('项目内按列表顺序给出完整文章 ID 列表（失效行也在内）', () => {
    expect(projectItemIds([{ article_id: 'a1' }, { article_id: 'a2' }])).toEqual(['a1', 'a2'])
  })
})

describe('上移 / 下移', () => {
  const rows = ['a', 'b', 'c']

  it('两端不可移，中间的可以', () => {
    expect(canMoveProjectArrangeItem(0, 3, 'up')).toBe(false)
    expect(canMoveProjectArrangeItem(0, 3, 'down')).toBe(true)
    expect(canMoveProjectArrangeItem(2, 3, 'down')).toBe(false)
    expect(canMoveProjectArrangeItem(1, 3, 'up')).toBe(true)
  })

  it('移动返回新数组，不改入参', () => {
    const next = moveProjectArrangeItem(rows, 2, 'up')
    expect(next).toEqual(['a', 'c', 'b'])
    expect(rows).toEqual(['a', 'b', 'c'])
  })

  it('越界原样返回（键盘与竞态都可能再点一次）', () => {
    expect(moveProjectArrangeItem(rows, 0, 'up')).toEqual(rows)
    expect(moveProjectArrangeItem(rows, 9, 'down')).toEqual(rows)
  })
})

describe('dedupeProjectArticleIds', () => {
  it('按首次出现顺序去重', () => {
    expect(dedupeProjectArticleIds(['a', 'b', 'a', 'c'])).toEqual(['a', 'b', 'c'])
  })
})

describe('summarizeProjectBatchInclude', () => {
  it('三个计数分别报出来，重复与不可收录不被"成功"掩盖', () => {
    expect(summarizeProjectBatchInclude({ included: 2, duplicated: 1, rejected: 1 })).toBe(
      '已收录 2 篇，1 篇已在项目中，1 篇不可收录',
    )
  })

  it('一篇都没新增时提示"没有新增收录"', () => {
    expect(summarizeProjectBatchInclude({ included: 0, duplicated: 2, rejected: 0 })).toBe(
      '没有新增收录，2 篇已在项目中',
    )
  })

  it('全零时给一个可读的兜底', () => {
    expect(summarizeProjectBatchInclude({ included: 0, duplicated: 0, rejected: 0 })).toBe(
      '没有可提交的文章',
    )
  })
})

describe('projectBatchOutcomeLabel', () => {
  it('四种结局都有项目侧文案', () => {
    expect(projectBatchOutcomeLabel('included')).toBe('已收录')
    expect(projectBatchOutcomeLabel('duplicated')).toBe('已在项目中')
    expect(projectBatchOutcomeLabel('not_includable')).toBe('不可收录')
    expect(projectBatchOutcomeLabel('missing')).toBe('文章不存在')
    expect(projectBatchOutcomeLabel('unknown')).toBe('未知结果')
  })
})

import { describe, expect, it } from 'vitest'

import {
  batchOutcomeLabel,
  canMoveArrangeItem,
  collectionItemIds,
  collectionOrderIds,
  dedupeArticleIds,
  moveArrangeItem,
  summarizeBatchInclude,
} from '~/utils/collectionArrange'

/**
 * 合集编排页的纯逻辑用例。
 *
 * 两级排序都是**全量替换**语义：上移 / 下移只改本地数组，提交时必须带上
 * 完整有序集合（漏一条会被后端整单拒绝）。这里锁的就是"换位"与"提交什么"。
 */

describe('canMoveArrangeItem / moveArrangeItem', () => {
  it('两端不能再朝外移动，中间可以', () => {
    expect(canMoveArrangeItem(0, 3, 'up')).toBe(false)
    expect(canMoveArrangeItem(0, 3, 'down')).toBe(true)
    expect(canMoveArrangeItem(1, 3, 'up')).toBe(true)
    expect(canMoveArrangeItem(1, 3, 'down')).toBe(true)
    expect(canMoveArrangeItem(2, 3, 'down')).toBe(false)
    expect(canMoveArrangeItem(0, 0, 'up')).toBe(false)
    expect(canMoveArrangeItem(0, 0, 'down')).toBe(false)
  })

  it('上移 / 下移返回新数组（不改入参），越界时原样返回', () => {
    const rows = [{ id: 'a' }, { id: 'b' }, { id: 'c' }]

    const up = moveArrangeItem(rows, 1, 'up')
    expect(up.map(row => row.id)).toEqual(['b', 'a', 'c'])
    expect(rows.map(row => row.id)).toEqual(['a', 'b', 'c'])

    const down = moveArrangeItem(rows, 1, 'down')
    expect(down.map(row => row.id)).toEqual(['a', 'c', 'b'])

    expect(moveArrangeItem(rows, 0, 'up').map(row => row.id)).toEqual(['a', 'b', 'c'])
    expect(moveArrangeItem(rows, 2, 'down').map(row => row.id)).toEqual(['a', 'b', 'c'])
  })
})

describe('排序请求体', () => {
  it('合集之间：按当前展示顺序给出完整 ID 列表', () => {
    expect(
      collectionOrderIds([
        { id: '3', title: '第三个' },
        { id: '1', title: '第一个' },
      ]),
    ).toEqual(['3', '1'])
  })

  it('合集内：失效行也算（后端要求提交完整集合，漏了会被拒）', () => {
    const items = [
      { article_id: 'a1', title: '第一篇', stale: false },
      { article_id: 'a2', title: '已失效的一篇', stale: true },
    ]
    expect(collectionItemIds(items)).toEqual(['a1', 'a2'])
  })
})

describe('dedupeArticleIds', () => {
  it('按首次出现顺序去重（同一篇勾两次只提交一次）', () => {
    expect(dedupeArticleIds(['a', 'b', 'a', 'c', 'b'])).toEqual(['a', 'b', 'c'])
    expect(dedupeArticleIds([])).toEqual([])
  })
})

describe('summarizeBatchInclude', () => {
  it('把三个计数拼成可读的一句话，重复与不可收录分别报出来', () => {
    expect(summarizeBatchInclude({ included: 2, duplicated: 1, rejected: 0 })).toBe(
      '已收录 2 篇，1 篇已在合集中',
    )
    expect(summarizeBatchInclude({ included: 1, duplicated: 2, rejected: 3 })).toBe(
      '已收录 1 篇，2 篇已在合集中，3 篇不可收录',
    )
    expect(summarizeBatchInclude({ included: 0, duplicated: 0, rejected: 2 })).toBe(
      '没有新增收录，2 篇不可收录',
    )
    expect(summarizeBatchInclude({ included: 0, duplicated: 1, rejected: 0 })).toBe(
      '没有新增收录，1 篇已在合集中',
    )
    expect(summarizeBatchInclude({ included: 0, duplicated: 0, rejected: 0 })).toBe(
      '没有可提交的文章',
    )
  })
})

describe('batchOutcomeLabel', () => {
  it('把后端结局翻成后台看得懂的文案', () => {
    expect(batchOutcomeLabel('included')).toBe('已收录')
    expect(batchOutcomeLabel('duplicated')).toBe('已在合集中')
    expect(batchOutcomeLabel('not_includable')).toBe('不可收录')
    expect(batchOutcomeLabel('missing')).toBe('文章不存在')
    expect(batchOutcomeLabel('unknown')).toBe('未知结果')
  })
})

import { describe, expect, it } from 'vitest'

import {
  collectionStaleReasonLabel,
  staleCollectionItemCount,
  staleCollectionItemIds,
} from '~/utils/collectionItems'

/**
 * 后台「合集收录行」失效标记与一键清理的纯逻辑。
 *
 * 失效判定本身在页面之外——后端把共用领域判定算好的 `stale` 与
 * `stale_reason` 发下来。这里只负责三件会算错且算错了看不出来的事：
 *   1. 一键清理要挑哪些行（挑错会删掉展示中的文章）；
 *   2. 按钮上的计数（为 0 时不该让人点一个空操作）；
 *   3. 失效原因的中文文案（运营要能看出为什么失效）。
 */

type Row = {
  article_id: string
  stale: boolean
  stale_reason: string
}

const rows: Row[] = [
  { article_id: 'live-1', stale: false, stale_reason: '' },
  { article_id: 'draft-2', stale: true, stale_reason: 'not_published' },
  { article_id: 'live-3', stale: false, stale_reason: '' },
  { article_id: 'deleted-4', stale: true, stale_reason: 'deleted' },
]

describe('合集收录：失效项挑选', () => {
  it('只挑 stale 为真的行，且保持收录顺序', () => {
    expect(staleCollectionItemIds(rows)).toEqual(['draft-2', 'deleted-4'])
    expect(staleCollectionItemCount(rows)).toBe(2)
  })

  it('没有失效行时返回空数组 / 0（清理按钮据此禁用）', () => {
    expect(staleCollectionItemIds([{ article_id: 'live', stale: false, stale_reason: '' }])).toEqual([])
    expect(staleCollectionItemIds([])).toEqual([])
    expect(staleCollectionItemCount([])).toBe(0)
  })

  it('stale 字段缺席（老契约 / 加载失败兜底）按展示中处理，不误删', () => {
    const partial = [{ article_id: 'a' }, { article_id: 'b', stale: true }] as Row[]

    expect(staleCollectionItemIds(partial)).toEqual(['b'])
    expect(staleCollectionItemCount(partial)).toBe(1)
  })
})

describe('合集收录：失效原因文案', () => {
  it('四种成因各有可读文案', () => {
    expect(collectionStaleReasonLabel('missing')).toBe('文章已删除')
    expect(collectionStaleReasonLabel('deleted')).toBe('文章已删除')
    expect(collectionStaleReasonLabel('not_published')).toBe('文章已不是已发布状态')
    expect(collectionStaleReasonLabel('not_visible')).toBe('文章已不可见')
  })

  it('仍展示的行不显示失效原因', () => {
    expect(collectionStaleReasonLabel('')).toBe('展示中')
    expect(collectionStaleReasonLabel(null)).toBe('展示中')
    expect(collectionStaleReasonLabel(undefined)).toBe('展示中')
  })

  it('未知原因（后端将来新增枚举）给出兜底文案，而不是空白', () => {
    expect(collectionStaleReasonLabel('something-new')).toBe('已失效')
  })
})

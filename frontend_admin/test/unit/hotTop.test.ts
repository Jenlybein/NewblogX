import { describe, expect, it } from 'vitest'
import {
  HOT_TOP_LIMIT,
  canMoveHotTop,
  hotTopIds,
  moveHotTop,
  staleHotTopIds,
} from '../../app/utils/hotTop'

/**
 * 后台「热门置顶」序列的纯逻辑。
 *
 * 页面本身（naive-ui 表格 + 弹窗）没有组件测试基线，所以排序、失效判定这些
 * **会算错而且算错了看不出来**的部分抽到 utils 里钉住：
 *   - 上移/下移错了，运营看到的顺序与首页顺序不一致；
 *   - 顺序提交是全量替换（后端在集合不一致时直接拒绝），漏掉失效项就会整单失败。
 */

type Row = { article_id: string, stale: boolean }

const rows: Row[] = [
  { article_id: 'a', stale: false },
  { article_id: 'b', stale: true },
  { article_id: 'c', stale: false },
]

describe('热门置顶：上移 / 下移', () => {
  it('上移与前一项交换，其余顺序不变', () => {
    expect(moveHotTop(rows, 2, 'up').map(row => row.article_id)).toEqual(['a', 'c', 'b'])
  })

  it('下移与后一项交换，其余顺序不变', () => {
    expect(moveHotTop(rows, 0, 'down').map(row => row.article_id)).toEqual(['b', 'a', 'c'])
  })

  it('首项上移、末项下移都是空操作（顺序原样）', () => {
    expect(moveHotTop(rows, 0, 'up').map(row => row.article_id)).toEqual(['a', 'b', 'c'])
    expect(moveHotTop(rows, 2, 'down').map(row => row.article_id)).toEqual(['a', 'b', 'c'])
  })

  it('越界下标不抛错、也不改变顺序', () => {
    expect(moveHotTop(rows, -1, 'up').map(row => row.article_id)).toEqual(['a', 'b', 'c'])
    expect(moveHotTop(rows, 3, 'up').map(row => row.article_id)).toEqual(['a', 'b', 'c'])
    expect(moveHotTop(rows, -1, 'down').map(row => row.article_id)).toEqual(['a', 'b', 'c'])
    expect(moveHotTop([], 0, 'down')).toEqual([])
  })

  it('返回新数组、不改动入参（页面靠新引用触发重渲染）', () => {
    const source = [...rows]
    const moved = moveHotTop(source, 1, 'up')

    expect(moved).not.toBe(source)
    expect(source.map(row => row.article_id)).toEqual(['a', 'b', 'c'])
    expect(moved.map(row => row.article_id)).toEqual(['b', 'a', 'c'])
  })

  it('canMoveHotTop 只放行两端之间的移动', () => {
    expect(canMoveHotTop(0, 3, 'up')).toBe(false)
    expect(canMoveHotTop(0, 3, 'down')).toBe(true)
    expect(canMoveHotTop(1, 3, 'up')).toBe(true)
    expect(canMoveHotTop(1, 3, 'down')).toBe(true)
    expect(canMoveHotTop(2, 3, 'up')).toBe(true)
    expect(canMoveHotTop(2, 3, 'down')).toBe(false)
    expect(canMoveHotTop(-1, 3, 'down')).toBe(false)
    // 空序列：任何方向都不能动
    expect(canMoveHotTop(0, 0, 'up')).toBe(false)
    expect(canMoveHotTop(0, 0, 'down')).toBe(false)
  })
})

describe('热门置顶：上限', () => {
  it('运营位最多 12 条（ADR-0017 决策 5）', () => {
    expect(HOT_TOP_LIMIT).toBe(12)
  })
})

describe('热门置顶：顺序提交与失效项', () => {
  it('顺序 ID 覆盖全部项（失效项也必须在里，漏一项后端整单拒绝）', () => {
    expect(hotTopIds(rows)).toEqual(['a', 'b', 'c'])
  })

  it('失效项 ID 只挑 stale 为真的行，且保持序列顺序', () => {
    expect(staleHotTopIds(rows)).toEqual(['b'])
    expect(staleHotTopIds([{ article_id: 'x', stale: true }, { article_id: 'y', stale: true }])).toEqual([
      'x',
      'y',
    ])
    expect(staleHotTopIds([{ article_id: 'z', stale: false }])).toEqual([])
    expect(staleHotTopIds([])).toEqual([])
  })

  it('stale 字段缺席（老契约/加载失败兜底）按有效处理', () => {
    const partial = [{ article_id: 'a' }, { article_id: 'b', stale: true }] as Row[]

    expect(staleHotTopIds(partial)).toEqual(['b'])
  })
})

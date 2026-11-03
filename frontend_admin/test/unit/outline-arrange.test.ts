import { describe, expect, it } from 'vitest'
import type {
  OutlineFlatRow,
  OutlineTreeNode,
} from '../../app/utils/outlineArrange'
import {
  addOutlineArticles,
  addOutlineFolder,
  buildOutlineNodes,
  canMoveOutlineRow,
  flattenOutlineTree,
  moveOutlineRow,
  outlineDepth,
  outlineIncludeTargetKey,
  outlineRowTitle,
} from '../../app/utils/outlineArrange'

/**
 * 后台树编辑器的纯逻辑：扁平视图（缩进 / 层级 / 顺序）与整树提交形状。
 *
 * 只断言外部行为——扁平行、嵌套 payload、移动后的顺序与层级。
 * 这些算错都不会报错：提交一棵错树会被服务端整单拒绝或静默换层，
 * 所以在这里钉死。
 */

function folder(id: string, parentID: string, title: string, children: OutlineTreeNode[] = []): OutlineTreeNode {
  return {
    id,
    parent_id: parentID,
    kind: 1,
    title,
    article_id: '',
    sort_order: 1,
    article_title: '',
    article_cover: '',
    stale: false,
    stale_reason: '',
    children,
  }
}

function article(id: string, parentID: string, articleID: string, title: string): OutlineTreeNode {
  return {
    id,
    parent_id: parentID,
    kind: 2,
    title: '',
    article_id: articleID,
    sort_order: 1,
    article_title: title,
    article_cover: '',
    stale: false,
    stale_reason: '',
    children: [],
  }
}

function labels(rows: OutlineFlatRow[]): string[] {
  return rows.map(row => `${outlineDepth(rows, row.key)}:${outlineRowTitle(row)}`)
}

function tree(): OutlineTreeNode[] {
  return [
    folder('f1', '', '第一章', [
      article('a1', 'f1', 'art1', '第一篇'),
      folder('f2', 'f1', '空章节', []),
    ]),
    article('a2', '', 'art2', '第二篇'),
    folder('f3', '', '第二章', []),
  ]
}

describe('flattenOutlineTree', () => {
  it('把嵌套树拍成 DFS 扁平行，保留层级与父节点', () => {
    const rows = flattenOutlineTree(tree())
    expect(labels(rows)).toEqual(['0:第一章', '1:第一篇', '1:空章节', '0:第二篇', '0:第二章'])
    expect(rows[0]!.parentKey).toBe('')
    expect(rows[1]!.parentKey).toBe('f1')
    expect(rows[2]!.parentKey).toBe('f1')
    expect(rows[2]!.kind).toBe(1)
    expect(rows[1]!.kind).toBe(2)
    // 空文件夹保留。
    expect(rows[2]!.key).toBe('f2')
  })

  it('空树返回空数组', () => {
    expect(flattenOutlineTree([])).toEqual([])
  })
})

describe('buildOutlineNodes', () => {
  it('把扁平行组装成嵌套 payload：已有节点带 id，新节点 id 留空', () => {
    const rows = flattenOutlineTree(tree())
    const payload = buildOutlineNodes(rows)
    expect(payload.map(node => node.title || node.article_id)).toEqual(['第一章', 'art2', '第二章'])
    expect(payload[0]!.id).toBe('f1')
    expect(payload[0]!.children.map(node => node.id)).toEqual(['a1', 'f2'])
    expect(payload[0]!.children[1]!.children).toEqual([])

    // 新建文件夹：没有服务端 ID，父节点靠扁平行里的 parentKey 表达。
    const withNew = addOutlineFolder(rows, 'f1', '新章节')
    const nested = buildOutlineNodes(withNew)
    const added = nested[0]!.children[2]!
    expect(added.id).toBe('')
    expect(added.title).toBe('新章节')
    expect(added.children).toEqual([])
  })
})

describe('moveOutlineRow：上移 / 下移', () => {
  it('同层内交换顺序，不改层级', () => {
    const rows = flattenOutlineTree(tree())
    const moved = moveOutlineRow(rows, 'a2', 'up')
    expect(labels(moved)).toEqual(['0:第二篇', '0:第一章', '1:第一篇', '1:空章节', '0:第二章'])
    expect(moved.find(row => row.key === 'a2')!.parentKey).toBe('')

    const upAgain = moveOutlineRow(rows, 'f3', 'up')
    expect(upAgain.map(row => row.key)).toEqual(['f1', 'a1', 'f2', 'f3', 'a2'])
    expect(canMoveOutlineRow(rows, 'f1', 'up')).toBe(false)
    expect(canMoveOutlineRow(rows, 'f3', 'down')).toBe(false)
  })
})

describe('moveOutlineRow：缩进 / 移出', () => {
  it('缩进挂到上一个兄弟文件夹下，成为其最后一个子节点', () => {
    const rows = flattenOutlineTree(tree())
    // 根层：第一章块、第二篇、第二章。第二篇的上一个兄弟是第一章（文件夹）。
    const moved = moveOutlineRow(rows, 'a2', 'indent')
    expect(labels(moved)).toEqual(['0:第一章', '1:第一篇', '1:空章节', '1:第二篇', '0:第二章'])
    const a2 = moved.find(row => row.key === 'a2')!
    expect(a2.parentKey).toBe('f1')
    const payload = buildOutlineNodes(moved)
    expect(payload[0]!.children.map(node => node.id)).toEqual(['a1', 'f2', 'a2'])
  })

  it('上一个兄弟是文章节点时不能缩进（文章不能做父节点）', () => {
    const rows = flattenOutlineTree(tree())
    expect(canMoveOutlineRow(rows, 'f2', 'indent')).toBe(false)
    expect(moveOutlineRow(rows, 'f2', 'indent')).toEqual(rows)
  })

  it('移出挂回父节点的父节点，并排在父节点之后', () => {
    const rows = flattenOutlineTree(tree())
    const moved = moveOutlineRow(rows, 'a1', 'outdent')
    expect(labels(moved)).toEqual(['0:第一章', '1:空章节', '0:第一篇', '0:第二篇', '0:第二章'])
    expect(moved.find(row => row.key === 'a1')!.parentKey).toBe('')
    expect(canMoveOutlineRow(rows, 'f1', 'outdent')).toBe(false)
  })

  it('文件夹带着整棵子树一起移动', () => {
    const rows = flattenOutlineTree(tree())
    const moved = moveOutlineRow(rows, 'f1', 'down')
    expect(moved.map(row => row.key)).toEqual(['a2', 'f1', 'a1', 'f2', 'f3'])
    expect(moved[1]!.parentKey).toBe('')
    expect(moved[2]!.parentKey).toBe('f1')
    expect(moved[3]!.parentKey).toBe('f1')
  })
})

describe('新建文件夹与批量收录', () => {
  it('根层新建空文件夹', () => {
    const rows = flattenOutlineTree(tree())
    const next = addOutlineFolder(rows, '', '第三章')
    expect(labels(next)).toEqual([
      '0:第一章',
      '1:第一篇',
      '1:空章节',
      '0:第二篇',
      '0:第二章',
      '0:第三章',
    ])
    expect(next[next.length - 1]!.id).toBe('')
    expect(next[next.length - 1]!.key).toMatch(/tmp/i)
  })

  it('把文章批量收录进指定文件夹，已在树里的文章跳过', () => {
    const rows = flattenOutlineTree(tree())
    const next = addOutlineArticles(rows, 'f3', [
      { id: 'art1', title: '第一篇', cover: 'c1' },
      { id: 'art3', title: '第三篇', cover: 'c3' },
    ])
    expect(labels(next)).toEqual([
      '0:第一章',
      '1:第一篇',
      '1:空章节',
      '0:第二篇',
      '0:第二章',
      '1:第三篇',
    ])
    expect(next.find(row => row.articleId === 'art3')!.parentKey).toBe('f3')
    expect(next.find(row => row.articleId === 'art3')!.articleTitle).toBe('第三篇')
    // 同一篇文章不能重复挂载。
    expect(next.filter(row => row.articleId === 'art1')).toHaveLength(1)
  })

  it('选中文章行时，收录目标退回它的父文件夹', () => {
    const rows = flattenOutlineTree(tree())
    expect(outlineIncludeTargetKey(rows, 'a1')).toBe('f1')
    expect(outlineIncludeTargetKey(rows, 'f1')).toBe('f1')
    expect(outlineIncludeTargetKey(rows, 'a2')).toBe('')
    expect(outlineIncludeTargetKey(rows, '')).toBe('')
  })
})

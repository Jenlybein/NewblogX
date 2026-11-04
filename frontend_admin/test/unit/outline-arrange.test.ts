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
  cleanupOutlineStaleRows,
  flattenOutlineTree,
  moveOutlineRow,
  moveOutlineRowToIndex,
  outlineDepth,
  outlineIncludeTargetKey,
  outlineRowTitle,
  outlineStaleCount,
  removeOutlineRow,
  renameOutlineFolder,
  staleReasonLabel,
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

function staleArticle(
  id: string, parentID: string, articleID: string, title: string, reason = 'not_published',
): OutlineTreeNode {
  return { ...article(id, parentID, articleID, title), stale: true, stale_reason: reason }
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

describe('重命名文件夹', () => {
  it('只改选中文件夹的标题并去掉两端空白，文章行与入参不受影响', () => {
    const rows = flattenOutlineTree(tree())
    const renamed = renameOutlineFolder(rows, 'f1', '  新的第一章  ')

    expect(renamed.find(row => row.key === 'f1')!.title).toBe('新的第一章')
    expect(renamed.find(row => row.key === 'f2')!.title).toBe('空章节')
    expect(renamed.find(row => row.key === 'a1')!.title).toBe('')
    expect(rows.find(row => row.key === 'f1')!.title).toBe('第一章')
    expect(buildOutlineNodes(renamed)[0]!.title).toBe('新的第一章')
  })

  it('文章行不能重命名；空标题与找不到的行原样返回', () => {
    const rows = flattenOutlineTree(tree())
    expect(renameOutlineFolder(rows, 'a1', '第一篇（改）')).toEqual(rows)
    expect(renameOutlineFolder(rows, 'f1', '   ')).toEqual(rows)
    expect(renameOutlineFolder(rows, 'missing', '新标题')).toEqual(rows)
  })
})

describe('移除节点', () => {
  it('移除文件夹时连同整棵子树一起移除，保存时不再出现这些节点', () => {
    const rows = flattenOutlineTree(tree())
    const next = removeOutlineRow(rows, 'f1')

    expect(next.map(row => row.key)).toEqual(['a2', 'f3'])
    expect(rows.map(row => row.key)).toEqual(['f1', 'a1', 'f2', 'a2', 'f3'])
    expect(buildOutlineNodes(next).map(node => node.id)).toEqual(['a2', 'f3'])
  })

  it('移除文章行只删这一行；找不到的行原样返回', () => {
    const rows = flattenOutlineTree(tree())
    const next = removeOutlineRow(rows, 'a1')
    expect(next.map(row => row.key)).toEqual(['f1', 'f2', 'a2', 'f3'])
    expect(removeOutlineRow(rows, 'missing')).toEqual(rows)
  })
})

describe('标记与清理已失效节点', () => {
  it('失效标记带原因，只统计失效的文章行（文件夹永不失效）', () => {
    const rows = flattenOutlineTree([
      folder('f1', '', '第一章', [staleArticle('a1', 'f1', 'art1', '第一篇', 'deleted')]),
      article('a2', '', 'art2', '第二篇'),
    ])
    expect(rows[1]!.stale).toBe(true)
    expect(rows[1]!.staleReason).toBe('deleted')
    expect(rows[0]!.stale).toBe(false)
    expect(outlineStaleCount(rows)).toBe(1)
  })

  it('一键清理只删失效文章节点，展示中的文章与文件夹保留', () => {
    const rows = flattenOutlineTree([
      folder('f1', '', '第一章', [
        staleArticle('a1', 'f1', 'art1', '草稿'),
        article('a2', 'f1', 'art2', '第二篇'),
      ]),
      staleArticle('a3', '', 'art3', '已删除', 'missing'),
    ])
    const next = cleanupOutlineStaleRows(rows)

    expect(next.map(row => row.key)).toEqual(['f1', 'a2'])
    expect(rows).toHaveLength(4)
    expect(outlineStaleCount(next)).toBe(0)
    expect(buildOutlineNodes(next)[0]!.children.map(node => node.article_id)).toEqual(['art2'])
  })

  it('没有失效行时清理是空操作（重复点击不产生变化）', () => {
    const rows = flattenOutlineTree(tree())
    expect(outlineStaleCount(rows)).toBe(0)
    expect(cleanupOutlineStaleRows(rows)).toEqual(rows)
    expect(cleanupOutlineStaleRows(cleanupOutlineStaleRows(rows))).toEqual(rows)
  })
})

describe('整树提交的幂等形状', () => {
  it('同一棵树读回两次提交同一个 payload（重复提交不产生变化）', () => {
    const first = buildOutlineNodes(flattenOutlineTree(tree()))
    const second = buildOutlineNodes(flattenOutlineTree(tree()))
    expect(second).toEqual(first)
    expect(first[0]!.children[0]!.article_id).toBe('art1')
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

/**
 * 失效原因文案（#225）。
 *
 * 原本住在 `utils/staleItems.ts`；旧扁平收录接口与「一键清理」退役后，那个文件
 * 只剩这一个小函数还被树编辑器用着，于是迁进本模块并删掉旧文件。
 *
 * 文案本身是运营读的东西：写成"未知"或留空，运营就看不出该不该清理这一行。
 * 枚举值与后端 `domain.StaleReason` 一一对应，后端将来新增值时前端必须落到
 * 兜底文案而不是空白。
 */
describe('失效原因文案（树节点标注用）', () => {
  it('四种成因各有可读文案', () => {
    expect(staleReasonLabel('missing')).toBe('文章已删除')
    expect(staleReasonLabel('deleted')).toBe('文章已删除')
    expect(staleReasonLabel('not_published')).toBe('文章已不是已发布状态')
    expect(staleReasonLabel('not_visible')).toBe('文章已不可见')
  })

  it('仍展示的行不显示失效原因', () => {
    expect(staleReasonLabel('')).toBe('展示中')
    expect(staleReasonLabel(null)).toBe('展示中')
    expect(staleReasonLabel(undefined)).toBe('展示中')
  })

  it('未知原因（后端将来新增枚举）给出兜底文案，而不是空白', () => {
    expect(staleReasonLabel('something-new')).toBe('已失效')
  })
})

/**
 * 拖拽排序的落点语义（决定清单 §8 第 37 条 / 票 #257）。
 *
 * 只按"落点上方的行"判定：上方是文件夹 → 成为它的第一个子行；上方是文章或落点在最前
 * → 成为兄弟。被拖的是一个块（文件夹连子树一起动），落点落在自己块内则什么都不做。
 * 这些规则错一个，保存出去就是另一棵树——所以每条都单独钉住。
 */
describe('moveOutlineRowToIndex：拖拽落点 → 扁平行', () => {
  function sample(): OutlineTreeNode[] {
    return [
      folder('f1', '', 'A', [article('a1', 'f1', '100', 'A-文章')]),
      folder('f2', '', 'B', []),
      article('a2', '', '200', 'B-文章'),
    ]
  }

  it('拖到首位：成为根层兄弟，排在最前', () => {
    const rows = flattenOutlineTree(sample())
    const next = moveOutlineRowToIndex(rows, 'a2', 0)

    expect(labels(next)).toEqual(['0:B-文章', '0:A', '1:A-文章', '0:B'])
  })

  it('落在文件夹下方：成为该文件夹的第一个子行（层级 +1）', () => {
    const rows = flattenOutlineTree(sample())
    // 落点下标 = 文件夹 B 之后（即 B 与其空子树之间）。
    const target = rows.findIndex(row => row.key === 'f2') + 1
    const next = moveOutlineRowToIndex(rows, 'a2', target)

    expect(outlineDepth(next, 'a2')).toBe(1)
    expect(outlineRowTitle(next[next.findIndex(row => row.key === 'a2')]!)).toBe('B-文章')
  })

  it('落在文章下方：成为同级兄弟（不偷偷缩进）', () => {
    const rows = flattenOutlineTree(sample())
    // A 的子树里那篇文章之后 → 与它同级，挂在 A 下。
    const target = rows.findIndex(row => row.key === 'a1') + 1
    const next = moveOutlineRowToIndex(rows, 'a2', target)

    expect(outlineDepth(next, 'a2')).toBe(1)
    expect(next[next.findIndex(row => row.key === 'a2') - 1]!.key).toBe('a1')
  })

  it('拖文件夹是整块移动：子树跟着走，子项层级不变', () => {
    const rows = flattenOutlineTree(sample())
    const end = rows.length
    const next = moveOutlineRowToIndex(rows, 'f1', end)

    // A 整块挪到最后：它和它的子项都还在，层级仍是 0 / 1。
    expect(labels(next)).toEqual(['0:B', '0:B-文章', '0:A', '1:A-文章'])
    expect(outlineDepth(next, 'a1')).toBe(1)
    expect(next[next.findIndex(row => row.key === 'a1') - 1]!.key).toBe('f1')
  })

  it('落点在自己块内：什么都不做（返回等价数组）', () => {
    const rows = flattenOutlineTree(sample())
    const index = rows.findIndex(row => row.key === 'f1')
    const next = moveOutlineRowToIndex(rows, 'f1', index + 1)

    expect(labels(next)).toEqual(labels(rows))
  })

  it('key 不存在时原样返回，不抛错', () => {
    const rows = flattenOutlineTree(sample())

    expect(labels(moveOutlineRowToIndex(rows, 'nope', 0))).toEqual(labels(rows))
  })
})

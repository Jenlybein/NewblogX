/**
 * 后台目录树编辑器的纯逻辑。
 *
 * 数据源是后端返回的**嵌套树**；编辑器把它拍成扁平行（缩进 + 层级 + 顺序），
 * 按钮式操作只改这份扁平行，保存时再组装回嵌套 payload。
 *
 * 为什么抽到 utils：树的结构错误不会报错——拍平丢了空文件夹、缩进挂到文章
 * 节点下、移出算错父级、移除漏掉子树，都会让保存后的树"看起来保存成功但
 * 层级不对"。这些语义与后端整树提交（SaveOutline）的结构校验逐条对应：
 * 页面负责产出正确的嵌套 payload，服务端负责校验与复用已有行。
 */

import type { OutlineNodeItem } from '~/types/api'

/** 节点类型：1 = 文件夹、2 = 文章（与后端 node kind 同一枚举）。 */
export type OutlineNodeKind = 1 | 2

/** 移动动作：上移 / 下移 / 缩进 / 移出。 */
export type OutlineMoveOp = 'up' | 'down' | 'indent' | 'outdent'

/** 扁平行：一段可渲染、可移动的树行。 */
export interface OutlineFlatRow {
  /** 稳定 key：已有节点用服务端 ID，新建节点用本地 tmp key。 */
  key: string
  /** 服务端 ID；新建节点为空串，保存后由服务端返回。 */
  id: string
  /** 父行 key；空串表示根层。 */
  parentKey: string
  kind: OutlineNodeKind
  /** 文件夹标题；文章节点为空串。 */
  title: string
  /** 文章 ID；文件夹节点为空串。 */
  articleId: string
  /** 文章标题快照；文件夹节点为空串。 */
  articleTitle: string
  /** 文章封面快照。 */
  cover: string
  /** 文章是否已失效（后端判定）；文件夹节点永远 false。 */
  stale: boolean
  staleReason: string
}

/** 整树提交里的一层节点（与后端 OutlineNodeRequest 对齐）。 */
export interface OutlineNodeInput {
  id: string
  kind: OutlineNodeKind
  title: string
  article_id: string
  children: OutlineNodeInput[]
}

/** 批量收录的候选文章（搜索结果的最小形状）。 */
export interface OutlineArticleCandidate {
  id: string
  title: string
  cover?: string
}

/** 后端返回的一棵树 → 扁平行（DFS 先序、兄弟保持服务端顺序）。 */
export function flattenOutlineTree(nodes: OutlineNodeItem[], parentKey = ''): OutlineFlatRow[] {
  const rows: OutlineFlatRow[] = []
  const walk = (items: OutlineNodeItem[], parent: string) => {
    for (const node of items) {
      const key = node.id || `tmp-${rows.length}`
      rows.push({
        key,
        id: node.id,
        parentKey: node.parent_id || parent,
        kind: node.kind === 2 ? 2 : 1,
        title: node.title,
        articleId: node.article_id,
        articleTitle: node.article_title,
        cover: node.article_cover,
        stale: node.stale,
        staleReason: node.stale_reason,
      })
      walk(node.children ?? [], key)
    }
  }
  walk(nodes, parentKey)
  return rows
}

/** 行在树里的深度（根层为 0）。父链缺失时按当前能算到的深度返回。 */
export function outlineDepth(rows: readonly OutlineFlatRow[], key: string): number {
  const byKey = new Map(rows.map(row => [row.key, row]))
  let depth = 0
  let cur = byKey.get(key)
  const seen = new Set<string>()
  while (cur && cur.parentKey && !seen.has(cur.key)) {
    seen.add(cur.key)
    cur = byKey.get(cur.parentKey)
    depth += 1
  }
  return depth
}

/** 行标题：文件夹用标题，文章用文章标题（拿不到时退回文章 ID）。 */
export function outlineRowTitle(row: OutlineFlatRow): string {
  if (row.kind === 1) return row.title
  return row.articleTitle || row.articleId
}

/** 选中行作为"收录目标"的文件夹 key：文件夹取自身，文章退回父文件夹。 */
export function outlineIncludeTargetKey(rows: readonly OutlineFlatRow[], key: string): string {
  const row = rows.find(item => item.key === key)
  if (!row) return ''
  return row.kind === 1 ? row.key : row.parentKey
}

/** 树里是否已挂载这篇文章（同树内唯一）。 */
export function hasOutlineArticle(rows: readonly OutlineFlatRow[], articleId: string): boolean {
  return rows.some(row => row.kind === 2 && row.articleId === articleId)
}

/** 新建文件夹的本地 key：与已有 key 不冲突。 */
export function newOutlineFolderKey(rows: readonly OutlineFlatRow[]): string {
  const used = new Set(rows.map(row => row.key))
  let n = 1
  while (used.has(`tmp-folder-${n}`)) n += 1
  return `tmp-folder-${n}`
}

/** 兄弟行的下标（同 parentKey，按扁平行顺序）。 */
function siblingIndices(rows: readonly OutlineFlatRow[], parentKey: string): number[] {
  const out: number[] = []
  rows.forEach((row, index) => {
    if (row.parentKey === parentKey) out.push(index)
  })
  return out
}

/** 行所在子树块的结束下标（不含）：下一个深度 ≤ 当前深度的行。 */
function blockEnd(rows: readonly OutlineFlatRow[], index: number): number {
  const depth = outlineDepth(rows, rows[index]!.key)
  for (let j = index + 1; j < rows.length; j += 1) {
    if (outlineDepth(rows, rows[j]!.key) <= depth) return j
  }
  return rows.length
}

/** 把子树块从原位置摘出，插到 insertAt（摘除前的位置）之后。 */
function insertBlockAfter(
  rows: readonly OutlineFlatRow[],
  index: number,
  end: number,
  insertAt: number,
  block: OutlineFlatRow[],
): OutlineFlatRow[] {
  const rest = [...rows.slice(0, index), ...rows.slice(end)]
  const at = insertAt > index ? insertAt - (end - index) : insertAt
  return [...rest.slice(0, at), ...block, ...rest.slice(at)]
}

/** 当前行能否执行该移动（页面据此禁用按钮）。 */
export function canMoveOutlineRow(
  rows: readonly OutlineFlatRow[],
  key: string,
  op: OutlineMoveOp,
): boolean {
  const index = rows.findIndex(row => row.key === key)
  if (index < 0) return false
  const row = rows[index]!
  const siblings = siblingIndices(rows, row.parentKey)
  const pos = siblings.indexOf(index)

  switch (op) {
    case 'up':
      return pos > 0
    case 'down':
      return pos >= 0 && pos < siblings.length - 1
    case 'indent': {
      if (pos <= 0) return false
      const prev = rows[siblings[pos - 1]!]!
      return prev.kind === 1
    }
    case 'outdent':
      return row.parentKey !== ''
    default:
      return false
  }
}

/** 执行一次按钮式移动，返回新数组；不可能时原样返回（不动入参）。 */
export function moveOutlineRow(
  rows: readonly OutlineFlatRow[],
  key: string,
  op: OutlineMoveOp,
): OutlineFlatRow[] {
  const index = rows.findIndex(row => row.key === key)
  if (index < 0 || !canMoveOutlineRow(rows, key, op)) return [...rows]

  const row = rows[index]!
  const siblings = siblingIndices(rows, row.parentKey)
  const pos = siblings.indexOf(index)
  const end = blockEnd(rows, index)

  if (op === 'up') {
    const prev = siblings[pos - 1]!
    const prevBlock = rows.slice(prev, index)
    return [...rows.slice(0, prev), ...rows.slice(index, end), ...prevBlock, ...rows.slice(end)]
  }
  if (op === 'down') {
    const next = siblings[pos + 1]!
    const nextEnd = blockEnd(rows, next)
    return [...rows.slice(0, index), ...rows.slice(end, nextEnd), ...rows.slice(index, end), ...rows.slice(nextEnd)]
  }
  if (op === 'indent') {
    const prevIndex = siblings[pos - 1]!
    const prev = rows[prevIndex]!
    const prevEnd = blockEnd(rows, prevIndex)
    const block = rows.slice(index, end).map(item => ({ ...item, parentKey: prev.key }))
    return insertBlockAfter(rows, index, end, prevEnd, block)
  }

  // outdent：父节点的父节点（父节点必然存在，canMove 已判定 parentKey 非空）。
  const parentIndex = rows.findIndex(item => item.key === row.parentKey)
  const parent = rows[parentIndex]!
  const parentEnd = blockEnd(rows, parentIndex)
  const block = rows.slice(index, end).map(item => ({ ...item, parentKey: parent.parentKey }))
  return insertBlockAfter(rows, index, end, parentEnd, block)
}

/**
 * 按 parentKey 重新归拢为 DFS 先序。
 *
 * 新建节点总是先追加到数组末尾；不归拢的话，它会显示在所有兄弟之后，
 * 保存时虽然凭 parentKey 仍能挂对，但界面顺序与提交顺序会分叉。
 */
export function regroupOutlineRows(rows: readonly OutlineFlatRow[]): OutlineFlatRow[] {
  const children = new Map<string, OutlineFlatRow[]>()
  for (const row of rows) {
    const list = children.get(row.parentKey) ?? []
    list.push(row)
    children.set(row.parentKey, list)
  }
  const out: OutlineFlatRow[] = []
  const seen = new Set<string>()
  const walk = (parentKey: string) => {
    for (const row of children.get(parentKey) ?? []) {
      if (seen.has(row.key)) continue
      seen.add(row.key)
      out.push(row)
      walk(row.key)
    }
  }
  walk('')
  // 父节点缺失 / 成环的行没有走到：原样保留在末尾，保存时服务端会拒绝，
  // 界面至少不会把它们静默丢掉。
  for (const row of rows) {
    if (!seen.has(row.key)) out.push(row)
  }
  return out
}

/** 重命名文件夹：文章节点没有自己的标题，空标题与不存在的行不改。 */
export function renameOutlineFolder(
  rows: readonly OutlineFlatRow[],
  key: string,
  title: string,
): OutlineFlatRow[] {
  const row = rows.find(item => item.key === key)
  const trimmed = title.trim()
  if (!row || row.kind !== 1 || !trimmed) return [...rows]
  return rows.map(item => (item.key === key ? { ...item, title: trimmed } : item))
}

/** 在指定文件夹（或根层）末尾新建一个空文件夹。 */
export function addOutlineFolder(
  rows: readonly OutlineFlatRow[],
  parentKey: string,
  title: string,
): OutlineFlatRow[] {
  const parent = rows.find(row => row.key === parentKey)
  const effectiveParent = parent && parent.kind === 1 ? parentKey : ''
  const row: OutlineFlatRow = {
    key: newOutlineFolderKey(rows),
    id: '',
    parentKey: effectiveParent,
    kind: 1,
    title,
    articleId: '',
    articleTitle: '',
    cover: '',
    stale: false,
    staleReason: '',
  }
  return regroupOutlineRows([...rows, row])
}

/** 把一批文章挂到指定文件夹（或根层），已在树里的跳过。 */
export function addOutlineArticles(
  rows: readonly OutlineFlatRow[],
  parentKey: string,
  articles: readonly OutlineArticleCandidate[],
): OutlineFlatRow[] {
  const parent = rows.find(row => row.key === parentKey)
  const effectiveParent = parent && parent.kind === 1 ? parentKey : ''
  const added: OutlineFlatRow[] = []
  for (const article of articles) {
    if (hasOutlineArticle([...rows, ...added], article.id)) continue
    added.push({
      key: `tmp-article-${article.id}`,
      id: '',
      parentKey: effectiveParent,
      kind: 2,
      title: '',
      articleId: article.id,
      articleTitle: article.title,
      cover: article.cover ?? '',
      stale: false,
      staleReason: '',
    })
  }
  return regroupOutlineRows([...rows, ...added])
}

/**
 * 移除一行及其整棵子树：文件夹里的文章 / 子文件夹不能变成孤儿。
 *
 * 保存时不出现在提交里的节点由服务端软删（SaveOutline），所以这里只需要
 * 把子树从扁平行里摘干净；入参不改，找不到的行原样返回。
 */
export function removeOutlineRow(
  rows: readonly OutlineFlatRow[],
  key: string,
): OutlineFlatRow[] {
  const index = rows.findIndex(row => row.key === key)
  if (index < 0) return [...rows]
  const end = blockEnd(rows, index)
  return [...rows.slice(0, index), ...rows.slice(end)]
}

/**
 * 失效行数量（只有文章节点会失效）。
 *
 * 判定来自后端读模型：文章后来被删 / 退回草稿 / 隐藏时节点保留并标失效，
 * 前端不另算一遍"文章现在还公开吗"。
 */
export function outlineStaleCount(rows: readonly OutlineFlatRow[]): number {
  return rows.filter(row => row.kind === 2 && row.stale).length
}

/** 一键清理失效行：只摘掉已失效的文章节点，展示中的文章与文件夹不动。 */
export function cleanupOutlineStaleRows(rows: readonly OutlineFlatRow[]): OutlineFlatRow[] {
  return rows.filter(row => !(row.kind === 2 && row.stale))
}

/** 失效原因文案表（与后端 domain.StaleReason 同一组枚举值）。 */
const STALE_REASON_LABELS: Record<string, string> = {
  '': '展示中',
  missing: '文章已删除',
  deleted: '文章已删除',
  not_published: '文章已不是已发布状态',
  not_visible: '文章已不可见',
}

/**
 * 失效原因文案（子节点标「已失效（原因）」时用）。
 *
 * 判定本身来自后端读模型（`stale` / `stale_reason`），前端只做文案映射——
 * 不在这里另算一次"文章现在还公开吗"，那会与后端分叉。
 * 未知枚举给兜底文案而不是空白：后端将来新增一种失效原因时，运营仍能看出
 * 这一行不正常，而不是看到一个空括号。
 *
 * 原住在 `utils/staleItems.ts`；旧扁平收录接口退役后那里只剩这一个宿主，
 * 于是随 #225 迁进本模块，旧文件删除。
 */
export function staleReasonLabel(reason: string | null | undefined): string {
  return STALE_REASON_LABELS[reason ?? ''] ?? '已失效'
}

/**
 * 拖拽落点 → 本地扁平行（决定清单 §8 第 37 条）。
 *
 * 语义只有两条，都是**按"落点上方的行"决定**（与常见树列表一致，也不需要在拖拽
 * 过程中维护悬停判定）：
 *
 *  - 上方是**文件夹** → 被拖的行成为它的**第一个子行**；
 *  - 上方是**文章**或落点在首位 → 被拖的行成为它的**兄弟**（同级、紧随其后）。
 *
 * 被拖的是一个**块**：文件夹连同它的子树一起移动，子树内部的 parentKey 不变。
 * 落点若落在自己的块内（等于"移到自己身上"）直接返回原数组。
 *
 * 顺序与层级仍然只是本地扁平行，保存时才整树一次性提交（ADR-0023 约束 7 不变）。
 */
export function moveOutlineRowToIndex(
  rows: readonly OutlineFlatRow[],
  key: string,
  targetIndex: number,
): OutlineFlatRow[] {
  const index = rows.findIndex(row => row.key === key)
  if (index < 0) return [...rows]

  const end = blockEnd(rows, index)
  // 落在自己的块里：没有可表达的移动。
  if (targetIndex > index && targetIndex < end) return [...rows]

  const block = rows.slice(index, end)
  const rest = [...rows.slice(0, index), ...rows.slice(end)]
  // 目标下标是"含被拖块"的坐标系 → 换算到"已移除被拖块"的坐标系。
  const rawInsert = targetIndex > index ? targetIndex - block.length : targetIndex
  const insertAt = Math.max(0, Math.min(rawInsert, rest.length))

  const above = insertAt > 0 ? rest[insertAt - 1] : undefined
  const parentKey = !above ? '' : above.kind === 1 ? above.key : above.parentKey

  const moved = block.map((row, offset) => (offset === 0 ? { ...row, parentKey } : row))
  const next = [...rest.slice(0, insertAt), ...moved, ...rest.slice(insertAt)]
  return regroupOutlineRows(next)
}

/** 扁平行 → 嵌套提交 payload：兄弟顺序 = 数组顺序，新节点 id 留空。 */
export function buildOutlineNodes(rows: readonly OutlineFlatRow[]): OutlineNodeInput[] {
  const children = new Map<string, OutlineFlatRow[]>()
  for (const row of rows) {
    const list = children.get(row.parentKey) ?? []
    list.push(row)
    children.set(row.parentKey, list)
  }
  const seen = new Set<string>()
  const build = (parentKey: string): OutlineNodeInput[] => {
    const items = children.get(parentKey) ?? []
    const out: OutlineNodeInput[] = []
    for (const row of items) {
      if (seen.has(row.key)) continue
      seen.add(row.key)
      out.push({
        id: row.id,
        kind: row.kind,
        title: row.kind === 1 ? row.title : '',
        article_id: row.kind === 2 ? row.articleId : '',
        children: build(row.key),
      })
    }
    return out
  }
  return build('')
}

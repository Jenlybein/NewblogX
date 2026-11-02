/**
 * 后台侧边栏的导航清单（**唯一的一份**）。
 *
 * 为什么抽到 utils 而不是留在 AdminShell.vue 里：
 * 菜单里每个 path 必须有一个真实存在的页面文件 —— 否则点进去就是 404，而这件事
 * 没有任何东西会报错。上一轮就踩过：侧边栏挂着「日志中心 → /logs」，
 * 而 `app/pages/logs/` 根本不存在（12 §8.6）。
 *
 * 菜单的**唯一来源**是一棵 `AdminNavNode` 树（支持两级：父项可只分组、没有自己的页面），
 * 对外同时暴露两种视图：
 *   - `ADMIN_NAV_TREE`：结构数据（父子归属、"只分组不下钻"的父项）；
 *   - `FLAT_NAV`：`flattenNav()` 产出的一维视图（标题 / 路由 / 图标 / 父级归属 / 深度）；
 *   - `buildAdminNavMenu()`：侧栏渲染视图（分组、展开态、父项随子项高亮）。
 *
 * 四条结构对账（页面文件存在、页面有入口、路径唯一、热门置顶位置）对 `FLAT_NAV`
 * 断言（见 test/unit/menu.test.ts）。这样一旦菜单出现子项，子项不会被静默漏检，
 * 位置断言也不会因为"父项 + 子项"的插入而假红。
 *
 * icon 用字符串而不是组件：utils 层不该 import Vue 组件，
 * 组件层再把它映射成具体图标（见 AdminNavTree.vue 的 ICONS）。
 */
export type AdminNavIcon =
  | 'chart'
  | 'shield'
  | 'article'
  | 'pin'
  | 'users'
  | 'settings'
  | 'photo'
  | 'bell'
  | 'database'
  | 'alert'

/** 菜单树节点。只负责分组的父项可以没有 `path`（例如站点配置拆域后的父项）。 */
export interface AdminNavNode {
  label: string
  /** 自身页面的绝对路径；分组父项可省略，此时它不渲染页面也不算可导航项。 */
  path?: string
  icon: AdminNavIcon
  children?: AdminNavNode[]
}

/** 扁平 UI 清单里的一项（AdminShell 直接消费）。 */
export interface AdminNavItem {
  label: string
  path: string
  icon: AdminNavIcon
}

/**
 * 拍平视图的一项：路由 + 标题 + 父级归属。
 *
 * `parentPath` 是**直接父级**的路径；父级只分组时为空。需要完整祖先链
 * （面包屑、父项展开）时用 `navTrail()`。
 */
export interface AdminNavFlatItem {
  label: string
  path?: string
  icon: AdminNavIcon
  parentLabel: string | null
  parentPath: string | null
  depth: number
  /** 是否在树里带子项（用来判定分组：分组可能没有 path，不能只看 path）。 */
  hasChildren: boolean
}

/** 菜单树（唯一来源）。父项 + 子项构成两级；父项自身仍可有页面。 */
export const ADMIN_NAV_TREE: AdminNavNode[] = [
  { label: '数据概览', path: '/', icon: 'chart' },
  { label: '文章审核', path: '/review', icon: 'shield' },
  {
    label: '文章管理',
    path: '/articles',
    icon: 'article',
    children: [
      // 热门置顶是独立运营位（ADR-0017 决策 5），层级上归到「文章管理」：先选文章，再排序。
      { label: '热门置顶', path: '/articles/hot-top', icon: 'pin' },
    ],
  },
  { label: '用户管理', path: '/users', icon: 'users' },
  { label: '站点配置', path: '/site', icon: 'settings' },
  { label: '媒体运营', path: '/media', icon: 'photo' },
  { label: '全局通知', path: '/notifications', icon: 'bell' },
  {
    label: '日志中心',
    path: '/logs',
    icon: 'database',
    children: [
      { label: '同步死信', path: '/logs/dead-letter', icon: 'alert' },
    ],
  },
]

/**
 * 把菜单树拍平成一维视图（深度优先、父项在前）。
 *
 * 子项紧跟在父项之后，因此"请求顺序"与两级化之前的扁平清单保持一致 —— 这正是
 * 位置类断言可以在拍平视图上继续成立的原因。
 */
export function flattenNav(
  nodes: AdminNavNode[],
  depth = 0,
  parent: AdminNavNode | null = null,
): AdminNavFlatItem[] {
  const flat: AdminNavFlatItem[] = []

  for (const node of nodes) {
    flat.push({
      label: node.label,
      path: node.path,
      icon: node.icon,
      parentLabel: parent?.label ?? null,
      parentPath: parent?.path ?? null,
      depth,
      hasChildren: (node.children?.length ?? 0) > 0,
    })

    if (node.children && node.children.length > 0) {
      flat.push(...flattenNav(node.children, depth + 1, node))
    }
  }

  return flat
}

/** 拍平视图：菜单项 → 路由 → 标题 → 父级归属的唯一映射。 */
export const FLAT_NAV: AdminNavFlatItem[] = flattenNav(ADMIN_NAV_TREE)

/**
 * 现有侧边栏的扁平清单，直接从拍平视图投影而来。
 *
 * `ADMIN_NAV` 只包含**可导航项**（有 path 的项）；分组父项会出现在 `FLAT_NAV` 里，
 * 但不会被渲染成链接。改造前后该清单逐项一致（标题 / 路由 / 图标 / 顺序）。
 */
export const ADMIN_NAV: AdminNavItem[] = FLAT_NAV
  .filter((item): item is AdminNavFlatItem & { path: string } => item.path !== undefined)
  .map(({ label, path, icon }) => ({ label, path, icon }))

/** 按当前路由查激活项（高亮的唯一来源）。只认有页面的项；精确匹配，未命中返回 undefined。 */
export function navItemByPath(
  path: string,
  flat: AdminNavFlatItem[] = FLAT_NAV,
): AdminNavFlatItem | undefined {
  if (!path) return undefined
  return flat.find(item => item.path === path)
}

/**
 * 从路由反查祖先链（根 → 父，不含自身）。
 *
 * 拍平视图是深度优先先序：某深度为 d 的项，它之前最近一个深度 d-1 的项就是它的父项。
 * 因此即使父项只分组（没有 path），也能从扁平列表还原层级。
 */
export function navTrail(path: string, flat: AdminNavFlatItem[] = FLAT_NAV): AdminNavFlatItem[] {
  const index = flat.findIndex(item => item.path !== undefined && item.path === path)
  if (index < 0) return []

  const trail: AdminNavFlatItem[] = []
  let depth = flat[index]!.depth

  for (let i = index - 1; i >= 0 && depth > 0; i -= 1) {
    const item = flat[i]!
    if (item.depth === depth - 1) {
      trail.unshift(item)
      depth -= 1
    }
  }

  return trail
}

/**
 * 面包屑的一项（根 → 当前）。
 *
 * `path` 是"点了会去哪"，所以只有**可点回退**的祖先才有值：
 *   - 当前项：`current: true`、`path: null`（当前页不需要再点一次）；
 *   - 只分组的父项：`path: null`（没有落地页，点它只会跳空页）；
 *   - 其余祖先：`path` 为分组自身的页面。
 */
export interface AdminBreadcrumbItem {
  label: string
  path: string | null
  current: boolean
}

/**
 * 从路由推导面包屑：祖先链来自 `navTrail()`，当前项来自 `navItemByPath()`。
 *
 * 为什么不在这里再建一份层级：菜单与面包屑必须讲同一个层级故事，
 * 否则深链接进入时菜单高亮与面包屑会各说各话。拍平视图是唯一来源。
 *
 * 未知路由返回空数组（不撒谎造一个不存在的祖先）；空的面包屑由消费方决定不渲染。
 */
export function buildAdminBreadcrumb(
  path: string,
  flat: AdminNavFlatItem[] = FLAT_NAV,
): AdminBreadcrumbItem[] {
  const current = navItemByPath(path, flat)
  if (!current) return []

  return [
    ...navTrail(path, flat).map(item => ({
      label: item.label,
      path: item.path ?? null,
      current: false,
    })),
    { label: current.label, path: null, current: true },
  ]
}

/** 分组节点的稳定 key：分组可能没有页面，不能用 path 当 key。 */
export function navGroupKey(node: Pick<AdminNavNode, 'label'>): string {
  return `group:${node.label}`
}

/**
 * 当前路由所在的所有祖先分组 key（根 → 父）。
 *
 * 只服务于展开态；高亮仍以精确 path 命中的激活项为准。
 */
export function activeGroupKeys(
  path: string,
  flat: AdminNavFlatItem[] = FLAT_NAV,
): string[] {
  // 祖先链上的每一项都是分组：没有分组的父项就不会有子项挂在它下面。
  return navTrail(path, flat).map(item => navGroupKey(item))
}

/**
 * 路由变化时结算展开态。
 *
 * 同一路由返回原数组：用户手动收起的分组不会被"含激活项默认展开"反弹。
 * 换路由时把新激活项的祖先分组并入，已展开的其它分组保持不动。
 */
export function reconcileExpandedGroups(
  activePath: string,
  previousPath: string | null,
  expandedKeys: readonly string[],
  flat: AdminNavFlatItem[] = FLAT_NAV,
): string[] {
  if (previousPath !== null && activePath === previousPath) {
    return [...expandedKeys]
  }
  return [...new Set([...expandedKeys, ...activeGroupKeys(activePath, flat)])]
}

/** 侧栏渲染视图里的可导航链接（子项或顶层单页）。 */
export interface AdminNavMenuLink {
  kind: 'link'
  key: string
  label: string
  path: string
  icon: AdminNavIcon
  depth: number
  /** 精确命中当前路由。 */
  active: boolean
}

/** 侧栏渲染视图里的分组（父项可无 self path；带自身 path 时标题本身也是链接）。 */
export interface AdminNavMenuGroup {
  kind: 'group'
  key: string
  label: string
  icon: AdminNavIcon
  /** 分组自身的页面；只分组时为 null（标题不渲染成链接，不跳空页）。 */
  path: string | null
  depth: number
  /** 自身页面命中，或任一子孙命中当前路由。 */
  active: boolean
  /** 由展开态集合决定，不在渲染层根据激活项强制展开。 */
  expanded: boolean
  children: AdminNavMenuEntry[]
}

export type AdminNavMenuEntry = AdminNavMenuLink | AdminNavMenuGroup

function buildMenuEntry(
  node: AdminNavNode,
  activePath: string,
  expandedKeys: ReadonlySet<string>,
  depth: number,
): AdminNavMenuEntry | null {
  const childrenNodes = node.children ?? []

  if (childrenNodes.length === 0) {
    // 没有页面也没有子项的节点在菜单里没有意义，不渲染。
    if (node.path === undefined) return null
    return {
      kind: 'link',
      key: node.path,
      label: node.label,
      path: node.path,
      icon: node.icon,
      depth,
      active: node.path === activePath,
    }
  }

  const children = childrenNodes
    .map(child => buildMenuEntry(child, activePath, expandedKeys, depth + 1))
    .filter((entry): entry is AdminNavMenuEntry => entry !== null)
  const key = navGroupKey(node)

  return {
    kind: 'group',
    key,
    label: node.label,
    icon: node.icon,
    path: node.path ?? null,
    depth,
    active: node.path === activePath || children.some(child => child.active),
    expanded: expandedKeys.has(key),
    children,
  }
}

/**
 * 把菜单树投影成侧栏渲染视图。
 *
 * 高亮规则：当前路由精确命中的子项 active；其父分组也 active（子孙命中即亮）。
 * 展开规则：只认 `expandedKeys`，默认展开由 `activeGroupKeys` /
 * `reconcileExpandedGroups` 负责，渲染层不会因为子项仍激活而反弹用户手动收起的分组。
 */
export function buildAdminNavMenu(
  tree: AdminNavNode[],
  activePath: string,
  expandedKeys: readonly string[] = [],
): AdminNavMenuEntry[] {
  const expanded = new Set(expandedKeys)
  return tree
    .map(node => buildMenuEntry(node, activePath, expanded, 0))
    .filter((entry): entry is AdminNavMenuEntry => entry !== null)
}

/** 四条结构对账的结果；数组为空 / `hotTopOrder` 为 null 表示通过。 */
export interface AdminNavReconcile {
  /** 可导航菜单项找不到页面文件：「<标题> <path>」。 */
  missingPageFiles: string[]
  /** 有页面文件、但拍平视图里没有任何可导航项指向它。 */
  unreachablePages: string[]
  /** 在拍平视图里出现不止一次的路由。 */
  duplicatePaths: string[]
  /** 不是以 `/` 开头的路由。 */
  nonAbsolutePaths: string[]
  /** 位置约定（ADR-0017 决策 5）违反说明；通过时为 null。 */
  hotTopOrder: string | null
}

/** 不重复时返回空数组；重复路径按去重后的集合顺序返回。 */
function findDuplicates(paths: string[]): string[] {
  const seen = new Map<string, number>()
  for (const path of paths) {
    seen.set(path, (seen.get(path) ?? 0) + 1)
  }
  return [...seen.entries()].filter(([, count]) => count > 1).map(([path]) => path)
}

/** ADR-0017 决策 5：拍平后「热门置顶」必须紧跟「文章管理」，且路径固定。 */
function hotTopOrderProblem(nav: AdminNavFlatItem[]): string | null {
  const articleIndex = nav.findIndex(item => item.label === '文章管理')
  if (articleIndex < 0) return '菜单里必须有「文章管理」'

  const hotTopIndex = nav.findIndex(item => item.label === '热门置顶')
  if (hotTopIndex < 0) return '菜单里必须有「热门置顶」'
  if (hotTopIndex !== articleIndex + 1) return '「热门置顶」必须紧跟在「文章管理」之后'

  const hotTop = nav[hotTopIndex]!
  if (hotTop.path !== '/articles/hot-top') {
    return '「热门置顶」的路径应是 /articles/hot-top'
  }
  return null
}

/**
 * 四条结构对账：在拍平视图上做，子项不会再被静默漏检。
 *
 * 约定：`login` / `setup` 是"不该出现在菜单里"而不是"忘了加菜单"，反向对账时排除。
 */
export function reconcileAdminNav(
  flat: AdminNavFlatItem[],
  pageFiles: string[],
): AdminNavReconcile {
  const navigable = flat.filter(
    (item): item is AdminNavFlatItem & { path: string } => item.path !== undefined,
  )

  const missingPageFiles = navigable
    .filter(item => !pageFileCandidates(item.path).some(file => pageFiles.includes(file)))
    .map(item => `${item.label} ${item.path}`)

  const reachable = new Set<string>()
  for (const item of navigable) {
    for (const candidate of pageFileCandidates(item.path)) {
      reachable.add(candidate)
    }
  }

  const unreachablePages = pageFiles.filter(
    file => file !== 'login' && file !== 'setup' && !reachable.has(file),
  )

  return {
    missingPageFiles,
    unreachablePages,
    duplicatePaths: findDuplicates(navigable.map(item => item.path)),
    nonAbsolutePaths: navigable.filter(item => !item.path.startsWith('/')).map(item => item.path),
    hotTopOrder: hotTopOrderProblem(navigable),
  }
}

/**
 * 把菜单 path 映射成"页面文件的相对路径候选"（相对 app/pages，不含 .vue 后缀）。
 *
 * Nuxt 的规则：`/x` → `x.vue` 或 `x/index.vue`；根路径 `/` → `index.vue`。
 * 返回候选而不是唯一答案，是因为两种写法都合法，测试只关心"至少存在一个"。
 */
export function pageFileCandidates(path: string): string[] {
  const clean = String(path ?? '').trim()
  if (clean === '' || clean === '/') {
    return ['index']
  }
  const trimmed = clean.replace(/^\/+|\/+$/g, '')
  if (!trimmed) {
    return ['index']
  }
  return [trimmed, `${trimmed}/index`]
}

import { readdirSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  ADMIN_NAV,
  ADMIN_NAV_TREE,
  FLAT_NAV,
  activeGroupKeys,
  buildAdminNavMenu,
  flattenNav,
  navItemByPath,
  navTrail,
  pageFileCandidates,
  reconcileAdminNav,
  reconcileExpandedGroups,
} from '../../app/utils/menu'
import type { AdminNavNode } from '../../app/utils/menu'

/**
 * 侧边栏菜单 ↔ 页面对账，以及菜单的**拍平视图**（#106）。
 *
 * 起因：上一轮侧边栏一直挂着「日志中心 → /logs」，而 `app/pages/logs/` 根本不存在，
 * 点进去 404。没有任何检查会报错 —— 入口存在、目标不存在，只有用户点得到。
 *
 * #106 之前四条对账全部直接遍历 `ADMIN_NAV`；一旦菜单出现子项，它们会同时
 * 失准（位置断言立刻失败、父子项静默漏检）。所以对账改为对**拍平视图**断言：
 * 树怎么分组都行，拍平后每个可导航项都必须被覆盖。
 */

// 用 cwd 而不是 import.meta.url：vitest 在 happy-dom 环境下 import.meta.url
// 不是 file:// 协议，fileURLToPath 会直接抛错。测试由包根的 pnpm test 触发，
// cwd 稳定是 frontend_admin/。
const pagesDir = join(process.cwd(), 'app', 'pages')

/** 递归收集 app/pages 下所有 .vue，返回相对路径（不含扩展名、统一用 /）。 */
function collectPageFiles(dir: string, prefix = ''): string[] {
  const out: string[] = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const rel = prefix ? `${prefix}/${entry.name}` : entry.name
    if (entry.isDirectory()) {
      out.push(...collectPageFiles(join(dir, entry.name), rel))
      continue
    }
    if (entry.name.endsWith('.vue')) {
      out.push(rel.slice(0, -'.vue'.length))
    }
  }
  return out
}

/** 改造前的扁平清单，逐字冻结：拍平视图不许改动任何一项的标题 / 路由 / 图标 / 顺序。 */
const LEGACY_ADMIN_NAV = [
  { label: '数据概览', path: '/', icon: 'chart' },
  { label: '文章审核', path: '/review', icon: 'shield' },
  { label: '文章管理', path: '/articles', icon: 'article' },
  { label: '热门置顶', path: '/articles/hot-top', icon: 'pin' },
  { label: '用户管理', path: '/users', icon: 'users' },
  { label: '站点配置', path: '/site', icon: 'settings' },
  { label: '媒体运营', path: '/media', icon: 'photo' },
  { label: '全局通知', path: '/notifications', icon: 'bell' },
  { label: '日志中心', path: '/logs', icon: 'database' },
  { label: '同步死信', path: '/logs/dead-letter', icon: 'alert' },
]

const pageFiles = collectPageFiles(pagesDir)

describe('后台菜单拍平视图', () => {
  it('拍平视图与改造前的扁平清单逐项一致（标题 / 路由 / 图标 / 顺序）', () => {
    expect(ADMIN_NAV).toEqual(LEGACY_ADMIN_NAV)

    const projected = FLAT_NAV.map(({ label, path, icon }) => ({ label, path, icon }))
    expect(projected).toEqual(LEGACY_ADMIN_NAV)

    // 两级化后拍平顺序不变：父项仍在原位、子项紧跟父项，所以上面的逐项冻结继续成立。
    // 语义子项下沉一级；其余项仍是顶层。
    expect(FLAT_NAV.find(item => item.label === '热门置顶')).toMatchObject({
      parentLabel: '文章管理',
      parentPath: '/articles',
      depth: 1,
    })
    expect(FLAT_NAV.find(item => item.label === '同步死信')).toMatchObject({
      parentLabel: '日志中心',
      parentPath: '/logs',
      depth: 1,
    })
    expect(FLAT_NAV.filter(item => item.depth === 0).map(item => item.label)).toEqual([
      '数据概览',
      '文章审核',
      '文章管理',
      '用户管理',
      '站点配置',
      '媒体运营',
      '全局通知',
      '日志中心',
    ])
  })

  it('flattenNav 把子项排在父项之后，并记录父级归属与深度', () => {
    const tree: AdminNavNode[] = [
      {
        label: '文章管理',
        path: '/articles',
        icon: 'article',
        children: [
          { label: '热门置顶', path: '/articles/hot-top', icon: 'pin' },
          {
            // 只分组的父项：没有自身页面，但仍是子项的父级。
            label: '合集',
            icon: 'article',
            children: [{ label: '合集编排', path: '/collections', icon: 'article' }],
          },
        ],
      },
      { label: '日志中心', path: '/logs', icon: 'database' },
    ]

    const flat = flattenNav(tree)
    expect(flat.map(item => item.label)).toEqual([
      '文章管理',
      '热门置顶',
      '合集',
      '合集编排',
      '日志中心',
    ])

    const hotTop = flat.find(item => item.label === '热门置顶')!
    expect(hotTop).toMatchObject({
      path: '/articles/hot-top',
      parentLabel: '文章管理',
      parentPath: '/articles',
      depth: 1,
    })

    const collection = flat.find(item => item.label === '合集编排')!
    expect(collection).toMatchObject({ parentLabel: '合集', parentPath: null, depth: 2 })

    const group = flat.find(item => item.label === '合集')!
    expect(group.path).toBeUndefined()
    expect(group.children).toBeUndefined()

    expect(flat.find(item => item.label === '日志中心')!.depth).toBe(0)
  })

  it('flattenNav 对空树 / 空子项返回空列表', () => {
    expect(flattenNav([])).toEqual([])
    const onlyGroup: AdminNavNode = { label: '空分组', icon: 'settings', children: [] }
    expect(flattenNav([onlyGroup]).map(item => item.label)).toEqual(['空分组'])
  })

  it('navItemByPath 按路由查激活项；没有页面就不算激活，未知路径返回 undefined', () => {
    expect(navItemByPath('/')?.label).toBe('数据概览')
    expect(navItemByPath('/articles/hot-top')?.label).toBe('热门置顶')
    expect(navItemByPath('/site/deco')).toBeUndefined()
    expect(navItemByPath('')).toBeUndefined()

    // 拍平后的子项也能被查中（两级化后高亮照旧走这一条缝）。
    const flat = flattenNav([
      {
        label: '站点配置',
        icon: 'settings',
        children: [{ label: '装饰', path: '/site/deco', icon: 'settings' }],
      },
    ])
    expect(navItemByPath('/site/deco', flat)?.label).toBe('装饰')
    // 只分组的父项不是可导航项。
    expect(navItemByPath('/site', flat)).toBeUndefined()
  })

  it('navTrail 返回根到父的层级链（不含自身），顶层项为空', () => {
    const flat = flattenNav([
      {
        label: '文章管理',
        path: '/articles',
        icon: 'article',
        children: [{ label: '热门置顶', path: '/articles/hot-top', icon: 'pin' }],
      },
      {
        label: '站点配置',
        icon: 'settings',
        children: [
          {
            label: '装饰',
            path: '/site/deco',
            icon: 'settings',
            children: [{ label: '光标', path: '/site/deco/cursor', icon: 'settings' }],
          },
        ],
      },
    ])

    expect(navTrail('/articles/hot-top', flat).map(item => item.label)).toEqual(['文章管理'])
    expect(navTrail('/site/deco/cursor', flat).map(item => item.label)).toEqual(['站点配置', '装饰'])
    expect(navTrail('/articles', flat)).toEqual([])
    expect(navTrail('/not-a-page', flat)).toEqual([])

    // 隔着兄弟节点也能找回父链（不会把前面的兄弟当父项）。
    const withSibling = flattenNav([
      {
        label: '文章管理',
        path: '/articles',
        icon: 'article',
        children: [
          { label: '热门置顶', path: '/articles/hot-top', icon: 'pin' },
          {
            label: '合集',
            icon: 'article',
            children: [{ label: '合集编排', path: '/collections', icon: 'article' }],
          },
        ],
      },
    ])
    expect(navTrail('/collections', withSibling).map(item => item.label)).toEqual([
      '文章管理',
      '合集',
    ])
  })
})

describe('后台菜单两级视图（分组 / 展开 / 高亮 / 标题）', () => {
  it('分组渲染：父项标题 + 子项链接，子项缩进一级', () => {
    const entries = buildAdminNavMenu(ADMIN_NAV_TREE, '/articles/hot-top', ['group:文章管理'])
    const group = entries.find(entry => entry.label === '文章管理')
    if (group?.kind !== 'group') throw new Error('「文章管理」应渲染成分组')

    expect(group).toMatchObject({
      kind: 'group',
      key: 'group:文章管理',
      path: '/articles',
      active: true,
      expanded: true,
      depth: 0,
    })
    expect(group.children).toEqual([
      expect.objectContaining({
        kind: 'link',
        key: '/articles/hot-top',
        label: '热门置顶',
        path: '/articles/hot-top',
        depth: 1,
        active: true,
      }),
    ])

    // 顶层单页仍是可导航链接，不会被包成组。
    expect(entries.find(entry => entry.label === '数据概览')).toMatchObject({
      kind: 'link',
      path: '/',
      active: false,
    })
  })

  it('父项没有自身页面时是纯分组：不产出可跳转 path，也不自动合成子项', () => {
    const tree: AdminNavNode[] = [
      {
        label: '站点配置',
        icon: 'settings',
        children: [{ label: '装饰', path: '/site/deco', icon: 'settings' }],
      },
    ]

    const entries = buildAdminNavMenu(tree, '/site/deco', ['group:站点配置'])
    const group = entries[0]
    if (group?.kind !== 'group') throw new Error('没有页面的父项应渲染成分组')

    expect(group.path).toBeNull()
    expect(group.active).toBe(true)
    expect(group.children).toHaveLength(1)
    expect(group.children[0]).toMatchObject({ path: '/site/deco', depth: 1, active: true })
  })

  it('无页面且无子项的节点不会渲染成死链（父分组里的空子项也会被剔除）', () => {
    const entries = buildAdminNavMenu(
      [
        { label: '数据概览', path: '/', icon: 'chart' },
        { label: '空节点', icon: 'settings' },
        {
          label: '站点配置',
          icon: 'settings',
          children: [
            { label: '空子项', icon: 'settings' },
            { label: '装饰', path: '/site/deco', icon: 'settings' },
          ],
        },
      ],
      '/',
      [],
    )

    expect(entries.map(entry => entry.label)).toEqual(['数据概览', '站点配置'])
    const group = entries[1]
    if (group?.kind !== 'group') throw new Error('「站点配置」应渲染成分组')
    expect(group.children.map(child => child.label)).toEqual(['装饰'])
  })

  it('深链接到子项时含激活项的分组默认展开', () => {
    expect(activeGroupKeys('/articles/hot-top')).toEqual(['group:文章管理'])
    expect(activeGroupKeys('/logs/dead-letter')).toEqual(['group:日志中心'])
    expect(activeGroupKeys('/')).toEqual([])
    expect(activeGroupKeys('/not-a-page')).toEqual([])

    const expanded = reconcileExpandedGroups('/articles/hot-top', null, [])
    expect(expanded).toEqual(['group:文章管理'])

    const group = buildAdminNavMenu(ADMIN_NAV_TREE, '/articles/hot-top', expanded).find(
      entry => entry.label === '文章管理',
    )
    expect(group?.kind === 'group' ? group.expanded : null).toBe(true)
  })

  it('用户手动收起后不会被高亮强制反弹', () => {
    const expanded = reconcileExpandedGroups('/articles/hot-top', null, [])
    const afterCollapse = expanded.filter(key => key !== 'group:文章管理')

    // 同一路由再次结算展开态（例如重渲染 / 子项仍激活）：保持用户的选择。
    expect(reconcileExpandedGroups('/articles/hot-top', '/articles/hot-top', afterCollapse)).toEqual([])

    const group = buildAdminNavMenu(ADMIN_NAV_TREE, '/articles/hot-top', afterCollapse).find(
      entry => entry.label === '文章管理',
    )
    expect(group?.kind === 'group' ? group.expanded : null).toBe(false)
  })

  it('路由切到另一组时只并入新组的祖先，不清掉已展开的组', () => {
    expect(
      reconcileExpandedGroups('/logs/dead-letter', '/articles/hot-top', ['group:文章管理']),
    ).toEqual(['group:文章管理', 'group:日志中心'])

    // 同一路径不重复并入。
    expect(
      reconcileExpandedGroups('/articles/hot-top', '/articles/hot-top', ['group:文章管理']),
    ).toEqual(['group:文章管理'])
  })

  it('父分组随子项高亮，兄弟组与顶层项不受影响', () => {
    const entries = buildAdminNavMenu(ADMIN_NAV_TREE, '/articles/hot-top', ['group:文章管理'])
    const articleGroup = entries.find(entry => entry.label === '文章管理')
    expect(articleGroup?.kind === 'group' ? articleGroup.active : false).toBe(true)
    expect(entries.find(entry => entry.label === '日志中心')?.active).toBe(false)
    expect(entries.find(entry => entry.label === '用户管理')?.active).toBe(false)

    // 亲自位于父页面时父项也亮，但子项不亮。
    const onParent = buildAdminNavMenu(ADMIN_NAV_TREE, '/articles', ['group:文章管理'])
    const parentGroup = onParent.find(entry => entry.label === '文章管理')
    if (parentGroup?.kind !== 'group') throw new Error('「文章管理」应渲染成分组')
    expect(parentGroup.active).toBe(true)
    expect(parentGroup.children[0]!.active).toBe(false)
  })

  it('标题取深层子项，面包屑祖先链取父分组', () => {
    expect(navItemByPath('/articles/hot-top')?.label).toBe('热门置顶')
    expect(navItemByPath('/articles')?.label).toBe('文章管理')
    expect(navTrail('/articles/hot-top').map(item => item.label)).toEqual(['文章管理'])
    expect(navTrail('/articles')).toEqual([])
  })
})

describe('后台菜单对账（两级结构）', () => {
  it('父项没有 path 属合法：不报缺失页面，只报真正缺页的子项', () => {
    const flat = flattenNav([
      { label: '数据概览', path: '/', icon: 'chart' },
      {
        label: '站点配置',
        icon: 'settings',
        children: [
          { label: '装饰', path: '/site/deco', icon: 'settings' },
          { label: '缺页域', path: '/site/missing', icon: 'settings' },
        ],
      },
    ])

    const report = reconcileAdminNav(flat, ['index', 'site/deco'])
    expect(report.missingPageFiles).toEqual(['缺页域 /site/missing'])
    // 分组父项没有 path，不会出现在重复 / 相对路径里。
    expect(report.duplicatePaths).toEqual([])
    expect(report.nonAbsolutePaths).toEqual([])
    expect(report.unreachablePages).toEqual([])
  })

  it('父项自身页面算可达（父 + 子都在菜单里）', () => {
    const flat = flattenNav([
      {
        label: '文章管理',
        path: '/articles',
        icon: 'article',
        children: [{ label: '热门置顶', path: '/articles/hot-top', icon: 'pin' }],
      },
    ])

    const report = reconcileAdminNav(flat, ['articles', 'articles/hot-top', 'login'])
    expect(report.unreachablePages).toEqual([])
    expect(report.missingPageFiles).toEqual([])
  })

  it('真实菜单的两级对账全绿', () => {
    const report = reconcileAdminNav(FLAT_NAV, pageFiles)
    expect(report).toEqual({
      missingPageFiles: [],
      unreachablePages: [],
      duplicatePaths: [],
      nonAbsolutePaths: [],
      hotTopOrder: null,
    })
  })
})

describe('后台菜单对账', () => {
  it('每个可导航菜单项都有对应的页面文件', () => {
    const report = reconcileAdminNav(FLAT_NAV, pageFiles)
    expect(report.missingPageFiles).toEqual([])
  })

  it('每个页面都能从菜单到达（除登录页与初始化页）', () => {
    const report = reconcileAdminNav(FLAT_NAV, pageFiles)
    expect(report.unreachablePages).toEqual([])
  })

  it('path 不重复、且都是绝对路径', () => {
    const report = reconcileAdminNav(FLAT_NAV, pageFiles)
    expect(report.duplicatePaths).toEqual([])
    expect(report.nonAbsolutePaths).toEqual([])
  })

  it('「热门置顶」紧跟「文章管理」（ADR-0017 决策 5 规定的位置）', () => {
    const report = reconcileAdminNav(FLAT_NAV, pageFiles)
    expect(report.hotTopOrder).toBeNull()

    const labels = FLAT_NAV.map(item => item.label)
    const articleIndex = labels.indexOf('文章管理')
    expect(articleIndex).toBeGreaterThanOrEqual(0)
    expect(FLAT_NAV[articleIndex + 1]?.label).toBe('热门置顶')
    expect(FLAT_NAV[articleIndex + 1]?.path).toBe('/articles/hot-top')
  })

  it('对账能发现漏掉的菜单项、多出的页面、重复 / 相对路径与位置错位', () => {
    // 故意把拍平视图弄坏，确认四条对账不是"永远绿"。
    const broken = flattenNav([
      { label: '数据概览', path: '/', icon: 'chart' },
      { label: '热门置顶', path: '/articles/hot-top', icon: 'pin' },
      { label: '日志中心', path: '/logs', icon: 'database' },
      { label: '重复项', path: '/logs', icon: 'database' },
      { label: '相对路径项', path: 'site', icon: 'settings' },
    ])

    const report = reconcileAdminNav(broken, ['index', 'articles', 'articles/hot-top', 'logs', 'orphan'])

    // 漏掉了「文章管理」（有页面 /articles，菜单里没有）→ 反向对账报出来。
    expect(report.unreachablePages).toEqual(['articles', 'orphan'])
    // 拍平视图里根本没有 /articles → 正向对账发现不了（这是反向对账的职责）；
    // 但 /site 这个相对路径不是真实页面，算"菜单指向不存在的页面"。
    expect(report.missingPageFiles).toEqual(['相对路径项 site'])
    expect(report.duplicatePaths).toEqual(['/logs'])
    expect(report.nonAbsolutePaths).toEqual(['site'])
    expect(report.hotTopOrder).toContain('文章管理')
  })

  it('位置约定能分别识别「缺热门置顶」「错位」「路径不符」', () => {
    const pages = ['articles', 'articles/hot-top', 'logs']
    const flatOf = (nodes: AdminNavNode[]) => flattenNav(nodes)

    expect(
      reconcileAdminNav(flatOf([{ label: '文章管理', path: '/articles', icon: 'article' }]), pages)
        .hotTopOrder,
    ).toContain('热门置顶')

    const misplaced = flatOf([
      { label: '文章管理', path: '/articles', icon: 'article' },
      { label: '日志中心', path: '/logs', icon: 'database' },
      { label: '热门置顶', path: '/articles/hot-top', icon: 'pin' },
    ])
    expect(reconcileAdminNav(misplaced, pages).hotTopOrder).toContain('紧跟')

    const wrongPath = flatOf([
      { label: '文章管理', path: '/articles', icon: 'article' },
      { label: '热门置顶', path: '/articles/hot', icon: 'pin' },
    ])
    expect(reconcileAdminNav(wrongPath, pages).hotTopOrder).toContain('/articles/hot-top')
  })

  it('反向对账排除登录页与初始化页，子项路径也算可达', () => {
    const flat = flattenNav([
      { label: '日志中心', path: '/logs', icon: 'database' },
      {
        label: '文章管理',
        path: '/articles',
        icon: 'article',
        children: [{ label: '热门置顶', path: '/articles/hot-top', icon: 'pin' }],
      },
    ])

    const report = reconcileAdminNav(flat, ['logs', 'articles', 'articles/hot-top', 'login', 'setup'])
    expect(report.unreachablePages).toEqual([])
    expect(report.missingPageFiles).toEqual([])
  })
})

describe('pageFileCandidates', () => {
  it('覆盖各种边界写法', () => {
    // 根路径
    expect(pageFileCandidates('/')).toEqual(['index'])
    expect(pageFileCandidates('')).toEqual(['index'])
    // 空值/纯空白：菜单里不该出现，但函数不能因此崩掉
    expect(pageFileCandidates(null as unknown as string)).toEqual(['index'])
    expect(pageFileCandidates('   ')).toEqual(['index'])
    // 只有斜杠：去掉首尾之后为空
    expect(pageFileCandidates('///')).toEqual(['index'])
    // 普通路径：两种 Nuxt 写法都作为候选
    expect(pageFileCandidates('/logs/')).toEqual(['logs', 'logs/index'])
    expect(pageFileCandidates('logs/dead-letter')).toEqual(['logs/dead-letter', 'logs/dead-letter/index'])
  })
})

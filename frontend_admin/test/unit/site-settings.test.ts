import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import {
  ADMIN_NAV_TREE,
  FLAT_NAV,
  activeGroupKeys,
  buildAdminNavMenu,
  navItemByPath,
  navTrail,
  pageFileCandidates,
} from '../../app/utils/menu'
import {
  ALL_SITE_SETTINGS_SECTIONS,
  CONTENT_PUBLISH_PAGES,
  SITE_INTERFACE_SECTIONS,
  SITE_SETTINGS_PAGES,
  SITE_SETTINGS_SECTIONS,
  SITE_STANDALONE_SECTIONS,
  SITE_USER_ACTION_SECTIONS,
  SITE_VERIFICATION_SECTIONS,
  siteSettingsSectionByPath,
} from '../../app/utils/siteSettings'

/**
 * 站点配置编辑域 → 独立路由的结构对账（ADR-0029 改写版）。
 *
 * 要守住的事：
 *
 *   1. 十一个编辑域各有一个可直接打开的 URL（页面文件真实存在，刷新保持当前域）；
 *   2. 每个页面只渲染自己那一个配置组件（不再有页内 tab，也不能顺手改兄弟页的字段）；
 *   3. 左侧菜单的归属与层级与路由一致：三个分组下的域深度 2、独立域深度 1；
 *   4. 共用 `site` 段端点的域（站点基础 / 登录设置 / 私信设置）保存必须走读-改-写。
 *
 * 分组本身是这次重组的核心断言：**按"谁受影响"分**（用户操作 / 验证码与通知 /
 * 站点），而不是按后端段编号排。分组一旦泄漏（例如把「邮件」放回站点配置平铺那一层），
 * 运营就会在两处看到同名条目。
 *
 * 为什么按源码断言：后台没有 e2e，naive-ui 的表单交互要 mock 一整套。
 * 与 `menu.test.ts` 的页面文件对账、`site-config.test.ts` 的文案断言同一套做法。
 * vitest 以 project root（frontend_admin/）为 cwd。
 */
const projectDir = process.cwd()
const pagesDir = join(projectDir, 'app', 'pages')
const settingsDir = join(projectDir, 'app', 'components', 'settings')

/** 期望值来自拆段方案，不复用被测模块的数据（避免自证）。 */
interface ExpectedSection {
  key: string
  label: string
  path: string
  /** 相对 app/pages，不含 .vue。 */
  page: string
  /** components/settings 下的配置组件。 */
  component: string
  /** 所属三级分组名；独立域为 null（直接挂在「站点配置」下）。 */
  group: string | null
  /** 菜单层级：分组下 depth 2，独立域 depth 1。 */
  depth: number
}

const EXPECTED_SECTIONS: ExpectedSection[] = [
  {
    key: 'site-basics',
    label: '站点基础',
    path: '/site',
    page: 'site/index',
    component: 'SettingsSiteBasics',
    group: '站点',
    depth: 2,
  },
  {
    key: 'site-login',
    label: '登录设置',
    path: '/site/login',
    page: 'site/login',
    component: 'SettingsSiteLogin',
    group: '用户操作',
    depth: 2,
  },
  {
    key: 'site-chat',
    label: '私信设置',
    path: '/site/chat',
    page: 'site/chat',
    component: 'SettingsSiteChat',
    group: '用户操作',
    depth: 2,
  },
  {
    key: 'oauth',
    label: '第三方登录',
    path: '/site/oauth',
    page: 'site/oauth',
    component: 'SettingsOAuth',
    group: '用户操作',
    depth: 2,
  },
  {
    key: 'captcha',
    label: '图形验证码',
    path: '/site/captcha',
    page: 'site/captcha',
    component: 'SettingsCaptcha',
    group: '验证码与通知',
    depth: 2,
  },
  {
    key: 'mail',
    label: '邮件',
    path: '/site/mail',
    page: 'site/mail',
    component: 'SettingsMail',
    group: '验证码与通知',
    depth: 2,
  },
  {
    key: 'sms',
    label: '短信',
    path: '/site/sms',
    page: 'site/sms',
    component: 'SettingsSMS',
    group: '验证码与通知',
    depth: 2,
  },
  // 独立域：既不属于"读者能主动触发的动作"，也不属于"为了证明你是你而发出去的消息"。
  { key: 'ai', label: 'AI 助手', path: '/site/ai', page: 'site/ai', component: 'SettingsAi', group: null, depth: 1 },
  {
    key: 'storage',
    label: '对象存储',
    path: '/site/storage',
    page: 'site/storage',
    component: 'SettingsStorage',
    group: null,
    depth: 1,
  },
  { key: 'log', label: '日志', path: '/site/log', page: 'site/log', component: 'SettingsLog', group: null, depth: 1 },
  {
    key: 'deco',
    label: '装饰',
    path: '/site/deco',
    page: 'site/deco',
    component: 'SettingsDeco',
    // 2026-11-27：装饰与右栏卡片合成「站点界面」分组（改的是读者站长什么样，
    // 与"谁能做什么"分开）。
    group: '站点界面',
    depth: 2,
  },
]

function sitePageFiles(): string[] {
  return readdirSync(join(pagesDir, 'site'), { withFileTypes: true })
    .filter(entry => entry.isFile() && entry.name.endsWith('.vue'))
    .map(entry => `site/${entry.name.slice(0, -'.vue'.length)}`)
    .sort()
}

function pageSource(page: string): string {
  return readFileSync(join(pagesDir, `${page}.vue`), 'utf8')
}

describe('站点配置编辑域清单（路由与组件映射的唯一来源）', () => {
  it('十一个编辑域的顺序、标题、路由与组件名固定', () => {
    expect(
      ALL_SITE_SETTINGS_SECTIONS.map(({ key, label, path, component }) => ({
        key,
        label,
        path,
        component,
      })),
    ).toEqual(
      EXPECTED_SECTIONS.map(({ key, label, path, component }) => ({
        key,
        label,
        path,
        component,
      })),
    )
    expect(ALL_SITE_SETTINGS_SECTIONS).toHaveLength(11)
    // 只有「站点基础」是「站点」分组的成员；登录/私信/第三方登录与图形码/邮件/短信
    // 各自归到「用户操作」与「验证码与通知」。
    expect(SITE_SETTINGS_PAGES.map(section => section.key)).toEqual(['site-basics'])
    expect(SITE_SETTINGS_SECTIONS).toHaveLength(10)
  })

  it('四个分组的成员固定，且互不重叠、覆盖全部域', () => {
    expect(SITE_USER_ACTION_SECTIONS.map(section => section.key)).toEqual([
      'site-login',
      'site-chat',
      'oauth',
    ])
    expect(SITE_VERIFICATION_SECTIONS.map(section => section.key)).toEqual(['captcha', 'mail', 'sms'])
    expect(SITE_INTERFACE_SECTIONS.map(section => section.key)).toEqual(['deco'])
    expect(SITE_STANDALONE_SECTIONS.map(section => section.key)).toEqual(['ai', 'storage', 'log'])

    // 顺序要与 ALL_SITE_SETTINGS_SECTIONS 一致（deco 在清单里排在 log 之后）。
    const grouped = [
      ...SITE_SETTINGS_PAGES,
      ...SITE_USER_ACTION_SECTIONS,
      ...SITE_VERIFICATION_SECTIONS,
      ...SITE_STANDALONE_SECTIONS,
      ...SITE_INTERFACE_SECTIONS,
    ].map(section => section.key)
    expect(new Set(grouped).size).toBe(grouped.length)
    expect(grouped).toEqual(ALL_SITE_SETTINGS_SECTIONS.map(section => section.key))
  })

  it('共用 site 段端点的域各自声明拥有的顶层块，且互不重叠', () => {
    const owners = ALL_SITE_SETTINGS_SECTIONS.filter(section => section.blocks.length > 0)
    expect(owners.map(section => section.key)).toEqual(['site-basics', 'site-login', 'site-chat'])

    const owned = owners.flatMap(section => section.blocks)
    expect(new Set(owned).size).toBe(owned.length)
    expect([...owned].sort()).toEqual(['chat', 'login', 'seo', 'site_info'])

    // 其余各域用自己整段端点，不参与 site 段的读-改-写。
    for (const section of ALL_SITE_SETTINGS_SECTIONS) {
      if (owners.includes(section)) continue
      expect(section.blocks, section.key).toEqual([])
    }
  })

  it('路由唯一、都是绝对路径，且全部挂在 /site 之下', () => {
    const paths = ALL_SITE_SETTINGS_SECTIONS.map(section => section.path)
    expect(new Set(paths).size).toBe(paths.length)
    for (const path of paths) {
      expect(path.startsWith('/site')).toBe(true)
      // 域必须靠路径区分，而不是查询参数或页面内状态。
      expect(path).not.toContain('?')
    }
  })

  it('siteSettingsSectionByPath 按路由反查；未知路由返回 undefined', () => {
    for (const section of EXPECTED_SECTIONS) {
      expect(siteSettingsSectionByPath(section.path)).toMatchObject({
        label: section.label,
        component: section.component,
      })
    }
    expect(siteSettingsSectionByPath('/site/missing')).toBeUndefined()
    expect(siteSettingsSectionByPath('')).toBeUndefined()
    // 搬到内容生产链之后，旧路由不再是站点配置域（它们只剩跳转页）。
    expect(siteSettingsSectionByPath('/site/content-filter')).toBeUndefined()
    expect(siteSettingsSectionByPath('/site/publish')).toBeUndefined()
  })
})

describe('站点配置菜单层级（三级）', () => {
  /**
   * 「站点配置」下不参与站点域清单的条目：运营位（右栏卡片）与新增的
   * 「文章审核设置」页（2026-11-27，它写 site 段的 article 块，但不属于
   * `SiteSettingsSection` 清单 —— 那套清单是"段 → 页"的映射）。
   */
  /**
   * 「站点配置」下不属于任何分组的裸挂页面。
   *
   * 2026-11-27 第二次调整后**一个都没有**：右栏卡片进了「站点界面」，
   * 文章审核设置进了「内容修改与发布」。这个常量留着是因为断言要能表达
   * "现在没有裸挂页" —— 空数组比删掉断言更清楚。
   */
  const OPERATIONAL_CHILDREN: { label: string; path: string }[] = []

  it('「站点配置」是纯分组，子项 = 五个分组 + 三个独立域（没有裸挂页）', () => {
    const group = ADMIN_NAV_TREE.find(node => node.label === '站点配置')
    expect(group).toBeDefined()
    expect(group!.path).toBeUndefined()

    const children = group!.children ?? []
    const groupNames = children.filter(child => child.path === undefined).map(child => child.label)
    expect(groupNames).toEqual(['站点', '用户操作', '验证码与通知', '内容修改与发布', '站点界面'])

    for (const name of groupNames) {
      const sub = children.find(child => child.label === name)!
      // 分组自身没有 landing page：点标题只展开/收起。
      expect(sub.path).toBeUndefined()
    }

    const standalone = children
      .filter(child => child.path !== undefined)
      .filter(child => !OPERATIONAL_CHILDREN.some(extra => extra.path === child.path))
    expect(standalone.map(({ label, path }) => ({ label, path }))).toEqual(
      SITE_STANDALONE_SECTIONS.map(({ label, path }) => ({ label, path })),
    )

    for (const extra of OPERATIONAL_CHILDREN) {
      expect(children.map(({ label, path }) => ({ label, path }))).toContainEqual(extra)
    }
    expect(children.length).toBe(5 + SITE_STANDALONE_SECTIONS.length + OPERATIONAL_CHILDREN.length)
  })

  it('三个分组各自挂上正确的域，且分组之间不重复', () => {
    const group = ADMIN_NAV_TREE.find(node => node.label === '站点配置')!
    const byName = (name: string) => group.children!.find(child => child.label === name)!

    expect(byName('站点').children!.map(({ label, path }) => ({ label, path }))).toEqual(
      SITE_SETTINGS_PAGES.map(({ label, path }) => ({ label, path })),
    )
    expect(byName('用户操作').children!.map(({ label, path }) => ({ label, path }))).toEqual(
      SITE_USER_ACTION_SECTIONS.map(({ label, path }) => ({ label, path })),
    )
    expect(byName('验证码与通知').children!.map(({ label, path }) => ({ label, path }))).toEqual(
      SITE_VERIFICATION_SECTIONS.map(({ label, path }) => ({ label, path })),
    )
    // 内容修改与发布（2026-11-27 从一级分组降为站点配置下的三级分组）：
    // 三个内容页 + 文章审核设置（审核规则与"改词表 / 定发布线"是同一条链的第三步）。
    expect(byName('内容修改与发布').children!.map(({ label, path }) => ({ label, path }))).toEqual([
      ...CONTENT_PUBLISH_PAGES.map(({ label, path }) => ({ label, path })),
      { label: '文章审核设置', path: '/site/article-review' },
    ])
    // 站点界面：装饰来自站点域清单，右栏卡片是运营位（不在清单里，显式挂上）。
    expect(byName('站点界面').children!.map(({ label, path }) => ({ label, path }))).toEqual([
      ...SITE_INTERFACE_SECTIONS.map(({ label, path }) => ({ label, path })),
      { label: '右栏卡片', path: '/site/sidebar-cards' },
    ])

    // 「邮件」「短信」只能出现在「验证码与通知」下 —— 双入口会让运营改错页。
    const allPaths = group.children!.flatMap(child => (child.children ?? []).map(grand => grand.path))
    expect(new Set(allPaths).size).toBe(allPaths.length)
  })

  it('十一个编辑域都能在菜单里查到，层级与父级归属正确', () => {
    for (const section of EXPECTED_SECTIONS) {
      const item = navItemByPath(section.path)
      expect(item).toMatchObject({ label: section.label, depth: section.depth })
      // 父分组都没有自身 path，所以子项拿不到 parentPath。
      expect(item!.parentPath).toBeNull()
      expect(item!.parentLabel).toBe(section.group ?? '站点配置')
    }
  })

  it('深链接到分组下的域：面包屑三层，两层分组都展开并高亮', () => {
    for (const section of EXPECTED_SECTIONS.filter(s => s.group !== null)) {
      expect(navTrail(section.path).map(item => item.label)).toEqual(['站点配置', section.group])
      expect(activeGroupKeys(section.path)).toEqual([
        'group:站点配置',
        `group:站点配置 / ${section.group}`,
      ])

      const entries = buildAdminNavMenu(ADMIN_NAV_TREE, section.path, activeGroupKeys(section.path))
      const siteConfig = entries.find(entry => entry.label === '站点配置')
      if (siteConfig?.kind !== 'group') throw new Error('「站点配置」应渲染成分组')
      expect(siteConfig.active).toBe(true)
      expect(siteConfig.expanded).toBe(true)

      const subGroup = siteConfig.children.find(child => child.label === section.group)
      if (subGroup?.kind !== 'group') throw new Error(`「${section.group}」应渲染成分组`)
      expect(subGroup.active).toBe(true)
      expect(subGroup.expanded).toBe(true)
      // 只亮当前页，兄弟页不亮。
      expect(subGroup.children.filter(child => child.active).map(child => child.label)).toEqual([
        section.label,
      ])
    }
  })

  it('深链接到独立域：仍是一层面包屑，只有「站点配置」展开', () => {
    for (const section of SITE_STANDALONE_SECTIONS) {
      expect(navTrail(section.path).map(item => item.label)).toEqual(['站点配置'])
      expect(activeGroupKeys(section.path)).toEqual(['group:站点配置'])
    }
  })

  it('分组展开态互不串味（key 带层级链）', () => {
    const expanded = activeGroupKeys('/site/chat')
    // 只展开自己那条链，不把兄弟分组一起打开。
    expect(expanded).not.toContain('group:站点配置 / 验证码与通知')
    // 装饰在「站点界面」组里（2026-11-27），展开的是它自己那条链。
    expect(activeGroupKeys('/site/deco')).toEqual([
      'group:站点配置',
      'group:站点配置 / 站点界面',
    ])
  })
})

describe('内容修改与发布（站点配置下的三级分组）', () => {
  it('三页同属「内容修改与发布」，挂在「站点配置」下、深度 3', () => {
    const siteConfig = ADMIN_NAV_TREE.find(node => node.label === '站点配置')!
    const group = (siteConfig.children ?? []).find(node => node.label === '内容修改与发布')
    expect(group).toBeDefined()
    // 一条链没有"总览页"可看，所以父项只分组。
    expect(group!.path).toBeUndefined()
    expect(group!.children!.map(({ label, path }) => ({ label, path }))).toEqual([
      ...CONTENT_PUBLISH_PAGES.map(({ label, path }) => ({ label, path })),
      { label: '文章审核设置', path: '/site/article-review' },
    ])

    for (const page of CONTENT_PUBLISH_PAGES) {
      const item = navItemByPath(page.path)
      // 站点配置(0) → 内容修改与发布(1) → 页面(2)
      expect(item).toMatchObject({ label: page.label, depth: 2 })
      expect(item!.parentLabel).toBe('内容修改与发布')
      expect(item!.parentPath).toBeNull()
    }

    // 关键词白名单（2026-11-27 拆页）：与关键词过滤同组、紧跟其后。
    expect(CONTENT_PUBLISH_PAGES.map(page => page.label)).toEqual([
      '关键词过滤',
      '关键词白名单',
      '文章发布',
    ])
    const keywordIndex = FLAT_NAV.findIndex(item => item.label === '关键词过滤')
    expect(FLAT_NAV[keywordIndex + 1]?.label).toBe('关键词白名单')
    expect(FLAT_NAV[keywordIndex + 1]?.path).toBe('/content/keywords/whitelist')

    // 旧位置不再有这些页面入口（旧地址只剩跳转页）。
    expect(navItemByPath('/review')).toBeUndefined()
    expect(navItemByPath('/site/content-filter')).toBeUndefined()
    expect(navItemByPath('/site/publish')).toBeUndefined()
  })

  it('「文章审核」回到审核组，深度 1：「审核(0) → 文章审核(1)」', () => {
    const group = ADMIN_NAV_TREE.find(node => node.label === '审核')!
    expect(group.children!.map(({ label }) => label)).toEqual(['图片审核', '关键词风险', '文章审核'])
    const item = navItemByPath('/review/article')
    expect(item).toMatchObject({ label: '文章审核', depth: 1 })
    expect(item!.parentLabel).toBe('审核')
    expect(item!.parentPath).toBeNull()
  })
})

describe('站点配置编辑域页面文件（刷新保持当前域）', () => {
  it('app/pages/site 下每个编辑域各一个页面文件，另有两个旧地址跳转页与「右栏卡片」', () => {
    const actual = sitePageFiles()
    const domainPages = EXPECTED_SECTIONS.map(section => section.page).sort()
    for (const page of domainPages) {
      expect(actual).toContain(page)
    }
    expect(actual).toContain('site/sidebar-cards')
    // 文章审核设置（2026-11-27）：写 site 段 article 块，但不在"段 → 页"清单里。
    expect(actual).toContain('site/article-review')
    // 旧地址跳转页：搬走的两页仍活着，只是只做跳转（页面数量断言在
    // outline-retirement.test.ts 里另有总账）。
    expect(actual).toContain('site/content-filter')
    expect(actual).toContain('site/publish')
  })

  it('每个路由都能解析到一个真实页面文件', () => {
    for (const section of EXPECTED_SECTIONS) {
      const candidates = pageFileCandidates(section.path)
        .map(candidate => `${candidate}.vue`)
        .filter(file => existsSync(join(pagesDir, file)))
      expect(candidates, section.path).toEqual([`${section.page}.vue`])
    }
  })

  it('每个页面只渲染自己那一个配置组件，不再有页内 tab 切换', () => {
    for (const section of EXPECTED_SECTIONS) {
      const source = pageSource(section.page)

      expect(source).toContain(
        `import ${section.component} from '~/components/settings/${section.component}.vue'`,
      )
      expect(source).toMatch(new RegExp(`<${section.component}(\\s|/|>)`, 'u'))

      const importedSettings = [...source.matchAll(/~\/components\/settings\/(\w+)\.vue/gu)].map(
        match => match[1],
      )
      expect(importedSettings).toEqual([section.component])

      // 域由 URL 决定：页面里不应再出现 tab 容器或活动态内存变量。
      expect(source).not.toMatch(/NTabs|NTabPane|useState/u)
    }
  })

  it('十一个编辑域的配置组件都在原位置', () => {
    for (const section of EXPECTED_SECTIONS) {
      expect(existsSync(join(settingsDir, `${section.component}.vue`)), section.component).toBe(true)
    }
  })

  it('共用 site 段端点的三页保存都走读-改-写，只覆盖自己拥有的块', () => {
    for (const section of ALL_SITE_SETTINGS_SECTIONS.filter(section => section.blocks.length > 0)) {
      const source = readFileSync(join(settingsDir, `${section.component}.vue`), 'utf8')
      // 保存前必须重新取一次最新整段（否则会覆盖其它页/无 UI 块）。
      expect(source, section.component).toContain('applySiteSectionPatch')
      const blocks = [...source.matchAll(/OWNED_BLOCKS\s*=\s*\[([^\]]*)\]/gu)].map(match =>
        match[1]!.split(',').map(raw => raw.trim().replace(/^["']|["']$/gu, '')).filter(Boolean),
      )
      expect(blocks, section.component).toHaveLength(1)
      expect(blocks[0], section.component).toEqual([...section.blocks])
    }
  })
})







import { createApp, defineComponent, h } from 'vue'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import AdminBreadcrumb from '~/components/admin/AdminBreadcrumb.vue'
import AdminNavTree from '~/components/admin/AdminNavTree.vue'
import {
  ADMIN_NAV_TREE,
  activeGroupKeys,
  buildAdminBreadcrumb,
  buildAdminNavMenu,
} from '~/utils/menu'
import type { AdminBreadcrumbItem, AdminNavMenuEntry } from '~/utils/menu'

/**
 * 内容区顶部面包屑的渲染与层级语义。
 *
 * 断言只碰用户在页面上能看到 / 能点到的语义：可点回退的 href、当前项的
 * aria-current、分隔符（单级时没有）、只分组父项不是链接。不断言标签结构细节。
 *
 * 和侧栏菜单同一套挂法：后台没装 @vue/test-utils，NuxtLink 注册成极简桩
 * （href = to）就能表达"用户点了哪一级回退"。
 */
const NuxtLinkStub = defineComponent({
  props: { to: { type: String, required: true } },
  setup(props, { slots }) {
    return () => h('a', { href: props.to }, slots.default?.())
  },
})

let app: ReturnType<typeof createApp> | null = null
let container: HTMLDivElement | null = null

function mount(children: () => unknown) {
  container = document.createElement('div')
  document.body.appendChild(container)
  app = createApp({ render: () => children() })
  app.component('NuxtLink', NuxtLinkStub)
  app.mount(container)
  return container
}

function mountBreadcrumb(items: AdminBreadcrumbItem[]) {
  return mount(() => h(AdminBreadcrumb, { items }))
}

beforeEach(() => {
  document.body.innerHTML = ''
})

afterEach(() => {
  app?.unmount()
  app = null
  container?.remove()
  container = null
})

describe('AdminBreadcrumb', () => {
  it('两级面包屑：父分组是可点回退链接，当前项是当前态、不可点', () => {
    const items = buildAdminBreadcrumb('/articles/hot-top')
    const el = mountBreadcrumb(items)

    expect(el.querySelector('nav')?.getAttribute('aria-label')).toBe('面包屑')
    expect(el.textContent).toContain('文章管理')
    expect(el.textContent).toContain('热门置顶')

    // 上级可点回退。
    const back = el.querySelector<HTMLAnchorElement>('a[href="/articles"]')
    expect(back).not.toBeNull()
    // 当前项不产出链接，只以当前态呈现。
    expect(el.querySelectorAll('a')).toHaveLength(1)
    const current = el.querySelector('[aria-current="page"]')
    expect(current?.textContent).toContain('热门置顶')
    // 两级之间正好一个分隔符。
    expect(el.querySelectorAll('[data-breadcrumb-separator]')).toHaveLength(1)
  })

  it('单级（根页 / 顶层页）没有分隔符', () => {
    for (const path of ['/', '/users']) {
      const el = mountBreadcrumb(buildAdminBreadcrumb(path))
      expect(el.querySelectorAll('a')).toHaveLength(0)
      expect(el.querySelectorAll('[data-breadcrumb-separator]')).toHaveLength(0)
      expect(el.querySelectorAll('[aria-current="page"]')).toHaveLength(1)
    }
  })

  it('只分组父项没有落地页：渲染成不可点文本，不产出空 href 链接', () => {
    const el = mountBreadcrumb([
      { label: '站点配置', path: null, current: false },
      { label: '装饰', path: null, current: true },
    ])

    expect(el.textContent).toContain('站点配置')
    expect(el.querySelector('a')).toBeNull()
    expect(el.querySelector('[aria-current="page"]')).not.toBeNull()
    expect(el.querySelectorAll('[data-breadcrumb-separator]')).toHaveLength(1)
  })

  it('多级链条按顺序渲染，祖先逐级可点、当前项收尾', () => {
    const items = [
      { label: '站点配置', path: null, current: false },
      { label: '装饰', path: '/site/deco', current: false },
      { label: '光标', path: null, current: true },
    ]
    const el = mountBreadcrumb(items)

    expect(el.textContent).toContain('站点配置')
    expect(el.querySelector<HTMLAnchorElement>('a[href="/site/deco"]')?.textContent).toContain('装饰')
    expect(el.querySelectorAll('a')).toHaveLength(1)
    expect(el.querySelectorAll('[data-breadcrumb-separator]')).toHaveLength(2)

    const current = el.querySelector('[aria-current="page"]')
    expect(current?.textContent).toContain('光标')
  })

  it('深链接进入时面包屑当前项与菜单高亮项一致', () => {
    const path = '/logs/dead-letter'
    const el = mount(() =>
      h('div', [
        h(AdminBreadcrumb, { items: buildAdminBreadcrumb(path) }),
        h(AdminNavTree, { entries: buildAdminNavMenu(ADMIN_NAV_TREE, path, activeGroupKeys(path)) }),
      ]),
    )

    const crumbCurrent = el.querySelector('[aria-current="page"]')
    const navActive = el.querySelector<HTMLAnchorElement>('a[href="/logs/dead-letter"]')
    expect(navActive?.getAttribute('aria-current')).toBe('page')
    expect(crumbCurrent?.textContent?.trim()).toBe(navActive?.textContent?.trim())
    // 面包屑的祖先也与菜单分组标题同源。
    expect(el.querySelector('nav')?.textContent).toContain('日志中心')
  })

  it('空面包屑（未知路由）不渲染导航', () => {
    const el = mountBreadcrumb([])
    expect(el.querySelector('nav')).toBeNull()
  })
})

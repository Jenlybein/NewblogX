import { createApp, defineComponent, h, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import AdminNavTree from '~/components/admin/AdminNavTree.vue'
import { ADMIN_NAV_TREE, buildAdminNavMenu } from '~/utils/menu'
import type { AdminNavMenuEntry } from '~/utils/menu'

/**
 * 侧栏两级菜单的渲染与交互。
 *
 * 断言只碰用户在页面上能看见/能点到的语义：链接的 href、aria-current、
 * 展开按钮的 aria-expanded、分组高亮态与缩进层级。不断言具体标签结构。
 *
 * NuxtLink 在这里注册成极简桩（href = to），因为后台没有装
 * @vue/test-utils，组件用原生 createApp 挂载就够表达用户做了什么。
 */
const NuxtLinkStub = defineComponent({
  props: { to: { type: String, required: true } },
  setup(props, { slots }) {
    return () => h('a', { href: props.to }, slots.default?.())
  },
})

let app: ReturnType<typeof createApp> | null = null
let container: HTMLDivElement | null = null

function mount(initial: AdminNavMenuEntry[]) {
  const entries = ref(initial)
  const toggled: string[] = []
  container = document.createElement('div')
  document.body.appendChild(container)

  app = createApp({
    render: () =>
      h(AdminNavTree, {
        entries: entries.value,
        onToggle: (key: string) => {
          toggled.push(key)
        },
      }),
  })
  app.component('NuxtLink', NuxtLinkStub)
  app.mount(container)

  return { container, entries, toggled }
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

describe('AdminNavTree', () => {
  it('分组渲染父项标题 + 子项链接，子项缩进一级且高亮父分组', () => {
    const { container } = mount(
      buildAdminNavMenu(ADMIN_NAV_TREE, '/articles/hot-top', ['group:文章管理']),
    )

    // 父项标题与子项文案都可见。
    expect(container.textContent).toContain('文章管理')
    expect(container.textContent).toContain('热门置顶')

    const child = container.querySelector<HTMLAnchorElement>('a[href="/articles/hot-top"]')
    expect(child).not.toBeNull()
    expect(child!.getAttribute('aria-current')).toBe('page')
    // 子项在 DOM 里就是缩进一级的条目。
    expect(child!.closest('[data-depth]')?.getAttribute('data-depth')).toBe('1')

    // 子项激活时其父分组呈现激活态，兄弟分组不亮。
    const groups = [...container.querySelectorAll<HTMLElement>('.admin-nav__group')]
    const articleGroup = groups.find(group => group.textContent?.includes('文章管理'))
    expect(articleGroup?.classList.contains('admin-nav__group--active')).toBe(true)
    const logsGroup = groups.find(group => group.textContent?.includes('日志中心'))
    expect(logsGroup?.classList.contains('admin-nav__group--active')).toBe(false)
  })

  it('展开的分组显示子项，收起的分组不显示子项', async () => {
    const expanded = buildAdminNavMenu(ADMIN_NAV_TREE, '/articles/hot-top', ['group:文章管理'])
    const { container, entries } = mount(expanded)

    expect(container.querySelector('a[href="/articles/hot-top"]')).not.toBeNull()
    const articleGroup = [...container.querySelectorAll<HTMLElement>('.admin-nav__group')].find(
      group => group.textContent?.includes('文章管理'),
    )
    const toggle = articleGroup?.querySelector<HTMLButtonElement>('button')
    expect(toggle?.getAttribute('aria-expanded')).toBe('true')

    // 用户收起：展开态来自 props，父组件把 key 移除后子项消失。
    const collapsed = buildAdminNavMenu(ADMIN_NAV_TREE, '/articles/hot-top', [])
    entries.value = collapsed
    await nextTick()
    expect(container.querySelector('a[href="/articles/hot-top"]')).toBeNull()
    expect(
      container.querySelector<HTMLButtonElement>('.admin-nav__group--active button')?.getAttribute('aria-expanded'),
    ).toBe('false')

    // 再展开：子项回来。
    entries.value = expanded
    await nextTick()
    expect(container.querySelector('a[href="/articles/hot-top"]')).not.toBeNull()
  })

  it('点击展开按钮抛出自个分组的 key；没有自身页面的父项不产出可跳转链接', () => {
    const tree = [
      {
        label: '站点配置',
        icon: 'settings' as const,
        children: [{ label: '装饰', path: '/site/deco', icon: 'settings' as const }],
      },
    ]
    const { container, toggled } = mount(
      buildAdminNavMenu(tree, '/site/deco', ['group:站点配置']),
    )

    // 只分组的父项没有 href，点标题不会跳到空页；也没有被合成出来的子项。
    expect(container.querySelector('a[href="/site"]')).toBeNull()
    expect(container.querySelector('a[href="/site/deco"]')).not.toBeNull()
    expect(container.querySelectorAll('a').length).toBe(1)

    const toggle = container.querySelector<HTMLButtonElement>('button')!
    toggle.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(toggled).toEqual(['group:站点配置'])

    // 分组标题本身是可读文本而不是链接。
    const group = container.querySelector('.admin-nav__group')
    expect(group?.textContent).toContain('站点配置')
  })

  it('顶层单页直接渲染为链接，不带展开按钮', () => {
    const { container } = mount(
      buildAdminNavMenu([{ label: '数据概览', path: '/', icon: 'chart' }], '/', []),
    )
    const home = container.querySelector('a[href="/"]')
    expect(home?.getAttribute('aria-current')).toBe('page')
    expect(container.querySelector('button')).toBeNull()
  })
})

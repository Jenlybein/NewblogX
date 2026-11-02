import { describe, expect, it } from 'vitest'
import {
  ADMIN_NAV_TREE,
  activeGroupKeys,
  buildAdminBreadcrumb,
  buildAdminNavMenu,
  flattenNav,
  navItemByPath,
  navTrail,
} from '../../app/utils/menu'
import type { AdminNavNode } from '../../app/utils/menu'

/**
 * 后台面包屑的数据层：把「当前路由」翻译成「根 → 当前」的面包屑项。
 *
 * 祖先链直接复用拍平视图的 `navTrail()` —— 面包屑不是第二份层级来源，
 * 否则菜单与面包屑迟早会讲两个故事。这里只断言数据，不碰渲染。
 */

describe('后台面包屑数据', () => {
  it('子页面显示两级：可点的父分组 + 当前子项', () => {
    const items = buildAdminBreadcrumb('/articles/hot-top')

    expect(items.map(item => item.label)).toEqual(['文章管理', '热门置顶'])
    // 父分组有落地页 → 可点回退；当前项是当前态且不可点。
    expect(items[0]).toMatchObject({ path: '/articles', current: false })
    expect(items[1]).toMatchObject({ path: null, current: true })
  })

  it('多级链条：祖先按根 → 父排列，中间层可点、只分组层不可点', () => {
    const flat = flattenNav([
      {
        // 只分组的顶层父项：没有自己的页面。
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

    const items = buildAdminBreadcrumb('/site/deco/cursor', flat)
    expect(items.map(item => item.label)).toEqual(['站点配置', '装饰', '光标'])
    expect(items.map(item => item.path)).toEqual([null, '/site/deco', null])
    expect(items.map(item => item.current)).toEqual([false, false, true])
  })

  it('当前项是当前态且不可点：最后一环永远 path 为 null、current 为 true', () => {
    for (const path of ['/', '/articles', '/articles/hot-top', '/logs/dead-letter']) {
      const items = buildAdminBreadcrumb(path)
      const last = items[items.length - 1]!
      expect(last.current).toBe(true)
      expect(last.path).toBeNull()
      // 当前项文案与菜单高亮项一致：两者都来自拍平视图。
      expect(last.label).toBe(navItemByPath(path)?.label)
    }
  })

  it('根页（无父级）退化为单级：只有当前项，没有祖先', () => {
    const items = buildAdminBreadcrumb('/')
    expect(items).toEqual([{ label: '数据概览', path: null, current: true }])
  })

  it('顶层单页同样是单级面包屑（菜单里没有父级就不编造）', () => {
    const items = buildAdminBreadcrumb('/users')
    expect(items).toEqual([{ label: '用户管理', path: null, current: true }])
  })

  it('未知路由不造面包屑：返回空数组，消费方不渲染', () => {
    expect(buildAdminBreadcrumb('/not-a-page')).toEqual([])
    expect(buildAdminBreadcrumb('')).toEqual([])
  })

  it('深链接进入时面包屑与菜单高亮指向同一项', () => {
    const path = '/logs/dead-letter'
    const items = buildAdminBreadcrumb(path)
    const menu = buildAdminNavMenu(ADMIN_NAV_TREE, path, activeGroupKeys(path))
    const logs = menu.find(entry => entry.label === '日志中心')
    if (logs?.kind !== 'group') throw new Error('「日志中心」应渲染成分组')

    const activeChild = logs.children.find(child => child.active)
    expect(activeChild?.label).toBe(items[items.length - 1]!.label)
    // 面包屑祖先与菜单祖先链同源。
    expect(items.slice(0, -1).map(item => item.label)).toEqual(
      navTrail(path).map(item => item.label),
    )
  })
})

/**
 * 后台侧边栏的导航清单（**唯一的一份**）。
 *
 * 为什么抽到 utils 而不是留在 AdminShell.vue 里：
 * 菜单里每个 path 必须有一个真实存在的页面文件 —— 否则点进去就是 404，而这件事
 * 没有任何东西会报错。上一轮就踩过：侧边栏挂着「日志中心 → /logs」，
 * 而 `app/pages/logs/` 根本不存在（12 §8.6）。
 *
 * 抽出来之后 `test/unit/menu.test.ts` 能直接扫 `app/pages` 做对账；
 * 顺带也进了覆盖率门槛（admin 只统计 app/utils/**）。
 *
 * icon 用字符串而不是组件：utils 层不该 import Vue 组件，
 * 组件层再把它映射成具体图标（见 AdminShell.vue 的 ICONS）。
 */
export type AdminNavIcon =
  | 'chart'
  | 'shield'
  | 'article'
  | 'users'
  | 'settings'
  | 'photo'
  | 'bell'
  | 'database'
  | 'alert'

export interface AdminNavItem {
  label: string
  path: string
  icon: AdminNavIcon
}

export const ADMIN_NAV: AdminNavItem[] = [
  { label: '数据概览', path: '/', icon: 'chart' },
  { label: '文章审核', path: '/review', icon: 'shield' },
  { label: '文章管理', path: '/articles', icon: 'article' },
  { label: '用户管理', path: '/users', icon: 'users' },
  { label: '站点配置', path: '/site', icon: 'settings' },
  { label: '媒体运营', path: '/media', icon: 'photo' },
  { label: '全局通知', path: '/notifications', icon: 'bell' },
  { label: '日志中心', path: '/logs', icon: 'database' },
  { label: '同步死信', path: '/logs/dead-letter', icon: 'alert' },
]

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

/**
 * 站点配置的八个域（**路由、菜单子项与页面文件的唯一清单**）。
 *
 * 为什么需要它：后端把站点配置拆成八个独立的段（读 `GET /site/admin/{name}`、
 * 写 `PUT /site/{name}`），后台也让每个域各占一个路由。菜单子项、页面文件和
 * 测试对账都从这一份清单派生，"菜单里有一条、页面文件里没有"这种静默 404
 * 才没有机会发生。
 *
 * 为什么这里只存组件名（字符串）而不 import 组件：utils 层不依赖 Vue，
 * 沿用 `menu.ts` 的同一约定；页面文件自己按原名 import 配置组件，
 * 组件不改名、不移动。
 *
 * 路由约定：域名即配置段名，`site` 段复用既有入口 `/site`（旧书签不失效），
 * 其余七段是 `/site/{段名}`。域由 URL 区分，刷新天然停留在当前域。
 */
export type SiteSettingsKey =
  | 'site'
  | 'ai'
  | 'mail'
  | 'storage'
  | 'oauth'
  | 'log'
  | 'upload'
  | 'deco'

/** 一个配置域：菜单标题、独立路由、以及承载它的配置组件名。 */
export interface SiteSettingsSection {
  /** 配置段名，与后端 `{name}` 一致。 */
  key: SiteSettingsKey
  /** 菜单子项与页面标题。 */
  label: string
  /** 独立路由（绝对路径）。 */
  path: string
  /** `components/settings/` 下的组件文件名（不含 `.vue`）。 */
  component: string
}

export const SITE_SETTINGS_SECTIONS: readonly SiteSettingsSection[] = [
  { key: 'site', label: '站点', path: '/site', component: 'SettingsSite' },
  { key: 'ai', label: 'AI 助手', path: '/site/ai', component: 'SettingsAi' },
  { key: 'mail', label: '邮件', path: '/site/mail', component: 'SettingsMail' },
  { key: 'storage', label: '对象存储', path: '/site/storage', component: 'SettingsStorage' },
  { key: 'oauth', label: '第三方登录', path: '/site/oauth', component: 'SettingsOAuth' },
  { key: 'log', label: '日志', path: '/site/log', component: 'SettingsLog' },
  { key: 'upload', label: '上传策略', path: '/site/upload', component: 'SettingsUpload' },
  { key: 'deco', label: '装饰', path: '/site/deco', component: 'SettingsDeco' },
]

/** 按路由反查配置域；未知路由返回 undefined（与 `navItemByPath` 同一语义）。 */
export function siteSettingsSectionByPath(path: string): SiteSettingsSection | undefined {
  if (!path) return undefined
  return SITE_SETTINGS_SECTIONS.find(section => section.path === path)
}

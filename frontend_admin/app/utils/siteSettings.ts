/**
 * 后台「站点配置」的编辑域（**路由、菜单子项与页面文件的唯一清单**）。
 *
 * 后端把运行时配置拆成九个独立的段（ADR-0029），其中八段在后台各有编辑页；
 * 第九段 `image_gc` 已有具名端点但没有编辑页/组件，所以不在本清单里。
 * 这里只描述「有编辑页的域」，不是全部配置段的清单。
 *
 * 2026-11 起「站点」这一段被拆成四页（ADR-0039）：基础信息与 SEO、登录策略、
 * 发布治理、私信各占一个路由。它们仍然同属 `site` 段、共用一对端点
 * （读 `GET /site/admin/site`、写 `PUT /site/site`），所以保存必须走
 * 「取最新整段 → 只覆盖本页拥有的顶层块 → 写回」（见 utils/siteSectionSave）。
 *
 * 为什么这里只存组件名（字符串）而不 import 组件：utils 层不依赖 Vue，
 * 沿用 `menu.ts` 的同一约定；页面文件自己按原名 import 配置组件，
 * 组件不改名、不移动。
 *
 * 路由约定：站点基础复用既有入口 `/site`（旧书签不失效），其余按段名拼装。
 */
import type { SiteRuntimeConfig } from '~/types/api'

export type SiteSettingsKey =
  | 'site-basics'
  | 'site-login'
  | 'site-publish'
  | 'site-chat'
  | 'ai'
  | 'mail'
  | 'storage'
  | 'oauth'
  | 'sms'
  | 'log'
  | 'upload'
  | 'deco'

/** site 段的顶层配置块名（只有这几个块会在保存时被本页覆盖）。 */
export type SiteSectionBlock = keyof SiteRuntimeConfig

/** 一个配置域：菜单标题、独立路由、承载它的配置组件名，以及它拥有的顶层块。 */
export interface SiteSettingsSection {
  /** 编辑域名，与后端 `{name}` 段一致（拆页后的四个域仍然是 `site`）。 */
  key: SiteSettingsKey
  /** 菜单子项与页面标题。 */
  label: string
  /** 独立路由（绝对路径）。 */
  path: string
  /** `components/settings/` 下的组件文件名（不含 `.vue`）。 */
  component: string
  /**
   * 本页拥有的 `site` 段顶层块。空数组表示"这一域用的是自己的整段端点"，
   * 不参与整段读写（mail / storage / oauth / log / upload / deco）。
   */
  blocks: readonly SiteSectionBlock[]
}

/** 「站点」分组的名字：菜单里是一层纯分组，没有自身页面（ADR-0039）。 */
export const SITE_SETTINGS_GROUP_LABEL = '站点'

/** 站点四页（都属于同一个 site 段，保存要走读-改-写）。 */
export const SITE_SETTINGS_PAGES: readonly SiteSettingsSection[] = [
  {
    key: 'site-basics',
    label: '站点基础',
    path: '/site',
    component: 'SettingsSiteBasics',
    blocks: ['site_info', 'seo'],
  },
  {
    key: 'site-login',
    label: '登录与发码',
    path: '/site/login',
    component: 'SettingsSiteLogin',
    blocks: ['login'],
  },
  {
    key: 'site-publish',
    label: '发布与审核',
    path: '/site/publish',
    component: 'SettingsSitePublish',
    blocks: ['article', 'comment'],
  },
  {
    key: 'site-chat',
    label: '私信',
    path: '/site/chat',
    component: 'SettingsSiteChat',
    blocks: ['chat'],
  },
]

/**
 * 其余七个编辑域：每个域一对独立端点（`GET /site/admin/<段>` / `PUT /site/<段>`），
 * 页面自己读写整段，不参与 site 段的读-改-写。
 */
export const SITE_SETTINGS_SECTIONS: readonly SiteSettingsSection[] = [
  { key: 'ai', label: 'AI 助手', path: '/site/ai', component: 'SettingsAi', blocks: [] },
  { key: 'mail', label: '邮件', path: '/site/mail', component: 'SettingsMail', blocks: [] },
  { key: 'storage', label: '对象存储', path: '/site/storage', component: 'SettingsStorage', blocks: [] },
  { key: 'oauth', label: '第三方登录', path: '/site/oauth', component: 'SettingsOAuth', blocks: [] },
  { key: 'sms', label: '短信', path: '/site/sms', component: 'SettingsSMS', blocks: [] },
  { key: 'log', label: '日志', path: '/site/log', component: 'SettingsLog', blocks: [] },
  { key: 'upload', label: '上传策略', path: '/site/upload', component: 'SettingsUpload', blocks: [] },
  { key: 'deco', label: '装饰', path: '/site/deco', component: 'SettingsDeco', blocks: [] },
]

/** 全部编辑域：站点四页在前，其余七个域在后（菜单顺序即此顺序）。 */
export const ALL_SITE_SETTINGS_SECTIONS: readonly SiteSettingsSection[] = [
  ...SITE_SETTINGS_PAGES,
  ...SITE_SETTINGS_SECTIONS,
]

/** 按路由反查配置域；未知路由返回 undefined（与 `navItemByPath` 同一语义）。 */
export function siteSettingsSectionByPath(path: string): SiteSettingsSection | undefined {
  if (!path) return undefined
  return ALL_SITE_SETTINGS_SECTIONS.find(section => section.path === path)
}

/** 按域名反查配置域。 */
export function siteSettingsSectionByKey(key: SiteSettingsKey): SiteSettingsSection | undefined {
  return ALL_SITE_SETTINGS_SECTIONS.find(section => section.key === key)
}

/** 取某个域拥有、且确实是 site 段顶层块的那些键（去重、保序）。 */
export function siteSectionBlocks(section: SiteSettingsSection): readonly SiteSectionBlock[] {
  return section.blocks
}

/**
 * 后台「站点配置」的编辑域（**路由、菜单子项与页面文件的唯一清单**）。
 *
 * 后端把运行时配置拆成十二个独立的段（ADR-0020 / ADR-0032 / ADR-0033 / ADR-0029），
 * 其中十一段在后台各有编辑页；`image_gc` 已有具名端点但没有编辑页/组件，所以不在
 * 本清单里。这里只描述「有编辑页的域」，不是全部配置段的清单。
 *
 * 2026-11 起菜单分两层组织（ADR-0029 改写版）：
 *   - **内容生产链**（关键词过滤 / 文章发布 / 文章审核）单独成一级分组，不再散落在
 *     「站点配置」（配置）与「审核」（队列）两处；
 *   - **站点配置**按「谁受影响」分组：用户操作（登录/私信/第三方）、验证码与通知
 *     （图形码/邮件/短信）、站点（基础信息），剩下 AI / 存储 / 日志 / 装饰 / 右栏卡片
 *     直接挂在这一层。
 *
 * 为什么按"谁受影响"而不是按段编号：段编号是后端实现顺序，运营读不懂；
 * 「登录设置」和「私信设置」都是读者能主动触发的动作，放在一组里才找得到。
 *
 * 为什么这里只存组件名（字符串）而不 import 组件：utils 层不依赖 Vue，
 * 沿用 `menu.ts` 的同一约定；页面文件自己按原名 import 配置组件，
 * 组件不改名、不移动。
 *
 * 路由约定：站点基础复用既有入口 `/site`（旧书签不失效），其余按段名拼装；
 * 被搬走的页面在旧地址留跳转页（`/site/content-filter` → `/content/keywords` 等）。
 */
import type { SiteRuntimeConfig } from '~/types/api'

export type SiteSettingsKey =
  | 'site-basics'
  | 'site-login'
  | 'site-chat'
  | 'captcha'
  | 'ai'
  | 'mail'
  | 'storage'
  | 'oauth'
  | 'sms'
  | 'log'
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

/**
 * 「站点配置」下的三个分组（ADR-0029 改写版：按「谁受影响」分，而不是按段编号排）。
 *
 * 三者都只是菜单上的一层分组、**没有自身页面**：点标题是展开/收起。
 * 分组自身不给路由，是因为它底下每一项都是独立页，多一个空落地页只会让
 * 运营多点一次。分组名字是运营读的，不是段名 —— 所以「用户操作」「验证码与通知」
 * 之下挂的可能是好几个段（邮件 + 短信 + 验证码 + 第三方登录）。
 */
export const SITE_SETTINGS_GROUP_LABEL = '站点'

/** 「用户操作」分组：读者能主动触发的动作（登录、私信、第三方登录）。 */
export const SITE_USER_ACTIONS_GROUP_LABEL = '用户操作'

/**
 * 「验证码与通知」分组：为了证明「你是你」而发出去的那条消息（图形码、邮件、短信）。
 *
 * 不叫「验证码」：邮件与短信两页除了发码，还管凭据、模板与签名，藏在「验证码」
 * 这个标题下会让人找不到改密钥的地方。
 */
export const SITE_VERIFICATION_GROUP_LABEL = '验证码与通知'

/** 站点页（属于 site 段，保存要走读-改-写）。 */
export const SITE_SETTINGS_PAGES: readonly SiteSettingsSection[] = [
  {
    key: 'site-basics',
    label: '站点基础',
    path: '/site',
    component: 'SettingsSiteBasics',
    blocks: ['site_info', 'seo'],
  },
]

/**
 * 其余编辑域：每个域一对独立端点（`GET /site/admin/<段>` / `PUT /site/<段>`），
 * 页面自己读写整段，不参与 site 段的读-改-写。
 *
 * `login` 这一域仍属 site 段（它和基础信息共用 `/site/site` 端点），所以它在
 * `blocks` 里声明 `login`；其余各域都是自己的段。
 */
export const SITE_SETTINGS_SECTIONS: readonly SiteSettingsSection[] = [
  {
    key: 'site-login',
    label: '登录设置',
    path: '/site/login',
    component: 'SettingsSiteLogin',
    // 拆段后这一页只剩「用户名密码登录」与登录失败限流（票 481）：
    // 邮箱/手机号/QQ 开关跟着通道走了，验证码闸门跟着验证码段走了，
    // 发码限流跟着邮件/短信段走了。它仍然属于 site 段，所以 blocks 声明 login。
    blocks: ['login'],
  },
  {
    key: 'site-chat',
    label: '私信设置',
    path: '/site/chat',
    component: 'SettingsSiteChat',
    blocks: ['chat'],
  },
  { key: 'oauth', label: '第三方登录', path: '/site/oauth', component: 'SettingsOAuth', blocks: [] },
  // 验证码与通知：图形码在前（它决定"要不要拦"，邮件/短信决定"怎么发"）。
  { key: 'captcha', label: '图形验证码', path: '/site/captcha', component: 'SettingsCaptcha', blocks: [] },
  { key: 'mail', label: '邮件', path: '/site/mail', component: 'SettingsMail', blocks: [] },
  { key: 'sms', label: '短信', path: '/site/sms', component: 'SettingsSMS', blocks: [] },
  // 站点级服务：与"用户操作"无关，也不发通知，各自独立。
  { key: 'ai', label: 'AI 助手', path: '/site/ai', component: 'SettingsAi', blocks: [] },
  { key: 'storage', label: '对象存储', path: '/site/storage', component: 'SettingsStorage', blocks: [] },
  { key: 'log', label: '日志', path: '/site/log', component: 'SettingsLog', blocks: [] },
  { key: 'deco', label: '装饰', path: '/site/deco', component: 'SettingsDeco', blocks: [] },
]

/**
 * 三个分组的成员表（唯一一份）。
 *
 * 写成"哪些域属于哪一组"而不是三处 filter：以前用两份白名单 filter 加一份
 * 六项否定式，新增一个域要同时改三处，而漏改的那一处不会报错 —— 域会悄悄从
 * 菜单里消失（`reconcileAdminNav` 只对"有 path 没页面"报警，不对"有页面没菜单"报警，
 * 那份文档漂移就是这么来的）。
 *
 * 不在表里的域直接挂在「站点配置」下（见 SITE_STANDALONE_SECTIONS）。
 */
const GROUP_MEMBERS: Readonly<Record<string, readonly SiteSettingsKey[]>> = {
  '用户操作': ['site-login', 'site-chat', 'oauth'],
  '验证码与通知': ['captcha', 'mail', 'sms'],
}

/** 取某个分组下的域（顺序取 GROUP_MEMBERS 里的顺序，也就是菜单顺序）。 */
function sectionsOfGroup(name: string): readonly SiteSettingsSection[] {
  const keys = GROUP_MEMBERS[name] ?? []
  return keys.flatMap(key => {
    const section = SITE_SETTINGS_SECTIONS.find(candidate => candidate.key === key)
    // 分组表里写了一个不存在的域是代码错误（拼错 key 或域被删了），
    // 让它在开发期就炸掉，而不是菜单里静默少一项。
    if (!section) throw new Error(`站点配置分组「${name}」引用了不存在的域: ${key}`)
    return [section]
  })
}

/** 「用户操作」分组下的域（顺序即菜单顺序）。 */
export const SITE_USER_ACTION_SECTIONS: readonly SiteSettingsSection[] = sectionsOfGroup('用户操作')

/** 「验证码与通知」分组下的域（顺序即菜单顺序）。 */
export const SITE_VERIFICATION_SECTIONS: readonly SiteSettingsSection[] = sectionsOfGroup('验证码与通知')

/** 直接挂在「站点配置」下、不属于上面两个分组的域。 */
export const SITE_STANDALONE_SECTIONS: readonly SiteSettingsSection[] = [
  ...SITE_SETTINGS_SECTIONS.filter(
    section => ![...GROUP_MEMBERS['用户操作']!, ...GROUP_MEMBERS['验证码与通知']!].includes(section.key),
  ),
]

/**
 * 内容生产链（ADR-0029 改写版）：改关键词 → 调发布策略 → 审文章。
 *
 * 这三页彼此是上下游关系，放在同一个一级分组里，不再散落在「站点配置」（配置）
 * 与「审核」（队列）两处。它们各自吃不同的数据源：关键词过滤是 content_filter 段、
 * 文章发布是 site 段的 article 块、文章审核是文章审核队列（不是配置）。
 * 所以这个分组**不出现在**站点配置域清单里，页面也不复用 `SiteSettingsSection`。
 */
export interface ContentPublishPage {
  label: string
  path: string
}

export const CONTENT_PUBLISH_GROUP_LABEL = '内容修改与发布'

export const CONTENT_PUBLISH_PAGES: readonly ContentPublishPage[] = [
  { label: '关键词过滤', path: '/content/keywords' },
  { label: '文章发布', path: '/content/publish' },
  { label: '文章审核', path: '/content/review' },
]

/** 全部编辑域：站点页在前，其余各域在后（菜单顺序另行按分组拼装）。 */
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



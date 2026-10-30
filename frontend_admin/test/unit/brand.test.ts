import { describe, expect, it } from 'vitest'

import {
  BRAND,
  BRAND_ASSETS,
  brandAbsoluteAssetUrl,
  brandAdminTitle,
  brandAiName,
  brandAssetUrl,
  brandCopyright,
  brandDescription,
  brandFullName,
  brandTitle,
} from '~/utils/brand'

/**
 * 后台的品牌单点定义。除了取值本身，这里钉两条与读者站**必须一致**的契约：
 * 英文名 / 中文名（字标结构），以及「标题为中文、字标为英文」的取值关系。
 * 后台覆盖率门槛卡在 95/90/95/95，这个模块是纯逻辑，必须全覆盖。
 */
describe('admin brand：品牌单点定义', () => {
  it('中英文名与字标结构：与读者站同一份品牌', () => {
    expect(BRAND.name.en).toBe('JellyBin')
    expect(BRAND.name.zh).toBe('果冻盒')
    expect(BRAND.wordmark.primary).toBe('JellyBin')
    expect(BRAND.wordmark.secondary).toBe('果冻盒')
    expect(brandFullName()).toBe('JellyBin（果冻盒）')
  })

  it('后台默认标题 = 英文名 + 管理后台', () => {
    expect(BRAND.adminLabel).toBe('管理后台')
    expect(brandAdminTitle()).toBe('JellyBin 管理后台')
  })

  it('页面标题：空值回落后台默认标题，已带品牌名不重复拼接', () => {
    expect(brandTitle()).toBe('JellyBin 管理后台')
    expect(brandTitle(null)).toBe('JellyBin 管理后台')
    expect(brandTitle('   ')).toBe('JellyBin 管理后台')
    expect(brandTitle('JellyBin 管理后台')).toBe('JellyBin 管理后台')
    expect(brandTitle('后台登录')).toBe('后台登录 · JellyBin 管理后台')
    expect(brandTitle('  仪表盘  ')).toBe('仪表盘 · JellyBin 管理后台')
  })

  it('描述回落：空值用品牌一句话介绍，有值则去除首尾空白', () => {
    expect(brandDescription()).toBe(BRAND.description)
    expect(brandDescription('  ')).toBe(BRAND.description)
    expect(brandDescription('  内容审核  ')).toBe('内容审核')
  })

  it('助手昵称统一为「捏捏」，站点配置的昵称优先', () => {
    expect(BRAND.assistant).toBe('捏捏')
    expect(brandAiName()).toBe('捏捏')
    expect(brandAiName(null)).toBe('捏捏')
    expect(brandAiName('   ')).toBe('捏捏')
    expect(brandAiName('  小果冻  ')).toBe('小果冻')
  })

  it('版权行带年份', () => {
    expect(brandCopyright(2026)).toBe('© 2026 JellyBin（果冻盒）')
    expect(brandCopyright(2030)).toBe('© 2030 JellyBin（果冻盒）')
  })

  it('站内资产路径跟随后台 app.baseURL（/admin/）', () => {
    expect(brandAssetUrl('/admin/', 'logo')).toBe('/admin/brand/logo-256.png')
    expect(brandAssetUrl('/admin', 'flat')).toBe('/admin/brand/jelly-flat.svg')
    expect(brandAssetUrl('', 'faviconRoot')).toBe('/favicon.ico')
    expect(BRAND_ASSETS.apple).toBe('brand/apple-touch-icon.png')
  })

  it('资产清单与读者站一致：不存在暗色专用的白色变体', () => {
    expect(Object.keys(BRAND_ASSETS).sort()).toEqual([
      'apple',
      'faviconRoot',
      'flat',
      'logo',
      'og',
    ])
    expect(BRAND_ASSETS).not.toHaveProperty('logoWhite')
    expect(Object.values(BRAND_ASSETS)).not.toContain('brand/logo-512-white.png')
  })

  it('绝对资产地址用于 OG / twitter（站点地址去尾部斜杠）', () => {
    expect(brandAbsoluteAssetUrl('https://blog.gentlybeing.cn/admin', 'og')).toBe(
      'https://blog.gentlybeing.cn/admin/brand/og-image.png',
    )
    expect(brandAbsoluteAssetUrl('https://blog.gentlybeing.cn/admin/', 'og')).toBe(
      'https://blog.gentlybeing.cn/admin/brand/og-image.png',
    )
    expect(brandAbsoluteAssetUrl('  ', 'og')).toBe('/brand/og-image.png')
  })
})

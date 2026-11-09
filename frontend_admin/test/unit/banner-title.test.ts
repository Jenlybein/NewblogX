import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

import { buildBannerCreatePayload } from '~/utils/bannerCover'
import { bannerTitleCell, bannerTitleText } from '~/utils/bannerTitle'

/**
 * 后台轮播标题（纯文本、可空）。
 *
 * 页面交给 Vue，这里锁三件会出事的事：
 *   1. 标题的收敛规则（只去首尾空白，不解析 HTML）；
 *   2. 创建 / 更新的请求体里标题字段一定存在（可空，但不能缺）；
 *   3. 页面与弹窗的落点：新增是**按钮 + 弹窗**（不再是常驻卡片），
 *      编辑复用同一张卡，且行上有编辑入口。
 *
 * #330 起「只改标题」那条独立路径已删除：新增与编辑共用 `BannerFormModal`，
 * 更新走完整表单的整体替换。所以这里不再断言 `saveBannerTitle`。
 */

const pageSource = readFileSync(join(process.cwd(), 'app/pages/media/index.vue'), 'utf8')
const modalSource = readFileSync(
  join(process.cwd(), 'app/components/media/BannerFormModal.vue'),
  'utf8',
)
const adminCss = readFileSync(join(process.cwd(), 'app/assets/css/admin.css'), 'utf8')

describe('bannerTitleText / bannerTitleCell', () => {
  it('只收敛首尾空白，HTML 标签原样保留（不解析、不剥离）', () => {
    expect(bannerTitleText('  春季上新  ')).toBe('春季上新')
    expect(bannerTitleText('<b>粗体</b>')).toBe('<b>粗体</b>')
  })

  it('空、纯空白、缺失都收敛成空串', () => {
    expect(bannerTitleText('')).toBe('')
    expect(bannerTitleText('   ')).toBe('')
    expect(bannerTitleText(undefined)).toBe('')
  })

  it('列表单元格对空标题给出可读占位，而不是一片空白', () => {
    expect(bannerTitleCell('春季上新')).toBe('春季上新')
    expect(bannerTitleCell('')).toBe('未填写')
    expect(bannerTitleCell(null)).toBe('未填写')
  })
})

describe('创建 / 更新的请求体带标题', () => {
  it('填了标题就把标题带进请求体（没有 crop 字段）', () => {
    const payload = buildBannerCreatePayload({
      coverImageId: '42',
      title: '春季上新',
      href: 'https://example.com',
      show: true,
    })

    expect(payload).toEqual({
      cover_image_id: '42',
      title: '春季上新',
      href: 'https://example.com',
      show: true,
    })
    expect('crop' in payload).toBe(false)
  })

  it('没填标题时带空串（标题可空，但字段不能缺）', () => {
    const payload = buildBannerCreatePayload({
      coverImageId: '42',
      href: '',
      show: false,
    })

    expect(payload.title).toBe('')
  })
})

describe('新增是按钮 + 弹窗，不是常驻卡片（#330）', () => {
  it('页面里有「新增轮播」按钮，点它才打开弹窗', () => {
    expect(pageSource).toMatch(/新增轮播/u)
    expect(pageSource).toMatch(/openCreateEditor/u)
    expect(pageSource).toMatch(/BannerFormModal/u)
  })

  it('页面里不再有常驻的「新增轮播」卡片，也没有只改标题的弹窗', () => {
    // 旧结构的三个特征：常驻卡片的标题、标题编辑函数、以及它用的写请求。
    // 断言按**代码字面量**取，不按散文取——注释里提到旧行为不该让用例变红。
    expect(pageSource).not.toMatch(/title="新增轮播"/u)
    expect(pageSource).not.toMatch(/openTitleEditor/u)
    expect(pageSource).not.toMatch(/'编辑标题'/u)
    expect(pageSource).not.toMatch(/saveBannerTitle/u)
  })

  it('弹窗宽度受控，不铺满全屏（且不能写在 scoped 样式里）', () => {
    // 宽度只能来自**全局**规则：`preset="card"` 会把 class 落到 naive-ui 自己渲染的
    // `.n-card` 上，那个节点没有本组件的 `data-v-*` 标记 —— 写在 scoped 里等于没写，
    // 会回落到 naive-ui 默认的 `max-width: calc(100vw - 32px)`（铺满全屏）。
    // 这条曾经就是真实 bug：第一版写在 `<style scoped>` 里，用户看到的是全屏弹窗。
    expect(adminCss).toMatch(/\.n-modal\.banner-form-modal\s*\{/u)
    expect(adminCss).toMatch(/\.n-modal\.banner-form-modal\s*\{[^}]*width:\s*560px/u)
    expect(modalSource).toMatch(/class="banner-form-modal"/u)

    const scopedStyle = modalSource.slice(modalSource.indexOf('<style scoped>'))
    expect(scopedStyle).not.toMatch(/\.banner-form-modal\s*\{/u)
  })
})

describe('编辑复用同一张卡，四个字段都能改（#330）', () => {
  it('弹窗同时提供标题 / 跳转地址 / 展示 / 封面', () => {
    for (const role of ['banner-form-title', 'banner-form-href', 'banner-form-show']) {
      expect(modalSource).toContain(role)
    }
    expect(modalSource).toMatch(/封面图片/u)
    expect(modalSource).toMatch(/跳转地址/u)
  })

  it('两种模式只有一个组件：创建与编辑都走 createBanner / updateBanner', () => {
    expect(modalSource).toMatch(/createBanner/u)
    expect(modalSource).toMatch(/updateBanner/u)
  })

  it('行上有编辑入口，不再只有编辑标题', () => {
    expect(pageSource).toMatch(/openEditEditor/u)
    expect(pageSource).toMatch(/编辑/u)
  })

  it('页面流程不再出现取景步骤：没有取景按钮/弹窗/crop 载荷（#204）', () => {
    expect(pageSource).not.toMatch(/手动取景|调整取景|重新取景|framingDialog|BannerFraming/u)
    expect(pageSource).not.toMatch(/crop/u)
  })
})

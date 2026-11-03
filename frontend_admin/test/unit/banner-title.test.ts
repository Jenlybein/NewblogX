import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it, vi } from 'vitest'

import { buildBannerCreatePayload, saveBannerTitle } from '~/utils/bannerCover'
import { bannerTitleCell, bannerTitleText } from '~/utils/bannerTitle'

/**
 * 后台轮播标题（纯文本、可空、可编辑）。
 *
 * 页面交给 Vue，这里锁三件会出事的事：
 *   1. 创建请求带不带标题；
 *   2. 编辑标题时是否保留该行其余字段、空标题能不能存（PUT 是整体替换）；
 *   3. 页面流程里不再有取景步骤（#204：取景按钮、弹窗与 crop 载荷都已删除）。
 * 另外用一条源码断言钉住"表单有输入、列表有标题列、行上有编辑入口"这三个落点
 * （页面组件需要整套 naive-ui 交互，这类落点沿用仓库既有的源码门禁写法）。
 */

const pageSource = readFileSync(
  join(process.cwd(), 'app/pages/media/index.vue'),
  'utf8',
)

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

describe('创建请求带标题', () => {
  it('填了标题就把标题带进创建请求（请求体没有 crop）', () => {
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

describe('saveBannerTitle：编辑已有轮播的标题', () => {
  const update = vi.fn(async () => undefined)

  it('把新标题写进请求体，并保留该行其余字段', async () => {
    await expect(
      saveBannerTitle(
        {
          id: '9',
          cover_image_id: '7',
          href: 'https://example.com',
          show: false,
          title: '旧标题',
        },
        '新标题',
        update,
      ),
    ).resolves.toBe(true)

    expect(update).toHaveBeenCalledWith('9', {
      cover_image_id: '7',
      title: '新标题',
      href: 'https://example.com',
      show: false,
    })
    const payload = update.mock.calls.at(-1)?.[1] as object
    expect('crop' in payload).toBe(false)
  })

  it('空标题可以保存（清空误填的标题）', async () => {
    await expect(
      saveBannerTitle(
        { id: '9', cover_image_id: '7', href: '/a', show: true, title: '旧标题' },
        '',
        update,
      ),
    ).resolves.toBe(true)

    expect(update).toHaveBeenCalledWith('9', expect.objectContaining({ title: '' }))
  })

  it('没有封面图片 ID 时不发写请求（更新接口无从整体替换）', async () => {
    await expect(
      saveBannerTitle({ id: '1', cover_image_id: '' }, '标题', update),
    ).resolves.toBe(false)
    expect(update).not.toHaveBeenCalled()
  })

  it('更新失败原样上抛（页面据此提示，不假装已保存）', async () => {
    const failing = vi.fn(async () => {
      throw new Error('网络中断')
    })

    await expect(
      saveBannerTitle({ id: '9', cover_image_id: '7' }, '标题', failing),
    ).rejects.toThrow('网络中断')
  })
})

describe('后台轮播页的标题落点', () => {
  it('创建表单有标题输入', () => {
    expect(pageSource).toMatch(/bannerForm\.title/u)
    expect(pageSource).toMatch(/label="标题"/u)
  })

  it('列表有标题列，空标题显示占位', () => {
    expect(pageSource).toMatch(/title:\s*'标题'/u)
    expect(pageSource).toMatch(/bannerTitleCell/u)
  })

  it('行上有编辑标题的入口，且保存走标题写请求', () => {
    expect(pageSource).toMatch(/编辑标题/u)
    expect(pageSource).toMatch(/saveBannerTitle/u)
  })

  it('页面流程不再出现取景步骤：没有取景按钮/弹窗/crop 载荷（#204）', () => {
    expect(pageSource).not.toMatch(/手动取景|调整取景|重新取景|framingDialog|BannerFraming/u)
    expect(pageSource).not.toMatch(/crop/u)
  })
})

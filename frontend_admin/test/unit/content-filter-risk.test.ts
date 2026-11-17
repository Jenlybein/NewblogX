import { describe, expect, it } from 'vitest'

import {
  CONTENT_FILTER_ACTION_OPTIONS,
  CONTENT_FILTER_MUTE_DAYS_DEFAULT,
  CONTENT_FILTER_MUTE_DAYS_MAX,
  CONTENT_FILTER_MUTE_DAYS_MIN,
  CONTENT_FILTER_STATUS_OPTIONS,
  CONTENT_FILTER_SURFACE_ACTIONS,
  CONTENT_FILTER_SURFACE_OPTIONS,
  contentFilterActionLabel,
  contentFilterCategoryLabel,
  contentFilterStatusLabel,
  contentFilterStatusTagType,
  contentFilterSurfaceLabel,
  contentFilterTierMeta,
  splitContentFilterSnippet,
} from '~/utils/contentFilterRisk'

/**
 * 关键词风险页的文案与切片口径（ADR-0032 / 票 396–399）。
 *
 * 这些函数看起来"只是映射"，但每一处都有错一次就误导运营的口径：
 * 状态漏一个 `account`、档位颜色反过来、rune 下标当 UTF-16 下标切 ——
 * 页面都不会报错，只是显示的结论是错的。所以逐条钉住。
 */

describe('文本面与状态的中文映射', () => {
  it('六个文本面都有中文；未知值原样返回（宁可露 code 也不猜）', () => {
    expect(contentFilterSurfaceLabel('article')).toBe('文章')
    expect(contentFilterSurfaceLabel('comment')).toBe('评论')
    expect(contentFilterSurfaceLabel('nickname')).toBe('昵称')
    expect(contentFilterSurfaceLabel('abstract')).toBe('简介')
    expect(contentFilterSurfaceLabel('chat')).toBe('私信')
    expect(contentFilterSurfaceLabel('collection')).toBe('合集')
    // 未知 code 不猜中文：猜错比露出 code 更难查。
    expect(contentFilterSurfaceLabel('whatever')).toBe('whatever')
    expect(contentFilterSurfaceLabel(null)).toBe('-')
    expect(contentFilterSurfaceLabel(undefined)).toBe('-')
  })

  it('四个处置状态都有中文（含 #399 新增的 account），空值退化成 -', () => {
    expect(contentFilterStatusLabel('pending')).toBe('待处理')
    expect(contentFilterStatusLabel('ignored')).toBe('已忽略')
    expect(contentFilterStatusLabel('deleted')).toBe('已删除内容')
    expect(contentFilterStatusLabel('account')).toBe('账号已处置')
    expect(contentFilterStatusLabel('')).toBe('-')
  })

  it('状态标签颜色：待处理黄、已删除红、账号已处置 info、其余默认', () => {
    expect(contentFilterStatusTagType('pending')).toBe('warning')
    expect(contentFilterStatusTagType('deleted')).toBe('error')
    expect(contentFilterStatusTagType('account')).toBe('info')
    expect(contentFilterStatusTagType('ignored')).toBe('default')
    expect(contentFilterStatusTagType(null)).toBe('default')
  })

  it('七个类目都有中文，未知值原样返回', () => {
    expect(contentFilterCategoryLabel('drugs')).toBe('毒品')
    expect(contentFilterCategoryLabel('gambling')).toBe('赌博')
    expect(contentFilterCategoryLabel('fraud')).toBe('诈骗')
    expect(contentFilterCategoryLabel('porn')).toBe('色情')
    expect(contentFilterCategoryLabel('fake_docs')).toBe('假证发票')
    expect(contentFilterCategoryLabel('politics')).toBe('涉政')
    expect(contentFilterCategoryLabel('violence')).toBe('暴恐')
    expect(contentFilterCategoryLabel('xxx')).toBe('xxx')
    expect(contentFilterCategoryLabel(null)).toBe('-')
  })
})

describe('档位：中文 + 颜色 + 上下文高亮', () => {
  it('拦截词给红、观察词给黄', () => {
    const block = contentFilterTierMeta('block')
    expect(block.label).toBe('拦截词')
    expect(block.type).toBe('error')

    const watch = contentFilterTierMeta('watch')
    expect(watch.label).toBe('观察词')
    expect(watch.type).toBe('warning')
  })

  it('未知档位按观察词处理（不把颜色当判据）', () => {
    expect(contentFilterTierMeta('weird').label).toBe('观察词')
    expect(contentFilterTierMeta(null).type).toBe('warning')
    expect(contentFilterTierMeta(undefined).label).toBe('观察词')
  })
})

describe('上下文切片按 rune 走（不是 UTF-16 code unit）', () => {
  it('中文片段按 rune 下标切', () => {
    // 「这里有冰毒」：start=2 end=4 → 命中「有冰」
    const got = splitContentFilterSnippet('这里有冰毒', 2, 4)
    expect(got).toEqual({ before: '这里', hit: '有冰', after: '毒' })
  })

  it('emoji 占两个 UTF-16 code unit：用 slice 会错位一个字，这里不会', () => {
    // 「😀冰毒😀」的 rune 序列是 [😀,冰,毒,😀]；start=1 end=3 应命中「冰毒」
    const got = splitContentFilterSnippet('😀冰毒😀', 1, 3)
    expect(got.before).toBe('😀')
    expect(got.hit).toBe('冰毒')
    expect(got.after).toBe('😀')
  })

  it('越界下标被钳进 [0, 长度]，且 end < start 时退化成空命中', () => {
    expect(splitContentFilterSnippet('abc', -5, 3)).toEqual({ before: '', hit: 'abc', after: '' })
    expect(splitContentFilterSnippet('abc', 0, 99)).toEqual({ before: '', hit: 'abc', after: '' })
    expect(splitContentFilterSnippet('abc', 2, 1)).toEqual({ before: 'ab', hit: '', after: 'c' })
  })

  it('空文本与 null 都不炸', () => {
    expect(splitContentFilterSnippet('', 0, 0)).toEqual({ before: '', hit: '', after: '' })
    expect(splitContentFilterSnippet(null as unknown as string, 0, 2)).toEqual({
      before: '',
      hit: '',
      after: '',
    })
  })
})

describe('处置动作（票 398 / 399）', () => {
  it('恰好四个动作：内容两个 + 账号两个', () => {
    expect(CONTENT_FILTER_ACTION_OPTIONS.map(option => option.value))
      .toEqual(['delete', 'ignore', 'ban', 'mute'])
  })

  it('动作的中文名，未知值原样返回', () => {
    expect(contentFilterActionLabel('delete')).toBe('删除内容')
    expect(contentFilterActionLabel('mute')).toBe('禁言账号')
    expect(contentFilterActionLabel('nope')).toBe('nope')
    expect(contentFilterActionLabel(null)).toBe('-')
  })

  it('禁言天数口径与后端一致：缺省 7、范围 1–365', () => {
    expect(CONTENT_FILTER_MUTE_DAYS_DEFAULT).toBe(7)
    expect(CONTENT_FILTER_MUTE_DAYS_MIN).toBe(1)
    expect(CONTENT_FILTER_MUTE_DAYS_MAX).toBe(365)
  })

  it('昵称 / 简介的删除是「重置资料」，私信面不可删', () => {
    expect(CONTENT_FILTER_SURFACE_ACTIONS.nickname?.deleteLabel).toBe('重置资料')
    expect(CONTENT_FILTER_SURFACE_ACTIONS.abstract?.deleteLabel).toBe('重置资料')
    expect(CONTENT_FILTER_SURFACE_ACTIONS.article?.deleteLabel).toBe('删除文章')
    // 私信没有软删语义：界面必须禁用删除并说明原因。
    expect(CONTENT_FILTER_SURFACE_ACTIONS.chat?.deletable).toBe(false)
    expect(CONTENT_FILTER_SURFACE_ACTIONS.article?.deletable).toBe(true)
  })
})

describe('筛选项清单', () => {
  it('文本面筛选项覆盖六个面，且与映射表同源', () => {
    expect(CONTENT_FILTER_SURFACE_OPTIONS.map(option => option.value))
      .toEqual(['article', 'comment', 'nickname', 'abstract', 'chat', 'collection'])
    for (const option of CONTENT_FILTER_SURFACE_OPTIONS) {
      expect(option.label).toBe(contentFilterSurfaceLabel(option.value))
    }
  })

  it('状态筛选项覆盖四个状态（含 #399 的 account），且与状态中文映射同源', () => {
    expect(CONTENT_FILTER_STATUS_OPTIONS.map(option => option.value))
      .toEqual(['pending', 'ignored', 'deleted', 'account'])
    for (const option of CONTENT_FILTER_STATUS_OPTIONS) {
      expect(option.label).toBe(contentFilterStatusLabel(option.value))
    }
  })
})



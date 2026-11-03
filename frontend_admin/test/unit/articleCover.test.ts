import { describe, expect, it } from 'vitest'

import { resolveArticleCoverSrc } from '~/utils/articleCover'

/**
 * 后台文章列表的封面槽（票 210）。
 *
 * 与读者站同一条规则：有封面就显示；缺封面、或这一行的封面已经加载失败，
 * 都只留一块空白（无文字、无纹理、对辅助技术隐藏），不再画 "NO COVER"。
 * 这里锁的是"这一格到底该不该出现 `<img>`"这个决定，而不是页面怎么摆位。
 */
describe('resolveArticleCoverSrc：缺封面与加载失败都退化成空白', () => {
  it('有封面就原样交给 <img>', () => {
    expect(resolveArticleCoverSrc({ cover: 'https://cdn.example.com/a.png' }, new Set())).toBe(
      'https://cdn.example.com/a.png',
    )
  })

  it('没有封面地址（缺字段 / null / 空串 / 纯空白）一律返回空串', () => {
    expect(resolveArticleCoverSrc({}, new Set())).toBe('')
    expect(resolveArticleCoverSrc({ cover: null }, new Set())).toBe('')
    expect(resolveArticleCoverSrc({ cover: '' }, new Set())).toBe('')
    expect(resolveArticleCoverSrc({ cover: '   ' }, new Set())).toBe('')
  })

  it('这一行已经加载失败过就返回空串（坏链接看起来只是没有图）', () => {
    const failed = new Set(['https://cdn.example.com/broken.png'])

    expect(resolveArticleCoverSrc({ cover: 'https://cdn.example.com/broken.png' }, failed)).toBe('')
    expect(resolveArticleCoverSrc({ cover: 'https://cdn.example.com/fresh.png' }, failed)).toBe(
      'https://cdn.example.com/fresh.png',
    )
  })

  it('失败记的是裁剪后的地址（与真正发出去的那条请求一致）', () => {
    const failed = new Set(['https://cdn.example.com/broken.png'])

    expect(resolveArticleCoverSrc({ cover: ' https://cdn.example.com/broken.png ' }, failed)).toBe('')
  })
})

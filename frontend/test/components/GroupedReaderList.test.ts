import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GroupedReaderList from '~/components/common/GroupedReaderList.vue'

/**
 * 读者侧分组展示（票 353，2026-11-12 调整尾块分隔线）。
 *
 * 与后台的差别只有两条，这层用例把它们钉死：
 *   1. 分组按保存顺序渲染，组内顺序照后端给的（前端不重排）；
 *   2. 没有分组的实体作为**尾块**渲染，前面用一条淡色细线 + 「未分组」标签隔开
 *      （用户明确要求：`---- 未分组 ----` 这样的分隔线，否则尾块会贴着上一组）。
 * 另外空分组整段不出现 —— 只有标题、下面什么都没有的分段只会让页面看起来坏了。
 */

vi.mock('~/components/common/FolderCardList.vue', () => ({
  default: {
    name: 'FolderCardList',
    props: { cards: { type: Array, required: true } },
    template: '<ul class="stub-card-list"><li v-for="c in cards" :key="c.id" :data-id="c.id">{{ c.title }}</li></ul>',
  },
}))

function card(id: string, title = id) {
  return { id, title, description: '', cover: '', href: `/collections/${id}`, countLabel: '1 篇' }
}

function tree(groups: { id: string; title: string; items: string[] }[], ungrouped: string[] = []) {
  return {
    groups: groups.map(g => ({ id: g.id, title: g.title, items: g.items.map(owner_id => ({ owner_id })) })),
    ungrouped: ungrouped.map(owner_id => ({ owner_id })),
  }
}

describe('读者侧分组：顺序', () => {
  it('分组按给定顺序渲染，组内卡片也照给定顺序', () => {
    const wrapper = mount(GroupedReaderList, {
      props: {
        cards: [card('a'), card('b'), card('c')],
        groups: tree(
          [
            { id: 'g2', title: '第二组', items: ['c', 'a'] },
            { id: 'g1', title: '第一组', items: ['b'] },
          ],
          [],
        ),
      },
    })

    const sections = wrapper.findAll('.grouped-reader-list__section')
    expect(sections).toHaveLength(2)
    // 分组顺序 = 后端给的顺序（g2 在前）。
    expect(sections[0]!.attributes('data-role')).toBe('reader-group-g2')
    expect(sections[0]!.text()).toContain('第二组')
    // 组内顺序 = 后端给的顺序（c 在 a 前）。
    expect(sections[0]!.findAll('[data-id]').map(n => n.attributes('data-id'))).toEqual(['c', 'a'])
  })

  it('空分组整段不渲染', () => {
    const wrapper = mount(GroupedReaderList, {
      props: {
        cards: [card('a')],
        groups: tree([
          { id: 'g1', title: '有内容', items: ['a'] },
          { id: 'g2', title: '空组', items: [] },
        ]),
      },
    })

    expect(wrapper.find('[data-role="reader-group-g2"]').exists()).toBe(false)
  })
})

describe('读者侧分组：尾块分隔线', () => {
  it('未分组实体作为尾块，前面有一条淡色细线和「未分组」标签', () => {
    const wrapper = mount(GroupedReaderList, {
      props: {
        cards: [card('a'), card('b')],
        groups: tree([{ id: 'g1', title: '第一组', items: ['a'] }], ['b']),
      },
    })

    const trailing = wrapper.get('[data-role="reader-group-trailing"]')
    expect(trailing.findAll('[data-id]').map(n => n.attributes('data-id'))).toEqual(['b'])
    // 分隔线只在"上面确实有分组"时出现：整页没有分组时它只是一段普通列表。
    expect(trailing.find('.grouped-reader-list__divider').exists()).toBe(true)
    expect(trailing.get('.grouped-reader-list__divider-label').text()).toBe('未分组')
  })

  it('每个分组头都有标题、计数与一条发丝线（此前整块没有样式）', () => {
    const wrapper = mount(GroupedReaderList, {
      props: {
        cards: [card('a'), card('b')],
        groups: tree([{ id: 'g1', title: '第一组', items: ['a', 'b'] }]),
      },
    })

    const head = wrapper.get('[data-role="reader-group-g1"] .grouped-reader-list__head')
    expect(head.get('.grouped-reader-list__title').text()).toBe('第一组')
    expect(head.get('.grouped-reader-list__count').text()).toContain('2')
    expect(head.find('.grouped-reader-list__line').exists()).toBe(true)
  })

  it('分组读面里没有的卡片也落到尾块（新建成还没进过分组）', () => {
    const wrapper = mount(GroupedReaderList, {
      props: {
        cards: [card('a'), card('b'), card('c')],
        groups: tree([{ id: 'g1', title: '第一组', items: ['a'] }], ['b']),
      },
    })

    const trailing = wrapper.get('[data-role="reader-group-trailing"]')
    expect(trailing.findAll('[data-id]').map(n => n.attributes('data-id'))).toEqual(['b', 'c'])
  })

  it('没有分组读面时退回一个平铺网格：不画分隔线，也不出现「未分组」', () => {
    const wrapper = mount(GroupedReaderList, {
      props: { cards: [card('a'), card('b')], groups: null },
    })

    expect(wrapper.find('.grouped-reader-list__section').exists()).toBe(true)
    expect(wrapper.findAll('[data-id]').map(n => n.attributes('data-id'))).toEqual(['a', 'b'])
    expect(wrapper.find('.grouped-reader-list__divider').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('未分组')
  })
})

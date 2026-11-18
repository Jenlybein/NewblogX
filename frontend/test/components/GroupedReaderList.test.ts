import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import { blockAfter, blockAfterContaining } from '../helpers/designFiles'

import GroupedReaderList from '~/components/common/GroupedReaderList.vue'

const COLLAPSE_SOURCE = 'app/components/common/GroupedReaderList.vue'

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

describe('读者侧分组：折叠', () => {
  /**
   * 票 437：折叠从 `display` 切换改成轨道过渡（`grid-template-rows 0fr ↔ 1fr`）。
   * 对外可观察的契约因此有条变化：收起后容器**仍挂载**（要有东西才能做高度过渡），
   * 靠 `inert` 保证里面的卡片不可聚焦、不进读屏。
   */
  it('点分组头只收起这一组的卡片，容器仍在但带 inert（展开后撤掉）', async () => {
    const wrapper = mount(GroupedReaderList, {
      props: {
        cards: [card('a'), card('b'), card('c')],
        groups: tree([
          { id: 'g1', title: '第一组', items: ['a'] },
          { id: 'g2', title: '第二组', items: ['b', 'c'] },
        ]),
      },
    })

    const head = wrapper.get('[data-role="reader-group-g1"] .grouped-reader-list__head')
    const toggle = head.get('.grouped-reader-list__toggle')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(toggle.attributes('aria-controls')).toBe('reader-group-panel-g1')

    const panel = wrapper.get('[data-role="reader-group-g1"] .grouped-reader-list__panel')
    // 展开态：容器可聚焦路径正常（没有 inert）。
    expect(panel.attributes('inert')).toBeUndefined()
    expect(panel.classes()).not.toContain('is-collapsed')

    await toggle.trigger('click')

    // 折叠：aria 状态翻转、加 is-collapsed、容器仍挂载且 inert。
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(head.classes()).toContain('is-collapsed')
    expect(panel.classes()).toContain('is-collapsed')
    expect(panel.attributes('inert')).toBeDefined()
    // 卡片没有被卸载：只是被轨道裁剪住，过渡结束前空间"被看见"地让出来。
    expect(panel.findAll('[data-id]').map(n => n.attributes('data-id'))).toEqual(['a'])

    // 别的组不受影响。
    const other = wrapper.get('[data-role="reader-group-g2"] .grouped-reader-list__panel')
    expect(other.classes()).not.toContain('is-collapsed')
    expect(other.attributes('inert')).toBeUndefined()

    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(panel.classes()).not.toContain('is-collapsed')
    expect(panel.attributes('inert')).toBeUndefined()
  })

  it('尾块的「未分组」分隔线也能收起 / 展开', async () => {
    const wrapper = mount(GroupedReaderList, {
      props: {
        cards: [card('a'), card('b')],
        groups: tree([{ id: 'g1', title: '第一组', items: ['a'] }], ['b']),
      },
    })

    const toggle = wrapper.get('.grouped-reader-list__divider-toggle')
    expect(toggle.attributes('aria-expanded')).toBe('true')

    await toggle.trigger('click')

    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(toggle.classes()).toContain('is-collapsed')

    const panel = wrapper.get('[data-role="reader-group-trailing"] .grouped-reader-list__panel')
    expect(panel.classes()).toContain('is-collapsed')
    expect(panel.attributes('inert')).toBeDefined()
    expect(panel.findAll('[data-id]')).toHaveLength(1)

    await toggle.trigger('click')
    expect(panel.classes()).not.toContain('is-collapsed')
    expect(panel.attributes('inert')).toBeUndefined()
  })

  /**
   * 组件测试拿不到计算样式，时长与轨道取值改在样式源码上钉。
   * 锁三件事：轨道是 0fr ↔ 1fr、展开取 base（200ms）而收起取 fast（120ms）、
   * 内容 opacity 始终 120ms，且「减少动态」下整段瞬时（不是放慢）。
   */
  it('折叠过渡：轨道 0fr ↔ 1fr，展开 200 / 收起 120 / 内容 opacity 120', () => {
    const source = readFileSync(join(process.cwd(), COLLAPSE_SOURCE), 'utf8')
    const panelBase = blockAfter(source, '.grouped-reader-list__panel {')
    const collapsed = blockAfter(source, '.grouped-reader-list__panel.is-collapsed {')
    const clip = blockAfter(source, '.grouped-reader-list__panel-clip {')
    const collapsedClip = blockAfter(source, '.grouped-reader-list__panel.is-collapsed .grouped-reader-list__panel-clip {')
    const reduced = blockAfterContaining(source, '@media (prefers-reduced-motion: reduce)', '.grouped-reader-list__panel')

    // 展开是默认态（目标态决定过渡时长）：1fr + base 档。
    expect(panelBase).toMatch(/display:\s*grid;/u)
    expect(panelBase).toMatch(/grid-template-rows:\s*1fr;/u)
    expect(panelBase).toContain('var(--motion-base)')
    expect(panelBase).toContain('var(--motion-ease)')
    // 收起态：0fr + fast 档。
    expect(collapsed).toMatch(/grid-template-rows:\s*0fr;/u)
    expect(collapsed).toContain('var(--motion-fast)')
    expect(collapsed).toContain('var(--motion-ease)')
    // 内容透明度两个方向都用 fast 档（120ms）。
    expect(clip).toContain('var(--motion-fast) var(--motion-ease)')
    expect(collapsedClip).toMatch(/opacity:\s*0;/u)
    // 轨道动画的承载层：0fr 真能收成 0 的前提。
    expect(clip).toMatch(/overflow:\s*hidden;/u)
    expect(clip).toMatch(/min-height:\s*0;/u)
    // 减少动态：瞬时（不是放慢）。
    expect(reduced).toContain('transition: none')
  })

  it('不再用 v-show / display 切换做折叠（也不写死毫秒）', () => {
    const source = readFileSync(join(process.cwd(), COLLAPSE_SOURCE), 'utf8')
    const style = source.slice(source.indexOf('<style scoped>')).replace(/\/\*[\s\S]*?\*\//gu, '')

    expect(source).not.toContain('v-show')
    // 收起不能靠整块 display: none（那会"啪"地跳一下）。
    expect(style).not.toMatch(/(?:^|\n)\s*display:\s*none;/u)
    // 时长只允许取 token，不写死毫秒。
    expect(style).not.toMatch(/\d+ms/u)
  })
})

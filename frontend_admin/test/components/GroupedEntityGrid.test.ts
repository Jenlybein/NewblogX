import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import GroupedEntityGrid from '~/components/articles/GroupedEntityGrid.vue'
import { useEntityGroups } from '~/composables/useEntityGroups'
import { nextTick } from 'vue'

/**
 * 合集 / 项目分组网格（票 348）。
 *
 * 钉的是四条交互语义，而不是像素：
 *   1. 未分组固定第一，空时整段消失；
 *   2. 拖动分组标题期间内容临时收起（否则长列表看不出拖到哪）；
 *   3. 任一拖拽结束只抛 orderChange，快照由页面取（组件是哑的）；
 *   4. 顺序的保存节奏由 useEntityGroups 负责：10 秒防抖 + 离开前 flush。
 */

const adminApi = vi.hoisted(() => ({
  getAdminCollections: vi.fn(),
  getAdminGroups: vi.fn(),
  replaceAdminGroupOrder: vi.fn(),
}))

vi.mock('~/services/admin', () => adminApi)

function card(id: string, title = id) {
  return {
    id,
    title,
    description: '描述',
    cover: '',
    fallback_cover: '',
    article_count: 2,
    displayed: true,
  }
}

function mountGrid(props: Record<string, unknown> = {}) {
  return mount(GroupedEntityGrid, {
    props: {
      kind: 'collection',
      groups: [{ id: 'g1', title: '第一组', items: [card('a')] }],
      ungrouped: [card('b')],
      ...props,
    },
    global: {
      stubs: {
        // 拖拽库在 happy-dom 里跑不起来：桩成透传容器，语义由事件断言。
        VueDraggable: {
          name: 'VueDraggable',
          props: ['modelValue', 'group', 'animation', 'handle', 'ghostClass'],
          emits: ['update:modelValue', 'end', 'start'],
          template: '<div><slot /></div>',
        },
      },
    },
  })
}

describe('分组网格：结构与未分组', () => {
  it('未分组固定第一，分组标题在其后', () => {
    const wrapper = mountGrid()

    const sections = wrapper.findAll('.grouped-grid__section')
    expect(sections[0]!.attributes('data-role')).toBe('group-ungrouped')
    expect(sections[1]!.attributes('data-role')).toBe('group-g1')
    expect(wrapper.get('[data-role="group-ungrouped"]').text()).toContain('未分组')
  })

  it('没有未分组条目时整段消失（空标题没有意义）', () => {
    const wrapper = mountGrid({ ungrouped: [] })
    expect(wrapper.find('[data-role="group-ungrouped"]').exists()).toBe(false)
  })

  it('分组里的空组给一句"把卡片拖进来"的提示', () => {
    const wrapper = mountGrid({ groups: [{ id: 'g1', title: '空组', items: [] }] })
    expect(wrapper.get('[data-role="group-empty-g1"]').text()).toContain('拖到这条标题上')
  })
})

describe('分组网格：拖拽', () => {
  it('卡片拖完只抛 orderChange（组件不发请求）', async () => {
    const wrapper = mountGrid()

    // 拖拽容器是桩组件：直接向它抛 end（真实拖拽在 happy-dom 里跑不起来）。
    const cardList = wrapper.findAllComponents({ name: 'VueDraggable' }).at(-1)!
    await cardList.vm.$emit('end')
    await flushPromises()

    expect(wrapper.emitted('orderChange')).toHaveLength(1)
    expect(adminApi.replaceAdminGroupOrder).not.toHaveBeenCalled()
  })

  it('拖动分组标题期间内容临时收起，松手后恢复', async () => {
    const wrapper = mountGrid()

    const groupList = wrapper.findAllComponents({ name: 'VueDraggable' })[1]!
    await groupList.vm.$emit('start')
    await nextTick()
    // 收起：卡片容器带 v-show=false（样式上不可见）。
    expect(wrapper.get('[data-role="collection-group-cards-g1"]').attributes('style')).toContain('display: none')

    await groupList.vm.$emit('end')
    await nextTick()
    expect(wrapper.get('[data-role="collection-group-cards-g1"]').attributes('style') ?? '').not.toContain('display: none')
  })

  it('分组标题上有改名与删除入口', () => {
    const wrapper = mountGrid()
    expect(wrapper.find('[data-role="group-rename-g1"]').exists()).toBe(true)
    expect(wrapper.find('[data-role="group-delete-g1"]').exists()).toBe(true)
  })
})

describe('useEntityGroups：归位与保存节奏', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
  })

  function setup() {
    const saveOrder = vi.fn().mockResolvedValue(undefined)
    const api = useEntityGroups({
      kind: 'collection',
      loadCards: async () => [card('a'), card('b'), card('c')],
      loadGroups: async () => ({
        groups: [{ id: 'g1', title: '第一组', items: [{ owner_id: 'a' }] }],
        ungrouped: [{ owner_id: 'b' }],
      }),
      saveOrder,
      onError: vi.fn(),
    })
    return { api, saveOrder }
  }

  it('按分组树归位；树里没有的卡片落到未分组末尾（不消失）', async () => {
    const { api } = setup()
    await api.refresh()

    expect(api.groups.value[0]!.items.map(i => i.id)).toEqual(['a'])
    // b 在未分组里，c 树里没有 → 也落到未分组末尾。
    expect(api.ungrouped.value.map(i => i.id)).toEqual(['b', 'c'])
  })

  it('拖完 10 秒才提交一次整树快照，离开前 flush 会立刻提交', async () => {
    const { api, saveOrder } = setup()
    await api.refresh()

    api.markOrderDirty()
    expect(saveOrder).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()
    expect(saveOrder).toHaveBeenCalledTimes(1)
    expect(saveOrder).toHaveBeenCalledWith({
      groups: [{ id: 'g1', items: ['a'] }],
      ungrouped: ['b', 'c'],
    })

    // 再来一次但不等防抖：flush 立刻提交。
    api.markOrderDirty()
    await api.flushOrder()
    expect(saveOrder).toHaveBeenCalledTimes(2)
  })
})

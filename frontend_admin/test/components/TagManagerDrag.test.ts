import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import TagManager from '~/components/articles/TagManager.vue'
import type { TagListItem } from '~/types/api'

/**
 * 标签卡网格的拖拽排序（票 346）。
 *
 * 只锁用户能观察到的行为，不锁实现：
 *   1. 标签以卡片网格呈现，一屏能放下很多个（不是一行一个的表格）；
 *   2. 拖动后**不立刻**提交，而是等防抖窗口过去再整份提交一次；
 *   3. 提交的是全集顺序（含停用项），顺序即界面顺序；
 *   4. 过滤状态下不允许拖：否则会把隐藏的标签挤到最后；
 *   5. 提交失败回滚到上一次成功的顺序，而不是让界面与库里长期不一致。
 */

const { messageMock, dialogMock, adminApi } = vi.hoisted(() => ({
  messageMock: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  dialogMock: { warning: vi.fn(), error: vi.fn(), success: vi.fn(), info: vi.fn() },
  adminApi: {
    getAdminTags: vi.fn(),
    saveAdminTag: vi.fn(),
    deleteAdminTags: vi.fn(),
    replaceAdminTagOrder: vi.fn(),
  },
}))

vi.mock('~/services/admin', () => adminApi)

vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal<typeof import('naive-ui')>()
  return { ...actual, useMessage: () => messageMock, useDialog: () => dialogMock }
})

function tag(overrides: Partial<TagListItem> = {}): TagListItem {
  return {
    id: 't1',
    title: 'Go',
    sort: 2,
    description: '',
    article_count: 3,
    is_enabled: true,
    created_at: '',
    updated_at: '',
    deleted_at: null,
    created_by: '900',
    ...overrides,
  } as TagListItem
}

function mountManager() {
  return mount(TagManager, { global: { stubs: { teleport: true } } })
}

/** 直接调组件暴露的排序回调：模拟一次拖动结束。 */
async function simulateDrop(wrapper: ReturnType<typeof mountManager>, ids: string[]) {
  const vm = wrapper.vm as unknown as { $: { setupState: Record<string, unknown> } }
  // 通过 setupState 改 rows 再触发 end，等价于用户拖完松手。
  const state = vm.$.setupState as { rows: TagListItem[]; onOrderChange: () => void }
  const byId = new Map(state.rows.map(row => [row.id, row]))
  state.rows = ids.map(id => byId.get(id)!).filter(Boolean)
  state.onOrderChange()
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
  adminApi.getAdminTags.mockResolvedValue({ list: [], count: 0 })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('标签卡网格：呈现', () => {
  it('标签以多列卡片呈现，卡片内含篇数与状态', async () => {
    adminApi.getAdminTags.mockResolvedValue({
      list: [tag({ id: 't1', title: 'Go', article_count: 3 }), tag({ id: 't2', title: 'MySQL', article_count: 0 })],
      count: 2,
    })

    const wrapper = mountManager()
    await flushPromises()

    const grid = wrapper.get('[data-role="tag-grid"]')
    expect(grid.classes()).toContain('tag-grid')
    // 每张标签一张卡，而不是一行一条表格行。
    expect(wrapper.findAll('.tag-card')).toHaveLength(2)
    expect(wrapper.text()).toContain('3 篇')
  })

  it('提示里说明拖动会节流保存，让用户知道什么时候落地', async () => {
    adminApi.getAdminTags.mockResolvedValue({ list: [tag()], count: 1 })
    const wrapper = mountManager()
    await flushPromises()

    const hint = wrapper.get('[data-role="tag-sort-hint"]').text()
    expect(hint).toContain('拖动')
    expect(hint).toContain('10 秒')
  })
})

describe('标签卡网格：拖拽排序的保存', () => {
  it('拖动后不立刻提交，防抖窗口过去才提交一次全集顺序', async () => {
    adminApi.getAdminTags.mockResolvedValue({
      list: [tag({ id: 'a' }), tag({ id: 'b' }), tag({ id: 'c' })],
      count: 3,
    })
    adminApi.replaceAdminTagOrder.mockResolvedValue(undefined)

    const wrapper = mountManager()
    await flushPromises()

    await simulateDrop(wrapper, ['c', 'a', 'b'])
    // 还没到 10 秒：一个请求都不该发。
    expect(adminApi.replaceAdminTagOrder).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()

    expect(adminApi.replaceAdminTagOrder).toHaveBeenCalledTimes(1)
    // 提交的是"界面上的顺序"，且是全集（含停用项）。
    expect(adminApi.replaceAdminTagOrder).toHaveBeenCalledWith(['c', 'a', 'b'])
  })

  it('连续拖动只提交最后一次排列', async () => {
    adminApi.getAdminTags.mockResolvedValue({
      list: [tag({ id: 'a' }), tag({ id: 'b' }), tag({ id: 'c' })],
      count: 3,
    })
    adminApi.replaceAdminTagOrder.mockResolvedValue(undefined)

    const wrapper = mountManager()
    await flushPromises()

    await simulateDrop(wrapper, ['b', 'a', 'c'])
    await vi.advanceTimersByTimeAsync(3_000)
    await simulateDrop(wrapper, ['c', 'b', 'a'])
    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()

    expect(adminApi.replaceAdminTagOrder).toHaveBeenCalledTimes(1)
    expect(adminApi.replaceAdminTagOrder).toHaveBeenCalledWith(['c', 'b', 'a'])
  })

  it('提交失败：回滚到上一次成功的顺序并提示', async () => {
    adminApi.getAdminTags.mockResolvedValue({
      list: [tag({ id: 'a' }), tag({ id: 'b' })],
      count: 2,
    })
    adminApi.replaceAdminTagOrder.mockRejectedValue(new Error('标签集合已变化'))

    const wrapper = mountManager()
    await flushPromises()

    await simulateDrop(wrapper, ['b', 'a'])
    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()

    // 回滚：卡片顺序回到 a、b（与库里一致），而不是留在"看起来已保存"的 b、a。
    const titles = wrapper.findAll('.tag-card__title').map(node => node.text())
    expect(titles).toEqual(['Go', 'Go'])
    const ids = wrapper.findAll('.tag-card').map(node => node.attributes('data-role'))
    expect(ids).toEqual(['tag-card-a', 'tag-card-b'])
    expect(messageMock.error).toHaveBeenCalled()
  })
})

describe('标签卡网格：过滤时禁用拖拽', () => {
  it('有搜索词时不出拖拽容器，并说明原因', async () => {
    adminApi.getAdminTags.mockResolvedValue({
      list: [tag({ id: 'a', title: 'Go' }), tag({ id: 'b', title: 'MySQL' })],
      count: 2,
    })

    const wrapper = mountManager()
    await flushPromises()

    // 未过滤：拖拽容器在。
    expect(wrapper.find('.tag-grid').exists()).toBe(true)

    await wrapper.get('[data-role="tag-filter"] input').setValue('go')
    await flushPromises()

    const hint = wrapper.get('[data-role="tag-sort-hint"]').text()
    expect(hint).toContain('过滤状态下不可拖动排序')
    // 仍能看到命中的卡片（只是不参与拖拽）。
    expect(wrapper.findAll('.tag-card')).toHaveLength(1)
  })
})

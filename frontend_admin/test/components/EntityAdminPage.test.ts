import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'

import EntityAdminPage from '~/components/articles/EntityAdminPage.vue'
import { useEntityGroups } from '~/composables/useEntityGroups'

/**
 * 合集 / 项目分组页（票 348）的编排逻辑。
 *
 * 这一层补的是"页面级接线"的分支：分组 CRUD 立即提交、拖拽快照落账、
 * 展示开关与删除的失败提示。网格本身的渲染与拖拽手势在
 * GroupedEntityGrid.test.ts 里，不重复。
 */

const messageMock = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }))
const dialogMock = vi.hoisted(() => ({ warning: vi.fn(), error: vi.fn(), success: vi.fn(), info: vi.fn() }))

const adminApi = vi.hoisted(() => ({
  getAdminCollections: vi.fn(),
  getAdminGroups: vi.fn(),
  replaceAdminGroupOrder: vi.fn(),
  saveAdminGroup: vi.fn(),
  deleteAdminGroup: vi.fn(),
  setCollectionDisplay: vi.fn(),
  deleteCollection: vi.fn(),
  setProjectDisplay: vi.fn(),
  deleteProject: vi.fn(),
}))

vi.mock('~/services/admin', () => adminApi)

vi.mock('naive-ui', async () => {
  const passthrough = (name: string) =>
    defineComponent({
      name,
      setup: (_props, { slots }) => () => h('div', [slots.header?.(), slots.default?.(), slots.footer?.()]),
    })
  return {
    NCard: passthrough('NCard'),
    NSpace: passthrough('NSpace'),
    NModal: passthrough('NModal'),
    NButton: defineComponent({
      name: 'NButton',
      props: { disabled: Boolean, loading: Boolean },
      setup(props, { slots }) {
        return () => h('button', { disabled: props.disabled || undefined }, slots.default?.())
      },
    }),
    NInput: defineComponent({
      name: 'NInput',
      props: { value: { type: String, default: '' } },
      emits: ['update:value'],
      setup(props, { emit }) {
        return () =>
          h('input', {
            value: props.value,
            onInput: (e: Event) => emit('update:value', (e.target as HTMLInputElement).value),
          })
      },
    }),
    useMessage: () => messageMock,
    useDialog: () => dialogMock,
  }
})

// 分组网格与编辑弹窗：本文件只关心页面接线，两者各留一个可断言的点。
vi.mock('~/components/articles/GroupedEntityGrid.vue', () => ({
  default: {
    name: 'GroupedEntityGrid',
    props: ['groups', 'ungrouped', 'kind', 'loading', 'togglingId', 'savingOrder'],
    emits: ['orderChange', 'edit', 'remove', 'toggle', 'groupCreate', 'groupRename', 'groupRemove'],
    setup(_props: unknown, { emit }: { emit: (e: string, ...a: unknown[]) => void }) {
      return () =>
        h('div', { 'data-role': 'grid-stub' }, [
          h('button', { 'data-role': 'stub-order', onClick: () => emit('orderChange', { groups: [], ungrouped: [] }) }),
          h('button', { 'data-role': 'stub-group-create', onClick: () => emit('groupCreate') }),
          h('button', {
            'data-role': 'stub-group-rename',
            onClick: () => emit('groupRename', { id: 'g1', title: '第一组' }),
          }),
          h('button', {
            'data-role': 'stub-group-remove',
            onClick: () => emit('groupRemove', { id: 'g1', title: '第一组' }),
          }),
          h('button', {
            'data-role': 'stub-toggle',
            onClick: () => emit('toggle', { id: 'c1', title: '合集', displayed: true }, false),
          }),
          h('button', {
            'data-role': 'stub-remove',
            onClick: () => emit('remove', { id: 'c1', title: '合集' }),
          }),
        ])
    },
  },
}))
vi.mock('~/components/articles/CollectionEditorOverlay.vue', () => ({
  default: { name: 'CollectionEditorOverlay', props: ['show', 'kind', 'entity'], setup: () => () => null },
}))

function card(id: string) {
  return { id, title: `合集${id}`, description: '', cover: '', fallback_cover: '', article_count: 1, displayed: true }
}

beforeEach(() => {
  vi.clearAllMocks()
  adminApi.getAdminCollections.mockResolvedValue({ list: [card('c1')], count: 1 })
  adminApi.getAdminGroups.mockResolvedValue({ groups: [], ungrouped: [{ owner_id: 'c1' }] })
  adminApi.replaceAdminGroupOrder.mockResolvedValue(undefined)
  adminApi.saveAdminGroup.mockResolvedValue(undefined)
  adminApi.deleteAdminGroup.mockResolvedValue(undefined)
  adminApi.setCollectionDisplay.mockResolvedValue(undefined)
  adminApi.deleteCollection.mockResolvedValue(undefined)
  vi.stubGlobal('useMessage', () => messageMock)
  vi.stubGlobal('useDialog', () => dialogMock)
})

function mountPage(kind: 'collection' | 'project' = 'collection') {
  return mount(EntityAdminPage, { props: { kind } })
}

describe('分组页：分组 CRUD 立即生效', () => {
  it('新建分组提交后立刻重拉列表，不等拖拽防抖', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-role="stub-group-create"]').trigger('click')
    await wrapper.get('[data-role="collection-group-title"]').setValue('  新组  ')
    await wrapper.get('[data-role="collection-group-submit"]').trigger('click')
    await flushPromises()

    expect(adminApi.saveAdminGroup).toHaveBeenCalledWith('collection', { title: '新组' })
    // 立即重拉：getAdminCollections + getAdminGroups 各被调了两次（挂载 + 保存后）。
    expect(adminApi.getAdminGroups).toHaveBeenCalledTimes(2)
  })

  it('改名带上 id，且不带空 id 字段', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-role="stub-group-rename"]').trigger('click')
    await wrapper.get('[data-role="collection-group-submit"]').trigger('click')
    await flushPromises()

    expect(adminApi.saveAdminGroup).toHaveBeenCalledWith('collection', { id: 'g1', title: '第一组' })
  })

  it('名称为空时确定不可点，也不发请求', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-role="stub-group-create"]').trigger('click')
    const submit = wrapper.get('[data-role="collection-group-submit"]')
    expect(submit.attributes('disabled')).toBeDefined()
    await submit.trigger('click')
    await flushPromises()

    expect(adminApi.saveAdminGroup).not.toHaveBeenCalled()
  })

  it('删除分组：确认后调接口，文案说明条目会退回未分组', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-role="stub-group-remove"]').trigger('click')
    const options = dialogMock.warning.mock.calls[0]![0] as { content: string; onPositiveClick: () => Promise<void> }
    expect(options.content).toContain('退回到「未分组」')

    await options.onPositiveClick()
    await flushPromises()
    expect(adminApi.deleteAdminGroup).toHaveBeenCalledWith('collection', 'g1')
  })

  it('分组保存失败时原样展示后端文案', async () => {
    adminApi.saveAdminGroup.mockRejectedValue(new Error('分组标题不能为空'))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-role="stub-group-create"]').trigger('click')
    await wrapper.get('[data-role="collection-group-title"]').setValue('x')
    await wrapper.get('[data-role="collection-group-submit"]').trigger('click')
    await flushPromises()

    expect(messageMock.error).toHaveBeenCalledWith('分组标题不能为空')
  })
})

describe('分组页：卡片动作', () => {
  it('展示开关失败时提示可读原因', async () => {
    adminApi.setCollectionDisplay.mockRejectedValue(new Error('网络断了'))
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-role="stub-toggle"]').trigger('click')
    await flushPromises()

    expect(messageMock.error).toHaveBeenCalledWith('展示开关保存失败：网络断了')
  })

  it('项目 kind 走项目接口', async () => {
    const wrapper = mountPage('project')
    await flushPromises()

    await wrapper.get('[data-role="stub-toggle"]').trigger('click')
    await flushPromises()

    expect(adminApi.setProjectDisplay).toHaveBeenCalledWith('c1', false)
    expect(adminApi.setCollectionDisplay).not.toHaveBeenCalled()
  })

  it('删除卡片走二次确认，成功后重拉列表', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('[data-role="stub-remove"]').trigger('click')
    const options = dialogMock.warning.mock.calls[0]![0] as { onPositiveClick: () => Promise<void> }
    await options.onPositiveClick()
    await flushPromises()

    expect(adminApi.deleteCollection).toHaveBeenCalledWith('c1')
    expect(adminApi.getAdminCollections).toHaveBeenCalledTimes(2)
  })
})

describe('useEntityGroups：拖拽快照落账', () => {
  it('applySnapshot 写回排列并标记脏（随后由防抖提交）', async () => {
    vi.useFakeTimers()
    const saveOrder = vi.fn().mockResolvedValue(undefined)
    const api = useEntityGroups({
      kind: 'collection',
      loadCards: async () => [card('a'), card('b')],
      loadGroups: async () => ({ groups: [{ id: 'g1', title: '第一组', items: [{ owner_id: 'a' }] }], ungrouped: [] }),
      saveOrder,
      onError: vi.fn(),
    })
    await api.refresh()

    // 把 b 拖进 g1、a 留在未分组。
    api.applySnapshot({ groups: [{ id: 'g1', items: ['b'] }], ungrouped: ['a'] })
    expect(api.groups.value[0]!.items.map(i => i.id)).toEqual(['b'])
    expect(api.ungrouped.value.map(i => i.id)).toEqual(['a'])

    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()
    expect(saveOrder).toHaveBeenCalledWith({ groups: [{ id: 'g1', items: ['b'] }], ungrouped: ['a'] })
    vi.useRealTimers()
  })

  it('保存失败时回滚到上一次成功的顺序，并提示', async () => {
    vi.useFakeTimers()
    const onError = vi.fn()
    const saveOrder = vi.fn().mockRejectedValue(new Error('集合已变化'))
    const api = useEntityGroups({
      kind: 'collection',
      loadCards: async () => [card('a'), card('b')],
      loadGroups: async () => ({ groups: [{ id: 'g1', title: '第一组', items: [{ owner_id: 'a' }] }], ungrouped: [{ owner_id: 'b' }] }),
      saveOrder,
      onError,
    })
    await api.refresh()

    api.applySnapshot({ groups: [{ id: 'g1', items: ['b'] }], ungrouped: ['a'] })
    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()

    expect(onError).toHaveBeenCalled()
    // 回滚：a 回到 g1、b 回到未分组。
    expect(api.groups.value[0]!.items.map(i => i.id)).toEqual(['a'])
    expect(api.ungrouped.value.map(i => i.id)).toEqual(['b'])
    vi.useRealTimers()
  })
})

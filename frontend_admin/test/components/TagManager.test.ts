import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import TagManager from '~/components/articles/TagManager.vue'
import type { TagListItem } from '~/types/api'

/**
 * 后台标签管理（票 322）。
 *
 * 只锁用户在页面上能看见 / 能做到的事：
 *   1. 列表展示全量标签（含停用）：标题 / 文章数 / 排序 / 启用状态；
 *   2. 按标题过滤；
 *   3. 新建后出现在列表里；
 *   4. 编辑停用标签后它**仍是停用**（这条是这页最容易退化的语义）；
 *   5. 删除被拒时把后端那句带篇数的可读文案原样展示，且列表不刷新。
 */

const { messageMock, dialogMock, adminApi } = vi.hoisted(() => ({
  messageMock: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  dialogMock: { warning: vi.fn(), error: vi.fn(), success: vi.fn(), info: vi.fn() },
  adminApi: {
    getAdminTags: vi.fn(),
    saveAdminTag: vi.fn(),
    deleteAdminTags: vi.fn(),
  },
}))

vi.mock('~/services/admin', () => adminApi)

// `useMessage` / `useDialog` 要拿假的（不在 provider 里会抛），其余组件保持真实实现。
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
  // NModal 内部是 Teleport：存根掉才能在同一棵树里断言弹窗内容。
  return mount(TagManager, { global: { stubs: { teleport: true } } })
}

beforeEach(() => {
  vi.clearAllMocks()
  adminApi.getAdminTags.mockResolvedValue({ list: [], count: 0 })
})

describe('标签管理：列表与过滤', () => {
  it('展示全量标签（含停用）：标题 / 文章数 / 排序 / 启用状态', async () => {
    adminApi.getAdminTags.mockResolvedValue({
      list: [tag({ id: 't1', title: 'Go', article_count: 3, sort: 2 }), tag({ id: 't2', title: '已废弃', article_count: 0, sort: 9, is_enabled: false })],
      count: 2,
    })

    const wrapper = mountManager()
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('Go')
    expect(text).toContain('已废弃')
    expect(text).toContain('3 篇')
    expect(text).toContain('停用')
    expect(text).toContain('共 2 个标签（含停用）')
  })

  it('按标题过滤：只留下命中的行，停用项同样能被搜到', async () => {
    adminApi.getAdminTags.mockResolvedValue({
      list: [tag({ id: 't1', title: 'Go' }), tag({ id: 't2', title: 'MySQL', is_enabled: false })],
      count: 2,
    })

    const wrapper = mountManager()
    await flushPromises()

    await wrapper.get('[data-role="tag-filter"] input').setValue('my')
    await flushPromises()

    expect(wrapper.text()).toContain('MySQL')
    expect(wrapper.text()).not.toContain('Go')
    expect(wrapper.text()).toContain('当前显示 1 个')
  })
})

describe('标签管理：新建与编辑', () => {
  it('新建：带上四栏提交，保存后刷新列表', async () => {
    adminApi.saveAdminTag.mockResolvedValue(undefined)

    const wrapper = mountManager()
    await flushPromises()

    await wrapper.get('[data-role="tag-create"]').trigger('click')
    await wrapper.get('[data-role="tag-editor-title"] input').setValue('  新标签  ')
    await wrapper.get('[data-role="tag-editor-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.saveAdminTag).toHaveBeenCalledWith({
      title: '新标签',
      description: '',
      sort: 0,
      is_enabled: true,
    })
    // 没有 id = 新建。
    expect(adminApi.saveAdminTag.mock.calls[0]![0]).not.toHaveProperty('id')
    // 保存后从服务端重拉：新标签要出现在列表里。
    expect(adminApi.getAdminTags).toHaveBeenCalledTimes(2)
    expect(messageMock.success).toHaveBeenCalled()
  })

  it('标题为空时拦住保存，不发任何写请求', async () => {
    const wrapper = mountManager()
    await flushPromises()

    await wrapper.get('[data-role="tag-create"]').trigger('click')
    await wrapper.get('[data-role="tag-editor-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.saveAdminTag).not.toHaveBeenCalled()
    expect(messageMock.warning).toHaveBeenCalled()
  })

  it('编辑停用标签：回填标题，保存后仍是停用（不会被悄悄放回全站可选）', async () => {
    const disabled = tag({ id: 't9', title: '老标签', is_enabled: false, sort: 4, description: '旧描述' })
    adminApi.getAdminTags.mockResolvedValue({ list: [disabled], count: 1 })
    adminApi.saveAdminTag.mockResolvedValue(undefined)

    const wrapper = mountManager()
    await flushPromises()

    await wrapper.get('[data-role="tag-edit"]').trigger('click')
    expect(wrapper.get('[data-role="tag-editor-title"] input').element).toHaveProperty('value', '老标签')

    await wrapper.get('[data-role="tag-editor-title"] input').setValue('老标签改名')
    await wrapper.get('[data-role="tag-editor-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.saveAdminTag).toHaveBeenCalledWith({
      id: 't9',
      title: '老标签改名',
      description: '旧描述',
      sort: 4,
      is_enabled: false,
    })
    // 刷新后该行仍是停用。
    expect(wrapper.text()).toContain('停用')
  })

  it('重复标题：把后端翻好的「名称重复」原样展示', async () => {
    adminApi.saveAdminTag.mockRejectedValue(new Error('名称重复'))

    const wrapper = mountManager()
    await flushPromises()

    await wrapper.get('[data-role="tag-create"]').trigger('click')
    await wrapper.get('[data-role="tag-editor-title"] input').setValue('Go')
    await wrapper.get('[data-role="tag-editor-save"]').trigger('click')
    await flushPromises()

    expect(messageMock.error).toHaveBeenCalledWith('名称重复')
  })
})

describe('标签管理：启停与删除', () => {
  it('停用：带上本行其余字段、只翻 is_enabled', async () => {
    const row = tag({ id: 't1', title: 'Go', sort: 2, description: '描述', is_enabled: true })
    adminApi.getAdminTags.mockResolvedValue({ list: [row], count: 1 })
    adminApi.saveAdminTag.mockResolvedValue(undefined)

    const wrapper = mountManager()
    await flushPromises()

    await wrapper.get('[data-role="tag-disable"]').trigger('click')
    await flushPromises()

    expect(adminApi.saveAdminTag).toHaveBeenCalledWith({
      id: 't1',
      title: 'Go',
      description: '描述',
      sort: 2,
      is_enabled: false,
    })
    expect(wrapper.text()).toContain('停用')
  })

  it('删除零引用标签：确认后调删除接口并刷新列表', async () => {
    adminApi.getAdminTags.mockResolvedValue({ list: [tag({ id: 't1', title: 'Go' })], count: 1 })
    adminApi.deleteAdminTags.mockResolvedValue(undefined)

    const wrapper = mountManager()
    await flushPromises()

    await wrapper.get('[data-role="tag-delete"]').trigger('click')
    const confirm = dialogMock.warning.mock.calls[0]![0] as { onPositiveClick: () => Promise<void> }
    await confirm.onPositiveClick()
    await flushPromises()

    expect(adminApi.deleteAdminTags).toHaveBeenCalledWith(['t1'])
    expect(messageMock.success).toHaveBeenCalledWith('删除成功')
    expect(adminApi.getAdminTags).toHaveBeenCalledTimes(2)
  })

  it('删除被拒：原样展示带篇数的可读文案，且列表不刷新', async () => {
    adminApi.getAdminTags.mockResolvedValue({ list: [tag({ id: 't1', title: 'Go', article_count: 3 })], count: 1 })
    adminApi.deleteAdminTags.mockRejectedValue(new Error('该标签已有 3 篇文章在用，请改用停用'))

    const wrapper = mountManager()
    await flushPromises()

    await wrapper.get('[data-role="tag-delete"]').trigger('click')
    const confirm = dialogMock.warning.mock.calls[0]![0] as { onPositiveClick: () => Promise<void> }
    await confirm.onPositiveClick()
    await flushPromises()

    const shown = messageMock.error.mock.calls[0]![0] as string
    expect(shown).toBe('该标签已有 3 篇文章在用，请改用停用')
    // 被拒 = 什么都没发生：不刷新、不报成功。
    expect(adminApi.getAdminTags).toHaveBeenCalledTimes(1)
    expect(messageMock.success).not.toHaveBeenCalled()
  })
})

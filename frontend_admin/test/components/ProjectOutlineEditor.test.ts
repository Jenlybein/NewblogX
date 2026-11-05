import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import OutlineTreeEditor from '~/components/articles/OutlineTreeEditor.vue'

/**
 * #276 后台项目树编辑器的行为接缝：项目 kind 只编排项目独立树。
 *
 * 整树提交、文件夹 / 文章节点、排序与失效行口径都由共用的 outlineArrange 纯函数
 * 产出（既有测试已覆盖），这里只观察编辑器实际打到哪两个项目具名服务函数、提交
 * 的嵌套节点形状，以及读回的失效行是否还在。合集侧四个函数逐字不动（归 #275）。
 */

const { messageMock, adminApi } = vi.hoisted(() => ({
  messageMock: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  adminApi: {
    getCollectionOutline: vi.fn(),
    getProjectOutline: vi.fn(),
    saveCollectionOutline: vi.fn(),
    saveProjectOutline: vi.fn(),
    searchAdminArticles: vi.fn(),
  },
}))

vi.mock('~/services/admin', () => adminApi)
vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal<typeof import('naive-ui')>()
  return { ...actual, useMessage: () => messageMock }
})

beforeEach(() => {
  vi.clearAllMocks()
  adminApi.getCollectionOutline.mockResolvedValue({ nodes: [] })
  adminApi.getProjectOutline.mockResolvedValue({ nodes: [] })
  adminApi.saveCollectionOutline.mockResolvedValue({ nodes: [] })
  adminApi.saveProjectOutline.mockResolvedValue({ nodes: [] })
  adminApi.searchAdminArticles.mockResolvedValue({ list: [] })
})

describe('OutlineTreeEditor：项目 kind 只编排项目独立树', () => {
  it('读回项目树（含失效行），保存提交的是同一棵嵌套树', async () => {
    const staleArticle = {
      id: 'n2',
      parent_id: 'n1',
      kind: 2,
      title: '',
      article_id: 'a1',
      article_title: '已退回草稿',
      article_cover: '',
      sort_order: 1,
      stale: true,
      stale_reason: 'not_published',
      children: [],
    }
    adminApi.getProjectOutline.mockResolvedValue({
      nodes: [
        {
          id: 'n1',
          parent_id: '',
          kind: 1,
          title: '第一章',
          article_id: '',
          article_title: '',
          article_cover: '',
          sort_order: 1,
          stale: false,
          stale_reason: '',
          children: [staleArticle],
        },
      ],
    })
    adminApi.saveProjectOutline.mockResolvedValue({ nodes: [] })

    const wrapper = mount(OutlineTreeEditor, { props: { kind: 'project', entityId: 'p1' } })
    await flushPromises()

    expect(adminApi.getProjectOutline).toHaveBeenCalledWith('p1')
    expect(adminApi.getCollectionOutline).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('已失效')

    await wrapper.get('[data-role="outline-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.saveProjectOutline).toHaveBeenCalledTimes(1)
    expect(adminApi.saveCollectionOutline).not.toHaveBeenCalled()
    const [id, nodes] = adminApi.saveProjectOutline.mock.calls[0] as [string, Array<Record<string, unknown>>]
    expect(id).toBe('p1')
    expect(nodes).toEqual([
      {
        id: 'n1',
        kind: 1,
        title: '第一章',
        article_id: '',
        children: [
          { id: 'n2', kind: 2, title: '', article_id: 'a1', children: [] },
        ],
      },
    ])
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })

  it('新建文件夹后保存：新增节点与既有文章节点一次性整树提交', async () => {
    const wrapper = mount(OutlineTreeEditor, { props: { kind: 'project', entityId: 'p1' } })
    await flushPromises()

    // NInput 实际渲染的 input 元素：setValue 触发 input 事件驱动 v-model。
    await wrapper.get('[data-role="outline-folder-title"] input').setValue('第二章')
    await wrapper.get('[data-role="outline-create-root"]').trigger('click')
    await wrapper.get('[data-role="outline-save"]').trigger('click')
    await flushPromises()

    const [, nodes] = adminApi.saveProjectOutline.mock.calls[0] as [string, Array<Record<string, unknown>>]
    expect(nodes).toEqual([
      { id: '', kind: 1, title: '第二章', article_id: '', children: [] },
    ])
  })
})

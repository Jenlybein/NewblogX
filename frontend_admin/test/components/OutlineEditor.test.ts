import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import OutlineTreeEditor from '~/components/articles/OutlineTreeEditor.vue'

/**
 * 目录树编排器（决定清单 §8 第 35–36 条）。
 *
 * 本轮把「独立的目录编排页」整体退役，编辑器抽成这个共用组件：合集弹窗与项目弹窗
 * 各挂一次，靠 `kind` 决定读哪棵树、存哪棵树。它自己带「保存」按钮、自己的 dirty 标记，
 * 与信息卡的保存互不干扰（页面级编排不该被标题校验挡住）。
 *
 * 这里锁四件事：
 *   1. 按 `kind` 读对应的树（合集 / 项目各一条接口）；
 *   2. 没有实体 ID 时明确提示「先保存」，且不发保存请求；
 *   3. 新建根层文件夹后保存，提交的是**嵌套节点**而不是扁平行；
 *   4. 保存成功抛 `saved`，让外层顺手刷新列表。
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

describe('OutlineTreeEditor：按 kind 读树', () => {
  it('合集 kind 读合集树，并把已有节点渲染成行', async () => {
    adminApi.getCollectionOutline.mockResolvedValue({
      nodes: [{ kind: 1, title: '第一章', children: [{ kind: 0, article_id: 'a1', title: '文章一' }] }],
    })

    const wrapper = mount(OutlineTreeEditor, { props: { kind: 'collection', entityId: 'c1' } })
    await flushPromises()

    expect(adminApi.getCollectionOutline).toHaveBeenCalledWith('c1')
    expect(adminApi.getProjectOutline).not.toHaveBeenCalled()
    // 文件夹 + 它下面的文章 = 两个树节点（NTree 自己的树线/缩进表达层级）。
    expect(wrapper.findAll('[data-outline-row]').length).toBeGreaterThanOrEqual(2)
    expect(wrapper.find('.n-tree').exists()).toBe(true)
    expect(wrapper.text()).toContain('第一章')
  })

  it('项目 kind 读项目树，保存也走项目接口', async () => {
    const wrapper = mount(OutlineTreeEditor, { props: { kind: 'project', entityId: 'p1' } })
    await flushPromises()

    expect(adminApi.getProjectOutline).toHaveBeenCalledWith('p1')
    expect(adminApi.getCollectionOutline).not.toHaveBeenCalled()

    await wrapper.get('[data-role="outline-create-root"]').trigger('click')
    await wrapper.get('[data-role="outline-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.saveProjectOutline).toHaveBeenCalledWith('p1', expect.any(Array))
    expect(adminApi.saveCollectionOutline).not.toHaveBeenCalled()
  })
})

describe('OutlineTreeEditor：没有实体 ID 时不给存', () => {
  it('提示先保存基本信息，点保存也不发请求', async () => {
    const wrapper = mount(OutlineTreeEditor, { props: { kind: 'collection', entityId: '' } })
    await flushPromises()

    expect(wrapper.find('[data-role="outline-needs-save"]').exists()).toBe(true)
    // 没有 ID 时整条工具栏都不渲染：连「保存」按钮都不该出现（避免点了才报错）。
    expect(wrapper.find('[data-role="outline-save"]').exists()).toBe(false)
    expect(adminApi.getCollectionOutline).not.toHaveBeenCalled()
    expect(adminApi.saveCollectionOutline).not.toHaveBeenCalled()
  })
})

describe('OutlineTreeEditor：保存提交嵌套节点', () => {
  it('读回的树原样保存：提交嵌套节点（不是扁平行），并抛 saved', async () => {
    adminApi.getCollectionOutline.mockResolvedValue({
      nodes: [{ kind: 1, title: '第一章', children: [{ kind: 0, article_id: 'a1', title: '文章一' }] }],
    })

    const wrapper = mount(OutlineTreeEditor, { props: { kind: 'collection', entityId: 'c2' } })
    await flushPromises()

    await wrapper.get('[data-role="outline-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.saveCollectionOutline).toHaveBeenCalledTimes(1)
    const [id, nodes] = adminApi.saveCollectionOutline.mock.calls[0] as [string, Array<Record<string, unknown>>]
    expect(id).toBe('c2')
    expect(nodes).toHaveLength(1)
    expect(nodes[0]).toMatchObject({ kind: 1, title: '第一章' })
    expect((nodes[0] as { children?: unknown[] }).children).toHaveLength(1)
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })

  it('文件夹名为空时拦住新建（只警告，不产生半成品行）', async () => {
    const wrapper = mount(OutlineTreeEditor, { props: { kind: 'collection', entityId: 'c4' } })
    await flushPromises()

    await wrapper.get('[data-role="outline-create-root"]').trigger('click')
    await flushPromises()

    expect(messageMock.warning).toHaveBeenCalled()
    expect(wrapper.find('[data-role="outline-empty"]').exists()).toBe(true)
  })

  it('没有任何行时保存提交空数组（清空整棵树也是合法操作）', async () => {
    const wrapper = mount(OutlineTreeEditor, { props: { kind: 'collection', entityId: 'c3' } })
    await flushPromises()

    await wrapper.get('[data-role="outline-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.saveCollectionOutline).toHaveBeenCalledWith('c3', [])
  })
})

describe('OutlineTreeEditor：NTree 承载树的渲染与拖拽', () => {
  const twoFolders = {
    nodes: [
      { kind: 1, title: '第一章', children: [{ kind: 0, article_id: 'a1', title: '文章一' }] },
      { kind: 1, title: '第二章', children: [] },
    ],
  }

  it('每行只留一个「⋯」操作入口，不再摊开一排按钮', async () => {
    adminApi.getCollectionOutline.mockResolvedValue(twoFolders)

    const wrapper = mount(OutlineTreeEditor, { props: { kind: 'collection', entityId: 'c10' } })
    await flushPromises()

    const rows = wrapper.findAll('[data-outline-row]')
    expect(rows.length).toBe(3)
    expect(wrapper.findAll('[data-role="outline-row-more"]').length).toBe(3)
    // 旧的六个按钮彻底退场（它们现在在「⋯」下拉里）。
    expect(wrapper.find('[data-role="outline-row-remove"]').exists()).toBe(false)
    expect(wrapper.find('[data-role="outline-row-rename"]').exists()).toBe(false)
    // 树本体里除了「⋯」没有别的按钮（上移/下移/缩进/移出都收进菜单了）。
    expect(wrapper.findAll('.n-tree button:not([data-role="outline-row-more"])')).toHaveLength(0)
  })

  it('第三方树的落点换算成扁平行下标：把「第二章」拖到「第一章」之前并保存', async () => {
    adminApi.getCollectionOutline.mockResolvedValue(twoFolders)

    const wrapper = mount(OutlineTreeEditor, { props: { kind: 'collection', entityId: 'c11' } })
    await flushPromises()

    // 行 key 直接取渲染出来的节点顺序（NTree 的 key 就是我们给的行 key）。
    const keys = wrapper.findAll('[data-outline-row]').map(node => node.attributes('data-outline-row') ?? '')
    expect(keys.length).toBe(3)

    const firstFolderKey = keys[0]!
    const secondFolderKey = keys[2]!

    // 不必真拖：NTree 的 @drop 载荷就是这个形状，组件内部走 outlineDropIndex → moveOutlineRowToIndex。
    wrapper.findComponent({ name: 'Tree' }).vm.$emit('drop', {
      node: { key: firstFolderKey },
      dragNode: { key: secondFolderKey },
      dropPosition: 'before',
    })
    await flushPromises()

    await wrapper.get('[data-role="outline-save"]').trigger('click')
    await flushPromises()

    const [, nodes] = adminApi.saveCollectionOutline.mock.calls[0] as [string, Array<{ title?: string }>]
    expect(nodes.map(node => node.title)).toEqual(['第二章', '第一章'])
  })
})

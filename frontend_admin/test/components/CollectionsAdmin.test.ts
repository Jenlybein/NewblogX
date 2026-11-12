import { flushPromises, mount } from '@vue/test-utils'
import { h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import CollectionCardGrid, {
  type CollectionCardItem,
} from '~/components/articles/CollectionCardGrid.vue'
import CollectionEditorOverlay from '~/components/articles/CollectionEditorOverlay.vue'

/**
 * 合集 / 项目的卡片网格与编辑弹窗（决定清单 §8 第 33–36 条）。
 *
 * 旧用例挂的是「页面里常驻一张大表单 + 一张 NDataTable」的形态，本轮整体改掉了：
 * 列表变成卡片网格（编辑/删除 hover 浮出），表单一律进居中弹窗，目录树在右侧抽屉。
 * 所以这里只锁新契约的几条关键行为：
 *   1. 卡片呈现（标题 / 描述 / 篇数），没有封面且没有回退时**不发空 src**；
 *   2. 编辑 / 删除只在卡内 hover 层，点击只抛事件，不发请求；
 *   3. 展示开关点击抛 `toggle(item, value)`；
 *   4. 弹窗：标题空不放行；保存按 kind 走对应接口；**新建后拿到 ID 才能编排目录树**；
 *   5. 编辑回填只带「已上传」的封面（回退封面是只读派生值，不能被存回去）。
 */

const { messageMock, adminApi } = vi.hoisted(() => ({
  messageMock: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  adminApi: {
    composeAdminEntity: vi.fn(),
    createCollection: vi.fn(),
    updateCollection: vi.fn(),
    setCollectionDisplay: vi.fn(),
    createProject: vi.fn(),
    updateProject: vi.fn(),
    setProjectDisplay: vi.fn(),
  },
}))

vi.mock('~/services/admin', () => adminApi)
vi.mock('~/services/image', () => ({ uploadImageByTask: vi.fn() }))

// 弹窗里嵌着的两块重组件：只关心弹窗本身的行为，替身各自留一个可断言的点。
// 树编辑器替身：必须实现 nodes() / markSaved()，因为弹窗的 compose 会向它取节点
// （取不到就 fail-closed，见 currentNodes 的注释）。没有 nodes() 的替身代表
// "编辑器坏了"，那正是另一条用例要覆盖的场景。
const treeStubBehavior = vi.hoisted(() => ({ withNodes: true, nodes: [] as unknown[] }))

vi.mock('~/components/articles/OutlineTreeEditor.vue', () => ({
  default: {
    name: 'OutlineTreeEditor',
    props: { kind: { type: String, required: true }, entityId: { type: String, required: true } },
    setup(props: { kind: string; entityId: string }, { expose }: { expose: (api: Record<string, unknown>) => void }) {
      const api: Record<string, unknown> = { markSaved: vi.fn() }
      if (treeStubBehavior.withNodes) api.nodes = () => treeStubBehavior.nodes
      expose(api)
      return () =>
        h('div', {
          'data-role': 'outline-tree-editor-stub',
          'data-kind': props.kind,
          'data-entity': props.entityId,
        })
    },
  },
}))
vi.mock('~/components/media/ImageCropDialog.vue', () => ({
  default: { name: 'ImageCropDialog', template: '<div data-role="image-crop-dialog-stub" />' },
}))

// `useMessage` 要拿假的（不在 naive-ui 的 provider 里会抛），其余组件保持真实实现。
vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal<typeof import('naive-ui')>()
  return { ...actual, useMessage: () => messageMock }
})

function card(overrides: Partial<CollectionCardItem> = {}): CollectionCardItem {
  return {
    id: 'c1',
    title: 'Nuxt 部署实战',
    description: '从零到上线',
    cover: '',
    fallback_cover: '',
    article_count: 7,
    displayed: true,
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  treeStubBehavior.withNodes = true
  treeStubBehavior.nodes = []
})

describe('CollectionCardGrid：卡片呈现与 hover 动作', () => {
  it('渲染标题 / 描述 / 篇数，并把 id 落在 data-id 上', () => {
    const wrapper = mount(CollectionCardGrid, { props: { kind: 'collection', items: [card()] } })
    const node = wrapper.get('[data-role="collection-card"]')

    expect(node.attributes('data-id')).toBe('c1')
    expect(node.text()).toContain('Nuxt 部署实战')
    expect(node.text()).toContain('从零到上线')
    expect(node.text()).toContain('7 篇')
  })

  it('没有封面也没有回退时不发空 src：渲染显式占位', () => {
    const wrapper = mount(CollectionCardGrid, { props: { kind: 'collection', items: [card()] } })

    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('无封面')
  })

  it('回退封面照读者的样子预览（有 fallback_cover 就渲染 img）', () => {
    const wrapper = mount(CollectionCardGrid, {
      props: { kind: 'collection', items: [card({ fallback_cover: 'https://cdn.test/a.jpg' })] },
    })

    expect(wrapper.get('img').attributes('src')).toBe('https://cdn.test/a.jpg')
  })

  it('编辑 / 删除只在卡片的 hover 层里，点击只抛事件', async () => {
    const wrapper = mount(CollectionCardGrid, { props: { kind: 'collection', items: [card()] } })

    const hover = wrapper.get('.collection-card__hover')
    expect(hover.find('[data-role="collection-card-edit"]').exists()).toBe(true)
    expect(hover.find('[data-role="collection-card-remove"]').exists()).toBe(true)

    await wrapper.get('[data-role="collection-card-edit"]').trigger('click')
    await wrapper.get('[data-role="collection-card-remove"]').trigger('click')

    expect(wrapper.emitted('edit')?.[0]).toEqual([expect.objectContaining({ id: 'c1' })])
    expect(wrapper.emitted('remove')?.[0]).toEqual([expect.objectContaining({ id: 'c1' })])
    // 组件是哑的：不自己发请求。
    expect(adminApi.updateCollection).not.toHaveBeenCalled()
    expect(adminApi.setCollectionDisplay).not.toHaveBeenCalled()
  })
})

describe('CollectionEditorOverlay：新建 / 编辑', () => {
  it('标题为空时拦住保存，不发任何写请求', async () => {
    const wrapper = mount(CollectionEditorOverlay, { props: { show: true, kind: 'collection', entity: null } })

    await wrapper.get('[data-role="collection-editor-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.createCollection).not.toHaveBeenCalled()
    expect(messageMock.warning).toHaveBeenCalled()
  })

  it('新建：走 createCollection；目录树是常驻右栏，没有自己的保存按钮', async () => {
    adminApi.createCollection.mockResolvedValue({ id: 'new-1' })
    const wrapper = mount(CollectionEditorOverlay, { props: { show: true, kind: 'collection', entity: null } })

    // 票 348 起两栏常驻：目录树一开始就在，不存在"先保存再点亮按钮"这回事。
    expect(wrapper.find('[data-role="collection-editor-tree"]').exists()).toBe(false)
    const drawer = wrapper.get('[data-role="collection-editor-drawer"]')
    expect(drawer.find('[data-role="outline-tree-editor-stub"]').exists()).toBe(true)

    await wrapper.get('input').setValue('新合集')
    await wrapper.get('[data-role="collection-editor-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.createCollection).toHaveBeenCalledWith(expect.objectContaining({ title: '新合集', displayed: true }))
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })

  it('目录树默认收起，点头部开关才展开', async () => {
    const wrapper = mount(CollectionEditorOverlay, {
      props: { show: true, kind: 'collection', entity: card({ id: 'c9' }) },
    })

    const drawer = wrapper.get('[data-role="collection-editor-drawer"]')
    const toggle = wrapper.get('[data-role="collection-editor-outline-toggle"]')

    // 默认收起：只显示信息卡。用 display 判断，因为 v-show 保留 DOM（不能用 exists）。
    expect(drawer.attributes('style')).toContain('display: none')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(toggle.text()).toContain('编排目录树')

    await toggle.trigger('click')

    expect(drawer.attributes('style') ?? '').not.toContain('display: none')
    expect(wrapper.get('[data-role="collection-editor-outline-toggle"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('[data-role="collection-editor-outline-toggle"]').text()).toContain('收起目录树')
  })

  it('收起状态下保存，仍然带着真实节点（不能因为收起就把树清空）', async () => {
    adminApi.composeAdminEntity.mockResolvedValue(undefined)
    adminApi.setCollectionDisplay.mockResolvedValue(undefined)
    // 有真实内容的一棵树：收起只是布局，保存必须原样提交。
    treeStubBehavior.nodes = [{ kind: 1, title: '第一章' }, { kind: 2, article_id: 'a1' }]

    const wrapper = mount(CollectionEditorOverlay, {
      props: { show: true, kind: 'collection', entity: card({ id: 'c9' }) },
    })

    // 全程不展开
    expect(wrapper.get('[data-role="collection-editor-drawer"]').attributes('style')).toContain('display: none')

    await wrapper.get('[data-role="collection-editor-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.composeAdminEntity).toHaveBeenCalledWith(
      'collection',
      'c9',
      expect.objectContaining({ nodes: [{ kind: 1, title: '第一章' }, { kind: 2, article_id: 'a1' }] }),
    )
  })

  it('编辑：保存走 updateCollection + setCollectionDisplay，不重复创建', async () => {
    adminApi.updateCollection.mockResolvedValue(undefined)
    adminApi.setCollectionDisplay.mockResolvedValue(undefined)
    const wrapper = mount(CollectionEditorOverlay, {
      props: { show: true, kind: 'collection', entity: card({ id: 'c9', title: '老合集' }) },
    })

    await wrapper.get('[data-role="collection-editor-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.createCollection).not.toHaveBeenCalled()
    // 票 348：字段与目录树走一次原子 compose，不再分两个请求。
    expect(adminApi.composeAdminEntity).toHaveBeenCalledWith(
      'collection',
      'c9',
      expect.objectContaining({ title: '老合集', nodes: expect.any(Array) }),
    )
    expect(adminApi.updateCollection).not.toHaveBeenCalled()
    expect(adminApi.setCollectionDisplay).toHaveBeenCalledWith('c9', true)
  })

  it('项目 kind 走项目接口（同一套弹窗，两个页面共用）', async () => {
    adminApi.updateProject.mockResolvedValue(undefined)
    adminApi.setProjectDisplay.mockResolvedValue(undefined)
    const wrapper = mount(CollectionEditorOverlay, {
      props: { show: true, kind: 'project', entity: card({ id: 'p1', title: '博客重构' }) },
    })

    await wrapper.get('[data-role="collection-editor-save"]').trigger('click')
    await flushPromises()

    expect(adminApi.composeAdminEntity).toHaveBeenCalledWith(
      'project',
      'p1',
      expect.objectContaining({ title: '博客重构' }),
    )
    expect(adminApi.updateCollection).not.toHaveBeenCalled()
  })

  it('取不到目录树节点时整单中止，绝不当成空树把树删光', async () => {
    // 编辑器在，但没暴露 nodes()：装配/替身出问题。
    treeStubBehavior.withNodes = false
    adminApi.composeAdminEntity.mockResolvedValue(undefined)
    adminApi.setCollectionDisplay.mockResolvedValue(undefined)

    const wrapper = mount(CollectionEditorOverlay, {
      props: { show: true, kind: 'collection', entity: card({ id: 'c7', title: '老合集' }) },
    })

    await wrapper.get('[data-role="collection-editor-save"]').trigger('click')
    await flushPromises()

    // compose 是整树覆盖：拿不到节点就不能发请求。
    expect(adminApi.composeAdminEntity).not.toHaveBeenCalled()
    expect(adminApi.setCollectionDisplay).not.toHaveBeenCalled()
    expect(messageMock.error).toHaveBeenCalled()
    treeStubBehavior.withNodes = true
  })

  it('编辑只回填已上传的封面：回退封面不会被当成用户选择存回去', async () => {
    adminApi.updateCollection.mockResolvedValue(undefined)
    adminApi.setCollectionDisplay.mockResolvedValue(undefined)
    const wrapper = mount(CollectionEditorOverlay, {
      props: {
        show: true,
        kind: 'collection',
        entity: card({ id: 'c2', cover: '', fallback_cover: 'https://cdn.test/fallback.jpg' }),
      },
    })

    // 预览用回退图（读者看到的样子）。
    expect(wrapper.get('.collection-overlay__cover-preview img').attributes('src')).toBe(
      'https://cdn.test/fallback.jpg',
    )

    await wrapper.get('[data-role="collection-editor-save"]').trigger('click')
    await flushPromises()

    // 但提交的是空封面 —— 不能把派生值写回数据库。
    expect(adminApi.composeAdminEntity).toHaveBeenCalledWith(
      'collection',
      'c2',
      expect.objectContaining({ cover: '' }),
    )
  })
})

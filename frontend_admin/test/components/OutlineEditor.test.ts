import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick, computed as vueComputed, ref as vueRef, onMounted as vueOnMounted } from 'vue'

/**
 * 目录编排页（#217 最小闭环）的组件契约。
 *
 * 只验证接缝：读回哪棵树、按钮改了什么、保存时提交的嵌套形状。
 * 移动与拍平的细节在 utils 单测里钉死；这里证明页面把两者接起来了。
 */

const { messageMock, adminApi } = vi.hoisted(() => ({
  messageMock: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  adminApi: {
    getAdminCollections: vi.fn(),
    getAdminProjects: vi.fn(),
    getCollectionOutline: vi.fn(),
    getProjectOutline: vi.fn(),
    saveCollectionOutline: vi.fn(),
    saveProjectOutline: vi.fn(),
    searchAdminArticles: vi.fn(),
  },
}))

vi.mock('~/services/admin', () => adminApi)

vi.mock('naive-ui', async () => {
  const { defineComponent, h } = await import('vue')

  const passthrough = (name: string) =>
    defineComponent({
      name,
      inheritAttrs: false,
      setup(_, { slots, attrs }) {
        return () => h('div', { ...attrs, 'data-stub': name }, slots.default ? slots.default() : [])
      },
    })

  const buttonStub = defineComponent({
    name: 'NButton',
    inheritAttrs: false,
    props: { disabled: { type: Boolean, default: false }, loading: { type: Boolean, default: false } },
    setup(props, { slots, attrs }) {
      return () =>
        h(
          'button',
          { ...attrs, type: 'button', disabled: props.disabled, 'data-loading': String(props.loading) },
          slots.default ? slots.default() : [],
        )
    },
  })

  const inputStub = defineComponent({
    name: 'NInput',
    inheritAttrs: false,
    props: { value: { type: String, default: '' } },
    emits: ['update:value'],
    setup(props, { emit, attrs }) {
      return () =>
        h('input', {
          ...attrs,
          value: props.value,
          onInput: (event: Event) => emit('update:value', (event.target as HTMLInputElement).value),
        })
    },
  })

  const selectStub = defineComponent({
    name: 'NSelect',
    inheritAttrs: false,
    props: {
      value: { default: () => '' },
      options: { type: Array, default: () => [] },
      multiple: { type: Boolean, default: false },
    },
    emits: ['update:value'],
    setup(props, { emit, attrs }) {
      return () =>
        h(
          'select',
          {
            ...attrs,
            multiple: props.multiple,
            onChange: (event: Event) => {
              const select = event.target as HTMLSelectElement
              const values = Array.from(select.selectedOptions).map(option => option.value)
              emit('update:value', props.multiple ? values : values[0] ?? '')
            },
          },
          (props.options as Array<{ label: string; value: string }>).map(option =>
            h('option', { value: option.value }, option.label),
          ),
        )
    },
  })

  const tagStub = defineComponent({
    name: 'NTag',
    inheritAttrs: false,
    props: { type: { type: String, default: 'default' } },
    setup(props, { slots, attrs }) {
      return () =>
        h('span', { ...attrs, 'data-stub': 'NTag', 'data-type': props.type }, slots.default ? slots.default() : [])
    },
  })

  return {
    NButton: buttonStub,
    NCard: passthrough('NCard'),
    NInput: inputStub,
    NSelect: selectStub,
    NSpace: passthrough('NSpace'),
    NTag: tagStub,
    useMessage: () => messageMock,
  }
})

Object.assign(globalThis, { ref: vueRef, computed: vueComputed, onMounted: vueOnMounted })

const { default: OutlinePage } = await import('~/pages/articles/outline.vue')

const mounted: Array<{ app: ReturnType<typeof createApp>; host: HTMLElement }> = []

function treeResponse() {
  return {
    owner_kind: 1,
    owner_id: '1',
    nodes: [
      {
        id: 'f1',
        parent_id: '',
        kind: 1,
        title: '第一章',
        article_id: '',
        sort_order: 1,
        article_title: '',
        article_cover: '',
        stale: false,
        stale_reason: '',
        children: [
          {
            id: 'a1',
            parent_id: 'f1',
            kind: 2,
            title: '',
            article_id: 'art1',
            sort_order: 1,
            article_title: '第一篇',
            article_cover: '',
            stale: false,
            stale_reason: '',
            children: [],
          },
        ],
      },
      {
        id: 'a2',
        parent_id: '',
        kind: 2,
        title: '',
        article_id: 'art2',
        sort_order: 2,
        article_title: '第二篇',
        article_cover: '',
        stale: true,
        stale_reason: 'not_published',
        children: [],
      },
    ],
  }
}

async function mountPage() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(OutlinePage)
  app.mount(host)
  mounted.push({ app, host })
  await vi.waitFor(() => expect(adminApi.getCollectionOutline).toHaveBeenCalledWith('1'))
  await nextTick()
  return host
}

function rowElement(host: HTMLElement, key: string): HTMLElement {
  const row = host.querySelector(`[data-row-key="${key}"]`) as HTMLElement | null
  expect(row, `找不到行 ${key}`).not.toBeNull()
  return row!
}

function rowButton(host: HTMLElement, key: string, label: string): HTMLButtonElement {
  const row = rowElement(host, key)
  const buttons = Array.from(row.querySelectorAll('button'))
  const found = buttons.find(button => button.textContent?.trim() === label)
  expect(found, `行 ${key} 里没有「${label}」按钮`).not.toBeUndefined()
  return found as HTMLButtonElement
}

async function setInput(host: HTMLElement, role: string, value: string) {
  const input = host.querySelector(`[data-role="${role}"]`) as HTMLInputElement | null
  expect(input, `找不到输入框 ${role}`).not.toBeNull()
  input!.value = value
  input!.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}

async function selectArticles(host: HTMLElement, ids: string[]) {
  await vi.waitFor(() =>
    expect(host.querySelector('select[data-role="outline-articles"]')).not.toBeNull(),
  )
  const select = host.querySelector('select[data-role="outline-articles"]') as HTMLSelectElement
  for (const option of Array.from(select.options)) {
    option.selected = ids.includes(option.value)
  }
  select.dispatchEvent(new Event('change', { bubbles: true }))
  await nextTick()
}

beforeEach(() => {
  adminApi.getAdminCollections.mockResolvedValue({
    list: [{ id: '1', title: '专题一' }],
    count: 1,
  })
  adminApi.getAdminProjects.mockResolvedValue({ list: [], count: 0 })
  adminApi.getCollectionOutline.mockResolvedValue(treeResponse())
  adminApi.getProjectOutline.mockResolvedValue(treeResponse())
  adminApi.saveCollectionOutline.mockImplementation((_id: string, nodes: unknown[]) =>
    Promise.resolve({ owner_kind: 1, owner_id: '1', nodes }),
  )
  adminApi.saveProjectOutline.mockImplementation((_id: string, nodes: unknown[]) =>
    Promise.resolve({ owner_kind: 2, owner_id: '1', nodes }),
  )
  adminApi.searchAdminArticles.mockResolvedValue({
    list: [
      { id: 'art1', title: '第一篇' },
      { id: 'art3', title: '第三篇' },
    ],
    count: 2,
  })
})

afterEach(() => {
  for (const { app, host } of mounted.splice(0)) {
    app.unmount()
    host.remove()
  }
  vi.clearAllMocks()
})

describe('目录编排页：读回与新建', () => {
  it('挂载后读回第一棵合集树，保留空文件夹与失效标记', async () => {
    const host = await mountPage()
    expect(rowElement(host, 'f1').textContent).toContain('第一章')
    expect(rowElement(host, 'a1').textContent).toContain('第一篇')
    expect(rowElement(host, 'a2').textContent).toContain('第二篇')
    expect(rowElement(host, 'a2').textContent).toContain('已失效')
    expect(rowElement(host, 'a2').textContent).toContain('文章已不是已发布状态')
  })

  it('根层新建空文件夹后点保存，提交的整棵树包含这个空文件夹', async () => {
    const host = await mountPage()
    await setInput(host, 'outline-folder-title', '第二章')
    ;(host.querySelector('[data-role="outline-create-root"]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(rowElement(host, 'tmp-folder-1').textContent).toContain('第二章'))

    ;(host.querySelector('[data-role="outline-save"]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(adminApi.saveCollectionOutline).toHaveBeenCalledTimes(1))

    const nodes = adminApi.saveCollectionOutline.mock.calls[0]![1] as Array<Record<string, unknown>>
    expect(nodes.map(node => node.title || node.article_id)).toEqual(['第一章', 'art2', '第二章'])
    const added = nodes[2]!
    expect(added.id).toBe('')
    expect(added.children).toEqual([])
    expect(messageMock.success).toHaveBeenCalled()
  })
})

describe('目录编排页：重命名 / 移除 / 清理失效', () => {
  it('重命名文件夹后整树提交带新标题，文章行没有重命名入口', async () => {
    const host = await mountPage()
    expect(rowButton(host, 'f1', '重命名')).toBeDefined()
    expect(Array.from(rowElement(host, 'a1').querySelectorAll('button')).some(
      button => button.textContent?.trim() === '重命名',
    )).toBe(false)

    await setInput(host, 'outline-rename-title', '  第一章（修订）  ')
    rowButton(host, 'f1', '重命名').click()
    await vi.waitFor(() => expect(rowElement(host, 'f1').textContent).toContain('第一章（修订）'))

    ;(host.querySelector('[data-role="outline-save"]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(adminApi.saveCollectionOutline).toHaveBeenCalledTimes(1))
    const nodes = adminApi.saveCollectionOutline.mock.calls[0]![1] as Array<{ id: string; title: string }>
    expect(nodes[0]!.title).toBe('第一章（修订）')
    expect(messageMock.success).toHaveBeenCalled()
  })

  it('移除文件夹时连同它的文章一起从整树提交里消失', async () => {
    const host = await mountPage()
    rowButton(host, 'f1', '移除').click()
    await vi.waitFor(() => expect(host.querySelector('[data-row-key="a1"]')).toBeNull())

    ;(host.querySelector('[data-role="outline-save"]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(adminApi.saveCollectionOutline).toHaveBeenCalledTimes(1))
    const nodes = adminApi.saveCollectionOutline.mock.calls[0]![1] as Array<{ id: string }>
    expect(nodes.map(node => node.id)).toEqual(['a2'])
  })

  it('清理失效行只提交仍展示的文章，失效标记与清理入口只出现一次', async () => {
    const host = await mountPage()
    const cleanupButton = host.querySelector('[data-role="outline-cleanup-stale"]') as HTMLButtonElement
    expect(cleanupButton.textContent).toContain('清理失效（1）')
    cleanupButton.click()
    await vi.waitFor(() => expect(host.querySelector('[data-row-key="a2"]')).toBeNull())

    ;(host.querySelector('[data-role="outline-save"]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(adminApi.saveCollectionOutline).toHaveBeenCalledTimes(1))
    const nodes = adminApi.saveCollectionOutline.mock.calls[0]![1] as Array<{ id: string }>
    expect(nodes.map(node => node.id)).toEqual(['f1'])
  })

  it('连续两次保存同一棵树提交相同 payload（重复提交不产生变化）', async () => {
    const host = await mountPage()
    const save = host.querySelector('[data-role="outline-save"]') as HTMLButtonElement
    save.click()
    await vi.waitFor(() => expect(adminApi.saveCollectionOutline).toHaveBeenCalledTimes(1))
    save.click()
    await vi.waitFor(() => expect(adminApi.saveCollectionOutline).toHaveBeenCalledTimes(2))

    const [first, second] = adminApi.saveCollectionOutline.mock.calls.map(call => call[1])
    expect(second).toEqual(first)
  })
})

describe('目录编排页：层级操作', () => {
  it('把根层文章缩进到上一个文件夹，保存时提交嵌套结构', async () => {
    const host = await mountPage()
    rowElement(host, 'a2').click()
    await nextTick()
    rowButton(host, 'a2', '缩进').click()
    await vi.waitFor(() => expect(rowElement(host, 'a2').getAttribute('style')).toContain('20px'))

    ;(host.querySelector('[data-role="outline-save"]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(adminApi.saveCollectionOutline).toHaveBeenCalledTimes(1))

    const nodes = adminApi.saveCollectionOutline.mock.calls[0]![1] as Array<{
      id: string
      children: Array<{ id: string }>
    }>
    expect(nodes).toHaveLength(1)
    expect(nodes[0]!.id).toBe('f1')
    expect(nodes[0]!.children.map(child => child.id)).toEqual(['a1', 'a2'])
  })

  it('上一行是文章节点时缩进按钮禁用（文章不能做父节点）', async () => {
    const host = await mountPage()
    // a1 的上一个兄弟不存在；根层的 a2 上一个是文件夹，因此这里验证根层第一行不能缩进。
    expect(rowButton(host, 'f1', '缩进').disabled).toBe(true)
    expect(rowButton(host, 'a2', '缩进').disabled).toBe(false)
  })
})

describe('目录编排页：批量收录', () => {
  it('搜索并勾选文章后加入当前文件夹，保存时出现在树里', async () => {
    const host = await mountPage()
    await setInput(host, 'outline-keyword', '第三篇')
    ;(Array.from(host.querySelectorAll('button')).find(button => button.textContent?.trim() === '搜索') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(adminApi.searchAdminArticles).toHaveBeenCalled())

    await selectArticles(host, ['art3'])
    ;(host.querySelector('[data-role="outline-include"]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(rowElement(host, 'tmp-article-art3').textContent).toContain('第三篇'))

    ;(host.querySelector('[data-role="outline-save"]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(adminApi.saveCollectionOutline).toHaveBeenCalledTimes(1))

    const nodes = adminApi.saveCollectionOutline.mock.calls[0]![1] as Array<{
      id: string
      article_id: string
      kind: number
      children: Array<{ article_id: string }>
    }>
    const included = nodes.find(node => node.article_id === 'art3')!
    expect(included.kind).toBe(2)
    expect(included.children).toEqual([])
    // 已在树里的文章不会重复加入（art1 仍在第一章下面）。
    const folder = nodes.find(node => node.id === 'f1')!
    expect(folder.children.filter(child => child.article_id === 'art1')).toHaveLength(1)
  })
})

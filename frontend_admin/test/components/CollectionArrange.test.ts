import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick, reactive as vueReactive, ref as vueRef, onMounted as vueOnMounted } from 'vue'

/**
 * 合集编排页的组件用例。
 *
 * 只验证接线：两级上移 / 下移各发什么请求、批量收录是否一次提交多篇、
 * 重复结局有没有被摆到界面上。naive-ui 用同形桩（按钮 / 下拉是真的原生元素）。
 */

const { messageMock, adminApi } = vi.hoisted(() => ({
  messageMock: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  adminApi: {
    getAdminCollections: vi.fn(),
    getAdminCollectionItems: vi.fn(),
    updateCollectionOrder: vi.fn(),
    updateCollectionItemOrder: vi.fn(),
    batchIncludeCollectionArticles: vi.fn(),
    removeCollectionArticle: vi.fn(),
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

  const tableStub = defineComponent({
    name: 'NDataTable',
    inheritAttrs: false,
    props: {
      data: { type: Array, default: () => [] },
      columns: { type: Array, default: () => [] },
    },
    setup(props) {
      return () =>
        h(
          'div',
          { 'data-stub': 'NDataTable' },
          (props.data as Array<Record<string, unknown>>).map((row, index) =>
            h(
              'div',
              { 'data-row': String(row.id ?? row.article_id) },
              (props.columns as Array<{ key?: string; render?: (r: unknown, i: number) => unknown }>).map(column =>
                h(
                  'div',
                  { 'data-col': column.key ?? '' },
                  column.render ? (column.render(row, index) as never) : String(row[column.key ?? ''] ?? ''),
                ),
              ),
            ),
          ),
        )
    },
  })

  const tagStub = defineComponent({
    name: 'NTag',
    inheritAttrs: false,
    props: { type: { type: String, default: 'default' } },
    setup(props, { slots, attrs }) {
      return () => h('span', { ...attrs, 'data-stub': 'NTag', 'data-type': props.type }, slots.default ? slots.default() : [])
    },
  })

  return {
    NButton: buttonStub,
    NCard: passthrough('NCard'),
    NDataTable: tableStub,
    NInput: inputStub,
    NSelect: selectStub,
    NSpace: passthrough('NSpace'),
    NTag: tagStub,
    useMessage: () => messageMock,
  }
})

Object.assign(globalThis, { ref: vueRef, reactive: vueReactive, onMounted: vueOnMounted })

const { default: ArrangePage } = await import('~/pages/articles/collection-arrange.vue')

const mounted: Array<{ app: ReturnType<typeof createApp>; host: HTMLElement }> = []

function collectionRow(overrides: Record<string, unknown> = {}) {
  return {
    id: '1',
    title: '合集一',
    description: '',
    cover: '',
    fallback_cover: '',
    displayed: true,
    article_count: 2,
    sort_order: 1,
    ...overrides,
  }
}

function itemRow(overrides: Record<string, unknown> = {}) {
  return { article_id: 'a1', sort_order: 1, title: '文章一', cover: '', stale: false, ...overrides }
}

async function mountPage() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(ArrangePage)
  app.mount(host)
  mounted.push({ app, host })
  await vi.waitFor(() => expect(adminApi.getAdminCollections).toHaveBeenCalled())
  await nextTick()
  return host
}

beforeEach(() => {
  adminApi.getAdminCollections.mockResolvedValue({
    list: [collectionRow({ id: '1', title: '合集一', sort_order: 1 }), collectionRow({ id: '2', title: '合集二', sort_order: 2 })],
    count: 2,
  })
  adminApi.getAdminCollectionItems.mockResolvedValue({
    list: [itemRow({ article_id: 'a1', sort_order: 1, title: '文章一' }), itemRow({ article_id: 'a2', sort_order: 2, title: '文章二' })],
    count: 2,
  })
  adminApi.updateCollectionOrder.mockResolvedValue(undefined)
  adminApi.updateCollectionItemOrder.mockResolvedValue(undefined)
  adminApi.removeCollectionArticle.mockResolvedValue(undefined)
  adminApi.searchAdminArticles.mockResolvedValue({
    list: [
      { id: 'a1', title: '文章一' },
      { id: 'a2', title: '文章二' },
      { id: 'a3', title: '文章三' },
    ],
    count: 3,
  })
  adminApi.batchIncludeCollectionArticles.mockResolvedValue({
    results: [
      { article_id: 'a2', outcome: 'included' },
      { article_id: 'a1', outcome: 'duplicated' },
      { article_id: 'a3', outcome: 'not_includable' },
    ],
    included: 1,
    duplicated: 1,
    rejected: 1,
  })
})

afterEach(() => {
  for (const { app, host } of mounted.splice(0)) {
    app.unmount()
    host.remove()
  }
})

function rowButton(host: HTMLElement, rowId: string, label: string): HTMLButtonElement {
  const buttons = Array.from(host.querySelectorAll(`[data-row="${rowId}"] button`))
  const found = buttons.find(button => button.textContent?.trim() === label)
  expect(found, `行 ${rowId} 里没有「${label}」按钮`).not.toBeUndefined()
  return found as HTMLButtonElement
}

async function chooseCollection(host: HTMLElement, id: string) {
  const select = host.querySelector('select[data-role="arrange-collection"]') as HTMLSelectElement
  expect(select).not.toBeNull()
  select.value = id
  select.dispatchEvent(new Event('change', { bubbles: true }))
  await nextTick()
}

async function selectArticles(host: HTMLElement, ids: string[]) {
  await vi.waitFor(() =>
    expect(host.querySelector('select[data-role="arrange-articles"]')).not.toBeNull(),
  )
  const select = host.querySelector('select[data-role="arrange-articles"]') as HTMLSelectElement
  for (const option of Array.from(select.options)) {
    option.selected = ids.includes(option.value)
  }
  select.dispatchEvent(new Event('change', { bubbles: true }))
  await nextTick()
}

describe('合集编排页：合集之间的排序', () => {
  it('下移一行时提交完整的有序合集列表，完成后重载列表', async () => {
    const host = await mountPage()

    rowButton(host, '1', '下移').click()
    await vi.waitFor(() => expect(adminApi.updateCollectionOrder).toHaveBeenCalledTimes(1))

    expect(adminApi.updateCollectionOrder).toHaveBeenCalledWith(['2', '1'])
    await vi.waitFor(() => expect(adminApi.getAdminCollections).toHaveBeenCalledTimes(2))
    expect(messageMock.success).toHaveBeenCalled()
  })

  it('第一行不能上移、最后一行不能下移（按钮禁用）', async () => {
    const host = await mountPage()

    expect((rowButton(host, '1', '上移') as HTMLButtonElement).disabled).toBe(true)
    expect((rowButton(host, '2', '下移') as HTMLButtonElement).disabled).toBe(true)
  })
})

describe('合集编排页：合集内的排序', () => {
  it('选中合集后加载收录行；上移提交完整集合（含失效行）', async () => {
    const host = await mountPage()
    await chooseCollection(host, '2')

    await vi.waitFor(() => expect(adminApi.getAdminCollectionItems).toHaveBeenCalledWith('2'))
    rowButton(host, 'a2', '上移').click()
    await vi.waitFor(() => expect(adminApi.updateCollectionItemOrder).toHaveBeenCalledTimes(1))

    expect(adminApi.updateCollectionItemOrder).toHaveBeenCalledWith('2', ['a2', 'a1'])
  })
})

describe('合集编排页：批量收录', () => {
  it('一次请求提交选中的多篇，重复与不可收录逐篇区分', async () => {
    const host = await mountPage()
    await chooseCollection(host, '1')

    const keywordInput = host.querySelector('input[data-role="arrange-keyword"]') as HTMLInputElement
    keywordInput.value = '文章'
    keywordInput.dispatchEvent(new Event('input', { bubbles: true }))
    ;(Array.from(host.querySelectorAll('button')).find(button => button.textContent?.trim() === '搜索') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(adminApi.searchAdminArticles).toHaveBeenCalledTimes(1))

    await selectArticles(host, ['a1', 'a2', 'a3'])
    ;(host.querySelector('button[data-role="batch-include"]') as HTMLButtonElement).click()
    await vi.waitFor(() => expect(adminApi.batchIncludeCollectionArticles).toHaveBeenCalledTimes(1))

    // 一次请求带上全部选中项（去重后）。
    expect(adminApi.batchIncludeCollectionArticles).toHaveBeenCalledWith('1', ['a1', 'a2', 'a3'])
    // 重复与不可收录的结果要摆到界面上，而不是只报"成功"。
    await vi.waitFor(() => expect(host.textContent).toContain('已在合集中'))
    expect(host.textContent).toContain('不可收录')
    expect(messageMock.success).toHaveBeenCalledWith(expect.stringContaining('已收录 1 篇'))
  })
})

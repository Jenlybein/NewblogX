import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick, computed as vueComputed, reactive as vueReactive, ref as vueRef, onMounted as vueOnMounted } from 'vue'

/**
 * 后台合集管理页的组件用例。
 *
 * 只验证**接线**：表单提交时发了什么请求、封面上传走的是不是"先定形再上传"、
 * 列表按哪种封面渲染、展示开关的结果有没有落到列表上。naive-ui 用与真组件
 * 同形的桩（输入 / 开关 / 按钮仍是真的原生元素），浮层只渲染开关状态。
 */

const { messageMock, dialogMock, adminApi, uploadMock, shapeMock, measureMock } = vi.hoisted(() => ({
  messageMock: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  dialogMock: { warning: vi.fn() },
  adminApi: {
    getAdminCollections: vi.fn(),
    createCollection: vi.fn(),
    updateCollection: vi.fn(),
    deleteCollection: vi.fn(),
    setCollectionDisplay: vi.fn(),
  },
  uploadMock: vi.fn(),
  shapeMock: vi.fn(),
  /** 量原图尺寸（#215）：默认 16:9 → 合规、不弹裁剪窗。 */
  measureMock: vi.fn(),
}))

vi.mock('~/services/admin', () => adminApi)
vi.mock('~/services/image', () => ({ uploadImageByTask: uploadMock }))

// 只替换"定形"这一条浏览器副作用（happy-dom 没有 canvas，ADR-0016）。
// 定形参数（COVER_SHAPE_OPTIONS）保持真实实现：测的正是"页面确实拿它去定形"。
vi.mock('~/utils/imageShaping', async (importOriginal) => {
  const actual = await importOriginal<typeof import('~/utils/imageShaping')>()
  return { ...actual, shapeForUpload: shapeMock, measureImage: measureMock }
})

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

  const formItemStub = defineComponent({
    name: 'NFormItem',
    inheritAttrs: false,
    props: { label: { type: String, default: '' } },
    setup(props, { slots, attrs }) {
      return () =>
        h('div', { ...attrs, 'data-stub': 'NFormItem', 'data-label': props.label }, [
          h('span', { 'data-role': 'label' }, props.label),
          slots.default ? slots.default() : null,
        ])
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

  const buttonStub = defineComponent({
    name: 'NButton',
    inheritAttrs: false,
    props: { disabled: { type: Boolean, default: false }, loading: { type: Boolean, default: false } },
    setup(props, { slots, attrs }) {
      return () =>
        h(
          'button',
          {
            ...attrs,
            type: 'button',
            disabled: props.disabled,
            'data-loading': String(props.loading),
          },
          slots.default ? slots.default() : [],
        )
    },
  })

  const switchStub = defineComponent({
    name: 'NSwitch',
    inheritAttrs: false,
    props: { value: { type: Boolean, default: false } },
    emits: ['update:value'],
    setup(props, { emit, attrs }) {
      return () =>
        h('button', {
          ...attrs,
          type: 'button',
          'data-role': 'NSwitch',
          'data-value': String(props.value),
          onClick: () => emit('update:value', !props.value),
        })
    }
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
              { 'data-row': String(row.id) },
              (props.columns as Array<{ key?: string; render?: (r: unknown, i: number) => unknown }>).map((column) =>
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

  const modalStub = defineComponent({
    name: 'NModal',
    inheritAttrs: false,
    props: { show: { type: Boolean, default: false } },
    setup(props, { slots }) {
      return () =>
        h('div', { 'data-stub': 'NModal', 'data-show': String(props.show) }, props.show && slots.default ? slots.default() : [])
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
    NForm: passthrough('NForm'),
    NFormItem: formItemStub,
    NInput: inputStub,
    NModal: modalStub,
    NSpace: passthrough('NSpace'),
    NSwitch: switchStub,
    NTag: tagStub,
    useMessage: () => messageMock,
    useDialog: () => dialogMock,
  }
})

// Nuxt 自动导入：script setup 顶层就用 ref / reactive / computed / onMounted。
Object.assign(globalThis, { ref: vueRef, reactive: vueReactive, computed: vueComputed, onMounted: vueOnMounted })

const { default: CollectionsPage } = await import('~/pages/articles/collections.vue')

const mounted: Array<{ app: ReturnType<typeof createApp>; host: HTMLElement }> = []

function collectionRow(overrides: Record<string, unknown> = {}) {
  return {
    id: '1',
    title: 'Nuxt 部署实战',
    description: '从零到上线',
    cover: '',
    fallback_cover: '',
    displayed: true,
    article_count: 3,
    sort_order: 1,
    ...overrides,
  }
}

async function mountPage() {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(CollectionsPage)
  app.mount(host)
  mounted.push({ app, host })
  await vi.waitFor(() => {
    expect(adminApi.getAdminCollections).toHaveBeenCalled()
  })
  await nextTick()
  return host
}

beforeEach(() => {
  adminApi.getAdminCollections.mockResolvedValue({ list: [collectionRow()], count: 1 })
  adminApi.createCollection.mockResolvedValue({ id: '2' })
  adminApi.updateCollection.mockResolvedValue(undefined)
  adminApi.deleteCollection.mockResolvedValue(undefined)
  adminApi.setCollectionDisplay.mockResolvedValue(undefined)
  uploadMock.mockResolvedValue({ image_id: '7', url: 'https://cdn.example.com/uploaded.png' })
  measureMock.mockReset()
  measureMock.mockResolvedValue({ width: 1920, height: 1080 })
  // 定形产物：16:9、1600×900 的新文件 + 它的本地预览地址。
  shapeMock.mockImplementation(async () => ({
    file: new File(['shaped'], 'cover.jpg', { type: 'image/jpeg' }),
    previewUrl: 'blob:shaped',
    size: { width: 1600, height: 900 },
  }))
  dialogMock.warning.mockImplementation((options: { onPositiveClick?: () => unknown }) => {
    void options.onPositiveClick?.()
  })
  // happy-dom 没有真实的 object URL：给一个确定的假实现，让预览与释放都可断言。
  if (typeof URL.createObjectURL !== 'function') {
    Object.assign(URL, { createObjectURL: () => 'blob:preview', revokeObjectURL: () => undefined })
  }
})

afterEach(() => {
  for (const { app, host } of mounted.splice(0)) {
    app.unmount()
    host.remove()
  }
})

function titleInput(host: HTMLElement): HTMLInputElement {
  const input = host.querySelector('input[data-role="collection-title"]')
  expect(input).not.toBeNull()
  return input as HTMLInputElement
}

async function setTitle(host: HTMLElement, value: string) {
  const input = titleInput(host)
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}

function submitButton(host: HTMLElement): HTMLButtonElement {
  const button = host.querySelector('button[data-role="collection-submit"]')
  expect(button).not.toBeNull()
  return button as HTMLButtonElement
}

function rowButton(host: HTMLElement, rowId: string, label: string): HTMLButtonElement {
  const buttons = Array.from(host.querySelectorAll(`[data-row="${rowId}"] button`))
  const found = buttons.find(button => button.textContent?.trim() === label)
  expect(found, `行 ${rowId} 里没有「${label}」按钮`).not.toBeUndefined()
  return found as HTMLButtonElement
}

function coverInput(host: HTMLElement): HTMLInputElement {
  const input = host.querySelector('input[data-role="collection-cover-input"]')
  expect(input).not.toBeNull()
  return input as HTMLInputElement
}

async function pickCover(host: HTMLElement, file: File) {
  const input = coverInput(host)
  Object.defineProperty(input, 'files', { value: [file], configurable: true })
  input.dispatchEvent(new Event('change', { bubbles: true }))
  await nextTick()
}

const coverFile = () => new File(['x'], 'cover.png', { type: 'image/png' })

describe('合集管理页：增删改', () => {
  it('新建合集：提交标题 / 描述 / 封面 / 展示开关，并在成功后刷新列表', async () => {
    const host = await mountPage()
    await setTitle(host, '  新合集  ')
    submitButton(host).click()
    await vi.waitFor(() => expect(adminApi.createCollection).toHaveBeenCalledTimes(1))

    expect(adminApi.createCollection).toHaveBeenCalledWith({
      title: '新合集',
      description: '',
      cover: '',
      displayed: true,
    })
    await vi.waitFor(() => expect(adminApi.getAdminCollections).toHaveBeenCalledTimes(2))
  })

  it('编辑合集：先回填，保存时只提交内容、不发创建请求', async () => {
    const host = await mountPage()
    rowButton(host, '1', '编辑').click()
    await nextTick()
    expect(titleInput(host).value).toBe('Nuxt 部署实战')

    await setTitle(host, 'Nuxt 部署实战（二版）')
    submitButton(host).click()
    await vi.waitFor(() => expect(adminApi.updateCollection).toHaveBeenCalledTimes(1))

    expect(adminApi.updateCollection).toHaveBeenCalledWith('1', {
      title: 'Nuxt 部署实战（二版）',
      description: '从零到上线',
      cover: '',
    })
    expect(adminApi.createCollection).not.toHaveBeenCalled()
  })

  it('删除合集：确认后只删合集本身', async () => {
    const host = await mountPage()
    rowButton(host, '1', '删除').click()
    await vi.waitFor(() => expect(adminApi.deleteCollection).toHaveBeenCalledWith('1'))
    await vi.waitFor(() => expect(adminApi.getAdminCollections).toHaveBeenCalledTimes(2))
  })

  it('标题为空时拦住提交，不发任何写请求', async () => {
    const host = await mountPage()
    await setTitle(host, '   ')
    submitButton(host).click()
    await nextTick()

    expect(adminApi.createCollection).not.toHaveBeenCalled()
    expect(messageMock.warning).toHaveBeenCalled()
  })
})

describe('合集管理页：封面定形（#200）', () => {
  it('选图后先定形成 16:9，只上传定形产物（原图不上传），保存带上传后的地址', async () => {
    const host = await mountPage()
    const original = coverFile()
    await pickCover(host, original)

    await vi.waitFor(() => expect(uploadMock).toHaveBeenCalledTimes(1))
    await vi.waitFor(() => expect(host.textContent).toContain('封面上传完成，点击保存'))
    // 定形拿到的是用户选的原图，上传拿到的是定形产物——两者不是同一个文件。
    expect(shapeMock).toHaveBeenCalledWith(original, expect.objectContaining({ target: { width: 1600, height: 900 } }))
    const uploadedFile = (uploadMock.mock.calls[0] as [File])[0]
    expect(uploadedFile).not.toBe(original)
    expect(uploadedFile.name).toBe('cover.jpg')
    expect(uploadedFile.type).toBe('image/jpeg')

    // 面板显示的是定形产物尺寸，与"已定形"标记。
    expect(host.textContent).toContain('1600 × 900')
    expect(host.textContent).toContain('已定形')

    await setTitle(host, '带封面的合集')
    submitButton(host).click()
    await vi.waitFor(() => expect(adminApi.createCollection).toHaveBeenCalledTimes(1))
    expect(adminApi.createCollection).toHaveBeenCalledWith(
      expect.objectContaining({ cover: 'https://cdn.example.com/uploaded.png' }),
    )
  })

  it('定形产物的本地预览地址在上传完成后释放（不占着那张图的内存）', async () => {
    const revoke = vi.fn()
    const originalRevoke = URL.revokeObjectURL
    Object.assign(URL, { revokeObjectURL: revoke })
    const host = await mountPage()
    await pickCover(host, coverFile())

    await vi.waitFor(() => expect(uploadMock).toHaveBeenCalledTimes(1))
    await vi.waitFor(() => expect(revoke).toHaveBeenCalledWith('blob:shaped'))
    Object.assign(URL, { revokeObjectURL: originalRevoke })
  })

  it('定形失败时报错、不上传、封面保持为空（不落半成品）', async () => {
    shapeMock.mockRejectedValueOnce(new Error('无法读取这张图片，请换一张'))
    const host = await mountPage()
    await pickCover(host, coverFile())

    await vi.waitFor(() => expect(messageMock.error).toHaveBeenCalled())
    expect(uploadMock).not.toHaveBeenCalled()

    await setTitle(host, '定形失败的合集')
    submitButton(host).click()
    await vi.waitFor(() => expect(adminApi.createCollection).toHaveBeenCalledTimes(1))
    expect(adminApi.createCollection).toHaveBeenCalledWith(expect.objectContaining({ cover: '' }))
  })
})

describe('合集管理页：裁剪窗（#215）', () => {
  const nonCompliant = { width: 1000, height: 1000 }

  function cropFrame(host: HTMLElement): HTMLElement | null {
    return host.querySelector('[data-role="crop-frame"]')
  }

  it('比例不合规（>1%）时只弹裁剪窗：不定形、不上传、现有封面不动', async () => {
    measureMock.mockResolvedValue(nonCompliant)
    const host = await mountPage()
    const original = coverFile()
    await pickCover(host, original)

    await vi.waitFor(() => expect(cropFrame(host)).not.toBeNull())
    expect(measureMock).toHaveBeenCalledWith(original)
    expect(shapeMock).not.toHaveBeenCalled()
    expect(uploadMock).not.toHaveBeenCalled()
    expect(host.textContent).toContain('图片比例不符合')
    expect(host.textContent).toContain('16:9')
  })

  it('确认裁剪：按选区定形成新文件再上传，并关掉裁剪窗', async () => {
    measureMock.mockResolvedValue(nonCompliant)
    const host = await mountPage()
    const original = coverFile()
    await pickCover(host, original)
    await vi.waitFor(() => expect(cropFrame(host)).not.toBeNull())

    const confirm = host.querySelector('[aria-label="确认裁剪"]') as HTMLButtonElement | null
    expect(confirm).not.toBeNull()
    ;(confirm as HTMLButtonElement).click()

    await vi.waitFor(() => expect(uploadMock).toHaveBeenCalledTimes(1))
    // 定形拿到的是原图 + 选区（形状由确认动作带进定形参数）。
    expect(shapeMock).toHaveBeenCalledWith(
      original,
      expect.objectContaining({
        target: { width: 1600, height: 900 },
        framing: expect.objectContaining({ zoom: 1 }),
      }),
    )
    expect(uploadMock.mock.calls[0]?.[0]).not.toBe(original)
    await vi.waitFor(() => expect(cropFrame(host)).toBeNull())
  })

  it('取消裁剪：中止本次上传，现有封面保持为空，不产生任何记录', async () => {
    measureMock.mockResolvedValue(nonCompliant)
    const host = await mountPage()
    await pickCover(host, coverFile())
    await vi.waitFor(() => expect(cropFrame(host)).not.toBeNull())

    const cancel = host.querySelector('[aria-label="取消裁剪"]') as HTMLButtonElement | null
    expect(cancel).not.toBeNull()
    ;(cancel as HTMLButtonElement).click()

    await vi.waitFor(() => expect(cropFrame(host)).toBeNull())
    expect(shapeMock).not.toHaveBeenCalled()
    expect(uploadMock).not.toHaveBeenCalled()
    expect(host.textContent).toContain('已取消本次上传')

    // 提交时封面上传地址仍是空串：这次选图当作没发生。
    await setTitle(host, '取消裁剪后的合集')
    submitButton(host).click()
    await vi.waitFor(() => expect(adminApi.createCollection).toHaveBeenCalledTimes(1))
    expect(adminApi.createCollection).toHaveBeenCalledWith(expect.objectContaining({ cover: '' }))
  })
})

describe('合集管理页：封面回退与占位', () => {
  it('没上传封面时回退到首篇文章的封面，并标明这是回退', async () => {
    adminApi.getAdminCollections.mockResolvedValue({
      list: [collectionRow({ cover: '', fallback_cover: 'https://cdn.example.com/first.png' })],
      count: 1,
    })
    const host = await mountPage()

    const img = host.querySelector('[data-row="1"] img') as HTMLImageElement | null
    expect(img?.getAttribute('src')).toBe('https://cdn.example.com/first.png')
    expect(host.querySelector('[data-row="1"]')?.textContent).toContain('回退首篇')
  })

  it('两张封面都没有时不留破图：不渲染 img，显示显式占位', async () => {
    adminApi.getAdminCollections.mockResolvedValue({ list: [collectionRow()], count: 1 })
    const host = await mountPage()

    expect(host.querySelector('[data-row="1"] img')).toBeNull()
    const placeholder = host.querySelector('[data-role="collection-cover-empty"]')
    expect(placeholder?.textContent).toContain('无封面')
  })
})

describe('合集管理页：展示开关', () => {
  it('拨动开关会发 PATCH，并把结果落回列表（不靠整表刷新）', async () => {
    const host = await mountPage()
    const toggle = host.querySelector('[data-row="1"] button[data-role="NSwitch"]')
    expect(toggle).not.toBeNull()
    ;(toggle as HTMLButtonElement).click()

    await vi.waitFor(() => expect(adminApi.setCollectionDisplay).toHaveBeenCalledWith('1', false))
    await vi.waitFor(() =>
      expect(host.querySelector('[data-row="1"] button[data-role="NSwitch"]')?.getAttribute('data-value')).toBe('false'),
    )
    expect(messageMock.success).toHaveBeenCalled()
  })

  it('请求失败时列表保持原状（不假装已隐藏）', async () => {
    adminApi.setCollectionDisplay.mockRejectedValueOnce(new Error('网络中断'))
    const host = await mountPage()
    const toggle = host.querySelector('[data-row="1"] button[data-role="NSwitch"]')
    ;(toggle as HTMLButtonElement).click()

    await vi.waitFor(() => expect(messageMock.error).toHaveBeenCalled())
    expect(host.querySelector('[data-row="1"] button[data-role="NSwitch"]')?.getAttribute('data-value')).toBe('true')
  })
})

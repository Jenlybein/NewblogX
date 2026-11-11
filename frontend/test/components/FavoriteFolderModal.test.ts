import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'

import FavoriteFolderModal from '~/components/favorite/FavoriteFolderModal.vue'

/**
 * 收藏夹选择器（票 345）。
 *
 * 用户原话：选了收藏夹却看不到文章、按钮又已经变成"已收藏"。所以这一层钉的是
 * 选择器本身的可操作性，而不是大卡的样子：
 *   1. 每个夹子是**两行**（名称 + 篇数徽标 / 简介一行），一屏能看十几个；
 *   2. 右侧是**圆形 ＋/✓**：点一下收藏、再点一下移出；
 *   3. 篇数徽标写的是真实篇数（来自 has_article + article_count）；
 *   4. 页面上不再出现"共 N 篇文章"这种占一整行的文案。
 */

const messageMock = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }))
const favoriteApi = vi.hoisted(() => ({
  getOwnFavoriteFolders: vi.fn(),
  createFavoriteFolder: vi.fn(),
  removeFavoriteFolderArticles: vi.fn(),
}))
const articleApi = vi.hoisted(() => ({ favoriteArticle: vi.fn() }))

vi.mock('~/services/favorite', () => favoriteApi)
vi.mock('~/services/article', () => articleApi)
vi.mock('~/composables/usePointerSpecular', () => ({ usePointerSpecular: () => ({ handlePointerMove: vi.fn() }) }))

vi.mock('naive-ui', async () => {
  const passthrough = (name: string) =>
    defineComponent({ name, setup: (_, { slots }) => () => h('div', [slots.default?.()]) })
  return {
    NModal: defineComponent({
      name: 'NModal',
      props: { show: { type: Boolean, default: false } },
      setup(props, { slots }) {
        return () => (props.show ? h('div', slots.default?.()) : null)
      },
    }),
    NCard: passthrough('NCard'),
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
    NEmpty: defineComponent({ name: 'NEmpty', setup: () => () => h('div') }),
    NSkeleton: defineComponent({ name: 'NSkeleton', setup: () => () => h('div') }),
    useMessage: () => messageMock,
  }
})

function folder(overrides: Record<string, unknown> = {}) {
  return {
    id: 'f1',
    title: '默认收藏',
    abstract: '稍后阅读',
    article_count: 3,
    is_default: true,
    has_article: false,
    ...overrides,
  }
}

/**
 * 挂载并"打开"选择器。
 *
 * 先以 show=false 挂载再改成 true：组件在 show 变真时才拉列表（watch），
 * 直接以 true 挂载不会触发那条 watch —— 那样测到的是空态，不是真实路径。
 */
async function mountChooser(folders = [folder()]) {
  favoriteApi.getOwnFavoriteFolders.mockResolvedValue({ list: folders, count: folders.length })
  const wrapper = mount(FavoriteFolderModal, {
    props: { show: false, articleId: 'a1', articleTitle: '一篇文章' },
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  favoriteApi.removeFavoriteFolderArticles.mockResolvedValue({})
  articleApi.favoriteArticle.mockResolvedValue({ is_favor: true, favor_count: 1 })
})

describe('收藏夹选择器：两行紧凑行', () => {
  it('每个夹子两行：名称 + 篇数徽标在第一行，简介在第二行', async () => {
    const wrapper = await mountChooser()
    await flushPromises()

    const row = wrapper.get('[data-role="favorite-folder-row-f1"]')
    expect(row.classes()).toContain('favorite-chooser-row')
    expect(row.text()).toContain('默认收藏')
    // 篇数徽标在行内，而不是"共 N 篇文章"那种独立一行。
    expect(wrapper.get('[data-role="favorite-folder-count-f1"]').text()).toBe('3 篇')
    expect(wrapper.text()).not.toContain('共 3 篇文章')
    // 简介在第二行。
    expect(row.get('.favorite-chooser-row__abstract').text()).toContain('稍后阅读')
  })

  it('已收藏的行显示 ✓，未收藏显示 ＋', async () => {
    const wrapper = await mountChooser([
      folder({ id: 'f1', has_article: true }),
      folder({ id: 'f2', title: '另一个夹', is_default: false, has_article: false }),
    ])
    await flushPromises()

    expect(wrapper.get('[data-role="favorite-folder-toggle-f1"]').text()).toBe('✓')
    expect(wrapper.get('[data-role="favorite-folder-toggle-f2"]').text()).toBe('＋')
    // 选中态同时落在 aria-pressed 上（键盘与读屏都能判断）。
    expect(wrapper.get('[data-role="favorite-folder-toggle-f1"]').attributes('aria-pressed')).toBe('true')
  })
})

describe('收藏夹选择器：＋/✓ 切换', () => {
  it('点 ＋ 收藏到该夹，并把权威结果抛给页面', async () => {
    const wrapper = await mountChooser([folder({ id: 'f1' })])
    await flushPromises()

    await wrapper.get('[data-role="favorite-folder-toggle-f1"]').trigger('click')
    await flushPromises()

    expect(articleApi.favoriteArticle).toHaveBeenCalledWith('a1', 'f1')
    expect(wrapper.emitted('updated')).toBeTruthy()
  })

  it('再点 ✓ 从该夹移出（不是"加入"）', async () => {
    const wrapper = await mountChooser([folder({ id: 'f1', has_article: true })])
    await flushPromises()

    await wrapper.get('[data-role="favorite-folder-toggle-f1"]').trigger('click')
    await flushPromises()

    expect(favoriteApi.removeFavoriteFolderArticles).toHaveBeenCalledWith({
      favoriteId: 'f1',
      articleIds: ['a1'],
    })
    expect(articleApi.favoriteArticle).not.toHaveBeenCalled()
  })
})

import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { nextTick, ref } from 'vue'

import ChipSelect from '~/components/common/ChipSelect.vue'

/**
 * 多列 chip 下拉（票 352）。
 *
 * 只锁用户能看见 / 能做到的事：
 *   1. 选项以流式 chip 呈现（不是一行一个的竖列表）；
 *   2. 单选点一下即选中并收起；多选可反复点、已选项有选中态；
 *   3. 面板内搜索能过滤，且**不提供**"无匹配时新建"；
 *   4. 选项上不显示文章数（密度优先，产品决定）；
 *   5. 键盘：上下移动高亮、回车选中、ESC 关闭。
 */

const options = [
  { label: 'Vue3', value: 't1' },
  { label: '前端', value: 't2' },
  { label: '后端', value: 't3' },
  { label: 'Java', value: 't4' },
]

function mountSelect(initial: string | string[] | null = null, multiple = false) {
  const model = ref<string | string[] | null>(initial)
  const wrapper = mount(ChipSelect, {
    props: {
      options,
      multiple,
      modelValue: model.value,
      'onUpdate:modelValue': (value: string | string[] | null) => {
        model.value = value
        void wrapper.setProps({ modelValue: value })
      },
      dataRole: 'test',
    },
    // NPopover 默认 Teleport 到 body：存根掉才能在同一棵树里断言面板内容。
    global: { stubs: { teleport: true } },
  })
  return { wrapper, model }
}

async function openPanel(wrapper: ReturnType<typeof mount>) {
  await wrapper.get('[data-role="test-trigger"]').trigger('click')
  await flushPromises()
  await nextTick()
}

describe('chip 下拉：呈现', () => {
  it('空值时显示占位文案', () => {
    const { wrapper } = mountSelect()
    expect(wrapper.get('[data-role="test-trigger"]').text()).toContain('请选择')
  })

  it('选项以 chip 网格呈现，而不是一行一个', async () => {
    const { wrapper } = mountSelect()
    await openPanel(wrapper)

    const chips = wrapper.findAll('[data-role="test-option"]')
    expect(chips).toHaveLength(4)
    // 每个 chip 是按钮且带 chip 类：布局由 CSS 的 flex-wrap 决定多列。
    expect(chips[0]!.classes()).toContain('chip-select__chip')
    expect(wrapper.get('[data-role="test-panel"]').find('.chip-select__chips').exists()).toBe(true)
  })

  it('选项上不显示文章数（只放名称）', async () => {
    const { wrapper } = mountSelect()
    await openPanel(wrapper)

    const text = wrapper.get('[data-role="test-panel"]').text()
    expect(text).toContain('Vue3')
    expect(text).not.toMatch(/\d+\s*篇/)
  })
})

describe('chip 下拉：选择', () => {
  it('单选：点一下即选中并收起', async () => {
    const { wrapper, model } = mountSelect()
    await openPanel(wrapper)

    await wrapper.findAll('[data-role="test-option"]')[2]!.trigger('click')
    await flushPromises()

    expect(model.value).toBe('t3')
    // 收起：以触发器上的 aria-expanded 为准（teleport 存根下 DOM 仍在，只是隐藏）。
    expect(wrapper.get('[data-role="test-trigger"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('[data-role="test-trigger"]').text()).toContain('后端')
  })

  it('多选：反复点可增删，已选项带选中态', async () => {
    const { wrapper, model } = mountSelect([], true)
    await openPanel(wrapper)

    await wrapper.findAll('[data-role="test-option"]')[0]!.trigger('click')
    await flushPromises()
    await wrapper.findAll('[data-role="test-option"]')[1]!.trigger('click')
    await flushPromises()
    expect(model.value).toEqual(['t1', 't2'])

    // 再点一次取消第一个。
    const first = wrapper.findAll('[data-role="test-option"]')[0]!
    expect(first.attributes('aria-selected')).toBe('true')
    await first.trigger('click')
    await flushPromises()
    expect(model.value).toEqual(['t2'])
  })

  it('清空按钮把值清空', async () => {
    const { wrapper, model } = mountSelect('t1')
    await wrapper.get('.chip-select__clear').trigger('click')
    await flushPromises()
    expect(model.value).toBeNull()
  })
})

describe('chip 下拉：搜索', () => {
  it('输入即过滤，且无匹配时不出现"新建"入口', async () => {
    const { wrapper } = mountSelect()
    await openPanel(wrapper)

    await wrapper.get('[data-role="test-search"] input').setValue('jav')
    await flushPromises()

    const chips = wrapper.findAll('[data-role="test-option"]')
    expect(chips).toHaveLength(1)
    expect(chips[0]!.text()).toBe('Java')

    await wrapper.get('[data-role="test-search"] input').setValue('不存在')
    await flushPromises()
    const panelText = wrapper.get('[data-role="test-panel"]').text()
    expect(panelText).toContain('没有匹配项')
    // 产品决定：多列面板里不做"无匹配时新建"。
    expect(panelText).not.toContain('新建')
  })
})

describe('chip 下拉：键盘', () => {
  it('上下键移动高亮，回车选中', async () => {
    const { wrapper, model } = mountSelect()
    const trigger = wrapper.get('[data-role="test-trigger"]')

    // 第一次 ↓ 打开面板。
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await flushPromises()
    expect(wrapper.find('[data-role="test-panel"]').exists()).toBe(true)

    // 再 ↓ 两次到第三项，回车选中。
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await flushPromises()
    await trigger.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(model.value).toBe('t3')
  })

  it('ESC 关闭面板', async () => {
    const { wrapper } = mountSelect()
    const trigger = wrapper.get('[data-role="test-trigger"]')

    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await flushPromises()
    expect(wrapper.find('[data-role="test-panel"]').exists()).toBe(true)

    await trigger.trigger('keydown', { key: 'Escape' })
    await flushPromises()
    expect(trigger.attributes('aria-expanded')).toBe('false')
  })
})

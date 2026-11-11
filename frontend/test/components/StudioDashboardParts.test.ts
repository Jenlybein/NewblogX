import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import ActivityHeatmap from '~/components/studio/ActivityHeatmap.vue'
import MetricCard from '~/components/studio/MetricCard.vue'

/**
 * 个人数据概览的两个新件（票 354）。
 *
 * 热力图最容易做成"只有鼠标能看"，所以这里重点钉可访问性：
 *   - 13 列 × 7 行（13 周）；
 *   - 每格是按钮，可聚焦，`aria-label` 里有日期与次数；
 *   - 聚焦时提示显示在网格下方（不依赖 hover 浮层）。
 *
 * MetricCard 钉"零值折叠"与"未读到显示占位而不是 0"。
 */

const TODAY = new Date(2026, 10, 11) // 2026-11-11（本地时区）

function mountHeatmap(activity: Record<string, number> = {}) {
  return mount(ActivityHeatmap, { props: { activity, days: 91, today: TODAY } })
}

describe('热力图：网格形状', () => {
  it('13 列 × 7 行（13 周）', () => {
    const wrapper = mountHeatmap()

    const grid = wrapper.get('[data-role="activity-heatmap"] .activity-heatmap__grid')
    // 网格是 grid-auto-flow: column + 7 行，所以格子数是 7 的倍数、列数是 13。
    const cells = grid.findAll('[role="gridcell"]')
    expect(cells.length % 7).toBe(0)
    expect(cells.length / 7).toBeGreaterThanOrEqual(13)
    expect(cells.length / 7).toBeLessThanOrEqual(14)
  })

  it('每天一格，格子带日期与次数的可读标签', () => {
    const wrapper = mountHeatmap({ '2026-11-10': 3 })

    const cell = wrapper.get('[data-role="activity-cell-2026-11-10"]')
    expect(cell.attributes('aria-label')).toBe('2026-11-10：3 次')
    expect(cell.attributes('data-level')).toBe('2')
  })

  it('没有动作的日期按 0 处理，不是缺格', () => {
    const wrapper = mountHeatmap({ '2026-11-10': 3 })

    const empty = wrapper.get('[data-role="activity-cell-2026-11-09"]')
    expect(empty.attributes('data-level')).toBe('0')
    expect(empty.attributes('aria-label')).toBe('2026-11-09：0 次')
  })
})

describe('热力图：可访问性', () => {
  it('每格是按钮（可 Tab 聚焦），不是不可交互的 div', () => {
    const wrapper = mountHeatmap()
    const cells = wrapper.findAll('.activity-heatmap__cell')
    expect(cells[0]!.element.tagName).toBe('BUTTON')
  })

  it('聚焦某格时提示显示在网格下方，含日期与次数', async () => {
    const wrapper = mountHeatmap({ '2026-11-10': 5 })

    await wrapper.get('[data-role="activity-cell-2026-11-10"]').trigger('focus')
    expect(wrapper.get('[data-role="activity-tip"]').text()).toContain('2026-11-10')
    expect(wrapper.get('[data-role="activity-tip"]').text()).toContain('5 次')
  })

  it('汇总行给出总次数与有动作的天数', () => {
    const wrapper = mountHeatmap({ '2026-11-09': 2, '2026-11-10': 3 })
    expect(wrapper.get('[data-role="activity-summary"]').text()).toContain('5 次')
    expect(wrapper.get('[data-role="activity-summary"]').text()).toContain('2 天')
  })
})

describe('MetricCard', () => {
  it('未读到数值时显示占位而不是 0', () => {
    const wrapper = mount(MetricCard, { props: { label: '阅读', value: null } })
    expect(wrapper.text()).toContain('—')
    expect(wrapper.text()).not.toContain('0')
  })

  it('零值可以要求视觉降级（折叠用的钩子）', () => {
    const wrapper = mount(MetricCard, { props: { label: '待审核', value: 0, dimWhenZero: true } })
    expect(wrapper.classes()).toContain('metric-card--dim')
  })

  it('带单位后缀与说明', () => {
    const wrapper = mount(MetricCard, { props: { label: '已发布', value: 3, suffix: '篇', hint: '含待审核' } })
    expect(wrapper.text()).toContain('篇')
    expect(wrapper.text()).toContain('含待审核')
  })
})

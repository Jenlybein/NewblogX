import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, inject, provide } from 'vue'

import AdminDashboard from '~/pages/index.vue'

/**
 * 站点数据概览（票 349）。
 *
 * 钉的是两条用户能感知的行为：
 *   1. **时间窗独立**：切换主趋势图的窗口只重发趋势那一个请求，不整页刷新；
 *   2. **零值折叠**：AI 用量为 0 时只留一行说明，不再占三分之一屏。
 *
 * 图表组件（ECharts）与 MetricCard 都换成极简替身：这里测的是页面的编排，
 * 不是 canvas 里画了什么。
 */

const adminApi = vi.hoisted(() => ({
  getDashboardSum: vi.fn(),
  getDataOverview: vi.fn(),
  getGrowthData: vi.fn(),
  getArticleYearData: vi.fn(),
  getArticleReviewTasks: vi.fn(),
  getLogs: vi.fn(),
  searchAdminArticles: vi.fn(),
  getAIUsage: vi.fn(),
}))

const messageMock = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }))

vi.mock('~/services/admin', () => adminApi)
vi.mock('~/components/admin/TrendChart.vue', () => ({
  default: defineComponent({
    name: 'TrendChart',
    props: { points: { type: Array, default: () => [] }, label: { type: String, default: '' }, spark: Boolean },
    setup(props) {
      return () => h('div', { 'data-role': `chart-${props.label}`, 'data-points': props.points.length })
    },
  }),
}))
vi.mock('~/components/media/MetricCard.vue', () => ({
  default: defineComponent({
    name: 'MetricCard',
    props: { label: { type: String, required: true }, value: { type: Number, default: null }, suffix: { type: String, default: '' }, hint: { type: String, default: '' } },
    setup(props) {
      return () => h('div', { 'data-role': `metric-${props.label}` }, `${props.label}=${props.value}`)
    },
  }),
}))

vi.mock('naive-ui', async (importOriginal) => {
  const actual = await importOriginal<typeof import('naive-ui')>()
  const passthrough = (name: string) =>
    defineComponent({ name, setup: (_, { slots }) => () => h('div', [slots.header?.(), slots.default?.()]) })
  return {
    ...actual,
    NCard: passthrough('NCard'),
    NSpace: passthrough('NSpace'),
    NAlert: passthrough('NAlert'),
    // 单选组必须真的把"点了哪个"传上去：只渲染按钮的替身会让
    // "切换时间窗"这条用例永远观察不到请求变化（点了等于没点）。
    NRadioGroup: defineComponent({
      name: 'NRadioGroup',
      props: { value: { type: [String, Number], default: '' } },
      emits: ['update:value'],
      setup(props, { slots, emit }) {
        provide('radio-select', (v: string | number) => emit('update:value', v))
        return () => h('div', { 'data-value': String(props.value) }, slots.default?.())
      },
    }),
    NRadioButton: defineComponent({
      name: 'NRadioButton',
      props: { value: { type: [String, Number], required: true } },
      setup(props, { slots }) {
        const select = inject<(v: string | number) => void>('radio-select', () => {})
        return () => h('button', { 'data-value': String(props.value), onClick: () => select(props.value) }, slots.default?.())
      },
    }),
    useMessage: () => messageMock,
  }
})

function growth(points: { date: string; count: number }[], range = 'week') {
  return { growth_rate: 0, growth_num: 0, date_count_list: points, range }
}

beforeEach(() => {
  vi.clearAllMocks()
  adminApi.getDashboardSum.mockResolvedValue({
    flow_count: 128, user_count: 9, article_count: 12, message_count: 3,
    comment_count: 4, new_login_count: 1, new_sign_count: 2,
  })
  adminApi.getDataOverview.mockResolvedValue({
    range: 'week', range_start: '2026-11-09T00:00:00+08:00', range_end: '2026-11-12T00:00:00+08:00',
    visits: 42, new_users: 2, new_articles: 3,
  })
  adminApi.getGrowthData.mockResolvedValue(growth([{ date: '2026-11-10', count: 5 }, { date: '2026-11-11', count: 7 }]))
  adminApi.getArticleYearData.mockResolvedValue({ date_count_list: [{ date: '2026-11', count: 4 }] })
  adminApi.getArticleReviewTasks.mockResolvedValue({ list: [], count: 0 })
  adminApi.getLogs.mockResolvedValue({ list: [], count: 0 })
  adminApi.searchAdminArticles.mockResolvedValue({ list: [], count: 0, pagination: {} })
  adminApi.getAIUsage.mockResolvedValue({
    available: true, range: 'week', granularity: 'day',
    prompt_tokens: 0, completion_tokens: 0, total_tokens: 0, calls: 0, failures: 0, points: [],
  })

  vi.stubGlobal('useMessage', () => messageMock)
})

async function mountPage() {
  const wrapper = mount(AdminDashboard, { global: { stubs: { ClientOnly: { template: '<div><slot /></div>' } } } })
  await flushPromises()
  return wrapper
}

describe('站点概览：KPI 与趋势', () => {
  it('两个访问 KPI 都用同一口径的提示', async () => {
    const wrapper = await mountPage()

    expect(wrapper.get('[data-role="metric-累计访问"]').text()).toContain('128')
    expect(wrapper.get('[data-role="metric-近 7 日访问"]').text()).toContain('12')
  })

  it('切换时间窗只重发趋势请求，不整页刷新', async () => {
    const wrapper = await mountPage()
    const callsAfterMount = adminApi.getGrowthData.mock.calls.length

    // 只找**主趋势图**那组时间窗里的「今年」按钮：AI 卡片也有一组同名按钮，
    // 用全局 findAll 会点到 AI 那组，测的就不是趋势窗了。
    const trendCard = wrapper.get('[data-role="admin-trend"]')
    const yearButton = trendCard.findAll('button').find(b => b.text() === '近 12 个月')!
    await yearButton.trigger('click')
    await flushPromises()

    // 只多了趋势那一次请求；sum / overview / 年份分布都没重发。
    expect(adminApi.getGrowthData.mock.calls.length).toBe(callsAfterMount + 1)
    expect(adminApi.getDashboardSum).toHaveBeenCalledTimes(1)
    expect(adminApi.getDataOverview).toHaveBeenCalledTimes(1)
    // 新窗口是 year。
    expect(adminApi.getGrowthData.mock.calls.at(-1)![1]).toBe('year')
  })

  it('口径提示写清"算的是哪段时间"', async () => {
    const wrapper = await mountPage()
    expect(wrapper.text()).toContain('含今天在内往前 7 天')
  })
})

describe('站点概览：空站提示', () => {
  it('只读不到汇总时，不许说"站点还没有内容"', async () => {
    adminApi.getDashboardSum.mockRejectedValue(new Error('数据库读取失败'))

    const wrapper = await mountPage()

    // 读不到 ≠ 没有内容：这时应该提示"汇总读不到"，而不是断言站点是空的。
    expect(wrapper.find('[data-role="admin-empty-site"]').exists()).toBe(false)
    expect(wrapper.get('[data-role="admin-sum-failed"]').exists()).toBe(true)
  })

  it('确实读到、且用户与文章都是 0 时，才提示站点没有内容', async () => {
    adminApi.getDashboardSum.mockResolvedValue({
      flow_count: 0, user_count: 0, article_count: 0, message_count: 0,
      comment_count: 0, new_login_count: 0, new_sign_count: 0,
    })

    const wrapper = await mountPage()

    expect(wrapper.get('[data-role="admin-empty-site"]').exists()).toBe(true)
    expect(wrapper.find('[data-role="admin-sum-failed"]').exists()).toBe(false)
  })

  it('内容 Top 5 明确按阅读量、只看已发布（默认综合排序会给出假 Top 5）', async () => {
    await mountPage()

    expect(adminApi.searchAdminArticles).toHaveBeenCalledWith({ page: 1, limit: 5, status: 3, sort: 6 })
  })
})

describe('站点概览：零值折叠', () => {
  it('AI 用量为 0 时折叠成一行，不画图也不排指标卡', async () => {
    const wrapper = await mountPage()

    const card = wrapper.get('[data-role="admin-ai-usage"]')
    expect(card.find('[data-role="admin-ai-zero"]').exists()).toBe(true)
    expect(card.text()).toContain('没有 AI 调用')
    // 折叠后不该再出现 AI 的指标卡与图。
    expect(card.find('[data-role="metric-总 token"]').exists()).toBe(false)
    expect(card.find('[data-role="chart-AI token"]').exists()).toBe(false)
  })

  it('有调用时展开指标卡与图', async () => {
    adminApi.getAIUsage.mockResolvedValue({
      available: true, range: 'week', granularity: 'day',
      prompt_tokens: 10, completion_tokens: 20, total_tokens: 30, calls: 2, failures: 0,
      points: [{ bucket: '2026-11-11', label: '11/11', calls: 2, prompt_tokens: 10, completion_tokens: 20, total_tokens: 30, failures: 0 }],
    })

    const wrapper = await mountPage()
    const card = wrapper.get('[data-role="admin-ai-usage"]')
    expect(card.find('[data-role="admin-ai-zero"]').exists()).toBe(false)
    expect(card.find('[data-role="metric-总 token"]').exists()).toBe(true)
  })

  it('日志库未启用时说明原因，不显示一排 0', async () => {
    adminApi.getAIUsage.mockResolvedValue({
      available: false, range: 'week', granularity: 'day',
      prompt_tokens: 0, completion_tokens: 0, total_tokens: 0, calls: 0, failures: 0, points: [],
    })

    const wrapper = await mountPage()
    expect(wrapper.get('[data-role="admin-ai-usage"]').text()).toContain('日志库未启用')
  })
})

describe('站点概览：会话失效', () => {
  it('汇总被判定为登录失效时，提示并跳登录，而不是把一排 0 当结论', async () => {
    const { ApiBusinessError } = await import('~/services/http/errors')
    adminApi.getDashboardSum.mockRejectedValue(new ApiBusinessError('请先登录', 1003))

    const navigateTo = vi.fn()
    vi.stubGlobal('navigateTo', navigateTo)

    await mountPage()

    expect(messageMock.error).toHaveBeenCalledWith('登录状态已过期，请重新登录')
    expect(navigateTo).toHaveBeenCalledWith({ path: '/login', query: { redirect: '/' } })
  })
})

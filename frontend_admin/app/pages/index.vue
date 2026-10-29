<script setup lang="ts">
import { NCard, NGrid, NGridItem, NList, NListItem, NRadioButton, NRadioGroup, NSpace, NTag, NThing, useMessage } from 'naive-ui'
import { getAIUsage, getArticleReviewTasks, getArticleYearData, getDashboardSum, getGrowthData } from '~/services/admin'
import type { AIUsageResponseData, ArticleReviewTaskItem, DateCountItem, SumResponseData } from '~/types/api'
import { formatDateTime, formatNumber } from '~/utils/format'
import { shortTokens } from '~/utils/aiTokens'
// 显式 import：components/admin/ 下的组件自动注册名是 AdminTokenChart，
// 模板里写 <TokenChart /> 会静默不渲染（这坑线上踩过一次）。
import AdminTokenChart from '~/components/admin/TokenChart.vue'

type GrowthType = 1 | 2 | 3
type AIUsageRange = 'today' | 'week' | 'month' | 'year'

const message = useMessage()
const loading = ref(false)
const sum = ref<SumResponseData>({
  flow_count: 0,
  user_count: 0,
  article_count: 0,
  message_count: 0,
  comment_count: 0,
  new_login_count: 0,
  new_sign_count: 0,
})
const growthSeries = ref<Array<{ type: GrowthType; title: string; note: string; data: DateCountItem[] }>>([
  { type: 1, title: '网站流量趋势', note: '近 7 天访问流量', data: [] },
  { type: 2, title: '文章发布趋势', note: '近 7 天发布文章', data: [] },
  { type: 3, title: '用户注册趋势', note: '近 7 天新增用户', data: [] },
])
const yearArticles = ref<DateCountItem[]>([])
const pendingReviews = ref<ArticleReviewTaskItem[]>([])

// AI 用量：四个周期共用一套响应形状，只换 range。
// available=false 表示日志库没启用 —— 那时要显示空态而不是一条零线，
// 否则会被读成"今天完全没用 AI"。
const aiRange = ref<AIUsageRange>('week')
const aiUsage = ref<AIUsageResponseData>({
  available: false,
  range: 'week',
  granularity: 'day',
  prompt_tokens: 0,
  completion_tokens: 0,
  total_tokens: 0,
  calls: 0,
  failures: 0,
  points: [],
})

const AI_RANGE_NOTES: Record<AIUsageRange, string> = {
  today: '今天 0 点起，按小时统计',
  week: '本周一起，按天统计',
  month: '本月 1 日起，按天统计',
  year: '今年 1 月起，按月统计',
}

const aiMetrics = computed(() => [
  { label: '总 token', value: shortTokens(aiUsage.value.total_tokens) },
  { label: '提示 token', value: shortTokens(aiUsage.value.prompt_tokens) },
  { label: '补全 token', value: shortTokens(aiUsage.value.completion_tokens) },
  { label: '调用次数', value: formatNumber(aiUsage.value.calls) },
  { label: '失败次数', value: formatNumber(aiUsage.value.failures) },
])

const metrics = computed(() => [
  { label: '总流量', value: sum.value.flow_count, note: 'flow_count' },
  { label: '用户总数', value: sum.value.user_count, note: `新增登录 ${sum.value.new_login_count}` },
  { label: '文章总数', value: sum.value.article_count, note: `评论 ${sum.value.comment_count}` },
  { label: '消息总量', value: sum.value.message_count, note: `新增注册 ${sum.value.new_sign_count}` },
])

function maxCount(items: DateCountItem[]) {
  return Math.max(1, ...items.map(item => item.count))
}

function hasPositiveCount(items: DateCountItem[]) {
  return items.some(item => item.count > 0)
}

function getBarHeight(item: DateCountItem, items: DateCountItem[]) {
  if (item.count <= 0) return '2px'
  return `${Math.max(8, (item.count / maxCount(items)) * 100)}%`
}

function formatShortDate(date: string) {
  const [, , month, day] = date.match(/^(\d{4})-(\d{2})-(\d{2})$/) ?? []
  if (month && day) return `${month}/${day}`
  return date
}

function setGrowthData(series: Array<{ type: GrowthType; title: string; note: string; data: DateCountItem[] }>, type: GrowthType, data: DateCountItem[]) {
  const target = series.find(item => item.type === type)
  if (target) target.data = data
}

async function refresh() {
  loading.value = true
  const [sumResult, flowGrowthResult, articleGrowthResult, userGrowthResult, articleYearResult, reviewResult, aiUsageResult] = await Promise.allSettled([
    getDashboardSum(),
    getGrowthData(1),
    getGrowthData(2),
    getGrowthData(3),
    getArticleYearData(),
    getArticleReviewTasks({ page: 1, limit: 5, status: 'pending' }),
    getAIUsage(aiRange.value),
  ])

  if (sumResult.status === 'fulfilled') sum.value = sumResult.value
  const nextGrowthSeries = growthSeries.value.map(series => ({ ...series }))
  if (flowGrowthResult.status === 'fulfilled') setGrowthData(nextGrowthSeries, 1, flowGrowthResult.value.date_count_list)
  if (articleGrowthResult.status === 'fulfilled') setGrowthData(nextGrowthSeries, 2, articleGrowthResult.value.date_count_list)
  if (userGrowthResult.status === 'fulfilled') setGrowthData(nextGrowthSeries, 3, userGrowthResult.value.date_count_list)
  growthSeries.value = nextGrowthSeries
  if (articleYearResult.status === 'fulfilled') yearArticles.value = articleYearResult.value.date_count_list
  if (reviewResult.status === 'fulfilled') {
    pendingReviews.value = reviewResult.value.list.slice(0, 5)
  }
  if (aiUsageResult.status === 'fulfilled') aiUsage.value = aiUsageResult.value

  if ([sumResult, flowGrowthResult, articleGrowthResult, userGrowthResult, articleYearResult, reviewResult].some(result => result.status === 'rejected')) {
    message.warning('部分数据暂时无法加载')
  }
  loading.value = false
}

/**
 * 单独加载 AI 用量。
 *
 * 切换周期时只重发这一个请求，不整页 refresh：其它卡片与周期无关，
 * 重新拉一遍只会让整个页面闪一下。
 */
async function loadAIUsage() {
  try {
    aiUsage.value = await getAIUsage(aiRange.value)
  } catch {
    message.error('AI 用量加载失败')
  }
}

/** 切换周期：NRadioGroup 的 update:value 是 string，这里收窄成联合类型再拉数据。 */
function onAIUsageRangeChange(value: string) {
  aiRange.value = value as AIUsageRange
  loadAIUsage()
}


onMounted(refresh)
</script>

<template>
  <NSpace vertical :size="20">
    <NGrid :cols="24" :x-gap="16" :y-gap="16" responsive="screen">
      <NGridItem v-for="metric in metrics" :key="metric.label" :span="6">
        <div class="admin-stat-card">
          <p class="muted m-0 text-sm">{{ metric.label }}</p>
          <div class="admin-stat-value">{{ formatNumber(metric.value) }}</div>
          <p class="muted m-0 mt-3 text-xs">{{ metric.note }}</p>
        </div>
      </NGridItem>
    </NGrid>

    <NCard class="admin-card" title="AI token 消耗">
      <template #header-extra>
        <NRadioGroup :value="aiRange" size="small" @update:value="onAIUsageRangeChange">
          <NRadioButton value="today">今日</NRadioButton>
          <NRadioButton value="week">本周</NRadioButton>
          <NRadioButton value="month">本月</NRadioButton>
          <NRadioButton value="year">本年</NRadioButton>
        </NRadioGroup>
      </template>

      <p class="muted m-0 mb-3 text-xs">
        {{ AI_RANGE_NOTES[aiRange] }}<template v-if="aiUsage.available && aiUsage.calls > aiUsage.failures"> · 每次请求计一次调用，长文评分会分块多次调用</template>
      </p>

      <div v-if="!aiUsage.available" class="admin-empty">日志库未启用，暂无 AI 用量数据</div>
      <div v-else>
        <div class="mb-4 flex flex-wrap gap-8">
          <div v-for="item in aiMetrics" :key="item.label">
            <p class="muted m-0 text-xs">{{ item.label }}</p>
            <p class="m-0 text-xl font-semibold tabular-nums">{{ item.value }}</p>
          </div>
        </div>
        <AdminTokenChart :points="aiUsage.points" />
      </div>
    </NCard>

    <NGrid :cols="24" :x-gap="20" :y-gap="20" responsive="screen">
      <NGridItem v-for="series in growthSeries" :key="series.type" :span="8">
        <NCard class="admin-card" :title="series.title">
          <p class="muted m-0 mb-3 text-xs">{{ series.note }}</p>
          <div v-if="series.data.length" class="admin-chart" :class="{ 'opacity-60': loading }">
            <div v-for="item in series.data" :key="`${series.type}-${item.date}`" class="admin-chart__item" :title="`${item.date}: ${item.count}`">
              <div class="admin-chart__bar-track">
                <div class="admin-chart__bar" :style="{ height: getBarHeight(item, series.data) }" />
              </div>
              <span class="admin-chart__count">{{ item.count }}</span>
              <span class="admin-chart__date">{{ formatShortDate(item.date) }}</span>
            </div>
          </div>
          <div v-else class="admin-empty">暂无趋势数据</div>
          <p v-if="series.data.length && !hasPositiveCount(series.data)" class="admin-chart__empty-note">近 7 天暂无新增数据</p>
        </NCard>
      </NGridItem>
    </NGrid>

    <NGrid :cols="24" :x-gap="20" :y-gap="20" responsive="screen">
      <NGridItem :span="16">
        <NCard class="admin-card" title="年度文章分布">
          <div v-if="yearArticles.length" class="admin-chart admin-chart--year">
            <div v-for="item in yearArticles" :key="item.date" class="admin-chart__item" :title="`${item.date}: ${item.count}`">
              <div class="admin-chart__bar-track">
                <div class="admin-chart__bar" :style="{ height: getBarHeight(item, yearArticles) }" />
              </div>
              <span class="admin-chart__count">{{ item.count }}</span>
              <span class="admin-chart__date">{{ item.date }}</span>
            </div>
          </div>
          <div v-else class="admin-empty">暂无文章分布数据</div>
        </NCard>
      </NGridItem>

      <NGridItem :span="8">
        <NCard class="admin-card" title="接口接入概览">
          <NList>
            <NListItem><NThing title="数据统计" description="/api/data/sum / growth / article-year" /></NListItem>
            <NListItem><NThing title="审核队列" description="/api/article-review" /></NListItem>
            <NListItem><NThing title="管理动作" description="用户、站点、媒体、日志按管理员接口接入" /></NListItem>
          </NList>
        </NCard>
      </NGridItem>
    </NGrid>

    <NGrid :cols="24" :x-gap="20" :y-gap="20" responsive="screen">
      <NGridItem :span="24">
        <NCard class="admin-card" title="待审核文章">
          <NList v-if="pendingReviews.length">
            <NListItem v-for="task in pendingReviews" :key="task.id">
              <NThing :title="task.article_title" :description="`${task.author_name} · ${formatDateTime(task.created_at)}`" />
              <template #suffix>
                <NuxtLink to="/review"><NTag type="warning">去审核</NTag></NuxtLink>
              </template>
            </NListItem>
          </NList>
          <div v-else class="admin-empty">暂无待审核任务</div>
        </NCard>
      </NGridItem>
    </NGrid>
  </NSpace>
</template>

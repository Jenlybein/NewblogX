<script setup lang="ts">
/**
 * 站点数据概览（票 349 重排为纯 BI 板）。
 *
 * 这一页只回答"站点现在怎么样"。七张卡各管一个问题，**各自带自己的时间窗**：
 * 共用一个全局 range 会让"我只想把访问量放大到今年"变成整页重排。
 *
 * 三条版式决定：
 *  1. 没有页头状态条、没有快捷入口 —— 它们是导航，不是数据；
 *  2. 一个可切换的主趋势图 + 三条火花线（访问 / 发文 / 注册），不再是一排等重的
 *     手写柱状图；
 *  3. **零值折叠**：AI 用量为 0 时只留一行「近 7 天没有 AI 调用」，不再占三分之一屏。
 *
 * 所有时间边界由后端按东八区切；口径提示直接写"算的是哪段时间"，
 * 用户不必自己再推一遍。
 */
import { computed, onMounted, ref } from 'vue'
import { NAlert, NCard, NRadioButton, NRadioGroup, NSpace, NTag, useMessage } from 'naive-ui'
import MetricCard from '~/components/media/MetricCard.vue'
import {
  getAIUsage,
  getArticleReviewTasks,
  getArticleYearData,
  getDashboardSum,
  getDataOverview,
  getGrowthData,
  getLogs,
  searchAdminArticles,
  type DataRange,
} from '~/services/admin'
import type { ActionAuditRecord, AIUsageResponseData, ArticleReviewTaskItem, DateCountItem, GrowthDataResponseData, OverviewData, SumResponseData } from '~/types/api'
import { formatDateTime, formatNumber } from '~/utils/format'
import { shortTokens } from '~/utils/aiTokens'

// 显式 import：components/admin/ 下的组件自动注册名是 AdminTrendChart，
// 模板里写 <TrendChart /> 会静默不渲染（这坑线上踩过一次）。
import AdminTrendChart from '~/components/admin/TrendChart.vue'

const message = useMessage()
const loading = ref(false)

/** 四个时间窗的中文名与口径提示（每个数字都要能说清算的是哪段）。 */
const RANGE_OPTIONS: { value: DataRange; label: string; note: string }[] = [
  { value: 'today', label: '今日', note: '今天 0 点（东八区）到现在' },
  { value: 'week', label: '本周', note: '本周一 0 点到现在' },
  { value: 'month', label: '本月', note: '本月 1 日 0 点到现在' },
  { value: 'year', label: '今年', note: '今年 1 月 1 日 0 点到现在' },
]

function noteOf(range: DataRange) {
  return RANGE_OPTIONS.find(item => item.value === range)?.note ?? ''
}

const sum = ref<SumResponseData>({
  range: 'today',
  flow_count: 0,
  user_count: 0,
  article_count: 0,
  message_count: 0,
  comment_count: 0,
  new_login_count: 0,
  new_sign_count: 0,
})

/** 主趋势图的时间窗（可切换）与它当前显示的类型。 */
const trendRange = ref<DataRange>('week')
const trendType = ref<1 | 2 | 3>(1)
const trend = ref<GrowthDataResponseData>({ growth_rate: 0, growth_num: 0, date_count_list: [], range: 'week' })

const TREND_TABS: { value: 1 | 2 | 3; label: string }[] = [
  { value: 1, label: '访问量' },
  { value: 2, label: '发文量' },
  { value: 3, label: '注册量' },
]

/** 三条火花线：各自固定看最近 7 天，用来给主图一个背景。 */
const sparkFlow = ref<DateCountItem[]>([])
const sparkArticle = ref<DateCountItem[]>([])
const sparkUser = ref<DateCountItem[]>([])

const overview = ref<OverviewData>({ range: 'week', range_start: '', range_end: '', visits: 0, visits_days: 0, new_users: 0, new_articles: 0 })
const yearArticles = ref<DateCountItem[]>([])
const pendingReviews = ref<ArticleReviewTaskItem[]>([])
const recentActions = ref<ActionAuditRecord[]>([])
const topArticles = ref<{ id: string; title: string; view_count: number }[]>([])

const aiRange = ref<DataRange>('week')
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

/** AI 用量折叠：没调用就是"没有"，不该占三分之一屏。 */
const aiIsZero = computed(() => aiUsage.value.available && aiUsage.value.calls === 0)

/**
 * AI 用量点转成图表要的 `{date, count}`。
 *
 * 用 token 总数而不是调用次数：这张图回答的是"花了多少"，调用次数已经在上面的
 * 指标卡里；两张图看同一个数字是浪费。
 */
const aiPoints = computed<DateCountItem[]>(() =>
  (aiUsage.value.points ?? []).map(point => ({ date: point.label, count: point.total_tokens })))

async function loadTrend() {
  try {
    trend.value = await getGrowthData(trendType.value, trendRange.value)
  } catch {
    message.error('趋势数据加载失败')
  }
}

async function loadAIUsage() {
  try {
    aiUsage.value = await getAIUsage(aiRange.value)
  } catch {
    message.error('AI 用量加载失败')
  }
}

async function refresh() {
  loading.value = true
  const [
    sumResult, overviewResult,
    flowSpark, articleSpark, userSpark,
    yearResult, reviewResult, actionResult, topResult,
    trendResult, aiResult,
  ] = await Promise.allSettled([
    getDashboardSum('today'),
    getDataOverview('week'),
    getGrowthData(1, 'week'),
    getGrowthData(2, 'week'),
    getGrowthData(3, 'week'),
    getArticleYearData(),
    getArticleReviewTasks({ page: 1, limit: 5, status: 'pending' }),
    getLogs('action', { page: 1, limit: 5 }),
    searchAdminArticles({ page: 1, limit: 5 }),
    getGrowthData(trendType.value, trendRange.value),
    getAIUsage(aiRange.value),
  ])

  if (sumResult.status === 'fulfilled') sum.value = sumResult.value
  if (overviewResult.status === 'fulfilled') overview.value = overviewResult.value
  if (flowSpark.status === 'fulfilled') sparkFlow.value = flowSpark.value.date_count_list
  if (articleSpark.status === 'fulfilled') sparkArticle.value = articleSpark.value.date_count_list
  if (userSpark.status === 'fulfilled') sparkUser.value = userSpark.value.date_count_list
  if (yearResult.status === 'fulfilled') yearArticles.value = yearResult.value.date_count_list
  if (reviewResult.status === 'fulfilled') pendingReviews.value = reviewResult.value.list.slice(0, 5)
  if (actionResult.status === 'fulfilled') recentActions.value = actionResult.value.list.slice(0, 5)
  if (topResult.status === 'fulfilled') {
    topArticles.value = (topResult.value.list ?? [])
      .map(item => ({ id: String(item.id), title: item.title ?? '', view_count: item.view_count ?? 0 }))
      .sort((a, b) => b.view_count - a.view_count)
      .slice(0, 5)
  }
  if (trendResult.status === 'fulfilled') trend.value = trendResult.value
  if (aiResult.status === 'fulfilled') aiUsage.value = aiResult.value

  // 任何一块失败都要说出来：静默显示一排 0 与"确实没有数据"完全不可区分。
  if ([sumResult, overviewResult, yearResult, reviewResult, actionResult, topResult, trendResult, aiResult].some(r => r.status === 'rejected')) {
    message.warning('部分数据暂时无法加载')
  }
  loading.value = false
}

/** 切换主图时间窗：只重发这一个请求，不整页刷新。 */
async function onTrendRangeChange(value: DataRange) {
  trendRange.value = value
  await loadTrend()
}

async function onTrendTypeChange(value: 1 | 2 | 3) {
  trendType.value = value
  await loadTrend()
}

async function onAIUsageRangeChange(value: DataRange) {
  aiRange.value = value
  await loadAIUsage()
}

onMounted(refresh)
</script>

<template>
  <NSpace vertical :size="16">
    <!-- ① 两个访问 KPI：同一口径（东八区自然日 + IP/UA 去重 + 过滤爬虫） -->
    <section class="metric-strip" data-role="admin-metrics">
      <MetricCard label="累计访问" :value="sum.flow_count" suffix="次" hint="同一 IP + 同一浏览器每天只算一次，已过滤爬虫" />
      <MetricCard label="近 7 日访问" :value="sparkFlow.reduce((n, p) => n + p.count, 0)" suffix="次" hint="最近 7 个东八区自然日" />
      <MetricCard label="用户总数" :value="sum.user_count" suffix="人" :hint="`今日新增注册 ${sum.new_sign_count}`" />
      <MetricCard label="文章总数" :value="sum.article_count" suffix="篇" :hint="`评论 ${sum.comment_count}`" />
    </section>

    <!-- ② 主趋势图：可切时间窗与指标 -->
    <NCard class="admin-card" data-role="admin-trend">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="admin-page__title">趋势</h2>
            <p class="admin-hint m-0">
              {{ noteOf(trendRange) }}<template v-if="trendRange === 'month' || trendRange === 'year'"> · 访问量只保留最近 30 天，更早的日期按 0 显示</template>
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <NRadioGroup :value="trendType" size="small" @update:value="(v: number) => onTrendTypeChange(v as 1 | 2 | 3)">
              <NRadioButton v-for="tab in TREND_TABS" :key="tab.value" :value="tab.value">{{ tab.label }}</NRadioButton>
            </NRadioGroup>
            <NRadioGroup :value="trendRange" size="small" @update:value="(v: string) => onTrendRangeChange(v as DataRange)">
              <NRadioButton v-for="opt in RANGE_OPTIONS" :key="opt.value" :value="opt.value">{{ opt.label }}</NRadioButton>
            </NRadioGroup>
          </div>
        </div>
      </template>

      <ClientOnly>
        <AdminTrendChart :points="trend.date_count_list" label="趋势" :height="240" />
        <template #fallback>
          <div class="admin-empty">图表加载中…</div>
        </template>
      </ClientOnly>
    </NCard>

    <!-- ③ 三条火花线 -->
    <div class="grid gap-4 md:grid-cols-3">
      <NCard v-for="spark in [
        { key: 'flow', label: '近 7 日访问', points: sparkFlow },
        { key: 'article', label: '近 7 日发文', points: sparkArticle },
        { key: 'user', label: '近 7 日注册', points: sparkUser },
      ]" :key="spark.key" class="admin-card" size="small">
        <p class="admin-hint m-0">{{ spark.label }}</p>
        <ClientOnly>
          <AdminTrendChart :points="spark.points" :label="spark.label" :height="64" spark />
          <template #fallback><div class="admin-empty" /></template>
        </ClientOnly>
      </NCard>
    </div>

    <div class="grid gap-4 lg:grid-cols-2">
      <!-- ④ 内容 Top 5 -->
      <NCard class="admin-card" title="内容 Top 5">
        <p class="admin-hint">按阅读量排序的已发布文章。</p>
        <ol v-if="topArticles.length" class="mt-2 space-y-2 text-sm" data-role="admin-top-articles">
          <li v-for="(item, index) in topArticles" :key="item.id" class="flex items-center justify-between gap-3">
            <NuxtLink :to="`/articles?id=${item.id}`" class="min-w-0 truncate hover:underline">
              <span class="mr-2 text-xs muted">{{ index + 1 }}</span>{{ item.title }}
            </NuxtLink>
            <span class="shrink-0 text-xs muted">{{ formatNumber(item.view_count) }} 阅读</span>
          </li>
        </ol>
        <div v-else class="admin-empty">还没有已发布文章</div>
      </NCard>

      <!-- ⑤ 最近动态 5 行 -->
      <NCard class="admin-card" title="最近管理动作">
        <ul v-if="recentActions.length" class="mt-2 space-y-2 text-sm" data-role="admin-recent-actions">
          <li v-for="item in recentActions" :key="item.event_id" class="flex items-center justify-between gap-3">
            <span class="min-w-0 truncate">{{ item.action_name || item.path }}</span>
            <span class="shrink-0 text-xs muted">{{ formatDateTime(item.ts) }}</span>
          </li>
        </ul>
        <div v-else class="admin-empty">暂无管理动作</div>
      </NCard>

      <!-- ⑥ 年度分布 -->
      <NCard class="admin-card" title="近 12 个月发文">
        <ClientOnly>
          <AdminTrendChart :points="yearArticles" label="近 12 个月发文" :height="180" />
          <template #fallback><div class="admin-empty" /></template>
        </ClientOnly>
      </NCard>

      <!-- ⑦ 系统健康：一张卡，不铺页头状态条 -->
      <NCard class="admin-card" title="系统健康">
        <div class="space-y-2 text-sm" data-role="admin-health">
          <div class="flex items-center justify-between">
            <span class="muted">数据读取</span>
            <NTag :type="loading ? 'default' : 'success'" size="small">{{ loading ? '读取中' : '正常' }}</NTag>
          </div>
          <div class="flex items-center justify-between">
            <span class="muted">待审核稿件</span>
            <NuxtLink to="/review" class="hover:underline">{{ pendingReviews.length }} 篇</NuxtLink>
          </div>
          <div class="flex items-center justify-between">
            <span class="muted">本周新增文章</span>
            <span>{{ overview.new_articles }} 篇</span>
          </div>
          <div class="flex items-center justify-between">
            <span class="muted">本周新增注册</span>
            <span>{{ overview.new_users }} 人</span>
          </div>
          <div class="flex items-center justify-between">
            <!-- 天数如实标出：访问量只保留最近 30 天，不能把 30 天的数字说成一年的。 -->
            <span class="muted">近 {{ overview.visits_days }} 天访问</span>
            <span>{{ overview.visits }} 次</span>
          </div>
        </div>
      </NCard>
    </div>

    <!-- ⑧ AI 用量：零值时折叠成一行 -->
    <NCard class="admin-card" data-role="admin-ai-usage">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h2 class="admin-page__title">AI 用量</h2>
          <NRadioGroup :value="aiRange" size="small" @update:value="(v: string) => onAIUsageRangeChange(v as DataRange)">
            <NRadioButton v-for="opt in RANGE_OPTIONS" :key="opt.value" :value="opt.value">{{ opt.label }}</NRadioButton>
          </NRadioGroup>
        </div>
      </template>

      <p v-if="!aiUsage.available" class="admin-hint" data-role="admin-ai-unavailable">
        日志库未启用，暂无 AI 用量数据。
      </p>
      <p v-else-if="aiIsZero" class="admin-hint" data-role="admin-ai-zero">
        {{ noteOf(aiRange) }}没有 AI 调用。
      </p>
      <div v-else>
        <div class="metric-strip mb-3">
          <MetricCard label="总 token" :value="aiUsage.total_tokens" :hint="`提示 ${shortTokens(aiUsage.prompt_tokens)} · 补全 ${shortTokens(aiUsage.completion_tokens)}`" />
          <MetricCard label="调用次数" :value="aiUsage.calls" hint="每次请求计一次，长文评分会分块多次调用" />
          <MetricCard label="失败次数" :value="aiUsage.failures" />
        </div>
        <ClientOnly>
          <AdminTrendChart :points="aiPoints" label="AI token" :height="160" />
          <template #fallback><div class="admin-empty" /></template>
        </ClientOnly>
      </div>
    </NCard>

    <NAlert v-if="!loading && !sum.user_count && !sum.article_count" type="info" :bordered="false" data-role="admin-empty-site">
      站点还没有内容。先去发布第一篇文章，这里的数字才会有意义。
    </NAlert>
  </NSpace>
</template>

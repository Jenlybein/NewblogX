<script setup lang="ts">
/**
 * 站点数据概览（票 349 重排为纯 BI 板；2026-11-12 二次重排 + 修"站内还没有文章"）。
 *
 * 这一页只回答"站点现在怎么样"。每张卡各管一个问题，**各自带自己的时间窗**：
 * 共用一个全局 range 会让"我只想把访问量放大到今年"变成整页重排。
 *
 * 上一版的三个问题（本次修掉）：
 *  1. 顶部没有一句人话 —— 一进来就是四个数字卡。现在先给一行"这周怎么样"；
 *  2. 指标卡**完全没有样式**：`.metric-card` 只写在读者站的样式表里，后台没有，
 *     于是四个数字是裸的内联文字。本次在 admin.css 补上这一组（两端同名同形）；
 *  3. 页面底下那句"站点还没有内容"是拿 `sum.user_count / article_count` 当
 *     "站点是否为空"的判据，而这两个请求失败时也是 0 —— **读不到 ≠ 没有内容**。
 *     现在只有"确实读到了、且两个数都是 0"才说空站；请求失败另起一条提示。
 *
 * 所有时间边界由后端按东八区切；口径提示直接写"算的是哪段时间"。
 */
import { computed, onMounted, ref } from 'vue'
import { NAlert, NCard, NRadioButton, NRadioGroup, NSpace, NTag, useMessage } from 'naive-ui'
import MetricCard from '~/components/media/MetricCard.vue'
import { isAuthLikeError } from '~/services/http/errors'
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

/** 两个"是否读到"的开关：空站提示必须建立在这上面，而不是把 0 当空。 */
const sumLoaded = ref(false)
const sumFailed = ref(false)

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

function sumOf(points: DateCountItem[]) {
  return points.reduce((total, point) => total + point.count, 0)
}

const weekVisits = computed(() => sumOf(sparkFlow.value))
const weekArticles = computed(() => sumOf(sparkArticle.value))
const weekSignups = computed(() => sumOf(sparkUser.value))
const yearTotal = computed(() => sumOf(yearArticles.value))

/** 一句话说清"这周怎么样"：数字先给结论，口径放在旁边的小字里。 */
const headline = computed(() => {
  if (loading.value) return '正在汇总站点数据…'
  return `最近 7 天，站点有 ${formatNumber(weekVisits.value)} 次访问、${weekArticles.value} 篇新文章、${weekSignups.value} 位新注册。`
})

/** 阅读 Top 5 的条形：按最高阅读量归一，条形让"第一与第五差多少"一眼看得出来。 */
const topMax = computed(() => Math.max(...topArticles.value.map(item => item.view_count), 1))

function topBarWidth(viewCount: number) {
  return `${Math.max(4, Math.round((viewCount / topMax.value) * 100))}%`
}

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
    // Top 5 必须按阅读量排序、只看已发布：默认综合排序在无关键词时并不按阅读量排，
    // 之前拿回来的是"任意 5 篇"，标题写着 Top 5 其实是假的。
    searchAdminArticles({ page: 1, limit: 5, status: 3, sort: 6 }),
    getGrowthData(trendType.value, trendRange.value),
    getAIUsage(aiRange.value),
  ])

  if (sumResult.status === 'fulfilled') {
    sum.value = sumResult.value
    sumLoaded.value = true
    sumFailed.value = false
  } else {
    sumFailed.value = true
  }

  // 会话真的没了（refresh_token 也没了）时，这一页只会得到一排 0 和三条失败提示，
  // 看起来像"站点数据全丢了"。这种情况必须说清楚并送回登录页 —— 0 不是结论。
  if (sumResult.status === 'rejected' && isAuthLikeError(sumResult.reason)) {
    loading.value = false
    message.error('登录状态已过期，请重新登录')
    await navigateTo({ path: '/login', query: { redirect: '/' } })
    return
  }
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
    <div class="admin-overview">
      <!-- ① 报头：一句话结论 + 刷新 -->
      <section class="admin-overview__masthead" data-role="admin-masthead">
        <div class="min-w-0">
          <p class="eyebrow">Site Pulse · 近 7 天</p>
          <p class="admin-overview__headline">{{ headline }}</p>
          <p class="admin-hint">
            访问量按东八区自然日 + IP/UA 去重，已过滤爬虫；文章与用户为累计值，不随下面的时间窗变化。
          </p>
        </div>
        <NTag :type="loading ? 'default' : sumFailed ? 'warning' : 'success'" size="small" round>
          {{ loading ? '读取中' : sumFailed ? '汇总读取失败' : '数据已同步' }}
        </NTag>
      </section>

      <!-- ② 四个累计 KPI -->
      <section class="metric-strip" data-role="admin-metrics">
        <MetricCard label="累计访问" :value="sum.flow_count" suffix="次" hint="同一 IP + 同一浏览器每天只算一次，已过滤爬虫" />
        <MetricCard label="近 7 日访问" :value="weekVisits" suffix="次" hint="最近 7 个东八区自然日" />
        <MetricCard label="用户总数" :value="sum.user_count" suffix="人" :hint="`今日新增注册 ${sum.new_sign_count}`" />
        <MetricCard label="文章总数" :value="sum.article_count" suffix="篇" :hint="`评论 ${sum.comment_count}`" />
      </section>

      <!-- ③ 主趋势图：可切时间窗与指标 -->
      <NCard class="admin-card admin-overview__trend" data-role="admin-trend">
        <template #header>
          <div class="admin-overview__card-head">
            <div>
              <h2 class="admin-overview__card-title">趋势</h2>
              <p class="admin-hint m-0">
                {{ noteOf(trendRange) }}<template v-if="trendRange === 'month' || trendRange === 'year'"> · 访问量只保留最近 30 天，更早的日期按 0 显示</template>
              </p>
            </div>
            <div class="admin-overview__controls">
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

      <!-- ④ 三条火花线：每条自己带一个总量，图只是形状 -->
      <div class="admin-overview__sparks">
        <NCard
          v-for="spark in [
            { key: 'flow', label: '近 7 日访问', total: weekVisits, unit: '次', points: sparkFlow },
            { key: 'article', label: '近 7 日发文', total: weekArticles, unit: '篇', points: sparkArticle },
            { key: 'user', label: '近 7 日注册', total: weekSignups, unit: '人', points: sparkUser },
          ]"
          :key="spark.key"
          class="admin-card"
          size="small">
          <div class="admin-overview__spark-head">
            <span class="admin-overview__spark-label">{{ spark.label }}</span>
            <span class="admin-overview__spark-value">{{ spark.total }}<small>{{ spark.unit }}</small></span>
          </div>
          <ClientOnly>
            <AdminTrendChart :points="spark.points" :label="spark.label" :height="56" spark />
            <template #fallback><div class="admin-empty" /></template>
          </ClientOnly>
        </NCard>
      </div>

      <div class="admin-overview__grid">
        <!-- ⑤ 内容 Top 5 -->
        <NCard class="admin-card" data-role="admin-top-card">
          <template #header>
            <div class="admin-overview__card-head">
              <h2 class="admin-overview__card-title">内容 Top 5</h2>
              <span class="admin-overview__card-note">按累计阅读</span>
            </div>
          </template>
          <ol v-if="topArticles.length" class="admin-overview__rank-list" data-role="admin-top-articles">
            <li v-for="(item, index) in topArticles" :key="item.id" class="admin-overview__rank-row">
              <span class="admin-overview__rank">{{ index + 1 }}</span>
              <div class="admin-overview__rank-body">
                <div class="admin-overview__rank-head">
                  <NuxtLink :to="`/articles?id=${item.id}`" class="admin-overview__rank-title">{{ item.title }}</NuxtLink>
                  <span class="admin-overview__rank-meta">{{ formatNumber(item.view_count) }} 阅读</span>
                </div>
                <span class="admin-overview__bar" aria-hidden="true">
                  <span class="admin-overview__bar-fill" :style="{ width: topBarWidth(item.view_count) }" />
                </span>
              </div>
            </li>
          </ol>
          <div v-else class="admin-empty">
            {{ sumLoaded && sum.article_count > 0 ? '取不到阅读排行，搜索索引可能还没同步' : '还没有已发布文章' }}
          </div>
        </NCard>

        <!-- ⑥ 最近动态 5 行 -->
        <NCard class="admin-card" data-role="admin-recent-card">
          <template #header>
            <div class="admin-overview__card-head">
              <h2 class="admin-overview__card-title">最近管理动作</h2>
              <span class="admin-overview__card-note">审计日志</span>
            </div>
          </template>
          <ul v-if="recentActions.length" class="admin-overview__list" data-role="admin-recent-actions">
            <li v-for="item in recentActions" :key="item.event_id" class="admin-overview__row">
              <span class="admin-overview__row-main">{{ item.action_name || item.path }}</span>
              <span class="admin-overview__row-meta">{{ formatDateTime(item.ts) }}</span>
            </li>
          </ul>
          <div v-else class="admin-empty">暂无管理动作</div>
        </NCard>

        <!-- ⑦ 年度分布 -->
        <NCard class="admin-card">
          <template #header>
            <div class="admin-overview__card-head">
              <h2 class="admin-overview__card-title">近 12 个月发文</h2>
              <span class="admin-overview__card-note">{{ yearTotal }} 篇</span>
            </div>
          </template>
          <ClientOnly>
            <AdminTrendChart :points="yearArticles" label="近 12 个月发文" :height="180" />
            <template #fallback><div class="admin-empty" /></template>
          </ClientOnly>
        </NCard>

        <!-- ⑧ 系统健康：一栏数字 + 一栏待处理 -->
        <NCard class="admin-card">
          <template #header>
            <div class="admin-overview__card-head">
              <h2 class="admin-overview__card-title">系统与待处理</h2>
              <NTag :type="loading ? 'default' : 'success'" size="small">{{ loading ? '读取中' : '数据正常' }}</NTag>
            </div>
          </template>
          <dl class="admin-overview__facts" data-role="admin-health">
            <div class="admin-overview__fact">
              <dt>待审核稿件</dt>
              <dd><NuxtLink to="/review">{{ pendingReviews.length }}<small>篇</small></NuxtLink></dd>
            </div>
            <div class="admin-overview__fact">
              <dt>本周新增文章</dt>
              <dd>{{ overview.new_articles }}<small>篇</small></dd>
            </div>
            <div class="admin-overview__fact">
              <dt>本周新增注册</dt>
              <dd>{{ overview.new_users }}<small>人</small></dd>
            </div>
            <div class="admin-overview__fact">
              <!-- 天数如实标出：访问量只保留最近 30 天，不能把 30 天的数字说成一年的。 -->
              <dt>近 {{ overview.visits_days }} 天访问</dt>
              <dd>{{ formatNumber(overview.visits) }}<small>次</small></dd>
            </div>
          </dl>
        </NCard>
      </div>

      <!-- ⑨ AI 用量：零值时折叠成一行 -->
      <NCard class="admin-card" data-role="admin-ai-usage">
        <template #header>
          <div class="admin-overview__card-head">
            <h2 class="admin-overview__card-title">AI 用量</h2>
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
    </div>

    <!-- 空站提示：只有"确实读到了、两个数都是 0"才成立。读不到另说。 -->
    <NAlert
      v-if="sumLoaded && !loading && !sum.user_count && !sum.article_count"
      type="info"
      :bordered="false"
      data-role="admin-empty-site">
      站点还没有内容。先去发布第一篇文章，这里的数字才会有意义。
    </NAlert>
    <NAlert v-else-if="sumFailed && !loading" type="warning" :bordered="false" data-role="admin-sum-failed">
      站点汇总这次没读到，上面的累计数字可能不准确；刷新页面可以重试。
    </NAlert>
  </NSpace>
</template>

<style scoped>
.admin-overview {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

/* 报头：不画大标题（页面标题在顶栏），只放一句结论。 */
.admin-overview__masthead {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem 1.5rem;
  padding: 1.125rem 1.25rem;
  border: 1px solid var(--color-line);
  border-radius: var(--radius-lg);
  background: var(--color-surface);
}

.admin-overview__headline {
  margin: 0.25rem 0 0.375rem;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: clamp(1.125rem, 2.2vw, 1.5rem);
  font-weight: 600;
  letter-spacing: -0.02em;
  line-height: 1.35;
}

.admin-overview__masthead .admin-hint {
  margin: 0;
}

.admin-overview__trend :deep(.n-card-header) {
  padding-bottom: 0.75rem;
}

.admin-overview__card-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem 1rem;
}

.admin-overview__card-title {
  margin: 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1rem;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.admin-overview__card-note {
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  letter-spacing: 0.06em;
}

.admin-overview__controls {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

/* 三条火花线：等宽三列，窄屏堆叠。 */
.admin-overview__sparks {
  display: grid;
  gap: 1rem;
  grid-template-columns: minmax(0, 1fr);
}

@media (min-width: 768px) {
  .admin-overview__sparks {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

.admin-overview__spark-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.5rem;
  margin-bottom: 0.25rem;
}

.admin-overview__spark-label {
  color: var(--color-ink-muted);
  font-size: 0.75rem;
}

.admin-overview__spark-value {
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1.125rem;
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}

.admin-overview__spark-value small {
  margin-left: 0.125rem;
  color: var(--color-ink-faint);
  font-family: var(--font-sans);
  font-size: 0.6875rem;
  font-weight: 400;
}

.admin-overview__grid {
  display: grid;
  gap: 1rem;
  grid-template-columns: minmax(0, 1fr);
}

@media (min-width: 1024px) {
  .admin-overview__grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

/* Top 5：名次 + 标题 + 阅读量 + 条形。 */
.admin-overview__rank-list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  gap: 0.625rem;
  list-style: none;
}

.admin-overview__rank-row {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
}

.admin-overview__rank {
  width: 1.25rem;
  flex: 0 0 auto;
  padding-top: 1px;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.75rem;
  font-variant-numeric: tabular-nums;
  text-align: right;
}

.admin-overview__rank-body {
  min-width: 0;
  flex: 1;
}

.admin-overview__rank-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.75rem;
}

.admin-overview__rank-title {
  min-width: 0;
  overflow: hidden;
  color: var(--color-ink);
  font-size: 0.875rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.admin-overview__rank-title:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}

.admin-overview__rank-meta {
  flex: 0 0 auto;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  font-variant-numeric: tabular-nums;
}

.admin-overview__bar {
  display: block;
  height: 3px;
  margin-top: 0.375rem;
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--color-line-faint);
}

.admin-overview__bar-fill {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--color-ink);
}

/* 行列表：动作 / 时间。 */
.admin-overview__list {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}

.admin-overview__row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.875rem;
  padding: 0.5rem 0;
  border-bottom: 1px solid var(--color-line-faint);
  font-size: 0.875rem;
}

.admin-overview__row:last-child {
  border-bottom: 0;
}

.admin-overview__row-main {
  min-width: 0;
  overflow: hidden;
  color: var(--color-ink);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.admin-overview__row-meta {
  flex: 0 0 auto;
  color: var(--color-ink-faint);
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  font-variant-numeric: tabular-nums;
}

/* 系统与待处理：四格数字，只有发丝线，不再套第二层卡片。 */
.admin-overview__facts {
  display: grid;
  margin: 0;
  gap: 0.5rem;
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.admin-overview__fact {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 0.625rem 0.75rem;
  border: 1px solid var(--color-line-faint);
  border-radius: var(--radius-md);
  background: var(--color-bg-subtle);
}

.admin-overview__fact dt {
  color: var(--color-ink-muted);
  font-size: 0.6875rem;
  line-height: 1.4;
}

.admin-overview__fact dd {
  margin: 0;
  color: var(--color-ink);
  font-family: var(--font-title);
  font-size: 1.25rem;
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  line-height: 1.2;
}

.admin-overview__fact dd a:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}

.admin-overview__fact small {
  margin-left: 0.125rem;
  color: var(--color-ink-faint);
  font-family: var(--font-sans);
  font-size: 0.6875rem;
  font-weight: 400;
}

.admin-overview :deep(.admin-empty) {
  min-height: 120px;
}
</style>

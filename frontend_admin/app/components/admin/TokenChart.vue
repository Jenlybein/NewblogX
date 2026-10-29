<script setup lang="ts">
/**
 * AI token 用量的趋势图。
 *
 * 为什么不引图表库：admin 目前零图表依赖，现有的 7 天趋势就是纯 CSS 柱
 * （见 admin.css 的 .admin-chart）。为一张图引入 echarts 要多 1MB 产物，
 * 而这里只需要"柱"和"折线"两种形态。
 *
 * 形态由点数决定（见 utils/aiTokens 的 pickChartType）：≤20 个点画柱，
 * 更多就画折线 —— 柱子窄于 12px 时只剩一条色块，趋势反而看不出来。
 */
import { barHeightPercent, buildLinePoints, maxTokens, pickChartType, shortTokens } from '~/utils/aiTokens'

type ChartPoint = {
  bucket: string
  label: string
  total_tokens: number
}

const props = defineProps<{ points: ChartPoint[] }>()

const kind = computed(() => pickChartType(props.points.length))
const max = computed(() => maxTokens(props.points.map(point => point.total_tokens)))
const values = computed(() => props.points.map(point => point.total_tokens))
const linePoints = computed(() => buildLinePoints(values.value))
// 折线下方补一块半透明面积，让"高峰/低谷"比一条细线更容易读。
const areaPoints = computed(() => (linePoints.value ? `0,100 ${linePoints.value} 100,100` : ''))
const midIndex = computed(() => Math.floor(props.points.length / 2))
</script>

<template>
  <div>
    <!-- 柱状：复用现有的 .admin-chart，与首页其它趋势图保持一致 -->
    <div v-if="kind === 'bar'" class="admin-chart">
      <div
        v-for="point in points"
        :key="point.bucket"
        class="admin-chart__item"
        :title="`${point.label}: ${point.total_tokens} token`"
      >
        <div class="admin-chart__bar-track">
          <div class="admin-chart__bar" :style="{ height: barHeightPercent(point.total_tokens, max) }" />
        </div>
        <span class="admin-chart__count">{{ shortTokens(point.total_tokens) }}</span>
        <span class="admin-chart__date">{{ point.label }}</span>
      </div>
    </div>

    <!-- 折线：点数多的时候用。preserveAspectRatio=none 会把线宽一起拉变形，
         所以线本身必须带 vector-effect="non-scaling-stroke"。 -->
    <div v-else class="admin-token-line">
      <svg
        class="admin-token-line__svg"
        viewBox="0 0 100 100"
        preserveAspectRatio="none"
        role="img"
        aria-label="AI token 用量趋势"
      >
        <polygon :points="areaPoints" class="admin-token-line__area" />
        <polyline :points="linePoints" class="admin-token-line__line" vector-effect="non-scaling-stroke" />
      </svg>
      <!-- 24~31 个刻度全渲染会糊成一片，只留首/中/尾三个 -->
      <div class="admin-token-line__axis">
        <span>{{ points[0]?.label }}</span>
        <span>{{ points[midIndex]?.label }}</span>
        <span>{{ points[points.length - 1]?.label }}</span>
      </div>
    </div>
  </div>
</template>

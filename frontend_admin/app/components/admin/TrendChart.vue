<script setup lang="ts">
/**
 * 折线趋势图（票 349，ECharts 按需引入）。
 *
 * 为什么换掉手写柱状图：手写图只能画柱，且没有缩放、没有坐标轴自适应，
 * 数据点一多就挤成一片。ECharts 只按需引 core + 折线 + 网格 + 提示 + 坐标轴，
 * 不打全量包。
 *
 * 三条口径：
 *  - 颜色只取设计 token（图表也要守"没有裸色值"这条），所以这里不写十六进制，
 *    统一从 CSS 变量读一次再喂给 ECharts；
 *  - 组件必须 `<ClientOnly>` 包住使用：ECharts 要 DOM 才能初始化；
 *  - 数据为空时画"暂无数据"，不是一条贴着 0 的线 —— 后者会被读成"确实是 0"。
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { DateCountItem } from '~/types/api'

echarts.use([LineChart, GridComponent, TooltipComponent, CanvasRenderer])

const props = withDefaults(defineProps<{
  points: DateCountItem[]
  /** 图的语义（写进 tooltip 与 aria-label）。 */
  label: string
  /** 高度（px）。主趋势图比火花线高。 */
  height?: number
  /** 火花线模式：去掉坐标轴与网格，只留形状。 */
  spark?: boolean
}>(), {
  height: 220,
  spark: false,
})

const host = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null

/**
 * 从设计 token 解析出**浏览器已算好的颜色**。
 *
 * 做法是往 DOM 里临时挂一个元素、把 `var(--color-*)` 写在它的 color 上，
 * 再读回 computed color。这样源码里不会出现任何裸色值（设计 token 门禁要求），
 * 拿到的又是 canvas 能直接用的具体颜色（`var()` 字符串 ECharts 画不出来）。
 */
function token(name: string): string {
  if (typeof window === 'undefined' || typeof document === 'undefined') return ''
  const probe = document.createElement('span')
  probe.style.color = `var(${name})`
  probe.style.display = 'none'
  document.body.appendChild(probe)
  const resolved = getComputedStyle(probe).color
  document.body.removeChild(probe)
  return resolved
}

const hasData = computed(() => props.points.some(point => point.count > 0))

function buildOption() {
  const ink = token('--color-ink')
  const muted = token('--color-ink-muted')
  const line = token('--color-line')
  const accent = token('--color-link')

  return {
    animationDuration: 420,
    grid: props.spark
      ? { left: 0, right: 0, top: 4, bottom: 4 }
      : { left: 8, right: 12, top: 16, bottom: 24, containLabel: true },
    tooltip: props.spark
      ? { show: false }
      : {
          trigger: 'axis',
          // 提示里给出完整日期与次数，不用用户去数坐标轴。
          formatter: (params: { name: string; value: number }[]) => {
            const first = params[0]
            return first ? `${first.name}<br/>${props.label}：${first.value}` : ''
          },
        },
    xAxis: {
      type: 'category',
      data: props.points.map(point => point.date),
      show: !props.spark,
      axisLine: { lineStyle: { color: line } },
      axisLabel: { color: muted, fontSize: 11 },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      show: !props.spark,
      splitLine: { lineStyle: { color: line } },
      axisLabel: { color: muted, fontSize: 11 },
    },
    series: [{
      type: 'line',
      smooth: true,
      showSymbol: !props.spark,
      symbolSize: 5,
      data: props.points.map(point => point.count),
      lineStyle: { color: props.spark ? muted : accent, width: props.spark ? 1.5 : 2 },
      itemStyle: { color: accent },
      areaStyle: props.spark ? undefined : { color: accent, opacity: 0.08 },
    }],
    // 无数据时不画线：一条贴着 0 的线会被读成"确实是 0"。
    graphic: hasData.value ? [] : [{
      type: 'text',
      left: 'center',
      top: 'middle',
      style: { text: '暂无数据', fill: muted, fontSize: 12 },
    }],
    textStyle: { color: ink },
  }
}

function render() {
  if (!chart) return
  chart.setOption(buildOption(), true)
}

onMounted(() => {
  if (!host.value) return
  chart = echarts.init(host.value)
  render()
  // 容器宽度变化时重算：卡片在栅格里，窗口一缩放宽度就变。
  window.addEventListener('resize', render)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', render)
  chart?.dispose()
  chart = null
})

watch(() => [props.points, props.label], render, { deep: true })
</script>

<template>
  <div
    ref="host"
    class="admin-chart"
    :style="{ height: `${height}px` }"
    role="img"
    :aria-label="`${label}趋势图`"
    :data-role="`chart-${label}`" />
</template>

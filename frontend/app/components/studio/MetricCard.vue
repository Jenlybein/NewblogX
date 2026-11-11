<script setup lang="ts">
/**
 * 指标卡（票 354）。
 *
 * 两端**逐字节相同**（scripts/check-image-shaping-parity.mjs 的 metric-card 一对）：
 * 读者站的个人概览与后台的站点概览要显示同一类数字，两处各写一份就会出现
 * "同一个数字在两页字号不同"这种没人会主动去比对的漂移。
 *
 * 三个刻意的取舍：
 *  - **数字只在首次挂载时动画一次**：每次进入页面都滚一遍数字会让人以为数据在跳。
 *    支持 `prefers-reduced-motion`：系统关掉动画时直接显示终值。
 *  - **零值不是错误**：`zero` 只是给调用方一个视觉降级的钩子（折叠成一行"无待处理"），
 *    卡片本身不把 0 画成红色。
 *  - 标签在上、数字在下：数字是主角，标签是注解。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { MOTION_EXPRESSIVE_MS } from '~/utils/motion'

const props = withDefaults(defineProps<{
  /** 指标名（一行，长文案会被截断）。 */
  label: string
  /** 数值；null 表示"还没读到"（显示占位而不是 0）。 */
  value?: number | null
  /** 单位后缀，如「篇」「次」。 */
  suffix?: string
  /** 说明（一行，可选）。 */
  hint?: string
  /** 数字为零时的视觉降级（折叠显示）。 */
  dimWhenZero?: boolean
}>(), {
  value: null,
  suffix: '',
  hint: '',
  dimWhenZero: false,
})

/** 动画中的显示值：从 0 跑到目标值一次。 */
const shown = ref(0)

const isZero = computed(() => (props.value ?? 0) === 0)
const hasValue = computed(() => props.value !== null && props.value !== undefined)

/** 动画只跑一次：挂载时把 0 补到目标值。 */
function animateTo(target: number) {
  const reduced = typeof window !== 'undefined'
    && typeof window.matchMedia === 'function'
    && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  if (reduced || target === 0) {
    shown.value = target
    return
  }
  const duration = MOTION_EXPRESSIVE_MS.countUp
  const start = performance.now()
  const step = (now: number) => {
    const progress = Math.min((now - start) / duration, 1)
    // easeOutCubic：末段慢下来，读数时不觉得数字在冲。
    const eased = 1 - Math.pow(1 - progress, 3)
    shown.value = Math.round(target * eased)
    if (progress < 1) requestAnimationFrame(step)
  }
  requestAnimationFrame(step)
}

onMounted(() => {
  if (hasValue.value) animateTo(props.value as number)
})

// 数据是异步到的：从 null 变成真实值时补一次动画。
watch(() => props.value, (next) => {
  if (next === null || next === undefined) return
  animateTo(next)
})
</script>

<template>
  <div class="metric-card" :class="{ 'metric-card--dim': dimWhenZero && isZero }" :data-role="`metric-${label}`">
    <span class="metric-card__label">{{ label }}</span>
    <span class="metric-card__value">
      <template v-if="hasValue">{{ shown }}</template>
      <template v-else>—</template>
      <span v-if="suffix" class="metric-card__suffix">{{ suffix }}</span>
    </span>
    <span v-if="hint" class="metric-card__hint">{{ hint }}</span>
  </div>
</template>

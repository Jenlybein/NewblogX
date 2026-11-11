<script setup lang="ts">
/**
 * 90 天创作热力图（票 354）。
 *
 * 13 列 × 7 行：每列一周（周一到周日），最后一行是本周。起点对齐到"周一起"，
 * 所以网格左边永远是周一 —— 不对齐的话用户没法把颜色和"上周三"对上。
 *
 * 三条可访问性要求（热力图最容易做成"只有鼠标能看"）：
 *  1. 每个格子是 `<button>`，可 Tab 聚焦、可读出 `aria-label`（日期 + 次数）；
 *  2. 聚焦时把提示文字显示在网格下方（不依赖 hover 的 tooltip）；
 *  3. 颜色不是唯一编码：提示里始终有"3 次"这样的文字。
 */
import { computed, ref } from 'vue'

const props = withDefaults(defineProps<{
  /** 稀疏的每日计数：key 是 `YYYY-MM-DD`，没有的日期按 0 处理。 */
  activity: Record<string, number>
  /** 覆盖天数（默认 91 = 13 周）。 */
  days?: number
  /** "今天"；不传用本地时间。测试里传固定值才能断言网格。 */
  today?: Date
}>(), {
  days: 91,
  today: undefined,
})

/** 把日期格式化成 `YYYY-MM-DD`（本地时区，与后端口径一致）。 */
function dayKey(date: Date): string {
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

/** 按本地时区取"今天"的零点。 */
function startOfDay(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

interface Cell {
  key: string
  count: number
  /** 0–4 的色阶档位。 */
  level: number
  /** 是否属于"未来"（本周里还没到的日子），渲染成空白。 */
  future: boolean
}

const cells = computed<Cell[]>(() => {
  const today = startOfDay(props.today ?? new Date())
  // 网格是**整周**的：从"本周周日"往回数 91 天，正好 13 列 × 7 行。
  //
  // 为什么不用"今天往前 91 天"：那样最后一列只到今天就断了，格子数不是 7 的倍数，
  // 网格右侧会缺一角；本周还没到的日子按 `future` 渲染成空白。
  const weekdayOfToday = (today.getDay() + 6) % 7 // 周一 = 0
  const end = new Date(today)
  end.setDate(end.getDate() + (6 - weekdayOfToday))
  const start = new Date(end)
  start.setDate(start.getDate() - (props.days - 1))

  const out: Cell[] = []
  const cursor = new Date(start)
  while (cursor <= end) {
    const key = dayKey(cursor)
    const count = props.activity[key] ?? 0
    out.push({
      key,
      count,
      level: levelOf(count),
      future: cursor > today,
    })
    cursor.setDate(cursor.getDate() + 1)
  }
  return out
})

/** 把次数映射成 0–4 档：阈值固定，保证不同周之间颜色可比。 */
function levelOf(count: number): number {
  if (count <= 0) return 0
  if (count === 1) return 1
  if (count <= 3) return 2
  if (count <= 6) return 3
  return 4
}

/** 聚焦格子的提示（键盘用户靠它读数）。 */
const focusedKey = ref('')
const focused = computed(() => cells.value.find(cell => cell.key === focusedKey.value) ?? null)

const total = computed(() => cells.value.reduce((sum, cell) => sum + cell.count, 0))

/** 有动作的天数：用来给"这段时间做了多少"一个更直观的说法。 */
const activeDays = computed(() => cells.value.filter(cell => cell.count > 0).length)
</script>

<template>
  <section class="activity-heatmap" data-role="activity-heatmap">
    <header class="activity-heatmap__head">
      <h3 class="activity-heatmap__title">近 90 天创作</h3>
      <span class="activity-heatmap__summary" data-role="activity-summary">
        {{ total }} 次 · {{ activeDays }} 天有动作
      </span>
    </header>

    <div
      class="activity-heatmap__grid"
      role="grid"
      :aria-label="`近 ${days} 天每天的新建与更新次数`">
      <button
        v-for="cell in cells"
        :key="cell.key"
        type="button"
        role="gridcell"
        class="activity-heatmap__cell"
        :class="`activity-heatmap__cell--l${cell.level}`"
        :data-role="`activity-cell-${cell.key}`"
        :data-level="cell.level"
        :aria-label="`${cell.key}：${cell.count} 次`"
        :aria-hidden="cell.future ? 'true' : undefined"
        :tabindex="cell.future ? -1 : 0"
        @focus="focusedKey = cell.key"
        @blur="focusedKey = ''">
        <span class="activity-heatmap__dot" aria-hidden="true" />
      </button>
    </div>

    <!-- 提示固定显示在网格下方：不依赖 hover 的浮层，键盘与触屏都能看到。 -->
    <p class="activity-heatmap__tip" data-role="activity-tip">
      <template v-if="focused">{{ focused.key }} · {{ focused.count }} 次</template>
      <template v-else>把光标移到格子上看具体日期与次数。</template>
    </p>
  </section>
</template>

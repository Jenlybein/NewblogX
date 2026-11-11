<script setup lang="ts">
/**
 * 90 天创作热力图（票 354，2026-11-12 重做）。
 *
 * 上一版只有"一排小方块"：没有月份、没有星期、没有图例，13×7 个 14px 的格
 * 缩在整宽卡片左上角，看上去像渲染坏了。这一版补三件事：
 *   1. 月份刻度（每列一周）+ 星期刻度（一 / 三 / 五）+ 色阶图例；
 *   2. 右侧一栏把数字说出来（总次数 / 有动作天数 / 连续天数 / 最活跃的一天）——
 *      颜色只回答"哪天多"，数字回答"一共多少"；
 *   3. 格子用容器宽度档 `--hm-cell`，窄屏不挤成一团。
 *
 * 每条可访问性要求都保留（热力图最容易做成"只有鼠标能看"）：
 *   - 每个格子是 `<button>`，可 Tab 聚焦、`aria-label` 里有日期与次数；
 *   - 聚焦时提示显示在网格下方（不依赖 hover 的 tooltip）；
 *   - 颜色不是唯一编码：提示里始终有"3 次"这样的文字。
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

/**
 * 把日期格式化成 `YYYY-MM-DD`，**按东八区**。
 *
 * 后端按 Asia/Shanghai 切天（见 clickhouse.CountDailyStudioActivity）。这里如果
 * 用浏览器本地时区，非 +08 的读者会看到整列错位一天 —— 热力图最忌讳"我的今天
 * 显示成昨天"。
 */
const DAY_FORMATTER = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
})

function dayKey(date: Date): string {
  // en-CA 的日期格式恰好是 YYYY-MM-DD，不用自己拼。
  return DAY_FORMATTER.format(date)
}

/**
 * 把 `YYYY-MM-DD` 解析成 UTC 零点。
 *
 * 网格里的日期算术全部在 UTC 上做：日期字符串本身就是"东八区的那一天"，
 * 用 UTC 当容器做加减不会受浏览器时区影响，也不会遇到夏令时。
 */
function parseDayKey(key: string): Date {
  const [y, m, d] = key.split('-').map(Number)
  return new Date(Date.UTC(y!, (m ?? 1) - 1, d ?? 1))
}

/** 把 UTC 容器格式化回 `YYYY-MM-DD`。 */
function formatUtcDay(date: Date): string {
  const y = date.getUTCFullYear()
  const m = String(date.getUTCMonth() + 1).padStart(2, '0')
  const d = String(date.getUTCDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
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
  // "今天"按东八区取：与后端切天的口径一致，非 +08 的读者也不会错一天。
  const todayKey = dayKey(props.today ?? new Date())
  const today = parseDayKey(todayKey)

  // 网格是**整周**的：从"本周周日"往回数 91 天，正好 13 列 × 7 行。
  //
  // 为什么不用"今天往前 91 天"：那样最后一列只到今天就断了，格子数不是 7 的倍数，
  // 网格右侧会缺一角；本周还没到的日子按 `future` 渲染成空白。
  const weekdayOfToday = (today.getUTCDay() + 6) % 7 // 周一 = 0
  const end = new Date(today)
  end.setUTCDate(end.getUTCDate() + (6 - weekdayOfToday))
  const start = new Date(end)
  start.setUTCDate(start.getUTCDate() - (props.days - 1))

  const out: Cell[] = []
  const cursor = new Date(start)
  while (cursor <= end) {
    const key = formatUtcDay(cursor)
    const count = props.activity[key] ?? 0
    out.push({
      key,
      count,
      level: levelOf(count),
      future: cursor > today,
    })
    cursor.setUTCDate(cursor.getUTCDate() + 1)
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

/** 按周切列：每列 7 天（周一起）。 */
const weeks = computed<Cell[][]>(() => {
  const out: Cell[][] = []
  for (let i = 0; i < cells.value.length; i += 7) {
    out.push(cells.value.slice(i, i + 7))
  }
  return out
})

const weekCount = computed(() => Math.max(weeks.value.length, 1))

/**
 * 月份刻度：每一列取该周第一天所在的月份，跨月时换标签。
 * 返回的 `start` 是 1 基列号，直接写进 `grid-column`。
 */
const months = computed<{ label: string; start: number; span: number; month: number }[]>(() => {
  const out: { label: string; start: number; span: number; month: number }[] = []
  weeks.value.forEach((week, index) => {
    const key = week[0]?.key
    if (!key) return
    const month = Number(key.slice(5, 7))
    const last = out[out.length - 1]
    if (last && last.month === month) {
      last.span += 1
      return
    }
    out.push({ label: `${month} 月`, start: index + 1, span: 1, month })
  })
  return out
})

/** 月份轨道与格子轨道用同一套列宽/间隙，标签才能对齐到列。 */
const monthTrackStyle = computed(() => ({
  gridTemplateColumns: `repeat(${weekCount.value}, var(--hm-cell))`,
}))

/** 聚焦格子的提示（键盘用户靠它读数）。 */
const focusedKey = ref('')
const focused = computed(() => cells.value.find(cell => cell.key === focusedKey.value) ?? null)

const total = computed(() => cells.value.reduce((sum, cell) => sum + cell.count, 0))

/** 有动作的天数：用来给"这段时间做了多少"一个更直观的说法。 */
const activeDays = computed(() => cells.value.filter(cell => cell.count > 0).length)

/** 连到今天为止的连续创作天数（今天还是 0 时从昨天往前数）。 */
const streak = computed(() => {
  const past = cells.value.filter(cell => !cell.future)
  let index = past.length - 1
  // 今天还没动笔不算断档：从昨天开始数。
  if (index >= 0 && past[index]!.count === 0) index -= 1
  let run = 0
  for (; index >= 0; index -= 1) {
    if (past[index]!.count === 0) break
    run += 1
  }
  return run
})

/** 最活跃的一天：并列时取更近的一天。 */
const bestDay = computed(() => {
  let best: Cell | null = null
  for (const cell of cells.value) {
    if (!best || cell.count >= best.count) best = cell
  }
  return best && best.count > 0 ? best : null
})

function shortDay(key: string): string {
  const [, month, day] = key.split('-')
  return `${Number(month)}/${Number(day)}`
}

/** 彩色的前三档之外，图例还要第四档（最深）；用同一组类名渲染，避免第二套色阶。 */
const LEGEND_LEVELS = [0, 1, 2, 3, 4] as const
</script>

<template>
  <section class="activity-heatmap" data-role="activity-heatmap">
    <header class="activity-heatmap__head">
      <div class="activity-heatmap__head-copy">
        <h3 class="activity-heatmap__title">近 90 天创作</h3>
        <p class="activity-heatmap__subtitle">每天新建与更新的次数，按东八区切天。</p>
      </div>
      <span class="activity-heatmap__summary" data-role="activity-summary">
        {{ total }} 次 · {{ activeDays }} 天有动作
      </span>
    </header>

    <div class="activity-heatmap__body">
      <div class="activity-heatmap__chart">
        <div class="activity-heatmap__chart-top">
          <span class="activity-heatmap__axis-spacer" aria-hidden="true" />
          <div class="activity-heatmap__months" aria-hidden="true" :style="monthTrackStyle">
            <span
              v-for="month in months"
              :key="`${month.start}-${month.label}`"
              class="activity-heatmap__month"
              :style="{ gridColumn: `${month.start} / span ${month.span}` }">
              {{ month.label }}
            </span>
          </div>
        </div>

        <div class="activity-heatmap__chart-main">
          <div class="activity-heatmap__weekdays" aria-hidden="true">
            <span v-for="(name, index) in ['一', '', '三', '', '五', '', '']" :key="index">{{ name }}</span>
          </div>

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
              :data-future="cell.future ? 'true' : undefined"
              :aria-label="`${cell.key}：${cell.count} 次`"
              :aria-hidden="cell.future ? 'true' : undefined"
              :tabindex="cell.future ? -1 : 0"
              @focus="focusedKey = cell.key"
              @blur="focusedKey = ''">
              <span class="activity-heatmap__dot" aria-hidden="true" />
            </button>
          </div>
        </div>

        <p class="activity-heatmap__tip" data-role="activity-tip">
          <template v-if="focused">{{ focused.key }} · {{ focused.count }} 次</template>
          <template v-else>把光标移到格子上看具体日期与次数。</template>
        </p>
      </div>

      <!-- 右栏：颜色说不清"一共多少"，这里用四个数字补上。 -->
      <dl class="activity-heatmap__facts">
        <div class="activity-heatmap__fact">
          <dt>总次数</dt>
          <dd>{{ total }}<small>次</small></dd>
        </div>
        <div class="activity-heatmap__fact">
          <dt>有动作</dt>
          <dd>{{ activeDays }}<small>天</small></dd>
        </div>
        <div class="activity-heatmap__fact">
          <dt>连续创作</dt>
          <dd>{{ streak }}<small>天</small></dd>
        </div>
        <div class="activity-heatmap__fact">
          <dt>最活跃</dt>
          <dd>
            <template v-if="bestDay">{{ shortDay(bestDay.key) }}<small>{{ bestDay.count }} 次</small></template>
            <template v-else>—</template>
          </dd>
        </div>
      </dl>
    </div>

    <!-- 色阶图例：颜色不是唯一编码，图例只做"深浅"提示。 -->
    <footer class="activity-heatmap__foot" aria-hidden="true">
      <span class="activity-heatmap__legend-label">少</span>
      <span
        v-for="level in LEGEND_LEVELS"
        :key="level"
        class="activity-heatmap__legend-cell"
        :class="`activity-heatmap__cell--l${level}`" />
      <span class="activity-heatmap__legend-label">多</span>
    </footer>
  </section>
</template>

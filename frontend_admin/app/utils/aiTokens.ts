/**
 * AI token 用量的纯展示逻辑。
 *
 * 放在 utils 而不是组件里：admin 的覆盖率门槛只统计 `app/utils/**`
 * （95/90/95/95），图形计算这类"有边界情况但没 UI"的逻辑放这里才测得到。
 */

/** 折线图的点数阈值：超过它就用折线。 */
export const LINE_CHART_THRESHOLD = 20

/**
 * 按点数选择图形。
 *
 * 产品规则是"超过 20 个点用折线"：柱子少于 ~12px 宽时就只剩一条色块，
 * 趋势反而看不出来。实际效果是 —— 今日(24)/本月(28~31) 折线，
 * 本周(7)/本年(12) 柱状。
 */
export function pickChartType(pointCount: number): 'bar' | 'line' {
  return pointCount > LINE_CHART_THRESHOLD ? 'line' : 'bar'
}

/** 取序列最大值，至少为 1，避免除零。 */
export function maxTokens(values: number[]): number {
  return Math.max(1, ...values.map(value => (Number.isFinite(value) ? value : 0)))
}

/** 柱高百分比。0 值的柱子留 2px，否则看不见"那天确实没数据"。 */
export function barHeightPercent(value: number, max: number): string {
  if (!Number.isFinite(value) || value <= 0) return '2px'
  const safeMax = Number.isFinite(max) && max > 0 ? max : 1
  // 上限必须钳住：max 传 0 或 NaN 时兜底成 1，不钳的话 5/1*100 = 500%，
  // 柱子会撑破容器盖住上面的标题。
  const percent = Math.min(100, Math.max(8, (value / safeMax) * 100))
  return `${percent}%`
}

/**
 * 生成 SVG polyline 的 points 串（viewBox 0 0 100 100）。
 *
 * 单点时给一条水平居中线而不是空串：只有一个数据点的周期（比如本年第 1 个月）
 * 如果返回空串，图上什么都没有，看起来像"加载失败"。
 */
export function buildLinePoints(values: number[], width = 100, height = 100): string {
  if (values.length === 0) return ''
  if (values.length === 1) return `0,${height / 2}`
  const max = maxTokens(values)
  const stepX = width / (values.length - 1)
  return values
    .map((value, index) => {
      const x = +(index * stepX).toFixed(2)
      const ratio = Math.max(0, Number.isFinite(value) ? value : 0) / max
      const y = +(height - ratio * height).toFixed(2)
      return `${x},${y}`
    })
    .join(' ')
}

/** 紧凑的 token 数字：1.2k / 3.4万 / 1.2亿。 */
export function shortTokens(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0'
  if (value < 1000) return String(Math.round(value))
  if (value < 10000) return `${(value / 1000).toFixed(1)}k`
  if (value < 100_000_000) return `${(value / 10000).toFixed(1)}万`
  return `${(value / 100_000_000).toFixed(1)}亿`
}

/** 场景标识 → 中文名。与后端 internal/ai/callmeta.go 的常量一一对应。 */
export const SCENE_LABELS: Record<string, string> = {
  scoring: '文章评分',
  metainfo: '元信息生成',
  overwrite: '文本改写',
  diagnose: '文本诊断',
  search_rewrite: '搜索改写',
  search_answer: '搜索问答',
}

/**
 * 场景的中文名。
 *
 * 认不出的场景原样返回而不是显示"未知"：后端新增场景而前端没跟上的时候，
 * 直接把 `xxx` 显示出来至少能让人看出"这里有个新场景"，吞掉就再也发现不了。
 */
export function sceneLabel(scene: string): string {
  const key = String(scene ?? '').trim()
  if (!key) return '-'
  return SCENE_LABELS[key] ?? key
}

/** 调用结果标签。后端存的是 UInt8。 */
export function aiResultLabel(success: number): string {
  return Number(success) === 1 ? '成功' : '失败'
}

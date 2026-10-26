/**
 * 死信（DLQ）与人话之间的映射。
 *
 * 为什么单独抽出来：页面里直接写 `stream === 'es_river' ? '...' : ...` 的话，
 * 这些词表既测不到（管理端覆盖率只统计 app/utils），也容易在新增 stream 时漏改。
 * 抽到 utils 后，`pagination`/`roles` 那一套"用单测钉住取值"的做法可以复用。
 */

/** 死信状态。后端的取值定义在 backend/internal/platform/cdc/deadletter/status.go。 */
export const DEAD_LETTER_STATUS = {
  pending: 'pending',
  resolved: 'resolved',
} as const

/** stream 的中文名。新增 CDC 流时要同步这里，否则页面会显示原始值。 */
const STREAM_LABELS: Record<string, string> = {
  es_river: '搜索同步（ES 直投）',
  outbox_relay: '领域事件（发件箱）',
  es_projection: '文章搜索投影',
  image_ref_river: '图片引用',
}

/** 状态的中文名。 */
const STATUS_LABELS: Record<string, string> = {
  pending: '待处理',
  resolved: '已解决',
}

/** 重投结果的中文名。取值与后端 dlq.Result*、回放日志的 result 列一致。 */
const RESULT_LABELS: Record<string, string> = {
  success: '成功',
  failed: '失败',
  skipped: '跳过',
  unsupported: '不支持重投',
}

/**
 * 查词表。
 *
 * 三个 label 函数共用这一份逻辑：空值给占位符、查到给中文、查不到原样返回。
 * 抽成一个函数是为了别把同一段三分支判断抄三遍 —— 抄三遍就多出三倍的未覆盖分支，
 * 覆盖率门槛会先红（这里已经踩过一次）。
 */
function pick(labels: Record<string, string>, value: string): string {
  const key = String(value ?? '').trim()
  if (!key) {
    return '-'
  }
  return labels[key] ?? key
}

export function streamLabel(stream: string): string {
  return pick(STREAM_LABELS, stream)
}

export function deadLetterStatusLabel(status: string): string {
  return pick(STATUS_LABELS, status)
}

export function replayResultLabel(result: string): string {
  return pick(RESULT_LABELS, result)
}

/** 结果对应的标签颜色（naive-ui 的 NTag type）。 */
export function replayResultTagType(result: string): 'success' | 'error' | 'warning' | 'default' {
  switch (String(result || '').trim()) {
    case 'success':
      return 'success'
    case 'failed':
      return 'error'
    case 'skipped':
    case 'unsupported':
      return 'warning'
    default:
      return 'default'
  }
}

/**
 * 哪些死信可以重投 —— 只用于页面上的提示，真正的判定在后端。
 *
 * 这里不复制"当前支持哪些流"的名单（那会变成第二份真相），只判断"该不该显示重投按钮"：
 * 已解决的不显示，其余都显示，点了由后端逐条告诉你能不能。
 */
export function canReplay(status: string): boolean {
  return String(status || '').trim() !== DEAD_LETTER_STATUS.resolved
}

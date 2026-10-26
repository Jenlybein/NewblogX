/**
 * 文章相关的展示文案。
 *
 * 数值/字符串必须与后端一致：
 *   - `publish_status`：`backend/internal/kernel/enum/article_status.go`
 *     `1 草稿 / 2 待审核 / 3 已发布 / 4 已驳回`
 *   - `visibility_status`：`backend/internal/kernel/enum/article_visibility_status.go`
 *     `visible / user_hidden / admin_hidden`
 *
 * 抽成独立模块而不是留在页面里，是为了能写测试把映射钉住——
 * 管理端的角色映射就曾把 1 和 2 写反（选"普通用户"实际提权成管理员），
 * 而页内函数没法单测，只能等用户发现。
 */
import type { components } from '~/types/openapi'

/** 发布状态。 */
export const PUBLISH_DRAFT = 1
/** 待审核。 */
export const PUBLISH_PENDING = 2
/** 已发布。 */
export const PUBLISH_PUBLISHED = 3
/** 已驳回。 */
export const PUBLISH_REJECTED = 4

const PUBLISH_LABELS: Record<number, string> = {
  [PUBLISH_DRAFT]: '草稿',
  [PUBLISH_PENDING]: '待审核',
  [PUBLISH_PUBLISHED]: '已发布',
  [PUBLISH_REJECTED]: '已驳回',
}

const VISIBILITY_LABELS: Record<string, string> = {
  visible: '公开',
  user_hidden: '作者隐藏',
  admin_hidden: '后台隐藏',
}

/** 发布状态文案。未知值原样显示，便于发现契约里出现了新枚举。 */
export function publishLabel(value?: number | null): string {
  if (value == null) return '-'
  return PUBLISH_LABELS[value] ?? String(value)
}

/** 可见性文案。未知值按「公开」显示（与后端默认值一致）。 */
export function visibilityLabel(value?: string | null): string {
  return VISIBILITY_LABELS[value ?? 'visible'] ?? VISIBILITY_LABELS.visible!
}

/** 是否属于"被后台隐藏"（据此决定显示「恢复」还是「隐藏」按钮）。 */
export function isAdminHidden(
  value: components['schemas']['myblogx_internal_search_domain.VisibilityStatus'] | undefined,
): boolean {
  return value === 'admin_hidden'
}

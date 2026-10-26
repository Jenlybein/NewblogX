import type {
  AIRuntimeConfig,
  ActionAuditRecord,
  ArticleReviewTaskListResponseData,
  ArticleSearchResponseData,
  ArticleYearDataResponseData,
  BannerModel,
  CdcEventRecord,
  GlobalNotifListItem,
  GrowthDataResponseData,
  HasMoreList,
  DeadLetterListData,
  DeadLetterReplayResult,
  ImageListResponseData,
  ListData,
  ReplayLogRecord,
  LoginEventRecord,
  RuntimeLogRecord,
  SiteRuntimeConfig,
  SumResponseData,
  UserListResponseData,
} from '~/types/api'

function api() {
  return useNuxtApp().$api
}

export function getDashboardSum() {
  return api().request<SumResponseData>('/data/sum')
}

export function getGrowthData(type: 1 | 2 | 3 = 3) {
  return api().request<GrowthDataResponseData>('/data/growth', { query: { type } })
}

export function getArticleYearData() {
  return api().request<ArticleYearDataResponseData>('/data/article-year')
}

export function getArticleReviewTasks(query: { page?: number; limit?: number; status?: string }) {
  return api().request<ArticleReviewTaskListResponseData>('/article-review', { query })
}

export function reviewArticleTask(id: string, payload: { status: 3 | 4; reason?: string }) {
  return api().request<unknown, typeof payload>(`/article-review-tasks/${id}/review`, {
    method: 'POST',
    body: payload,
  })
}

export function getAdminUsers() {
  return api().request<UserListResponseData>('/users/admin/list')
}

export function createAdminUser(payload: { username: string; password: string; nickname?: string | null; email?: string | null }) {
  return api().request<unknown, typeof payload>('/users/admin', {
    method: 'POST',
    body: payload,
  })
}

export function updateAdminUser(payload: {
  user_id: string
  username?: string | null
  nickname?: string | null
  avatar_image_id?: string | null
  abstract?: string | null
  role?: number | null
  status?: number | null
}) {
  return api().request<unknown, typeof payload>('/users/admin/info', {
    method: 'PUT',
    body: payload,
  })
}

// `status` is kept as the backend query name, but it filters publish_status.
export function searchAdminArticles(query: { page?: number; limit?: number; key?: string; status?: number }) {
  return api().request<ArticleSearchResponseData>('/search/articles', {
    query: {
      type: 5,
      page_mode: 'count',
      sort: 1,
      ...query,
    },
  })
}

export function setArticleAdminVisibility(articleId: string, visibility: 'hide' | 'show') {
  return api().request<unknown>(`/articles/${articleId}/admin/${visibility}`, { method: 'POST' })
}

export function deleteAdminArticles(id_list: string[]) {
  return api().request<unknown, { id_list: string[] }>('/articles', { method: 'DELETE', body: { id_list } })
}

export function getSiteRuntimeConfig() {
  return api().request<SiteRuntimeConfig>('/site/admin/site')
}

export function getAiRuntimeConfig() {
  return api().request<AIRuntimeConfig>('/site/admin/ai')
}

export function updateSiteRuntimeConfig(payload: SiteRuntimeConfig) {
  return api().request<unknown, SiteRuntimeConfig>('/site/site', { method: 'PUT', body: payload })
}

export function updateAiRuntimeConfig(payload: AIRuntimeConfig) {
  return api().request<unknown, AIRuntimeConfig>('/site/ai', { method: 'PUT', body: payload })
}

export function getBanners(query: { page?: number; limit?: number; show?: boolean }) {
  return api().request<HasMoreList<BannerModel>>('/banners', { query })
}

export function createBanner(payload: { cover_image_id?: string | null; href?: string; show?: boolean }) {
  return api().request<unknown, typeof payload>('/banners', { method: 'POST', body: payload })
}

export function deleteBanners(id_list: string[]) {
  return api().request<unknown, { id_list: string[] }>('/banners', { method: 'DELETE', body: { id_list } })
}

export function getImages(query: { page?: number; limit?: number }) {
  return api().request<ImageListResponseData>('/images', { query })
}

export function deleteImages(id_list: string[]) {
  return api().request<unknown, { id_list: string[] }>('/images', { method: 'DELETE', body: { id_list } })
}

export function getGlobalNotifications(query: { type: 1 | 2; page?: number; limit?: number }) {
  return api().request<HasMoreList<GlobalNotifListItem>>('/global_notif', { query })
}

export function createGlobalNotification(payload: {
  title: string
  content: string
  icon?: string
  href?: string
  user_visible_rule?: number
  expire_time?: string | null
}) {
  return api().request<unknown, typeof payload>('/global_notif', { method: 'POST', body: payload })
}

export function deleteGlobalNotifications(id_list: string[]) {
  return api().request<unknown, { id_list: string[] }>('/global_notif', { method: 'DELETE', body: { id_list } })
}

export type LogKind = 'runtime' | 'action' | 'login' | 'cdc'

export function getLogs(kind: 'runtime', query: Record<string, unknown>): Promise<ListData<RuntimeLogRecord>>
export function getLogs(kind: 'action', query: Record<string, unknown>): Promise<ListData<ActionAuditRecord>>
export function getLogs(kind: 'login', query: Record<string, unknown>): Promise<ListData<LoginEventRecord>>
export function getLogs(kind: 'cdc', query: Record<string, unknown>): Promise<ListData<CdcEventRecord>>
export function getLogs(kind: LogKind, query: Record<string, unknown>) {
  return api().request(`/logs/${kind}`, { query })
}

// ── 死信（DLQ）与回放日志 ─────────────────────────────────────

export function getDeadLetters(query: {
  page?: number
  limit?: number
  stream?: string
  status?: string
  cdc_job_id?: string
}) {
  return api().request<DeadLetterListData>('/logs/dead-letter', { query })
}

/**
 * 重投若干条死信。
 *
 * 逐条返回结果（success / failed / skipped / unsupported），一条失败不影响同批其他记录。
 */
export function replayDeadLetters(idList: string[]) {
  return api().request<ListData<DeadLetterReplayResult>, { id_list: string[] }>('/logs/dead-letter/replay', {
    method: 'POST',
    body: { id_list: idList },
  })
}

/** 回放日志：每次重投尝试的时间线（含跳过与不支持）。 */
export function getReplayLogs(query: { page?: number; limit?: number; stream?: string; result?: string }) {
  return api().request<ListData<ReplayLogRecord>>('/logs/replay', { query })
}

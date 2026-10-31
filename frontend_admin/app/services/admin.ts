import type {
  AICallRecord,
  AIUsageResponseData,
  AIRuntimeConfig,
  AIProviderItem,
  AIProviderPayload,
  VendorItem,
  ActionAuditRecord,
  ArticleReviewTaskListResponseData,
  ArticleSearchResponseData,
  ArticleYearDataResponseData,
  BannerModel,
  BannerWritePayload,
  CdcEventRecord,
  DecoRuntimeConfig,
  GlobalNotifListItem,
  GrowthDataResponseData,
  HasMoreList,
  HotTopAdminItem,
  DeadLetterListData,
  DeadLetterReplayResult,
  ImageListResponseData,
  ListData,
  ReplayLogRecord,
  LoginEventRecord,
  RuntimeLogRecord,
  LogRuntimeConfig,
  MailRuntimeConfig,
  OAuthRuntimeConfig,
  SiteRuntimeConfig,
  StorageRuntimeConfig,
  SumResponseData,
  UploadRuntimeConfig,
  UserListResponseData,
} from '~/types/api'

/** 可读写的配置段。与后端 platform/config 的段名常量一一对应（八段）。 */
export type RuntimeSectionName =
  | 'site'
  | 'ai'
  | 'mail'
  | 'storage'
  | 'oauth'
  | 'log'
  | 'upload'
  | 'deco'

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

// ── 热门置顶（运营位，ADR-0017 决策 5）────────────────────────
//
// 唯一真相是 `article_hot_top_models`，与 ES 无关。后台两处消费：
// 文章管理列表的行内开关、热门置顶页的排序 / 清理。

/** 后台置顶序列：含序号与 stale 行（公开接口 `/articles/hot-top` 只在有数据时才返回）。 */
export function getAdminHotTop() {
  return api().request<ListData<HotTopAdminItem>>('/articles/hot-top/admin')
}

/** 置顶一篇文章（幂等：已置顶再调不会产生第二行）。 */
export function addArticleHotTop(articleId: string) {
  return api().request<unknown>(`/articles/${articleId}/admin/hot-top`, { method: 'POST' })
}

/** 取消置顶（幂等）；后端会把余下序号压实。 */
export function removeArticleHotTop(articleId: string) {
  return api().request<unknown>(`/articles/${articleId}/admin/hot-top`, { method: 'DELETE' })
}

/**
 * 全量替换置顶顺序。
 *
 * 传的是**完整**序列（失效项也算）：后端在提交集合与库里的集合不一致时直接拒绝，
 * 所以先在上移/下移后拿最新列表再提交，别只传变化的两条。
 */
export function updateHotTopOrder(articleIds: string[]) {
  return api().request<unknown, { article_ids: string[] }>('/articles/hot-top/order', {
    method: 'PATCH',
    body: { article_ids: articleIds },
  })
}

export function getSiteRuntimeConfig() {
  return api().request<SiteRuntimeConfig>('/site/admin/site')
}

export function getAiRuntimeConfig() {
  return api().request<AIRuntimeConfig>('/site/admin/ai')
}

/*
 * 模型配置列表。
 *
 * 列表接口的 data 是 {list, count}（后端用 kernel.ListData 包了一层），
 * 所以这里取 .list 再返回，页面拿到的就是数组。
 */
export function getAiVendors() {
  return api().request<ListData<VendorItem>>('/ai/providers/vendors').then(res => res.list)
}

export function getAiProviders() {
  return api().request<ListData<AIProviderItem>>('/ai/providers').then(res => res.list)
}

export function createAiProvider(payload: AIProviderPayload) {
  return api().request<AIProviderItem, AIProviderPayload>('/ai/providers', {
    method: 'POST',
    body: payload,
  })
}

export function updateAiProvider(id: string, payload: AIProviderPayload) {
  return api().request<unknown, AIProviderPayload>(`/ai/providers/${id}`, {
    method: 'PUT',
    body: payload,
  })
}

export function enableAiProvider(id: string) {
  return api().request<unknown>(`/ai/providers/${id}/enable`, { method: 'POST' })
}

export function removeAiProvider(id: string) {
  return api().request<unknown>(`/ai/providers/${id}`, { method: 'DELETE' })
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

export function createBanner(payload: BannerWritePayload) {
  return api().request<unknown, BannerWritePayload>('/banners', { method: 'POST', body: payload })
}

/** 更新轮播（取景就靠它立即生效）。整体替换：href / show 也必须一起带上。 */
export function updateBanner(id: string, payload: BannerWritePayload) {
  return api().request<unknown, BannerWritePayload>(`/banners/${id}`, { method: 'PUT', body: payload })
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

export type LogKind = 'runtime' | 'action' | 'login' | 'cdc' | 'ai'

export function getLogs(kind: 'runtime', query: Record<string, unknown>): Promise<ListData<RuntimeLogRecord>>
export function getLogs(kind: 'action', query: Record<string, unknown>): Promise<ListData<ActionAuditRecord>>
export function getLogs(kind: 'login', query: Record<string, unknown>): Promise<ListData<LoginEventRecord>>
export function getLogs(kind: 'cdc', query: Record<string, unknown>): Promise<ListData<CdcEventRecord>>
export function getLogs(kind: 'ai', query: Record<string, unknown>): Promise<ListData<AICallRecord>>
export function getLogs(kind: LogKind, query: Record<string, unknown>) {
  return api().request(`/logs/${kind}`, { query })
}

/** AI token 用量趋势：今日/本周/本月/本年共用这一个接口，只换 range。 */
export function getAIUsage(range: 'today' | 'week' | 'month' | 'year') {
  return api().request<AIUsageResponseData>('/data/ai-usage', { query: { range } })
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

// ── 其余配置段 ────────────────────────────────────────────────
//
// 八个段共用同一条路由（`GET /site/admin/{name}` / `PUT /site/{name}`），
// 请求体与响应结构随 name 变。所以这里用泛型包一层，再给出具名包装 ——
// 具名的那几个让页面里的调用点能一眼看出在读写哪一段。

export function getRuntimeSection<T>(name: RuntimeSectionName) {
  return api().request<T>(`/site/admin/${name}`)
}

export function updateRuntimeSection<T>(name: RuntimeSectionName, payload: T) {
  return api().request<unknown, T>(`/site/${name}`, { method: 'PUT', body: payload })
}

export function getMailRuntimeConfig() {
  return getRuntimeSection<MailRuntimeConfig>('mail')
}

export function updateMailRuntimeConfig(payload: MailRuntimeConfig) {
  return updateRuntimeSection('mail', payload)
}

export function getStorageRuntimeConfig() {
  return getRuntimeSection<StorageRuntimeConfig>('storage')
}

export function updateStorageRuntimeConfig(payload: StorageRuntimeConfig) {
  return updateRuntimeSection('storage', payload)
}

export function getOAuthRuntimeConfig() {
  return getRuntimeSection<OAuthRuntimeConfig>('oauth')
}

export function updateOAuthRuntimeConfig(payload: OAuthRuntimeConfig) {
  return updateRuntimeSection('oauth', payload)
}

export function getLogRuntimeConfig() {
  return getRuntimeSection<LogRuntimeConfig>('log')
}

export function updateLogRuntimeConfig(payload: LogRuntimeConfig) {
  return updateRuntimeSection('log', payload)
}

export function getUploadRuntimeConfig() {
  return getRuntimeSection<UploadRuntimeConfig>('upload')
}

export function updateUploadRuntimeConfig(payload: UploadRuntimeConfig) {
  return updateRuntimeSection('upload', payload)
}

/** 装饰段（第 8 段 deco）：光标挂件的开关与八个数值参数。 */
export function getDecoRuntimeConfig() {
  return getRuntimeSection<DecoRuntimeConfig>('deco')
}

export function updateDecoRuntimeConfig(payload: DecoRuntimeConfig) {
  return updateRuntimeSection('deco', payload)
}

import type {
  AICallRecord,
  AIUsageResponseData,
  AIRuntimeConfig,
  AIProviderItem,
  AIProviderPayload,
  VendorItem,
  AdminSidebarCardsResponseData,
  SidebarCardsSavePayload,
  ActionAuditRecord,
  ArticleReviewBatchRequest,
  ArticleReviewBatchResponse,
  ArticleReviewHandleRequest,
  ArticleReviewReasonItem,
  ArticleReviewTaskListResponseData,
  ArticleSearchResponseData,
  ArticleYearDataResponseData,
  BannerModel,
  BannerWritePayload,
  CdcEventRecord,
  CollectionAdminCard,
  CollectionCreatePayload,
  CollectionUpdatePayload,
  DecoRuntimeConfig,
  GlobalNotifListItem,
  GrowthDataResponseData,
  HasMoreList,
  HotTopAdminItem,
  DeadLetterListData,
  DeadLetterReplayResult,
  ImageListResponseData,
  ListData,
  CollectionOutlineSavePayload,
  CollectionOutlineTreeResponseData,
  OutlineNodePayload,
  ProjectAdminCard,
  ProjectOutlineSavePayload,
  ProjectOutlineTreeResponseData,
  ProjectCreatePayload,
  ProjectUpdatePayload,
  ReplayLogRecord,
  LoginEventRecord,
  RuntimeLogRecord,
  LogRuntimeConfig,
  MailRuntimeConfig,
  OAuthRuntimeConfig,
  SiteRuntimeConfig,
  StorageRuntimeConfig,
  SumResponseData,
  TagListItem,
  TagWritePayload,
  UploadRuntimeConfig,
  UserListResponseData,
} from '~/types/api'

/** 可读写的配置段。与后端段注册表（platform/config/registry.go）的九段一一对应（ADR-0029）。 */
export type RuntimeSectionName =
  | 'site'
  | 'ai'
  | 'mail'
  | 'storage'
  | 'oauth'
  | 'log'
  | 'upload'
  | 'deco'
  | 'image_gc'

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

export function getArticleReviewTasks(query: {
  page?: number
  limit?: number
  status?: string
  // 风险排序由服务端做（风险不落库，前端只能收到已经排好序的一页）。
  sort?: 'created_at' | 'risk'
}) {
  return api().request<ArticleReviewTaskListResponseData>('/article-review', { query })
}

export function reviewArticleTask(id: string, payload: ArticleReviewHandleRequest) {
  return api().request<unknown, ArticleReviewHandleRequest>(`/article-review-tasks/${id}/review`, {
    method: 'POST',
    body: payload,
  })
}

/**
 * 批量处理审核任务（spec 决策 32）。
 *
 * 与单条接口同一份后端用例：批量不是第二套规则。响应里的 `failed` 是**正常
 * 结局**——某条任务可能已被别人处理，不该让整批失败。
 */
export function batchReviewArticleTasks(ids: string[], payload: Omit<ArticleReviewBatchRequest, 'ids'>) {
  return api().request<ArticleReviewBatchResponse, ArticleReviewBatchRequest>(
    '/article-review-tasks/batch-review',
    {
      method: 'POST',
      body: { ids, ...payload },
    },
  )
}

/**
 * 驳回原因的固定列表（值 + 文案由后端唯一词表下发）。
 *
 * 前端不手抄中文：渲染 label、提交 value，两边都不认识中文（#308 验收标准）。
 */
export function getArticleReviewReasons() {
  return api()
    .request<ListData<ArticleReviewReasonItem>>('/article-review/reasons')
    .then(res => res.list ?? [])
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

// ── 标签（全站词库，仅管理员可写）─────────────────────────────
//
// 后台管理页要看见停用项（否则没法重新启用它们），所以列表固定带
// include_disabled=true。发布页的下拉是另一条读面（`/articles/tags/options`，
// 只含启用项），别把这两个混用。

/** 后台标签列表：含停用项，文章数按关联表实时统计。 */
export function getAdminTags() {
  return api().request<ListData<TagListItem>>('/articles/tags', {
    query: { include_disabled: true },
  })
}

/**
 * 新建 / 更新标签。
 *
 * 更新是读—改—写：`sort` / `description` / `is_enabled` 省略即保持原值。
 * 后台编辑弹窗总会带上这四栏的当前值，所以"只改动某一栏"在线上表现为只变了那一栏。
 */
export function saveAdminTag(payload: TagWritePayload) {
  return api().request<unknown, TagWritePayload>('/articles/tags', { method: 'PUT', body: payload })
}

/**
 * 删除标签（只对零引用标签开放）。
 *
 * 被文章引用时后端拒绝并给出可读文案（"该标签已有 N 篇文章在用，请改用停用"）：
 * 调用方把 `error.message` 原样展示，不要用一句笼统的"删除失败"盖掉它。
 */
export function deleteAdminTags(idList: string[]) {
  return api().request<unknown, { id_list: string[] }>('/articles/tags', {
    method: 'DELETE',
    body: { id_list: idList },
  })
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

// ── 合集（管理员编排的文章集合）───────────────────────────────

/** 后台合集列表：含未展示的合集、排序号与回退封面。 */
export function getAdminCollections() {
  return api().request<ListData<CollectionAdminCard>>('/collections/admin')
}

/** 新建合集。`displayed` 显式传：合集允许"先建好、不展示"。 */
export function createCollection(payload: CollectionCreatePayload) {
  return api().request<{ id: string }, CollectionCreatePayload>('/collections', { method: 'POST', body: payload })
}

/** 改合集内容（标题 / 描述 / 封面）。展示开关走 setCollectionDisplay。 */
export function updateCollection(id: string, payload: CollectionUpdatePayload) {
  return api().request<unknown, CollectionUpdatePayload>(`/collections/${id}`, { method: 'PUT', body: payload })
}

/** 删合集（软删，不删文章）。 */
export function deleteCollection(id: string) {
  return api().request<unknown>(`/collections/${id}`, { method: 'DELETE' })
}

/** 切换合集是否展示给读者。 */
export function setCollectionDisplay(id: string, displayed: boolean) {
  return api().request<unknown, { displayed: boolean }>(`/collections/${id}/display`, {
    method: 'PATCH',
    body: { displayed },
  })
}

// ── 项目（与合集并列的独立内容实体）───────────────────────────

/** 后台项目列表：含未展示的项目、排序号与回退封面。 */
export function getAdminProjects() {
  return api().request<ListData<ProjectAdminCard>>('/projects/admin')
}

/** 新建项目。`displayed` 显式传：项目允许"先建好、不展示"。 */
export function createProject(payload: ProjectCreatePayload) {
  return api().request<{ id: string }, ProjectCreatePayload>('/projects', { method: 'POST', body: payload })
}

/** 改项目内容（标题 / 描述 / 封面）。展示开关走 setProjectDisplay。 */
export function updateProject(id: string, payload: ProjectUpdatePayload) {
  return api().request<unknown, ProjectUpdatePayload>(`/projects/${id}`, { method: 'PUT', body: payload })
}

/** 删项目（软删，不删文章）。 */
export function deleteProject(id: string) {
  return api().request<unknown>(`/projects/${id}`, { method: 'DELETE' })
}

/** 切换项目是否展示给读者。 */
export function setProjectDisplay(id: string, displayed: boolean) {
  return api().request<unknown, { displayed: boolean }>(`/projects/${id}/display`, {
    method: 'PATCH',
    body: { displayed },
  })
}

// ── 目录树（一个编辑器服务合集与项目）─────────────────────────────
//
// 收录 / 顺序 / 层次 / 失效清理全部走整树提交：两个实体各有自己的具名端点
// 与具名树表；旧共享端点与旧共用表已随 #277 一起下线，没有兼容层。

/**
 * 后台读回某个合集的整棵目录树（含空文件夹与失效标记）。
 *
 * #275 迁移：改走合集具名端点 `GET /collections/<id>/outline-tree`。它是公开读
 * 端点，管理员例外靠请求里携带的身份判定（默认 `auth` 不能关）——不带令牌会被
 * 当成匿名读者，未展示的合集读成空树，下一次整树提交就会把整棵树软删掉。
 */
export function getCollectionOutline(id: string) {
  return api().request<CollectionOutlineTreeResponseData>(`/collections/${id}/outline-tree`)
}

/** 整棵树一次性提交到合集具名树（新建文件夹 / 批量收录 / 层级与顺序一起落库）。 */
export function saveCollectionOutline(id: string, nodes: OutlineNodePayload[]) {
  return api().request<CollectionOutlineTreeResponseData, CollectionOutlineSavePayload>(
    `/collections/${id}/outline-tree`,
    { method: 'PUT', body: { nodes } },
  )
}

/** 后台读回某个项目的整棵目录树（#276：项目具名端点的管理员读面）。 */
export function getProjectOutline(id: string) {
  return api().request<ProjectOutlineTreeResponseData>(`/projects/${id}/outline-tree`)
}

/** 整棵树一次性提交到项目具名端点（#276：写 project_outline_node_models）。 */
export function saveProjectOutline(id: string, nodes: OutlineNodePayload[]) {
  return api().request<ProjectOutlineTreeResponseData, ProjectOutlineSavePayload>(
    `/projects/${id}/outline-tree`,
    { method: 'PUT', body: { nodes } },
  )
}

export function getSiteRuntimeConfig() {  return api().request<SiteRuntimeConfig>('/site/admin/site')
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

/** 管理端右栏卡片整表（含停用，按运营顺序）。 */
export function getAdminSidebarCards() {
  return api().request<AdminSidebarCardsResponseData>('/site/admin/sidebar_cards')
}

/**
 * 整表保存右栏卡片。
 *
 * 不是逐条增删：列表顺序就是读者侧顺序，后端据此把顺序号重排成连续的
 * 0..n-1；停用项必须一起提交，否则会被这次整表替换物理删掉。
 */
export function updateAdminSidebarCards(payload: SidebarCardsSavePayload) {
  return api().request<unknown, SidebarCardsSavePayload>('/site/admin/sidebar_cards', {
    method: 'PUT',
    body: payload,
  })
}

export function getBanners(query: { page?: number; limit?: number; show?: boolean }) {
  return api().request<HasMoreList<BannerModel>>('/banners', { query })
}

export function createBanner(payload: BannerWritePayload) {
  return api().request<unknown, BannerWritePayload>('/banners', { method: 'POST', body: payload })
}

/** 更新轮播（改标题等字段立即生效）。整体替换：href / show / 标题也必须一起带上。 */
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
// 九段各有具名端点（`GET /site/admin/<段名>` / `PUT /site/<段名>`），
// 请求体与响应形状按段固定（ADR-0029）。这里仍用泛型包一层再给出具名包装，
// 让页面里的调用点能一眼看出在读写哪一段；类型由生成的 openapi.d.ts 派生。

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

/** 装饰段（九段里的 deco）：光标挂件的开关与十二个数值参数，形状来自具名契约。 */
export function getDecoRuntimeConfig() {
  return getRuntimeSection<DecoRuntimeConfig>('deco')
}

export function updateDecoRuntimeConfig(payload: DecoRuntimeConfig) {
  return updateRuntimeSection('deco', payload)
}

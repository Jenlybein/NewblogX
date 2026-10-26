/**
 * 前端视图模型 / 适配层。
 *
 * 2026-10-26 起的原则：**能对上契约的类型一律从契约派生**，本文件只放契约里没有的东西。
 * 每个留在这里的类型都要能回答「为什么契约里没有它」，答不上来的就该迁走。
 *
 * 派生方式见 `./contract.ts`：响应体用 `RequiredSchema<"...">`（递归去掉可选），
 * 请求体用 `Schema<"...">`。
 */
import type { Schema, RequiredSchema } from "./contract";

// ── 纯 UI 形状：不对应任何 HTTP 接口 ─────────────────────────────

/** 统一响应外壳。各接口的 data 由 `Schema<>` 取，外壳本身不进契约。 */
export interface ApiEnvelope<T> {
  code: number
  msg: string
  data: T
}

/** 本地列表泛型：契约里是 `Envelope-...-ListData-...` 这种超长实例名，这里只保留载荷部分。 */
export interface HasMoreList<T> {
  list: T[]
  has_more: boolean
}

/** 本地列表泛型（带总数）。 */
export interface LogListData<T> {
  list: T[]
  count: number
}

// ── 从契约派生 ────────────────────────────────────────────────

export type DateCountItem = RequiredSchema<"internal_analytics_adapter_http.DateCountItem">;

export type SumResponseData = RequiredSchema<"internal_analytics_adapter_http.SumResponse">;

export type GrowthDataResponseData = RequiredSchema<"internal_analytics_adapter_http.GrowthDataResponse">;

export type ArticleYearDataResponseData = RequiredSchema<"internal_analytics_adapter_http.ArticleYearDataResponse">;

export type UserDetailResponseData = RequiredSchema<"myblogx_internal_user_domain.User">;

export type ArticleReviewTaskItem = RequiredSchema<"internal_article_adapter_http.ArticleReviewTaskItem">;

export type ArticleReviewTaskListResponseData = RequiredSchema<"internal_article_adapter_http.ArticleReviewTaskListResponse">;

export type BannerModel = RequiredSchema<"internal_site_adapter_http.BannerItem">;

/** 全局通知项。契约里的 `herf` 拼写错误字段前端从未使用，故以契约（`href`）为准。 */
export type GlobalNotifListItem = RequiredSchema<"internal_notification_adapter_http.GlobalListResponse">;

export type RuntimeLogRecord = RequiredSchema<"myblogx_internal_analytics_adapter_clickhouse.RuntimeLogRecord">;

export type ActionAuditRecord = RequiredSchema<"myblogx_internal_analytics_adapter_clickhouse.ActionAuditRecord">;

export type LoginEventRecord = RequiredSchema<"myblogx_internal_analytics_adapter_clickhouse.LoginEventRecord">;

export type CdcEventRecord = RequiredSchema<"myblogx_internal_analytics_adapter_clickhouse.CdcEventRecord">;

export type AnyLogRecord = RuntimeLogRecord | ActionAuditRecord | LoginEventRecord | CdcEventRecord;

export type CreateImageUploadTaskRequest = Schema<"internal_media_adapter_http.CreateTaskRequest">;

export type CreateImageUploadTaskResponseData = RequiredSchema<"internal_media_adapter_http.CreateTaskResponse">;

export type UploadTaskStatusResponseData = RequiredSchema<"internal_media_adapter_http.TaskStatusResponse">;

export type UserListResponseData = LogListData<UserListItem>

// ── 保留手写的部分（每条都写了理由） ──────────────────────────

/**
 * 用户列表项。
 *
 * 后端 `AdminUserItemResponse` 现已包含 `role` / `status`（之前没有，
 * 导致列表「角色」列恒空、编辑弹窗却能改）。类型因此可以纯派生自契约。
 */
export type UserListItem = RequiredSchema<"internal_user_adapter_http.AdminUserItemResponse">;
/**
 * 搜索文章项。
 *
 * 之前这里用 `Omit<..., "visibility_status">` 把枚举放宽回三值，因为契约里
 * `myblogx_internal_search_domain.VisibilityStatus` 只声明了 `"visible"`。
 * 根因是搜索模块自己重新声明了一份单值枚举；现已改为别名到
 * `kernel/enum.ArticleVisibilityStatus`，契约因此有三个值，这里不再需要放宽。
 */
export type SearchArticleItem = RequiredSchema<"myblogx_internal_search_domain.Item">;
export type SearchPagination = RequiredSchema<"myblogx_internal_search_domain.Pagination">;

export type ArticleSearchResponseData = Omit<
  RequiredSchema<"myblogx_internal_search_domain.Response">,
  "list"
> & {
  list: SearchArticleItem[]
}

/**
 * 站点运行时配置。
 *
 * 契约里是 `/site/{name}` 按 name 返回不同配置块（`kernel.SiteConfigData` 是
 * `map[string]any`），没有固定字段；而管理端需要把多块配置拼成一个对象来编辑，
 * 所以这个类型是**前端组装的视图模型**，不对应任何单一响应。
 */
export interface SiteRuntimeConfig {
  site_info: {
    title: string
    logo: string
    beian: string
    mode: number
  }
  project: {
    title: string
    icon: string
    web_path: string
  }
  seo: {
    keywords: string
    description: string
  }
  about: Record<string, string>
  login: Record<string, boolean | number>
  index_right: { list: unknown[] }
  article: { skip_examining: boolean }
  comment: { skip_examining: boolean }
}

/** AI 运行时配置。同上：由多块 `/site/{name}` 拼装，契约里没有对应 schema。 */
export interface AIRuntimeConfig {
  enable: boolean
  secret: string
  base_url: string
  chat_model: string
  reason_model: string
  timeout_sec: number
  max_input_chars: number
  temperature: number
  daily_quota: number
  abstract: string
  nickname: string
  avatar: string
}

/** 媒体库列表项。后端返回的是原始对象（无固定 schema），加索引签名以便透传扩展字段。 */
export interface ImageListItem {
  id?: string
  url?: string
  path?: string
  name?: string
  status?: string
  created_at?: string
  [key: string]: unknown
}

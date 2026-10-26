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
 * **契约里没有 `role` / `status`**：后端 `AdminUserItemResponse` 只返回
 * `id/nickname/avatar/username/created_at/ip/addr/last_login_at`，
 * 而 `users/index.vue` 渲染了「角色」列并支持编辑它 —— 于是这一列**永远是空的**。
 * 这是契约与页面的真实不一致，需要在后端补字段或前端撤掉该列之间做产品决策，
 * 所以这里如实标注为「后端不返回」，不擅自改成 `RequiredSchema` 把缺口藏起来。
 */
export type UserListItem = RequiredSchema<"internal_user_adapter_http.AdminUserItemResponse"> & {
  /** 后端当前不返回，恒为 undefined。 */
  role?: number
  /** 后端当前不返回，恒为 undefined。 */
  status?: number
}

/**
 * 搜索文章项。
 *
 * **不能直接用契约**：`myblogx_internal_search_domain.Item.visibility_status`
 * 的枚举只有 `"visible"`（搜索引擎只索引可见文章），而文章管理页要按
 * `"admin_hidden"` 判断是否显示「恢复」按钮。直接派生会让那几处比较编译不过。
 *
 * 这暴露了一个真实缺陷：**管理端文章列表用的是 `/search/articles`，而它永远只返回
 * 可见文章，所以「恢复已隐藏文章」这个流程根本走不到。** 需要先定方案
 * （给管理端单独的文章列表接口，或让搜索对管理员返回全部），再改类型。
 */
export type SearchArticleItem = Omit<
  RequiredSchema<"myblogx_internal_search_domain.Item">,
  "visibility_status"
> & {
  visibility_status: "visible" | "user_hidden" | "admin_hidden"
}

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

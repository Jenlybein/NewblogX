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

/** 本地列表泛型（带总数）：对应 `kernel.ListData`，即 `res.OkWithList` 的形状。 */
export interface ListData<T> {
  list: T[]
  count: number
}

// ── 从契约派生 ────────────────────────────────────────────────

export type DateCountItem = RequiredSchema<"internal_analytics_adapter_http.DateCountItem">;

export type SumResponseData = RequiredSchema<"internal_analytics_adapter_http.SumResponse">;

export type GrowthDataResponseData = RequiredSchema<"internal_analytics_adapter_http.GrowthDataResponse">;

export type ArticleYearDataResponseData = RequiredSchema<"internal_analytics_adapter_http.ArticleYearDataResponse">;

/**
 * 当前登录用户详情（`GET /users/detail`）。
 *
 * 之前这里写的是 `RequiredSchema<"myblogx_internal_user_domain.User">` —— 那是用户**实体**，
 * 而该接口返回的是用例层的 `app.DetailView`。之所以没报错，是因为 `domain.User`
 * 当时还被 `/users`（管理端列表）那批**死代码**的注解引用着、恰好留在契约里。
 * 删掉那批幽灵端点后 `domain.User` 从契约消失，这里立刻编译不过，才暴露出接错了对象。
 */
export type UserDetailResponseData = RequiredSchema<"myblogx_internal_user_app.DetailView">;

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

export type UserListResponseData = ListData<UserListItem>

// ── 死信（DLQ）与回放日志 ──────────────────────────────────────

/** 一条死信：某次同步失败留下的证据。字段与 `cdc_dead_letter` 表一一对应。 */
export type DeadLetterItem = RequiredSchema<"internal_analytics_adapter_http.DeadLetterItemResponse">;

export type DeadLetterListData = ListData<DeadLetterItem>;

/** 重投单条结果。result 取值 success / failed / skipped / unsupported。 */
export type DeadLetterReplayResult = RequiredSchema<"internal_analytics_adapter_http.DeadLetterReplayResultResponse">;

/** 回放日志（ClickHouse replay_event_logs）：每次重投尝试的时间线。 */
export type ReplayLogRecord = RequiredSchema<"myblogx_internal_analytics_adapter_clickhouse.ReplayEventRecord">;

export type ImageListItem = RequiredSchema<"internal_media_adapter_http.ImageItem">;

/** 图片列表载荷。后端 `res.OkWithList` 返回 `{list, count}`，**没有 `has_more`**。 */
export type ImageListResponseData = ListData<ImageListItem>;

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

/**
 * 邮件（腾讯云 SES）运行时配置。
 *
 * secret_key 由后端用 `******` 遮蔽；保存时原样回传即表示"没改"。
 * secret_id 不遮蔽 —— 它是"用哪个密钥"，不是密钥本身，
 * 遮起来管理员没法确认自己正在改哪个账号。
 */
export interface MailRuntimeConfig {
  secret_id: string
  secret_key: string
  /** 只支持 ap-guangzhou / ap-hongkong */
  region: string
  /** 国际站账号需要填 ses.intl.tencentcloudapi.com，留空用默认 */
  endpoint: string
  from_address: string
  from_name: string
  /** 控制台里的模板 ID；模板变量为 action / code / timeout */
  template_id: number
  timeout_sec: number
}

/** 对象存储（七牛云）运行时配置。secret_key 同样以占位符回显。 */
export interface StorageRuntimeConfig {
  enable: boolean
  access_key: string
  secret_key: string
  bucket: string
  /** 资源访问域名 */
  uri: string
  region: string
  /** 对象 key 前缀；图片引用关系靠它从 URL 反解 */
  prefix: string
  /** 单张图片大小上限（MB） */
  size: number
  /** 上传凭证有效期（秒） */
  expiry: number
  callback_url: string
}

/** 第三方登录（QQ 互联）运行时配置。app_key 以占位符回显。 */
export interface OAuthRuntimeConfig {
  app_id: string
  app_key: string
  redirect: string
}

/**
 * 日志运行时配置。
 *
 * 后端只接受可改的那部分：app / dir / wal_dir / stdout_format 属于进程级
 * 配置，提交了也会被覆盖回当前值（日志写哪儿不该由网页决定）。
 */
export interface LogRuntimeConfig {
  app: string
  dir: string
  wal_dir: string
  level: string
  stdout_format: string
  request_log_enabled: boolean
  query_default_limit: number
  query_max_limit: number
  error: {
    capture_stack: boolean
    capture_min_level: string
    stack_max_bytes: number
    cause_chain_depth: number
  }
  trace: {
    enabled: boolean
    request_id_equals_trace_id: boolean
    inherit_from_gateway: boolean
    gateway_header_priority: string
  }
  cleanup: {
    enabled: boolean
    retention_days: number
    run_at: string
  }
}

/** 上传策略。白名单是整体替换，不是追加。 */
export interface UploadRuntimeConfig {
  whitelist: string[]
}

/** 首次初始化状态。 */
export interface SetupStatusData {
  /** 为真表示库里还没有管理员，初始化入口开放 */
  needs_admin: boolean
}

/** 创建第一个管理员的请求体。 */
export interface SetupAdminPayload {
  username: string
  password: string
  email?: string
  nickname?: string
}

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

/** 轮播取景参数：创建 / 更新请求里的 `crop`。 */
export type BannerCrop = RequiredSchema<"internal_site_adapter_http.BannerCrop">;

/**
 * 后台创建 / 更新轮播时使用的**完整**请求体。
 *
 * 契约把 `href` / `show` / `crop` 标成可选（为了兼容还没接取景的老调用方），
 * 但后台写入时三个字段总是显式带上；这里收成必填，免得页面漏传 `crop` 时
 * 悄悄退回居中。字段名仍全部派生自契约。
 */
export type BannerWritePayload = Required<Schema<"internal_site_adapter_http.BannerCreateRequest">> & {
  crop: BannerCrop;
};

/** 全局通知项。契约里的 `herf` 拼写错误字段前端从未使用，故以契约（`href`）为准。 */
export type GlobalNotifListItem = RequiredSchema<"internal_notification_adapter_http.GlobalListResponse">;

export type RuntimeLogRecord = RequiredSchema<"myblogx_internal_analytics_adapter_clickhouse.RuntimeLogRecord">;

export type ActionAuditRecord = RequiredSchema<"myblogx_internal_analytics_adapter_clickhouse.ActionAuditRecord">;

export type LoginEventRecord = RequiredSchema<"myblogx_internal_analytics_adapter_clickhouse.LoginEventRecord">;

export type AICallRecord = RequiredSchema<"myblogx_internal_analytics_adapter_clickhouse.AICallRecord">;

/** AI token 用量趋势：四个周期共用同一形状，只有 points 的内容随粒度变。 */
export type AIUsageResponseData = RequiredSchema<"myblogx_internal_analytics_adapter_clickhouse.AIUsageSummary">;

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

// ── 热门置顶（运营位）─────────────────────────────────────────

/**
 * `/articles/hot-top/admin` 的列表项。
 *
 * 后台唯一的置顶状态来源（文章管理列表的按钮状态也用它）：带排序号与失效标记，
 * 比公开接口多出 stale 行。**不要**改去读 ES 文档里的字段（ADR-0017 决策 5）。
 */
export type HotTopAdminItem = RequiredSchema<"internal_article_adapter_http.ArticleHotTopAdminItem">;

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
  login: SiteLoginRuntimeConfig
  index_right: { list: unknown[] }
  article: {
    skip_examining: boolean
    /**
     * 「发布与展示」三个开关（ADR-0017）。默认全开，由后端
     * `site_default_settings.yaml` 的 site.article 段补种，前端不另写默认值。
     * 判定在服务端（app 层与读过滤处），这里只负责编辑。
     */
    allow_non_admin_publish: boolean
    allow_non_admin_draft: boolean
    show_non_admin_articles: boolean
  }
  comment: { skip_examining: boolean }
}

/**
 * 站点配置里的「登录策略」子块。
 *
 * 它属于 site 段，所以保存时是和站点基础信息一起整段提交的——
 * 后台把它们放在同一个 Tab 里，避免两个组件各存一份 site 段互相覆盖。
 */
export interface SiteLoginRuntimeConfig {
  qq_login: boolean
  username_pwd_login: boolean
  email_login: boolean
  captcha: boolean
  /** 邮箱验证码有效期，单位：分钟 */
  email_code_timeout: number
  /** 登录失败统计窗口，单位：分钟 */
  login_fail_window_minute: number
  login_fail_user_max: number
  login_fail_ip_max: number
  /** 邮件发送限流窗口，单位：秒 */
  email_send_window_second: number
  email_send_per_email_max: number
  email_send_per_ip_max: number
}

/**
 * 站点级 AI 设置。同上：由 `/site/admin/ai` 返回，契约里没有对应 schema。
 *
 * 只保留"站点怎么用 AI"：总闸、展示信息、输入限制。
 * 用哪家模型、密钥、接口地址都在 ai_provider_models 表里，由
 * `/ai/providers` 系列接口单独管理。
 */
export interface AIRuntimeConfig {
  enable: boolean
  max_input_chars: number
  temperature: number
  timeout_sec: number
  daily_quota: number
  nickname: string
  avatar: string
  abstract: string
}

/** 一条模型配置（密钥字段已被后端遮蔽为 ******）。 */
export interface AIProviderItem {
  id: string
  name: string
  vendor: string
  model: string
  enabled: boolean
  fields: Record<string, string>
}

/** 供应商需要的单个配置项定义（后端下发，前端据此渲染表单）。 */
export interface VendorFieldItem {
  key: string
  label: string
  required: boolean
  /** 敏感字段：用密码框，回显时是占位符 */
  secret: boolean
  placeholder: string
  default: string
}

/** 供应商元数据。 */
export interface VendorItem {
  id: string
  name: string
  note: string
  fields: VendorFieldItem[]
}

/** 新增 / 修改模型配置的请求体。 */
export interface AIProviderPayload {
  name: string
  vendor: string
  model: string
  fields: Record<string, string>
  enabled: boolean
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

/**
 * 装饰配置（第 8 段 deco）。
 *
 * `charm` 下是一个总开关 + 十二个数值参数（站标尺寸、挂绳几何 / 物理、约束刚度、
 * 描边、静止轻晃、帧率上限与指针端偏移 X / Y）。
 * 段结构、路由与响应形状都不变（响应本来就是自由键值块）。
 */
export interface DecoRuntimeConfig {
  charm: {
    /** 光标挂件总开关；默认开。 */
    enable: boolean
    /** 站标显示尺寸，单位 px，合法区间 15–70。 */
    logo_size: number
    /** 挂绳长度，单位 px，合法区间 40–240。 */
    rope_length: number
    /** Verlet 链节点数，单位 节，合法区间 4–30（整数）。 */
    segments: number
    /** 描边起始宽度，单位 px，合法区间 1–6（保留一位小数）；收尾固定为 0.6 倍。 */
    line_width: number
    /** 重力加速度，单位 px/s²，合法区间 400–4000。 */
    gravity: number
    /** 每步速度保持系数，无量纲，合法区间 0.9–1；1 表示不衰减（可能持续振荡）。 */
    damping: number
    /** 距离约束刚度，无量纲，合法区间 0.2–1；1 最硬，越小越弹。 */
    stiffness: number
    /** 静止摆动频率，单位 Hz，合法区间 0–1.5；0 表示关掉轻晃。 */
    idle_sway_hz: number
    /** 静止摆动加速度幅度，单位 px/s²，合法区间 0–40；0 表示关掉轻晃（收敛后停帧）。 */
    idle_sway_amp: number
    /** 最高渲染帧率，单位 fps，合法区间 30–160（整数）；它只管每秒画几次，不影响绳子手感。 */
    fps_cap: number
    /** 指针端偏移水平分量，单位 px，合法区间 -24–24（整数，允许负值）；指针端节点 = 指针热点 + 它。 */
    pointer_offset_x: number
    /** 指针端偏移垂直分量，单位 px，合法区间 -24–24（整数，允许负值；负值往上偏）。 */
    pointer_offset_y: number
  }
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

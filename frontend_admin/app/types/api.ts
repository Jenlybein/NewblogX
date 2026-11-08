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

// 驳回原因的固定词表与驳回请求体：两者都从契约派生，前端不再手写副本
// （contracts.md §4：请求体用 `Schema<"...">`，响应体用 `RequiredSchema<"...">`）。
export type ArticleReviewReasonItem = RequiredSchema<"internal_article_adapter_http.ArticleReviewReasonItem">;

export type ArticleReviewHandleRequest = Schema<"internal_article_adapter_http.ArticleReviewHandleRequest">;

export type ArticleReviewBatchRequest = Schema<"internal_article_adapter_http.ArticleReviewBatchRequest">;

export type ArticleReviewBatchResponse = RequiredSchema<"internal_article_adapter_http.ArticleReviewBatchResponse">;

export type BannerModel = RequiredSchema<"internal_site_adapter_http.BannerItem">;

/**
 * 后台创建 / 更新轮播时使用的**完整**请求体。
 *
 * 契约把 `href` / `show` / `title` 标成可选（标题本来就允许空），
 * 但后台写入时四个字段总是显式带上；这里收成必填，免得页面漏传标题时
 * 悄悄把已有标题清成空串。字段名仍全部派生自契约。
 */
export type BannerWritePayload = Required<Schema<"internal_site_adapter_http.BannerCreateRequest">>;

/** 管理端一张右栏卡片：含停用项，带服务端口径的顺序号。 */
export type AdminSidebarCardItem = RequiredSchema<"internal_site_adapter_http.AdminSidebarCardItem">;

/** 管理端卡片列表载荷（含停用项，按运营顺序）。 */
export type AdminSidebarCardsResponseData = RequiredSchema<"internal_site_adapter_http.AdminSidebarCardsResponse">;

/**
 * 整表保存的请求体。
 *
 * 契约把单张卡的 `title` / `enabled` 标成可选，但后台每次保存都显式带全字段
 * （清空标题时提交空串，由后端回退默认标题），所以这里收成必填。
 * 契约里没有 `sort_order`：顺序由 `list` 的下标决定。
 */
export type SidebarCardsSavePayload = RequiredSchema<"internal_site_adapter_http.SidebarCardsSaveRequest">;

/**
 * 读者侧一张右栏卡片。
 *
 * 后台不消费读者接口，但拖动排序的验收要求把"后台顺序 → 读者渲染顺序"
 * 放在同一条用例里验证；读者渲染纯函数按这个形状取数。
 */
export type SiteSidebarCardItem = RequiredSchema<"internal_site_adapter_http.SidebarCardItem">;

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

// ── 合集（运营编排的文章集合）─────────────────────────────────

/**
 * 后台合集列表的一行。
 *
 * 带 `fallback_cover`：没上传封面时读者会看到的首篇封面。后台列表按读者的
 * 样子预览，但编辑表单必须能区分"我传了封面"与"这是回退来的"，所以两个字段
 * 在契约里是分开的，不合并成"生效封面"。
 */
export type CollectionAdminCard = RequiredSchema<"internal_article_adapter_http.CollectionAdminCard">;

/** 建合集的请求体。契约里除标题外都可选，后台写入时四个字段总是显式带上。 */
export type CollectionCreatePayload = Required<Schema<"internal_article_adapter_http.CollectionCreateRequest">>;

/** 改合集内容的请求体：只有标题 / 描述 / 封面，展示开关走独立接口。 */
export type CollectionUpdatePayload = Required<Schema<"internal_article_adapter_http.CollectionUpdateRequest">>;

/**
 * 后台项目列表的一行。
 *
 * 带 `fallback_cover`：没上传封面时读者会看到的首篇封面。后台列表按读者的
 * 样子预览，但编辑表单必须能区分"我传了封面"与"这是回退来的"。
 */
export type ProjectAdminCard = RequiredSchema<"internal_article_adapter_http.ProjectAdminCard">;

/** 建项目的请求体。契约里除标题外都可选，后台写入时四个字段总是显式带上。 */
export type ProjectCreatePayload = Required<Schema<"internal_article_adapter_http.ProjectCreateRequest">>;

/** 改项目内容的请求体：只有标题 / 描述 / 封面，展示开关走独立接口。 */
export type ProjectUpdatePayload = Required<Schema<"internal_article_adapter_http.ProjectUpdateRequest">>;

/** 项目之间排序的请求体：完整有序的项目 ID 列表。 */
export type ProjectOrderPayload = Required<Schema<"internal_article_adapter_http.ProjectOrderRequest">>;

// ── 目录树（合集 / 项目共用的一张树表）─────────────────────────
//
// 一个树编辑器服务两个实体：请求 / 响应形状完全相同，只有路由前缀不同
// （/collections/{id}/outline 与 /projects/{id}/outline）。类型因此也不按实体复制。

/**
 * 后台目录树的一行（嵌套）。
 *
 * 文件夹节点：`kind=1`、`article_id` 为空、`title` 是文件夹标题。
 * 文章节点：`kind=2`、`title` 为空，标题 / 封面读 `article_title` / `article_cover`。
 * `stale` 只对文章节点有意义：文章后来被删 / 退回草稿 / 隐藏时节点保留并标失效。
 */
export type OutlineNodeItem = RequiredSchema<"internal_article_adapter_http.OutlineNodeResponse">;

/** 后台读回的整棵嵌套树（含空文件夹）。 */
export type OutlineTreeResponseData = RequiredSchema<"internal_article_adapter_http.OutlineTreeResponse">;

/** 整树提交的一层节点。已有节点带 id，新节点 id 留空。 */
export type OutlineNodePayload = RequiredSchema<"internal_article_adapter_http.OutlineNodeRequest">;

// ── 具名形状（两个实体各有自己的契约 schema）─────────────────────
//
// 两端只从**本 app 的** openapi.d.ts 派生，不手写字段、不跨 app 共享。
// #277 收缩后旧共享端点与它的整树提交 schema 一起退役，后台只消费这两份形状。

/** 合集具名树的读模型（GET /collections/{id}/outline-tree）。 */
export type CollectionOutlineTreeResponseData = RequiredSchema<"internal_article_adapter_http.CollectionOutlineTreeResponse">;

/** 项目具名树的读模型（GET /projects/{id}/outline-tree）。 */
export type ProjectOutlineTreeResponseData = RequiredSchema<"internal_article_adapter_http.ProjectOutlineTreeResponse">;

/** 合集具名树整树提交的请求体。 */
export type CollectionOutlineSavePayload = Required<Schema<"internal_article_adapter_http.CollectionOutlineSaveRequest">>;

/** 项目具名树整树提交的请求体。 */
export type ProjectOutlineSavePayload = Required<Schema<"internal_article_adapter_http.ProjectOutlineSaveRequest">>;

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
 * 九段运行时配置：全部从生成的契约派生（#283 / ADR-0029）。
 *
 * 参数化 /site/{name} 与自由键值块已退场，每段各有具名响应/请求 schema；
 * 后台不再手写投影，字段名与类型都在编译期由 openapi.d.ts 给出。
 * 站点登录策略子块只是 site 段里的一个字段，不再是独立视图模型。
 */
export type SiteRuntimeConfig = RequiredSchema<"internal_site_adapter_http.SiteSectionSiteResponse">

// 发布质量闸门与投稿配额的配置形状（票 304 / 306）：从契约派生，
// 前端不再手写一份字段清单（contracts.md §4）。
export type QualityGateConfig = RequiredSchema<"myblogx_conf_site.QualityGate">;
export type SiteLoginRuntimeConfig = RequiredSchema<"myblogx_conf_site.Login">
export type AIRuntimeConfig = RequiredSchema<"internal_site_adapter_http.SiteSectionAiResponse">
export type MailRuntimeConfig = RequiredSchema<"internal_site_adapter_http.SiteSectionMailResponse">
export type StorageRuntimeConfig = RequiredSchema<"internal_site_adapter_http.SiteSectionStorageResponse">
export type OAuthRuntimeConfig = RequiredSchema<"internal_site_adapter_http.SiteSectionOauthResponse">
export type LogRuntimeConfig = RequiredSchema<"internal_site_adapter_http.SiteSectionLogResponse">
export type UploadRuntimeConfig = RequiredSchema<"internal_site_adapter_http.SiteSectionUploadResponse">
export type DecoRuntimeConfig = RequiredSchema<"internal_site_adapter_http.SiteSectionDecoResponse">
export type ImageGCRuntimeConfig = RequiredSchema<"internal_site_adapter_http.SiteSectionImageGcResponse">

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

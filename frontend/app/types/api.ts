import type { Nullable, OptionalFields, RequiredSchema, Schema } from "./contract";

export type ThemeMode = "light" | "dark";

export interface ApiEnvelope<T> {
  code: number;
  msg: string;
  data: T;
}

export type SiteSeoData = RequiredSchema<"internal_site_adapter_http.SEOResponse">;

/**
 * 站点运行时配置（`GET /api/site/site` 的 data）。
 *
 * 契约里这条 data 是 `map[string]any`，派生不出类型，只能手写 —— 手写就会漂移：
 * 这里曾声明后端不存在的 `site_info.subtitle` / `project.abstract`，又漏掉后端
 * 真有的 `site_info.logo` / `beian`。现在按 `backend/conf/site/enter.go` 对齐，
 * 并由 `test/gates/site-runtime-config-parity.test.ts` 逐字段比对，再漂就红。
 *
 * `site_info` 是完整投影（与 `site.SiteInfo` 逐字段相等）；`project` 等子块读者站
 * 只用得到一部分，所以是子集 —— 但同样不许出现后端没有的字段。
 */
export interface SiteRuntimeConfig {
  site_info: {
    title?: string;
    logo?: string;
    beian?: string;
    mode?: number;
  };
  project: {
    title?: string;
  };
  login: {
    qq?: boolean;
    email?: boolean;
    pwd?: boolean;
  };
  index_right: {
    list?: Array<{
      title: string;
      abstract?: string;
    }>;
  };
  article: {
    show_author?: boolean;
  };
  comment: {
    toggle?: boolean;
  };
}

export type SiteAiInfo = RequiredSchema<"internal_site_adapter_http.AIResponse">;

/**
 * 装饰配置（`GET /api/site/deco` 的 data，后端第 8 个运行时配置段）。
 *
 * 与 `SiteRuntimeConfig` 一样是手写的：契约里这条 data 是自由键值块，
 * 派生不出逐字段类型。按 `backend/conf/conf_deco.go` 对齐 ——
 * 本票只有 `charm.enable` / `charm.logo_size` 两个参数，
 * 挂绳长度、节点数等留给票 #41。
 *
 * 字段全都可选：读者站不能假设某个后台版本一定发出了哪些键，
 * 缺失时由 `clampCharmLogoSize` / 门控的默认值兜底。
 */
export interface SiteDecoConfig {
  charm?: {
    enable?: boolean;
    logo_size?: number;
  };
}

export type BannerItem = RequiredSchema<"internal_site_adapter_http.BannerItem">;

/** 轮播图列表载荷。之前服务层手写了同名形状（`{list, has_more}`），漏了契约里的 `count`。 */
export type BannerListData = RequiredSchema<"myblogx_internal_kernel.HasMoreListData-internal_site_adapter_http_BannerItem">;

export interface OptionItem {
  label: string;
  value: string;
}

export type CreateImageUploadTaskRequest = Schema<"internal_media_adapter_http.CreateTaskRequest">;

export type CreateImageUploadTaskResponseData = RequiredSchema<"internal_media_adapter_http.CreateTaskResponse">;

export type UploadTaskStatusResponseData = RequiredSchema<"internal_media_adapter_http.TaskStatusResponse">;

export type AiArticleMetainfo = RequiredSchema<"internal_ai_adapter_http.MetaInfoResponse">;

export type AiOverwriteMode = "polish" | "grammar_fix" | "style_transform";

export type AiOverwriteRequest = Schema<"internal_ai_adapter_http.OverwriteRequest">;

export type AiDiagnoseRequest = Schema<"internal_ai_adapter_http.DiagnoseRequest">;

export type AiDiagnoseIssueType = "可读性" | "逻辑" | "完整度" | "结构" | "重复" | "语言" | "语气";
export type AiDiagnoseSeverity = "低" | "中" | "高";

export interface AiDiagnoseIssue {
  type: AiDiagnoseIssueType;
  severity: AiDiagnoseSeverity;
  reason: string;
  evidence: string;
  suggestion: string;
}

export interface AiDiagnoseResponseData {
  summary: string;
  issues: AiDiagnoseIssue[];
}

export type AiArticleScoringRequest = Schema<"internal_ai_adapter_http.ScoringRequest">;

export type AiArticleScoringPosition = RequiredSchema<"myblogx_internal_ai_scoring.ArticleScorePosition">;


export type AiArticleScoringIssue = RequiredSchema<"myblogx_internal_ai_scoring.ArticleScoreIssue">;


export type AiArticleScoringResponseData = RequiredSchema<"internal_ai_adapter_http.ScoringResponse">;

export type FavoriteFolderItem = RequiredSchema<"internal_article_adapter_http.FavoriteListItem">;


export type FavoriteFolderListData = RequiredSchema<"myblogx_internal_kernel.ListData-internal_article_adapter_http_FavoriteListItem">;

/**
 * 收藏夹写操作（创建 / 更新共用）的请求体。
 *
 * 之前叫 `FavoriteFolderCreatePayload` 且被当成**返回值**用，还混进了
 * `article_count / is_default / user_id` 这类只属于响应的字段。
 * 后端 `PUT /articles/favorite` 实际只接收 `{id, title, cover, abstract}`，
 * 且**不返回任何数据**。
 */
export type FavoriteFolderWritePayload = Schema<"internal_article_adapter_http.FavoriteRequest">;

export type SearchArticleItem = RequiredSchema<"myblogx_internal_search_domain.Item">;

export type SearchArticleResponse = RequiredSchema<"myblogx_internal_search_domain.Response">;

/**
 * 首页「热门置顶」区块的一张卡片。
 *
 * 之前这里写着 12 个字段（含 tags / digg_count 等），但 `/articles/top` 实际
 * 返回的是**文章 ID 字符串数组**，于是模板里的 `article.id`、`article.cover`、
 * `article.title` 全是 undefined，整块区域是空的、链接指向 `/article/undefined`。
 * 现在后端改返回卡片，类型直接派生自契约。
 */
export type ArticleTopItem = RequiredSchema<"internal_article_adapter_http.ArticleTopCardResponse">;
// ArticleUpdateRequest 全是指针（PATCH 语义：null 表示清空该字段）。
export type ArticleWritePayload = Nullable<
  Schema<"internal_article_adapter_http.ArticleUpdateRequest">,
  "title" | "abstract" | "content" | "category_id" | "tag_ids" | "cover_image_id"
  | "comments_toggle"
>;

export type ArticleCreateResult = RequiredSchema<"internal_article_adapter_http.ArticleCreateResponse">;

export type ArticleDetail = Nullable<
  RequiredSchema<"internal_article_adapter_http.ArticleDetailResponse">,
  "category_id" | "cover_image_id" | "author_avatar_image_id"
>;

export type CommentRootItem = RequiredSchema<"myblogx_internal_comment_app.RootItem">;

export type CommentRootListData = RequiredSchema<"myblogx_internal_kernel.HasMoreListData-myblogx_internal_comment_app_RootItem">;


export type CommentReplyItem = RequiredSchema<"myblogx_internal_comment_app.ReplyItem">;


export type CommentReplyListData = RequiredSchema<"internal_comment_adapter_http.ReplyListResponse">;

/**
 * 用户主页的展示信息。
 *
 * `avatar_image_id` 必须放回可选：它在 Go 里是 `*ctype.ID` 且带 `omitempty`，
 * 空值时**字段整个不出现在 JSON 里**（不是 `null`）。这类"omitempty 指针"字段
 * 用 `RequiredSchema` 会导致构造对象字面量时被迫编造一个值。
 *
 * 曾经这里还挂着 `digg_count` / `favor_count` / `comment_count` 三个可选字段，
 * 用来迁就 `ProfileHeroCard` 的三项展示。但后端从来没有这三个累计值，
 * 那三项于是恒显示 0；现已撤掉 UI 与这三个字段，不再在类型里留缺口。
 */
export type UserBaseInfo = OptionalFields<
  RequiredSchema<"internal_user_adapter_http.BaseInfoResponse">,
  "avatar_image_id"
>

/**
 * 文章页右侧的作者卡。
 *
 * 后端现在返回 `{id, username, nickname, avatar, article_count,
 * article_visited_count, fans_count}`；之前只返回前四个，而页面在渲染
 * 「文章数 / 访问量 / 粉丝数」，所以那三项恒为 0。
 */
export type ArticleAuthorInfo = RequiredSchema<"internal_article_adapter_http.ArticleAuthorInfoResponse">;
export type UserSelfDetail = RequiredSchema<"myblogx_internal_user_app.DetailView">;

/**
 * 发送邮箱验证码的响应。
 *
 * 后端返回的是 `{"id": "..."}`（见 `auth/adapter/http/dto.go` 的 `SendEmailResponse`）。
 * 之前这里手写成 `{email_id, email}`，于是三个调用点读的 `payload.email_id` **恒为 undefined**，
 * 邮箱验证码登录/注册/绑定全部会卡在"请先发送验证码"。已按契约改成 `id`。
 */
export type EmailVerifyResponse = RequiredSchema<"internal_auth_adapter_http.SendEmailResponse">;

export type HistoryArticleItem = RequiredSchema<"internal_article_adapter_http.ArticleViewHistoryItem">;


export type HistoryArticleListData = RequiredSchema<"myblogx_internal_kernel.HasMoreListData-internal_article_adapter_http_ArticleViewHistoryItem">;


export type FollowUserItem = RequiredSchema<"myblogx_internal_social_domain.FollowingItem">;

export type FollowUserListData = RequiredSchema<"myblogx_internal_kernel.HasMoreListData-myblogx_internal_social_domain_FollowingItem">;

export type FanUserItem = RequiredSchema<"myblogx_internal_social_domain.FansItem">;

export type FanUserListData = RequiredSchema<"myblogx_internal_kernel.HasMoreListData-myblogx_internal_social_domain_FansItem">;

export type FavoriteArticleItem = RequiredSchema<"internal_article_adapter_http.FavoriteArticleListItem">;


export type FavoriteArticleListData = RequiredSchema<"myblogx_internal_kernel.HasMoreListData-internal_article_adapter_http_FavoriteArticleListItem">;


export type UserSessionListData = RequiredSchema<"myblogx_internal_kernel.ListData-internal_auth_adapter_http_SessionItem">;

export type MessageSummary = RequiredSchema<"internal_notification_adapter_http.UnreadResponse">;

export type MessagePreference = Schema<"internal_notification_adapter_http.ConfRequest">;

// deleted_at 只在软删后才有；操作人/已读时间可能为 null（Go 侧是指针）。
export type SiteMessageItem = Nullable<
  OptionalFields<RequiredSchema<"internal_notification_adapter_http.MessageItem">, "deleted_at">,
  "action_user_id" | "action_user_avatar" | "read_at"
>;

export type SiteMessageListData = RequiredSchema<"myblogx_internal_kernel.HasMoreListData-internal_notification_adapter_http_MessageItem">;

export type GlobalNoticeItem = RequiredSchema<"internal_notification_adapter_http.GlobalListResponse">;

export type GlobalNoticeListData = RequiredSchema<"myblogx_internal_kernel.HasMoreListData-internal_notification_adapter_http_GlobalListResponse">;

// 没有消息时后端返回 last_msg_time: null；deleted_at 只在软删后才有。
export type ChatSessionItem = OptionalFields<
  Nullable<RequiredSchema<"myblogx_internal_chat_domain.SessionItem">, "last_msg_time">,
  "deleted_at"
>;

export type ChatSessionListData = RequiredSchema<"myblogx_internal_kernel.ListData-myblogx_internal_chat_domain_SessionItem">;

// deleted_at 只在软删后才有，后端可能不返回。
export type ChatMessageItem = OptionalFields<
  RequiredSchema<"myblogx_internal_chat_domain.MessageItem">,
  "deleted_at"
>;

export type ChatMessageListData = RequiredSchema<"myblogx_internal_kernel.ListData-myblogx_internal_chat_domain_MessageItem">;

export type ChatWsTicketData = RequiredSchema<"internal_chat_adapter_http.WsTicketResponse">;

export interface ChatSocketEnvelope<TData = unknown> {
  code: number;
  msg: string;
  data: TData;
}

export interface ChatSocketOutgoingMessage {
  receiver_id: string;
  msg_type: 1 | 2 | 7;
  content: string;
}

// UpdateInfoRequest 的字段在 Go 里全是 *指针（`*string` / `*ctype.ID`），
// JSON 里可以是 null；但 swag 把指针印成非空类型，所以这里显式补 null。
export type UserProfileUpdatePayload = Nullable<
  Schema<"internal_user_adapter_http.UpdateInfoRequest">,
  "username" | "nickname" | "abstract" | "avatar_image_id" | "home_style_id"
>;

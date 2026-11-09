import type { Nullable, OptionalFields, RequiredSchema, Schema } from "./contract";

export type ThemeMode = "light" | "dark";

export interface ApiEnvelope<T> {
  code: number;
  msg: string;
  data: T;
}

export type SiteSeoData = RequiredSchema<"internal_site_adapter_http.SEOResponse">;

/** 站点运行时配置（GET /api/site/site 的 data）：整段从九段具名契约派生。 */
export type SiteRuntimeConfig = RequiredSchema<"internal_site_adapter_http.SiteSectionSiteResponse">;

export type SiteAiInfo = RequiredSchema<"internal_site_adapter_http.AIResponse">;

/**
 * 装饰配置（GET /api/site/deco 的 data，九段里的 deco 段）。
 *
 * 形状从九段具名契约派生；数值字段仍可能为 null（票 #77），由归一化函数回落
 * 默认值 —— null 与缺键同义，不能被 `Number(null) === 0` 读成"管理员填了 0"。
 */
export type SiteDecoConfig = {
  charm?: {
    [K in keyof RequiredSchema<"conf.Charm">]?: RequiredSchema<"conf.Charm">[K] | null;
  } | null;
};

export type BannerItem = RequiredSchema<"internal_site_adapter_http.BannerItem">;

/**
 * 右栏卡片（票 111）：种类、顺序、标题与各卡数据都从 `GET /site/sidebar_cards` 取。
 *
 * 形状直接由契约派生；`kind` 在契约里是 string（后端存的是字符串），
 * 前端用 `~/utils/sidebarCards` 的 `isSidebarCardKind` 收窄成内置种类。
 */
export type SiteSidebarCardItem = RequiredSchema<"internal_site_adapter_http.SidebarCardItem">;

/** 读者侧右栏卡片列表的 data 载荷。 */
export type SiteSidebarCardsData = RequiredSchema<"internal_site_adapter_http.SidebarCardsResponse">;

/** 轮播图列表载荷。之前服务层手写了同名形状（`{list, has_more}`），漏了契约里的 `count`。 */
export type BannerListData = RequiredSchema<"myblogx_internal_kernel.HasMoreListData-internal_site_adapter_http_BannerItem">;

/**
 * 下拉选项的纯 UI 形状（不对应任何接口）。
 *
 * 刻意用 `type` 而不是 `interface`：只有类型别名与匿名对象类型带隐式索引签名，
 * 才能赋给 naive-ui 那些 `[k: string]: unknown` 的选项类型；`interface` 没有隐式
 * 索引签名，直接喂给 `NSelect :options` 会报 "Property 'type' is missing"（#320）。
 */
export type OptionItem = {
  label: string;
  value: string;
};

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
 * 标签下拉的列表载荷（`GET /articles/tags/options`）。
 *
 * data 恒为 `kernel.ListData` 的 `{list, count}`；服务层曾把它当数组 `.map`，
 * 于是抛错又被兜底吞掉，下拉永远空白（#320）。
 */
export type TagListData = RequiredSchema<"myblogx_internal_kernel.ListData-internal_article_adapter_http_TagListItem">;

/** 分类下拉的列表载荷（`GET /articles/category/options?user_id=`），形状同上。 */
export type CategoryListData = RequiredSchema<"myblogx_internal_kernel.ListData-internal_article_adapter_http_CategoryListItem">;

/**
 * 收藏夹写操作（创建 / 更新共用）的请求体。
 *
 * 之前叫 `FavoriteFolderCreatePayload` 且被当成**返回值**用，还混进了
 * `article_count / is_default / user_id` 这类只属于响应的字段。
 * 后端 `PUT /articles/favorite` 实际只接收 `{id, title, cover, abstract}`，
 * 且**不返回任何数据**。
 */
export type FavoriteFolderWritePayload = Schema<"internal_article_adapter_http.FavoriteRequest">;

/**
 * 个人主页「分类」栏要显示的字段（#321）。
 *
 * 契约里 `internal_article_adapter_http.CategoryListItem` 还带 nickname / avatar /
 * created_at 等管理侧字段；分类栏只用得到 id / title / article_count。用 `Pick` 从契约
 * 派生而不是手写 interface —— 字段名与类型仍由契约编译期校验。
 */
export type ProfileCategoryItem = Pick<
  RequiredSchema<"internal_article_adapter_http.CategoryListItem">,
  "id" | "title" | "article_count"
>;

/** `GET /api/articles/category` 的 data：`{list, count}`，条目是 `CategoryListItem`。 */
export type ProfileCategoryListData =
  RequiredSchema<"myblogx_internal_kernel.ListData-internal_article_adapter_http_CategoryListItem">;

export type SearchArticleItem = RequiredSchema<"myblogx_internal_search_domain.Item">;

export type SearchArticleResponse = RequiredSchema<"myblogx_internal_search_domain.Response">;

/**
 * 首页「热门置顶」运营位的一张卡片（`GET /articles/hot-top`，公开读）。
 *
 * 与作者置顶（`/articles/top?user_id=`）的卡片形状相同，但数据源不同：
 * 前者是管理员挑选并手动排序的运营位，后者是作者钉在自己主页上的那一篇（ADR-0017）。
 */
export type ArticleHotTopItem = RequiredSchema<"internal_article_adapter_http.ArticleHotTopCardResponse">;

export type ArticleHotTopListData =
  RequiredSchema<"myblogx_internal_kernel.ListData-internal_article_adapter_http_ArticleHotTopCardResponse">;

/**
 * 合集（管理员编排的有序文章集合，读者侧只读）。
 *
 * 类型全部从契约派生：字段名/类型由 `internal_article_adapter_http.Collection*`
 * 决定，后端改名会立刻编译报错。合集与「热门置顶」是两套数据——前者有
 * 标题/描述/封面/展示开关，后者只是首页运营位。
 */
export type CollectionCard = RequiredSchema<"internal_article_adapter_http.CollectionCardResponse">;

export type CollectionListData =
  RequiredSchema<"myblogx_internal_kernel.ListData-internal_article_adapter_http_CollectionCardResponse">;

/** 合集详情：合集本身 + 嵌套目录树（票 219 起扁平 articles[] 退役）。 */
export type CollectionDetail = RequiredSchema<"internal_article_adapter_http.CollectionDetailResponse">;

/**
 * 项目（与合集并列的独立内容实体，读者侧只读）。
 *
 * 与合集的契约形状相同、但**是两套定义**：字段名/类型由
 * `internal_article_adapter_http.Project*` 决定，后端改名会立刻编译报错。
 * 项目有自己的表与后台维护页，不与合集共用数据。
 */
export type ProjectCard = RequiredSchema<"internal_article_adapter_http.ProjectCardResponse">;

export type ProjectListData =
  RequiredSchema<"myblogx_internal_kernel.ListData-internal_article_adapter_http_ProjectCardResponse">;

/** 项目详情：项目本身 + 嵌套目录树（票 219 起扁平 articles[] 退役）。 */
export type ProjectDetail = RequiredSchema<"internal_article_adapter_http.ProjectDetailResponse">;

// ── 具名树端点（#274 expand）────────────────────────────────────
//
// 两个具名形状都从**本 app 的** openapi.d.ts 派生，不手写、不跨 app 共享。
// 本票只让类型可用：详情页 / 阅读页左栏仍消费旧共享形状，切换归 #275 / #276。

/** 合集具名树读模型；current / previous / next 可为 null。 */
export type CollectionOutlineTree = Nullable<
  RequiredSchema<"internal_article_adapter_http.CollectionOutlineTreeResponse">,
  "current" | "previous" | "next"
>;

/** 项目具名树读模型；current / previous / next 可为 null。 */
export type ProjectOutlineTree = Nullable<
  RequiredSchema<"internal_article_adapter_http.ProjectOutlineTreeResponse">,
  "current" | "previous" | "next"
>;

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

/**
 * 点赞写响应（ADR-0024 决策 3/4）：收敛后的状态 + 权威 `digg_count`。
 */
export type ArticleDiggResult = RequiredSchema<"internal_article_adapter_http.ArticleDiggResponse">;

/**
 * 收藏写响应（ADR-0024 决策 4/6）：用工编辑（POST 收藏 / DELETE 移除）都回填
 * 权威 `is_favor` 与 `favor_count`，前端直接 patch。
 */
export type ArticleFavoriteResult = RequiredSchema<"internal_article_adapter_http.ArticleFavoriteResponse">;

/**
 * 批量取消收藏的写响应（评审票 248 / P0-2）。
 *
 * 与单条响应的区别：多出 `removed_count` / `failed_count`，部分失败不再被吞成
 * “全部成功”。全部失败时后端返回非 0 业务码，不会走到这个类型。
 */
export type ArticleFavoriteRemoveResult =
  RequiredSchema<"internal_article_adapter_http.ArticleFavoriteRemoveResponse">;

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

// 消息列表响应（list / count / has_more / next_cursor）从契约派生；
// 列表已是升序（最旧在前），前端不再反转。
export type ChatMessageListData = RequiredSchema<"myblogx_internal_chat_domain.MessageListData">;

export type ChatWsTicketData = RequiredSchema<"internal_chat_adapter_http.WsTicketResponse">;

/** POST /api/chat/messages 的请求体（契约派生，含幂等键 client_msg_id）。 */
export type ChatSendMessagePayload = RequiredSchema<"internal_chat_adapter_http.SendMessageRequest">;

export interface ChatSocketEnvelope<TData = unknown> {
  code: number;
  msg: string;
  data: TData;
}

/**
 * WebSocket 下行通知的载荷（#201：WS 只做通知，发送走 HTTP）。
 *
 * - 新消息通知：`chat.message.sent` → `{ message_id, sender_id, receiver_id, session_id, ... }`
 * - 已读回执：`chat.message.read` → `{ reader_id, session_id, msg_id_list, ... }`（msg_type=8）
 *
 * 两者都带 `session_id`，收到后前端刷新会话与未读；只刷新会话、不把正文塞进
 * 通知帧 —— 正文由接收方走 HTTP 重新拉取，避免两处真相。
 */
export interface ChatNotificationPayload {
  session_id: string;
  message_id?: string;
  sender_id?: string;
  receiver_id?: string;
  reader_id?: string;
  msg_id_list?: string[];
  msg_type?: number;
  send_time?: string;
  read_at?: string;
}

// UpdateInfoRequest 的字段在 Go 里全是 *指针（`*string` / `*ctype.ID`），
// JSON 里可以是 null；但 swag 把指针印成非空类型，所以这里显式补 null。
export type UserProfileUpdatePayload = Nullable<
  Schema<"internal_user_adapter_http.UpdateInfoRequest">,
  "username" | "nickname" | "abstract" | "avatar_image_id" | "home_style_id"
>;


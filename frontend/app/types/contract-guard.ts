/**
 * 契约守卫：`api.ts` 与 `swagger` 的字段集合必须一致。
 *
 * # 这个文件解决什么问题
 *
 * 后端的契约（swagger.json → openapi.d.ts）一直在生成，但业务代码一个都没用 ——
 * 全部走的 `api.ts` 里手写的类型。结果是**后端改字段名/删字段，前端不会有任何报错**，
 * 只会在运行时静默拿到 `undefined`。
 *
 * 这里用类型级别的断言把两者钉在一起：契约里少一个字段、多一个字段，或者 api.ts
 * 拼错一个字段名，`nuxi typecheck` 就会在这份文件上报错，并直接告诉你差哪几个字段。
 *
 * # 为什么不是"直接用契约类型"
 *
 * 契约现在的字段全部是可选的（swag 的 required 语义没做），直接拿来当业务类型会
 * 让每个字段都多出 `undefined`，可读性和安全性反而更差。所以保留 `api.ts` 作为
 * 面向 UI 的显式形状，但**用守卫保证它不会悄悄漂移**。
 *
 * # 维护方式
 *
 * 新增/改动接口后：
 *   1. `pnpm openapi:types` 重新生成契约；
 *   2. `pnpm typecheck` —— 若这里报错，说明后端字段变了，前端要跟着改；
 *   3. 新增的 api.ts 类型请补一条断言（映射见下方列表）。
 *
 * # 覆盖率（2026-10-26）
 *
 * api.ts 有 72 个带字段的类型，这里覆盖 31 个；未覆盖的主要是两类：
 *   - **列表包装**（`{list, count}`）：后端用匿名结构体返回，契约里没有对应的
 *     schema，**没有可比对的对象**；要让它们也能守，得先在后端给列表响应命名；
 *   - **纯 UI 形状**（`ApiEnvelope`、`OptionItem`、`ChatSocketEnvelope` 等）：
 *     不对应任何接口响应。
 */
import type * as Api from "~/types/api";
import type { Schema } from "~/types/contract";

/** 断言：条件为 true 才通过；否则把差异写在类型里，让报错直接指出缺哪个字段。 */
type Assert<T extends true> = T;

/** 键集合相等（可选性不参与比较：契约全是可选，api.ts 是必填）。 */
type KeysCheck<A, B> =
  [Exclude<keyof A, keyof B>] extends [never]
    ? [Exclude<keyof B, keyof A>] extends [never]
      ? true
      : { extraInContract: Exclude<keyof B, keyof A> }
    : { missingInContract: Exclude<keyof A, keyof B> };

/** `AiArticleMetainfo` ↔ `internal_ai_adapter_http.MetaInfoResponse` */
export type GuardAiArticleMetainfo = Assert<KeysCheck<Api.AiArticleMetainfo, Schema<"internal_ai_adapter_http.MetaInfoResponse">>>;
/** `AiArticleScoringRequest` ↔ `internal_ai_adapter_http.ScoringRequest` */
export type GuardAiArticleScoringRequest = Assert<KeysCheck<Api.AiArticleScoringRequest, Schema<"internal_ai_adapter_http.ScoringRequest">>>;
/** `AiArticleScoringResponseData` ↔ `internal_ai_adapter_http.ScoringResponse` */
export type GuardAiArticleScoringResponseData = Assert<KeysCheck<Api.AiArticleScoringResponseData, Schema<"internal_ai_adapter_http.ScoringResponse">>>;
/** `AiDiagnoseRequest` ↔ `internal_ai_adapter_http.DiagnoseRequest` */
export type GuardAiDiagnoseRequest = Assert<KeysCheck<Api.AiDiagnoseRequest, Schema<"internal_ai_adapter_http.DiagnoseRequest">>>;
/** `AiOverwriteRequest` ↔ `internal_ai_adapter_http.OverwriteRequest` */
export type GuardAiOverwriteRequest = Assert<KeysCheck<Api.AiOverwriteRequest, Schema<"internal_ai_adapter_http.OverwriteRequest">>>;
/** `ArticleCreateResult` ↔ `internal_article_adapter_http.ArticleCreateResponse` */
export type GuardArticleCreateResult = Assert<KeysCheck<Api.ArticleCreateResult, Schema<"internal_article_adapter_http.ArticleCreateResponse">>>;
/** `ArticleDetail` ↔ `internal_article_adapter_http.ArticleDetailResponse` */
export type GuardArticleDetail = Assert<KeysCheck<Api.ArticleDetail, Schema<"internal_article_adapter_http.ArticleDetailResponse">>>;
/** `ArticleWritePayload` ↔ `internal_article_adapter_http.ArticleUpdateRequest` */
export type GuardArticleWritePayload = Assert<KeysCheck<Api.ArticleWritePayload, Schema<"internal_article_adapter_http.ArticleUpdateRequest">>>;
/** `BannerItem` ↔ `internal_site_adapter_http.BannerItem` */
export type GuardBannerItem = Assert<KeysCheck<Api.BannerItem, Schema<"internal_site_adapter_http.BannerItem">>>;
/** `ChatMessageItem` ↔ `myblogx_internal_chat_domain.MessageItem` */
export type GuardChatMessageItem = Assert<KeysCheck<Api.ChatMessageItem, Schema<"myblogx_internal_chat_domain.MessageItem">>>;
/** `ChatSessionItem` ↔ `myblogx_internal_chat_domain.SessionItem` */
export type GuardChatSessionItem = Assert<KeysCheck<Api.ChatSessionItem, Schema<"myblogx_internal_chat_domain.SessionItem">>>;
/** `CommentManageItem` ↔ `myblogx_internal_comment_app.ManageItem` */
export type GuardCommentManageItem = Assert<KeysCheck<Api.CommentManageItem, Schema<"myblogx_internal_comment_app.ManageItem">>>;
/** `CommentRootItem` ↔ `myblogx_internal_comment_app.RootItem` */
export type GuardCommentRootItem = Assert<KeysCheck<Api.CommentRootItem, Schema<"myblogx_internal_comment_app.RootItem">>>;
/** `CreateImageUploadTaskRequest` ↔ `internal_media_adapter_http.CreateTaskRequest` */
export type GuardCreateImageUploadTaskRequest = Assert<KeysCheck<Api.CreateImageUploadTaskRequest, Schema<"internal_media_adapter_http.CreateTaskRequest">>>;
/** `CreateImageUploadTaskResponseData` ↔ `internal_media_adapter_http.CreateTaskResponse` */
export type GuardCreateImageUploadTaskResponseData = Assert<KeysCheck<Api.CreateImageUploadTaskResponseData, Schema<"internal_media_adapter_http.CreateTaskResponse">>>;
/** `FanUserItem` ↔ `myblogx_internal_social_domain.FansItem` */
export type GuardFanUserItem = Assert<KeysCheck<Api.FanUserItem, Schema<"myblogx_internal_social_domain.FansItem">>>;
/** `FollowUserItem` ↔ `myblogx_internal_social_domain.FollowingItem` */
export type GuardFollowUserItem = Assert<KeysCheck<Api.FollowUserItem, Schema<"myblogx_internal_social_domain.FollowingItem">>>;
/** `GlobalNoticeItem` ↔ `internal_notification_adapter_http.GlobalListResponse` */
export type GuardGlobalNoticeItem = Assert<KeysCheck<Api.GlobalNoticeItem, Schema<"internal_notification_adapter_http.GlobalListResponse">>>;
/** `MessagePreference` ↔ `internal_notification_adapter_http.ConfRequest` */
export type GuardMessagePreference = Assert<KeysCheck<Api.MessagePreference, Schema<"internal_notification_adapter_http.ConfRequest">>>;
/** `MessageSummary` ↔ `internal_notification_adapter_http.UnreadResponse` */
export type GuardMessageSummary = Assert<KeysCheck<Api.MessageSummary, Schema<"internal_notification_adapter_http.UnreadResponse">>>;
/** `SearchArticleItem` ↔ `myblogx_internal_search_domain.Item` */
export type GuardSearchArticleItem = Assert<KeysCheck<Api.SearchArticleItem, Schema<"myblogx_internal_search_domain.Item">>>;
/** `SearchArticleResponse` ↔ `myblogx_internal_search_domain.Response` */
export type GuardSearchArticleResponse = Assert<KeysCheck<Api.SearchArticleResponse, Schema<"myblogx_internal_search_domain.Response">>>;
/** `SearchPagination` ↔ `myblogx_internal_search_domain.Pagination` */
export type GuardSearchPagination = Assert<KeysCheck<Api.SearchPagination, Schema<"myblogx_internal_search_domain.Pagination">>>;
/** `SiteAiInfo` ↔ `internal_site_adapter_http.AIResponse` */
export type GuardSiteAiInfo = Assert<KeysCheck<Api.SiteAiInfo, Schema<"internal_site_adapter_http.AIResponse">>>;
/** `SiteMessageItem` ↔ `internal_notification_adapter_http.MessageItem` */
export type GuardSiteMessageItem = Assert<KeysCheck<Api.SiteMessageItem, Schema<"internal_notification_adapter_http.MessageItem">>>;
/** `SiteSeoData` ↔ `internal_site_adapter_http.SEOResponse` */
export type GuardSiteSeoData = Assert<KeysCheck<Api.SiteSeoData, Schema<"internal_site_adapter_http.SEOResponse">>>;
/** `UploadTaskStatusResponseData` ↔ `internal_media_adapter_http.TaskStatusResponse` */
export type GuardUploadTaskStatusResponseData = Assert<KeysCheck<Api.UploadTaskStatusResponseData, Schema<"internal_media_adapter_http.TaskStatusResponse">>>;
/**
 * 后端 BaseInfoResponse 从未提供这三个计数，用户主页的点赞/收藏/评论数因此恒为 0（app/pages/users/[id].vue 里还留着手写 0 的补丁）。补后端聚合后请删掉这条豁免。
 */
type UserBaseInfoPhantom = "digg_count" | "favor_count" | "comment_count";
/** `UserBaseInfo`（去掉已知缺口字段）↔ `internal_user_adapter_http.BaseInfoResponse` */
export type GuardUserBaseInfo = Assert<KeysCheck<Omit<Api.UserBaseInfo, UserBaseInfoPhantom>, Schema<"internal_user_adapter_http.BaseInfoResponse">>>;
/** `UserProfileUpdatePayload` ↔ `internal_user_adapter_http.UpdateInfoRequest` */
export type GuardUserProfileUpdatePayload = Assert<KeysCheck<Api.UserProfileUpdatePayload, Schema<"internal_user_adapter_http.UpdateInfoRequest">>>;
/** `UserSelfDetail` ↔ `myblogx_internal_user_app.DetailView` */
export type GuardUserSelfDetail = Assert<KeysCheck<Api.UserSelfDetail, Schema<"myblogx_internal_user_app.DetailView">>>;
/** `UserSessionItem` ↔ `internal_auth_adapter_http.SessionItem` */
export type GuardUserSessionItem = Assert<KeysCheck<Api.UserSessionItem, Schema<"internal_auth_adapter_http.SessionItem">>>;

import type { Nullable, OptionalFields, RequiredSchema, Schema } from "./contract";

export type ThemeMode = "light" | "dark";

export interface ApiEnvelope<T> {
  code: number;
  msg: string;
  data: T;
}

export interface ApiEmptyObject {
  [key: string]: never;
}

export type SiteSeoData = RequiredSchema<"internal_site_adapter_http.SEOResponse">;

export interface SiteRuntimeConfig {
  site_info: {
    title?: string;
    subtitle?: string;
    mode?: number;
  };
  project: {
    title?: string;
    abstract?: string;
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

export type BannerItem = RequiredSchema<"internal_site_adapter_http.BannerItem">;

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

export type AiArticleScoringType = 1 | 2 | 3;
export type AiArticleScoreLevel = "较差文章" | "不合格文章" | "常规文章" | "优质文章" | "精品文章";
export type AiArticleType =
  | "散文"
  | "小说"
  | "诗歌"
  | "剧本"
  | "童话"
  | "传说"
  | "记叙文"
  | "日记"
  | "新闻"
  | "说明文"
  | "教程"
  | "科普"
  | "报告"
  | "邮件"
  | "通知"
  | "发言稿"
  | "议论文"
  | "评论"
  | "文案"
  | "网文";

export type AiArticleScoreDimensionName =
  | "clarity"
  | "structure"
  | "completeness"
  | "readability"
  | "persuasiveness"
  | "language";

export type AiArticleScoringRequest = Schema<"internal_ai_adapter_http.ScoringRequest">;

export interface AiArticleScoringPosition {
  paragraph: number;
  quote: string;
}

export interface AiArticleScoringIssue {
  positions: AiArticleScoringPosition[];
  reason: string;
  suggestion: string;
}

export interface AiArticleScoringDimension {
  name: AiArticleScoreDimensionName;
  score: number;
  reason?: string;
}

export type AiArticleScoringResponseData = RequiredSchema<"internal_ai_adapter_http.ScoringResponse">;

export interface FavoriteFolderItem {
  id: string;
  user_id: string;
  title: string;
  cover: string;
  abstract: string;
  is_default: boolean;
  article_count: number;
  nickname?: string;
  avatar?: string;
  has_article: boolean;
}

export type FavoriteFolderListData = RequiredSchema<"myblogx_internal_kernel.ListData-internal_article_adapter_http_FavoriteListItem">;

export interface FavoriteFolderCreatePayload {
  id: string;
  user_id: string;
  title: string;
  cover: string;
  abstract: string;
  is_default: boolean;
  article_count: number;
}

export interface SearchTag {
  id: string;
  title: string;
}

export interface SearchCategory {
  id: string;
  title: string;
}

export interface SearchAuthor {
  id: string;
  nickname: string;
  avatar: string;
}

export type SearchPagination = RequiredSchema<"myblogx_internal_search_domain.Pagination">;

export type SearchArticleItem = RequiredSchema<"myblogx_internal_search_domain.Item">;

export type SearchArticleResponse = RequiredSchema<"myblogx_internal_search_domain.Response">;

export interface ArticleTopItem {
  id: string;
  title: string;
  abstract: string;
  cover: string;
  view_count: number;
  digg_count: number;
  comment_count: number;
  favor_count: number;
  tags: string[];
  user_nickname: string;
  user_avatar: string;
  category_title: string;
}

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

export interface CommentRootListData {
  list: CommentRootItem[];
  count?: number;
  has_more: boolean;
}

export interface CommentReplyItem {
  created_at: string;
  content: string;
  user_id: string;
  reply_id: string;
  digg_count: number;
  reply_count: number;
  is_digg: boolean;
  relation: number;
  status: number;
  user_nickname: string;
  user_avatar: string;
  reply_user_nickname: string;
}

export type CommentReplyListData = RequiredSchema<"internal_comment_adapter_http.ReplyListResponse">;

export interface UserBaseInfo {
  id: string;
  code_age: number;
  avatar: string;
  avatar_image_id?: string | null;
  nickname: string;
  abstract?: string;
  view_count: number;
  fans_count: number;
  follow_count: number;
  article_visited_count: number;
  article_count: number;
  favor_count?: number;
  digg_count?: number;
  comment_count?: number;
  favorites_visibility: boolean;
  followers_visibility: boolean;
  fans_visibility: boolean;
  home_style_id: string | null;
  relation: number;
  place: string;
}

export interface ArticleAuthorInfo {
  author_id: string;
  article_count: number;
  article_visited_count: number;
  fans_count: number;
}

export type UserSelfDetail = RequiredSchema<"myblogx_internal_user_app.DetailView">;

export interface UserLikeTagItem {
  id: string;
  title: string;
}

export interface EmailVerifyPayload {
  email_id: string;
  email: string;
}

export interface ArticleListItem {
  id: string;
  created_at: string;
  updated_at: string;
  title: string;
  abstract: string;
  content: string;
  cover: string;
  view_count: number;
  digg_count: number;
  comment_count: number;
  favor_count: number;
  comments_toggle: boolean;
  status: number;
  tags: string[];
  user_top: boolean;
  admin_top: boolean;
  category_title: string;
  user_nickname: string;
  user_avatar: string;
}

export interface ArticleListResponse {
  list: ArticleListItem[];
  count: number;
}

export interface HistoryArticleItem {
  updated_at: string;
  title: string;
  cover: string;
  nickname: string;
  avatar: string;
  user_id: string;
  article_id: string;
}

export interface HistoryArticleListData {
  list: HistoryArticleItem[];
  has_more: boolean;
}

export type CommentManageItem = RequiredSchema<"myblogx_internal_comment_app.ManageItem">;

export type CommentManageListData = RequiredSchema<"myblogx_internal_kernel.ListData-myblogx_internal_comment_app_ManageItem">;

export type FollowUserItem = RequiredSchema<"myblogx_internal_social_domain.FollowingItem">;

export type FollowUserListData = RequiredSchema<"myblogx_internal_kernel.HasMoreListData-myblogx_internal_social_domain_FollowingItem">;

export type FanUserItem = RequiredSchema<"myblogx_internal_social_domain.FansItem">;

export type FanUserListData = RequiredSchema<"myblogx_internal_kernel.HasMoreListData-myblogx_internal_social_domain_FansItem">;

export interface FavoriteArticleItem {
  favorited_at: string;
  article_id: string;
  title: string;
  abstract: string;
  cover: string;
  view_count: number;
  digg_count: number;
  comment_count: number;
  favor_count: number;
  user_nickname: string;
  user_avatar: string;
  article_status: number;
}

export interface FavoriteArticleListData {
  list: FavoriteArticleItem[];
  has_more: boolean;
}

export interface LoginLogItem {
  id: string;
  created_at: string;
  updated_at: string;
  user_id: string;
  ip: string;
  addr: string;
  ua: string;
  user_nickname: string;
  user_avatar: string;
}

export interface LoginLogListData {
  list: LoginLogItem[];
  count: number;
}

export type UserSessionItem = RequiredSchema<"internal_auth_adapter_http.SessionItem">;

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

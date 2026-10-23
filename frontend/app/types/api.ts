export type ThemeMode = "light" | "dark";

export interface ApiEnvelope<T> {
  code: number;
  msg: string;
  data: T;
}

export interface ApiEmptyObject {
  [key: string]: never;
}

export interface SiteSeoData {
  site_title: string;
  project_title: string;
  logo: string;
  icon: string;
  keywords: string;
  description: string;
}

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

export interface SiteAiInfo {
  enable: boolean;
  nickname: string;
  avatar: string;
  abstract: string;
}

export interface BannerItem {
  id: string;
  cover: string;
  href: string;
  show: boolean;
}

export interface OptionItem {
  label: string;
  value: string;
}

export interface CreateImageUploadTaskRequest {
  file_name: string;
  size: number;
  mime_type: string;
  hash: string;
}

export interface CreateImageUploadTaskResponseData {
  skip_upload: boolean;
  upload_id?: string;
  provider?: string;
  bucket?: string;
  object_key?: string;
  upload_token?: string;
  region?: string;
  expire_at?: string;
  max_size?: number;
  image_id?: string;
  status?: string;
  url?: string;
  hash?: string;
}

export interface UploadTaskStatusResponseData {
  upload_id: string;
  image_id?: string;
  status: string;
  url?: string;
  error_msg?: string;
  hash?: string;
}

export interface AiArticleMetainfo {
  title: string;
  abstract: string;
  category?: {
    id: string;
    title: string;
  } | null;
  tags: Array<{
    id: string;
    title: string;
  }>;
}

export type AiOverwriteMode = "polish" | "grammar_fix" | "style_transform";

export interface AiOverwriteRequest {
  mode: AiOverwriteMode;
  selection_text: string;
  prefix_text?: string;
  suffix_text?: string;
  article_title: string;
  target_style?: string;
}

export interface AiDiagnoseRequest {
  selection_text: string;
  prefix_text?: string;
  suffix_text?: string;
  article_title: string;
}

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

export interface AiArticleScoringRequest {
  type: AiArticleScoringType;
  article_id: string;
  title?: string;
  content?: string;
}

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

export interface AiArticleScoringResponseData {
  has_score: boolean;
  record_id?: string;
  article_id: string;
  created_at?: string;
  ai_total_score?: number;
  total_score?: number;
  score_level?: AiArticleScoreLevel;
  article_type?: AiArticleType;
  dimensions?: AiArticleScoringDimension[];
  main_issues?: AiArticleScoringIssue[];
  overall_comment?: string;
}

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

export interface FavoriteFolderListData {
  list: FavoriteFolderItem[];
  count: number;
}

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

export interface SearchPagination {
  mode: "has_more" | "count";
  page: number;
  limit: number;
  has_more: boolean;
  total?: number;
  total_pages?: number;
}

export interface SearchArticleItem {
  id: string;
  created_at: string;
  updated_at: string;
  title: string;
  abstract?: string;
  cover: string;
  view_count: number;
  digg_count: number;
  comment_count: number;
  favor_count: number;
  comments_toggle: boolean;
  publish_status: number;
  visibility_status: "visible" | "user_hidden" | "admin_hidden";
  tags: SearchTag[];
  category?: SearchCategory;
  author: SearchAuthor;
  highlight?: {
    title?: string;
    abstract?: string;
  };
}

export interface SearchArticleResponse {
  list: SearchArticleItem[];
  pagination: SearchPagination;
}

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

export interface ArticleWritePayload {
  title: string;
  abstract?: string;
  content: string;
  category_id?: string | null;
  tag_ids?: string[];
  cover_image_id?: string | null;
  comments_toggle?: boolean;
  visibility_status?: "visible" | "user_hidden";
  status: 1 | 2;
}

export interface ArticleCreateResult {
  id: string;
  title: string;
  category_id?: string | null;
  tag_ids?: string[];
  cover_image_id?: string | null;
  comments_toggle: boolean;
  status: number;
  publish_status: number;
  visibility_status: "visible" | "user_hidden" | "admin_hidden";
}

export interface ArticleDetail {
  id: string;
  created_at: string;
  updated_at: string;
  title: string;
  abstract: string;
  content: string;
  cover: string;
  cover_image_id?: string | null;
  view_count: number;
  digg_count: number;
  comment_count: number;
  favor_count: number;
  comments_toggle: boolean;
  status: number;
  tags: string[];
  author_id: string;
  author_avatar_image_id?: string | null;
  author_avatar: string;
  author_abstract: string;
  author_created_time: string;
  author_name: string;
  author_username: string;
  category_name: string;
  category_id?: string | null;
  tag_ids?: string[];
  is_digg: boolean;
  is_favor: boolean;
  publish_status: number;
  visibility_status: "visible" | "user_hidden" | "admin_hidden";
}

export interface CommentRootItem {
  id: string;
  created_at: string;
  content: string;
  user_id: string;
  reply_id: string;
  root_id: string;
  digg_count: number;
  reply_count: number;
  is_digg: boolean;
  relation: number;
  status: number;
  user_nickname: string;
  user_avatar: string;
}

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

export interface CommentReplyListData {
  root_id: string;
  reply_count: number;
  list: CommentReplyItem[];
  count?: number;
  has_more: boolean;
}

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

export interface UserSelfDetail {
  id: string;
  created_at: string;
  username: string;
  nickname: string;
  avatar: string;
  avatar_image_id?: string | null;
  abstract: string;
  register_source: number;
  code_age: number;
  favorites_visibility?: boolean;
  followers_visibility?: boolean;
  fans_visibility?: boolean;
  home_style_id?: string | null;
  like_tag_ids?: string[];
  like_tag_items?: UserLikeTagItem[];
  updated_username_date?: string | null;
}

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

export interface CommentManageItem {
  id: string;
  created_at: string;
  content: string;
  digg_count: number;
  reply_count: number;
  user_id: string;
  user_nickname: string;
  user_avatar: string;
  article_id: string;
  article_title: string;
  article_cover: string;
}

export interface CommentManageListData {
  list: CommentManageItem[];
  count: number;
}

export interface FollowUserItem {
  followed_user_id: string;
  followed_nickname: string;
  followed_avatar: string;
  followed_abstract: string;
  follow_time: string;
  relation: number;
}

export interface FollowUserListData {
  list: FollowUserItem[];
  has_more: boolean;
}

export interface FanUserItem {
  fans_user_id: string;
  fans_nickname: string;
  fans_avatar: string;
  fans_abstract: string;
  follow_time: string;
  relation: number;
}

export interface FanUserListData {
  list: FanUserItem[];
  has_more: boolean;
}

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

export interface UserSessionItem {
  id: string;
  ip: string;
  addr: string;
  ua: string;
  created_at: string;
  last_seen_at?: string | null;
  expires_at: string;
  is_current: boolean;
}

export interface UserSessionListData {
  list: UserSessionItem[];
  count: number;
}

export interface MessageSummary {
  comment_msg_count: number;
  digg_favor_msg_count: number;
  private_msg_count: number;
  system_msg_count: number;
  global_msg_count: number;
}

export interface MessagePreference {
  digg_notice_enabled: boolean;
  comment_notice_enabled: boolean;
  favor_notice_enabled: boolean;
  private_chat_notice_enabled: boolean;
}

export interface SiteMessageItem {
  id: string;
  created_at: string;
  updated_at: string;
  type: number;
  receiver_id: string;
  action_user_id: string | null;
  action_user_nickname: string | null;
  action_user_avatar: string | null;
  content: string;
  article_id: string;
  comment_id: string;
  article_title: string;
  link_title: string;
  link_herf: string;
  is_read: boolean;
  read_at: string | null;
}

export interface SiteMessageListData {
  list: SiteMessageItem[];
  has_more: boolean;
}

export interface GlobalNoticeItem {
  id: string;
  create_at: string;
  title: string;
  icon: string;
  content: string;
  herf: string;
  is_read: boolean;
}

export interface GlobalNoticeListData {
  list: GlobalNoticeItem[];
  has_more: boolean;
}

export interface ChatSessionItem {
  session_id: string;
  receiver_id: string;
  receiver_nickname: string;
  receiver_avatar: string;
  relation: number;
  last_msg_content: string;
  last_msg_time: string | null;
  unread_count: number;
  is_top: boolean;
  is_mute: boolean;
  deleted_at?: string | null;
}

export interface ChatSessionListData {
  list: ChatSessionItem[];
  count: number;
}

export interface ChatMessageItem {
  id: string;
  sender_id: string;
  receiver_id: string;
  session_id: string;
  content: string;
  send_time: string;
  msg_status: number;
  msg_type: number;
  is_self: boolean;
  is_read: boolean;
  deleted_at?: string | null;
}

export interface ChatMessageListData {
  list: ChatMessageItem[];
  count: number;
}

export interface ChatWsTicketData {
  ticket?: string;
  [key: string]: unknown;
}

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

export interface UserProfileUpdatePayload {
  username?: string | null;
  nickname?: string | null;
  avatar_image_id?: string | null;
  abstract?: string | null;
  like_tag_ids?: string[];
  favorites_visibility?: boolean | null;
  followers_visibility?: boolean | null;
  fans_visibility?: boolean | null;
  home_style_id?: string | null;
}

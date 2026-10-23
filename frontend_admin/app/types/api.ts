export interface ApiEnvelope<T> {
  code: number
  msg: string
  data: T
}

export interface DateCountItem {
  date: string
  count: number
}

export interface SumResponseData {
  flow_count: number
  user_count: number
  article_count: number
  message_count: number
  comment_count: number
  new_login_count: number
  new_sign_count: number
}

export interface GrowthDataResponseData {
  growth_rate: number
  growth_num: number
  date_count_list: DateCountItem[]
}

export interface ArticleYearDataResponseData {
  date_count_list: DateCountItem[]
}

export interface UserDetailResponseData {
  id: string
  username: string
  nickname: string
  avatar?: string
  abstract?: string
  role?: number
  status?: number
}

export interface UserListItem {
  id: string
  nickname: string
  avatar: string
  username: string
  created_at: string
  ip: string
  addr: string
  last_login_at: string
  role?: number
  status?: number
}

export interface UserListResponseData {
  list: UserListItem[]
  count: number
}

export interface ArticleReviewTaskItem {
  id: string
  article_id: string
  author_id: string
  article_title: string
  author_name: string
  publish_status: number
  stage: 'manual'
  source: 'create' | 'edit' | 'resubmit'
  status: 'pending' | 'approved' | 'rejected' | 'canceled'
  reason: string
  created_at: string
  reviewed_at?: string | null
  reviewed_by?: string | null
}

export interface ArticleReviewTaskListResponseData {
  count: number
  list: ArticleReviewTaskItem[]
}

export interface SearchArticleItem {
  id: string
  created_at: string
  updated_at: string
  title: string
  abstract?: string
  cover?: string
  view_count: number
  digg_count: number
  comment_count: number
  favor_count: number
  comments_toggle: boolean
  publish_status: number
  visibility_status: 'visible' | 'user_hidden' | 'admin_hidden'
  author?: { id?: string; nickname?: string; username?: string }
  category?: { id?: string; title?: string; name?: string }
  tags?: Array<{ id?: string; title?: string; name?: string }>
}

export interface SearchPagination {
  mode: 'has_more' | 'count'
  page: number
  limit: number
  has_more: boolean
  total?: number
  total_pages?: number
}

export interface ArticleSearchResponseData {
  list: SearchArticleItem[]
  pagination: SearchPagination
}

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

export interface BannerModel {
  id: string
  created_at: string
  updated_at: string
  show: boolean
  cover: string
  href: string
}

export interface HasMoreList<T> {
  list: T[]
  has_more: boolean
}

export interface GlobalNotifListItem {
  id: string
  create_at: string
  title: string
  icon: string
  content: string
  herf?: string
  href?: string
  is_read: boolean
}

export interface LogListData<T> {
  list: T[]
  count: number
}

export interface RuntimeLogRecord {
  event_id: string
  ts: string
  service: string
  level: string
  message: string
  request_id: string
  trace_id: string
  user_id: string
  method: string
  path: string
  status_code: number
  latency_ms: number
  event_name: string
  error_code: string
  error_message: string
  extra_json: string
}

export interface ActionAuditRecord extends RuntimeLogRecord {
  action_name: string
  target_type: string
  target_id: string
  success: number
}

export interface LoginEventRecord extends RuntimeLogRecord {
  username: string
  login_type: string
  success: number
  reason: string
  addr: string
  ua: string
}

export interface CdcEventRecord extends RuntimeLogRecord {
  cdc_job_id: string
  stream: string
  source_table: string
  action: string
  target_key: string
  retry_count: number
  result: string
}

export type AnyLogRecord = RuntimeLogRecord | ActionAuditRecord | LoginEventRecord | CdcEventRecord

export interface ImageListItem {
  id?: string
  url?: string
  path?: string
  name?: string
  status?: string
  created_at?: string
  [key: string]: unknown
}

export interface CreateImageUploadTaskRequest {
  file_name: string
  size: number
  mime_type: string
  hash: string
}

export interface CreateImageUploadTaskResponseData {
  skip_upload: boolean
  upload_id?: string
  provider?: string
  bucket?: string
  object_key?: string
  upload_token?: string
  region?: string
  expire_at?: string
  max_size?: number
  image_id?: string
  status?: string
  url?: string
  hash?: string
}

export interface UploadTaskStatusResponseData {
  upload_id?: string
  image_id?: string
  status?: string
  url?: string
  error_msg?: string
  hash?: string
}

/**
 * 评论管理页的筛选拼装（2026-11-27）。
 *
 * 抽成纯函数的原因：这几条规则错了不会报错、只会静默查错东西 ——
 * "用户输入框填的是 ID 还是昵称"、"日期区间怎么变成自然日"、"空筛选要不要发
 * 空字符串给后端" 都属于必须钉住的行为，放在组件里没法测，放在这里一行一条用例。
 */

/** 管理端列表的筛选态（页面上"已提交"的那一份，不是输入框里的待提交值）。 */
export interface CommentAdminFilters {
  /** 评论内容关键词（子串）。 */
  keyword: string
  /** 用户筛选的原始输入：可能是用户 ID，也可能是昵称片段。 */
  userInput: string
  /** 文章 ID（只支持按 ID 定位文章，按标题搜会引入一条额外的文章查询链路）。 */
  articleInput: string
  /** 状态：全部 / 已发布 / 已删除。 */
  status: 'all' | 'published' | 'deleted'
  /** 时间区间（毫秒时间戳，来自日期选择器）；null 表示不限。 */
  range: [number, number] | null
}

/** 后端 `GET /comments/admin/list` 接受的查询参数（undefined = 不传）。 */
export interface CommentAdminQuery {
  page: number
  limit: number
  keyword?: string
  user_id?: string
  user_keyword?: string
  article_id?: string
  from?: string
  to?: string
  status: 'all' | 'published' | 'deleted'
}

/**
 * 把"用户输入"拆成 `user_id` 或 `user_keyword`。
 *
 * 判据是**全数字**：用户 ID 是雪花 ID（18 位），昵称里出现纯数字的情况极少；
 * 反过来把 18 位 ID 当昵称模糊匹配会扫全表还匹配不到，所以数字优先当 ID。
 * 空输入返回空对象 —— 不传参数，而不是传空串（后端对空串也会当成 LIKE '%%'）。
 */
export function splitUserInput(raw: string): { user_id?: string; user_keyword?: string } {
  const value = raw.trim()
  if (!value) return {}
  return /^\d+$/u.test(value) ? { user_id: value } : { user_keyword: value }
}

/**
 * 毫秒时间戳 → 自然日 `YYYY-MM-DD`，**按本地时区**。
 *
 * 后端 `adminDayBoundary` 用 `time.ParseInLocation(..., time.Local)` 解析 `from`/`to`，
 * 日期选择器给的是**本地零点**；若这里用 `toISOString()`（UTC），东八区的"今天"
 * 会被切成前一天，选"今天"就查不到当天的评论。两端必须用同一个时区口径。
 */
export function dayOf(timestamp: number): string {
  const date = new Date(timestamp)
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, "0")
  const day = String(date.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}

/**
 * 拼装列表查询参数。
 *
 * 三条规矩：
 *   1. 空白输入一律**不传**（`undefined`），让后端走"不限"分支而不是 LIKE '%%'；
 *   2. `status` 恒定传（`all` 是后端认的枚举值，不是"缺省"）；
 *   3. 时间区间两端都传或都不传 —— 只传一头会让筛选语义变成"从那以后"，与页面文案不符。
 */
export function buildCommentAdminQuery(
  filters: CommentAdminFilters,
  page: number,
  limit: number,
): CommentAdminQuery {
  const userQuery = splitUserInput(filters.userInput)
  const keyword = filters.keyword.trim()
  const articleID = filters.articleInput.trim()

  return {
    page,
    limit,
    keyword: keyword || undefined,
    article_id: articleID || undefined,
    status: filters.status,
    from: filters.range ? dayOf(filters.range[0]) : undefined,
    to: filters.range ? dayOf(filters.range[1]) : undefined,
    ...userQuery,
  }
}

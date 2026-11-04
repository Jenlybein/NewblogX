import type { UserDetailResponseData } from '~/types/api'

/**
 * 后台登录。
 *
 * 固定带上 `scope: 'admin'`：管理后台只应该拿到管理员会话。
 * 服务端在**验证密码之后、创建会话之前**校验角色，非管理员直接返回
 * `auth.admin_required`，不签发令牌、不写 refresh cookie。
 *
 * 为什么不在前端先判断角色：登录返回里没有角色（只有 token），
 * 而且前端拿到的任何东西都可以被改 —— 判断必须在服务端。
 *
 * 边界要清楚：这只拦"登录那一刻"。Refresh 读的是 session 表、不读 claims，
 * 零 DDL 下没地方记 scope，所以刷新后的令牌是普通令牌；
 * 真正的授权始终由后端每请求查库取 role（AdminMiddleware）决定。
 */
export function loginWithPassword(payload: {
  username: string
  password: string
  /** 站点开启图形验证码时必须带上（后端 CaptchaMiddleware 对这两个字段 required）。 */
  captchaId?: string
  captchaCode?: string
}) {
  const { $api } = useNuxtApp()
  const { captchaId, captchaCode, ...rest } = payload
  return $api.request<string, Record<string, unknown>>('/users/login', {
    method: 'POST',
    body: {
      ...rest,
      scope: 'admin',
      // 只在拿到验证码时附带：站点关闭验证码时后端会直接跳过中间件，多余字段不参与校验。
      ...(captchaId && captchaCode ? { captcha_id: captchaId, captcha_code: captchaCode } : {}),
    },
    auth: false,
    retryAuth: false,
  })
}

/** 图形验证码：`GET /api/imagecaptcha` 返回 `{captcha_id, base64}`，base64 是可直接塞 img 的 data URI。 */
export function getLoginCaptcha() {
  const { $api } = useNuxtApp()
  return $api.request<{ captcha_id: string; base64: string }>('/imagecaptcha', {
    method: 'GET',
    auth: false,
    retryAuth: false,
  })
}

/**
 * 读公开站点配置，只为拿到 `login.captcha` 这个开关。
 *
 * 为什么读公开接口而不是加一个新探针：`PublicSite()` 返回的正是整份站点配置
 * （`conf.Site` 里就有 `Login` 段），所以后台登录页不需要后端任何改动就能知道
 * "这次要不要填验证码"。见票 263。
 */
export function getPublicSiteLoginFlags() {
  const { $api } = useNuxtApp()
  return $api.request<{ login?: { captcha?: boolean } }>('/site/site', {
    method: 'GET',
    auth: false,
    retryAuth: false,
  })
}

export function getSelfUserDetail() {
  const { $api } = useNuxtApp()
  return $api.request<UserDetailResponseData>('/users/detail')
}

export function refreshAccessToken() {
  const config = useRuntimeConfig()
  return $fetch<{ code: number; msg: string; data: string }>('/users/refresh', {
    baseURL: String(config.public.apiBase || '/api'),
    method: 'POST',
    credentials: 'include',
    retry: 0,
  })
}

export function logoutCurrentSession() {
  const { $api } = useNuxtApp()
  return $api.request<unknown>('/users/logout', { method: 'POST', retryAuth: false })
}

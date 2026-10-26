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
export function loginWithPassword(payload: { username: string; password: string }) {
  const { $api } = useNuxtApp()
  return $api.request<string, { username: string; password: string; scope: 'admin' }>('/users/login', {
    method: 'POST',
    body: { ...payload, scope: 'admin' },
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

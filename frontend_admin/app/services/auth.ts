import type { UserDetailResponseData } from '~/types/api'

export function loginWithPassword(payload: { username: string; password: string }) {
  const { $api } = useNuxtApp()
  return $api.request<string, typeof payload>('/users/login', {
    method: 'POST',
    body: payload,
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

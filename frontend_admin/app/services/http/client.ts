import { ApiBusinessError, isAuthLikeError } from '~/services/http/errors'
import type { ApiEnvelope } from '~/types/api'

export interface ApiRequestOptions<TBody = unknown> {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  query?: Record<string, unknown>
  body?: TBody
  auth?: boolean
  retryAuth?: boolean
  signal?: AbortSignal
}

export interface ApiClient {
  request<TResponse, TBody = unknown>(path: string, options?: ApiRequestOptions<TBody>): Promise<TResponse>
}

type RequestBody = BodyInit | Record<string, unknown> | null | undefined

function normalizeBaseURL(baseURL: string) {
  return baseURL.endsWith('/') ? baseURL.slice(0, -1) : baseURL
}

function unwrapEnvelope<T>(payload: ApiEnvelope<T>, statusCode?: number) {
  if (payload.code !== 0) {
    throw new ApiBusinessError(payload.msg || '请求失败', payload.code, payload.data, statusCode)
  }
  return payload.data
}

export function createApiClient(baseURL: string) {
  const normalizedBaseURL = normalizeBaseURL(baseURL)

  return {
    async request<TResponse, TBody = unknown>(path: string, options: ApiRequestOptions<TBody> = {}) {
      const authStore = useAuthStore()
      const headers = new Headers()

      if (options.auth !== false && authStore.accessToken) {
        headers.set('Authorization', `Bearer ${authStore.accessToken}`)
      }

      const execute = async () => {
        const response = await $fetch.raw<ApiEnvelope<TResponse>>(path, {
          baseURL: normalizedBaseURL,
          method: options.method ?? 'GET',
          query: options.query,
          body: options.body as RequestBody,
          headers,
          credentials: 'include',
          retry: 0,
          signal: options.signal,
        })

        return unwrapEnvelope<TResponse>(response._data as ApiEnvelope<TResponse>, response.status)
      }

      try {
        return await execute()
      } catch (error) {
        if (options.auth === false || options.retryAuth === false || !isAuthLikeError(error)) {
          throw error
        }

        const refreshed = await authStore.refreshSession()
        if (!refreshed) throw error

        headers.set('Authorization', `Bearer ${authStore.accessToken}`)
        return await execute()
      }
    },
  } satisfies ApiClient
}

import { ApiBusinessError, ApiTimeoutError, isAuthLikeError } from '~/services/http/errors'
import type { ApiEnvelope } from '~/types/api'

export interface ApiRequestOptions<TBody = unknown> {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  query?: Record<string, unknown>
  body?: TBody
  auth?: boolean
  retryAuth?: boolean
  signal?: AbortSignal
  // 单次请求超时（毫秒），缺省 DEFAULT_TIMEOUT_MS；传 0 表示不设上限。
  timeoutMs?: number
}

// 默认请求超时。
//
// 连接断开是会立刻 reject 的，真正难办的是连接停顿 —— 对端不回也不断，Promise
// 既不 resolve 也不 reject。没有这个兜底，任何一次停顿都会让调用方永远等下去。
const DEFAULT_TIMEOUT_MS = 15000

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

      // 每次尝试各起一个计时器：刷新令牌后的重试是全新请求，
      // 不能沿用上一次已经开始倒计时的控制器。
      const execute = async () => {
        const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS

        const controller = new AbortController()
        let timedOut = false
        const timer: ReturnType<typeof setTimeout> | null = timeoutMs > 0
          ? setTimeout(() => {
              timedOut = true
              controller.abort()
            }, timeoutMs)
          : null

        const outer = options.signal
        const forwardAbort = () => controller.abort()
        if (outer) {
          if (outer.aborted) {
            controller.abort()
          }
          else {
            outer.addEventListener('abort', forwardAbort, { once: true })
          }
        }

        try {
          const response = await $fetch.raw<ApiEnvelope<TResponse>>(path, {
            baseURL: normalizedBaseURL,
            method: options.method ?? 'GET',
            query: options.query,
            body: options.body as RequestBody,
            headers,
            credentials: 'include',
            retry: 0,
            signal: controller.signal,
          })

          return unwrapEnvelope<TResponse>(response._data as ApiEnvelope<TResponse>, response.status)
        }
        catch (error) {
          // abort 是通用错误，翻译成"超时"，调用方才分得清是网络停了还是用户取消。
          if (timedOut) {
            throw new ApiTimeoutError(path, timeoutMs)
          }
          throw error
        }
        finally {
          if (timer) {
            clearTimeout(timer)
          }
          outer?.removeEventListener('abort', forwardAbort)
        }
      }

      try {
        return await execute()
      } catch (error) {
        // 超时不重试：连不上就是连不上，再试一次只是让用户多等一轮。
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

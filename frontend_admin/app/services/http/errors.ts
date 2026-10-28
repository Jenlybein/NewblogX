export class ApiBusinessError<T = unknown> extends Error {
  code: number
  data?: T
  statusCode?: number

  constructor(message: string, code: number, data?: T, statusCode?: number) {
    super(message)
    this.name = 'ApiBusinessError'
    this.code = code
    this.data = data
    this.statusCode = statusCode
  }
}

export function isApiBusinessError(error: unknown): error is ApiBusinessError {
  return error instanceof ApiBusinessError
}

// 请求超时。
//
// 客户端超时是自己调用 AbortController.abort() 实现的，抛出来的是通用 abort 错误，
// 调用方分不清"网络停了"和"用户主动取消"。换成独立类型后调用方才能按类型区分。
export class ApiTimeoutError extends Error {
  path: string
  timeoutMs: number

  constructor(path: string, timeoutMs: number) {
    super(`请求超时（超过 ${Math.round(timeoutMs / 1000)} 秒未响应），请检查网络后重试`)
    this.name = 'ApiTimeoutError'
    this.path = path
    this.timeoutMs = timeoutMs
  }
}

export function isApiTimeoutError(error: unknown): error is ApiTimeoutError {
  return error instanceof ApiTimeoutError
}

export function isAuthLikeError(error: unknown) {
  if (!(error instanceof Error)) return false
  if (error instanceof ApiBusinessError && ([401, 403].includes(error.code) || [401, 403].includes(error.statusCode ?? 0))) {
    return true
  }
  const message = error.message.toLowerCase()
  return ['登录', '未登录', '令牌', 'token', '鉴权', 'expired', 'refresh', 'unauthorized', 'forbidden'].some(keyword =>
    message.includes(keyword.toLowerCase()),
  )
}

export class ApiBusinessError<T = unknown> extends Error {
  code: number;
  data?: T;
  statusCode?: number;

  constructor(message: string, code: number, data?: T, statusCode?: number) {
    super(message);
    this.name = "ApiBusinessError";
    this.code = code;
    this.data = data;
    this.statusCode = statusCode;
  }
}

export function isApiBusinessError(error: unknown): error is ApiBusinessError {
  return error instanceof ApiBusinessError;
}

/**
 * 请求超时。
 *
 * # 为什么需要单独一个错误类型
 *
 * 客户端超时是自己调用 `AbortController.abort()` 实现的，抛出来的是一个通用的
 * abort 错误。调用方分不清"网络停了"和"用户主动取消"，只能显示"请求失败"，
 * 而后者其实是预期行为、不该报错。换成独立类型后，调用方可以按类型区分。
 */
export class ApiTimeoutError extends Error {
  /** 超时的请求路径，便于日志定位。 */
  path: string;
  /** 本次请求用了多久的超时上限（毫秒）。 */
  timeoutMs: number;

  constructor(path: string, timeoutMs: number) {
    super(`请求超时（超过 ${Math.round(timeoutMs / 1000)} 秒未响应），请检查网络后重试`);
    this.name = "ApiTimeoutError";
    this.path = path;
    this.timeoutMs = timeoutMs;
  }
}

export function isApiTimeoutError(error: unknown): error is ApiTimeoutError {
  return error instanceof ApiTimeoutError;
}

export function isAuthLikeError(error: unknown) {
  if (!(error instanceof Error)) return false;
  if (error instanceof ApiBusinessError && [401, 403].includes(error.code)) return true;
  const message = error.message.toLowerCase();
  return ["登录", "未登录", "令牌", "token", "鉴权", "expired", "refresh"].some((keyword) =>
    message.includes(keyword.toLowerCase()),
  );
}

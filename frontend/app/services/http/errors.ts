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

/**
 * 刷新令牌之后是否值得重试本次请求。
 *
 * # 为什么写请求一律不重试
 *
 * 重试会把请求**重放**一遍。对读请求这只是多花一次往返；对写请求则是副作用
 * 执行两次——线上实测点一次赞产生两次 `PUT /digg`，用户看到的是第一次的失败
 * 提示，服务端却已经写成功了第二遍，进而撞上唯一索引。
 *
 * 判别依据只能放在**方法**这一侧：后端业务码只有 0 / 1001 / 1002 三个值，
 * 所有业务失败都塌缩成 1001，无法据此区分"未登录"和"参数错误"。也因此
 * {@link isAuthLikeError} 那套按中文关键词匹配的兜底仍然存在——它只用于决定
 * "要不要刷新令牌"，不再独自决定"要不要重放写请求"。
 *
 * @param method 本次请求的方法；缺省按读请求处理。
 * @param retryAuth 调用方显式关闭重试时传 false。
 */
export function shouldRetryAuth(
  method: string | undefined,
  error: unknown,
  retryAuth?: boolean,
): boolean {
  if (retryAuth === false) return false;
  if (!isAuthLikeError(error)) return false;
  // 只有幂等的读可以安全重放；写请求重放等于重复执行副作用。
  return (method ?? "GET").toUpperCase() === "GET";
}

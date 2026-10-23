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

export function isAuthLikeError(error: unknown) {
  if (!(error instanceof Error)) return false;
  if (error instanceof ApiBusinessError && [401, 403].includes(error.code)) return true;
  const message = error.message.toLowerCase();
  return ["登录", "未登录", "令牌", "token", "鉴权", "expired", "refresh"].some((keyword) =>
    message.includes(keyword.toLowerCase()),
  );
}

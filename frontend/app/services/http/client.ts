import {
  ApiBusinessError,
  ApiTimeoutError,
  isAuthLikeError,
  shouldRetryAuth,
} from "~/services/http/errors";
import type { ApiEnvelope } from "~/types/api";

export interface ApiRequestOptions<TBody = unknown> {
  method?: "GET" | "POST" | "PUT" | "DELETE" | "PATCH";
  query?: Record<string, unknown>;
  body?: TBody;
  auth?: boolean;
  retryAuth?: boolean;
  signal?: AbortSignal;
  /**
   * 单次请求超时（毫秒），缺省 {@link DEFAULT_TIMEOUT_MS}；传 0 表示不设上限。
   *
   * 只该给"确实可能跑很久"的接口放宽（比如转 LLM 的 AI 打分）。默认不设上限的写法
   * 会让界面在连接停顿时一直转圈，正是这个超时要消灭的现象。
   */
  timeoutMs?: number;
}

/**
 * 默认请求超时。
 *
 * # 为什么必须有这个兜底
 *
 * 连接**断开**是会立刻 reject 的，界面会正常报错；真正难办的是连接**停顿**
 * —— 对端不回也不断，Promise 既不 resolve 也不 reject。之前这里没有超时，
 * 于是任何一次停顿都会让调用方永远等下去：按钮保持 loading，用户以为系统在忙，
 * 而服务端可能压根没收到请求。
 *
 * 15 秒足够覆盖所有常规接口（实测服务端 p99 在几百毫秒量级），又不至于让用户
 * 盯着转圈等太久。
 */
const DEFAULT_TIMEOUT_MS = 15000;

export interface ApiClient {
  request<TResponse, TBody = unknown>(path: string, options?: ApiRequestOptions<TBody>): Promise<TResponse>;
  requestEnvelope<TResponse, TBody = unknown>(path: string, options?: ApiRequestOptions<TBody>): Promise<ApiEnvelope<TResponse>>;
}

type RequestBody = BodyInit | Record<string, unknown> | null | undefined;

function normalizeBaseURL(baseURL: string) {
  return baseURL.endsWith("/") ? baseURL.slice(0, -1) : baseURL;
}

function unwrapEnvelope<T>(payload: ApiEnvelope<T>, statusCode?: number) {
  if (payload.code !== 0) {
    throw new ApiBusinessError(payload.msg || "请求失败", payload.code, payload.data, statusCode);
  }
  return payload.data;
}

/**
 * 给一次请求套上超时兜底，超时抛出 {@link ApiTimeoutError}。
 *
 * # 为什么单独抽成一个函数
 *
 * 它不碰 Nuxt 的任何东西（没有 store、没有 `$fetch`），输入只有"跑多久算超时"
 * 和"怎么发这次请求"，所以能直接单测 —— 而"超时后必须 reject、且报的是超时
 * 而不是通用 abort"正是这里唯一值得钉住的行为。
 *
 * # 调用方传进来的 signal 为什么要连坐
 *
 * 上层取消时（比如切换页面），底下的请求必须跟着停，否则会留下一堆没人要的
 * 在途请求，还会在已销毁的组件上改状态。
 *
 * @param timeoutMs 超时上限；小于等于 0 表示不设上限（留给确实不能中断的场景）。
 */
export async function withRequestTimeout<T>(
  path: string,
  timeoutMs: number,
  outer: AbortSignal | undefined,
  send: (signal: AbortSignal) => Promise<T>,
): Promise<T> {
  if (timeoutMs <= 0) {
    return send(outer ?? new AbortController().signal);
  }

  const controller = new AbortController();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, timeoutMs);

  const forwardAbort = () => controller.abort();
  if (outer) {
    if (outer.aborted) {
      controller.abort();
    } else {
      outer.addEventListener("abort", forwardAbort, { once: true });
    }
  }

  try {
    return await send(controller.signal);
  } catch (error) {
    // abort 是个通用错误，这里翻译成"超时"，调用方才分得清
    // 是网络停了（要提示重试）还是用户主动取消（不该报错）。
    if (timedOut) {
      throw new ApiTimeoutError(path, timeoutMs);
    }
    throw error;
  } finally {
    clearTimeout(timer);
    outer?.removeEventListener("abort", forwardAbort);
  }
}

export function createApiClient(baseURL: string) {
  const normalizedBaseURL = normalizeBaseURL(baseURL);

  async function executeRequest<TResponse, TBody = unknown>(
    path: string,
    options: ApiRequestOptions<TBody>,
  ) {
    const authStore = useAuthStore();
    const headers = new Headers();

    if (options.auth !== false && authStore.accessToken) {
      headers.set("Authorization", `Bearer ${authStore.accessToken}`);
    }

    if (import.meta.server) {
      const cookieHeaders = useRequestHeaders(["cookie"]);
      if (cookieHeaders.cookie) headers.set("cookie", cookieHeaders.cookie);
    }

    // 每次尝试各起一个计时器：刷新令牌后的重试是一次全新请求，
    // 不能沿用上一次已经开始倒计时的控制器，否则重试很可能一上来就超时。
    const execute = async () => {
      const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;

      return withRequestTimeout<ApiEnvelope<TResponse>>(path, timeoutMs, options.signal, async (signal) => {
        const response = await $fetch.raw<ApiEnvelope<TResponse>>(path, {
          baseURL: normalizedBaseURL,
          method: options.method ?? "GET",
          query: options.query,
          body: options.body as RequestBody,
          headers,
          credentials: "include",
          retry: 0,
          signal,
        });

        const payload = response._data as ApiEnvelope<TResponse>;
        if (payload.code !== 0) {
          throw new ApiBusinessError(payload.msg || "请求失败", payload.code, payload.data, response.status);
        }
        return payload;
      });
    };

    try {
      return await execute();
    } catch (error) {
      // 匿名请求没有令牌可刷新。
      //
      // 写请求不重试：重放会重复执行副作用 —— 点一次赞产生两次 PUT，
      // 用户看到第一次的失败、服务端却写了第二遍，进而撞唯一索引。
      // 超时也不是重试的理由：连不上就是连不上，再试一次只是让用户多等一轮。
      if (options.auth === false || !shouldRetryAuth(options.method, error, options.retryAuth)) {
        // 鉴权失败时不重试，但要让界面停止声称"已登录"：否则用户会看到一个
        // 明明登录着却处处失败的状态。不 await —— 本次错误仍要立刻抛给调用方。
        if (options.auth !== false && isAuthLikeError(error)) {
          void authStore.markPossiblyExpired();
        }
        throw error;
      }

      const refreshed = await authStore.refreshSession();

      if (!refreshed) {
        throw error;
      }

      headers.set("Authorization", `Bearer ${authStore.accessToken}`);

      return await execute();
    }
  }

  return {
    async request<TResponse, TBody = unknown>(
      path: string,
      options: ApiRequestOptions<TBody> = {},
    ): Promise<TResponse> {
      const payload = await executeRequest<TResponse, TBody>(path, options);
      return unwrapEnvelope<TResponse>(payload);
    },
    requestEnvelope<TResponse, TBody = unknown>(
      path: string,
      options: ApiRequestOptions<TBody> = {},
    ) {
      return executeRequest<TResponse, TBody>(path, options);
    },
  } satisfies ApiClient;
}

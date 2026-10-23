import { ApiBusinessError, isAuthLikeError } from "~/services/http/errors";
import type { ApiEnvelope } from "~/types/api";

export interface ApiRequestOptions<TBody = unknown> {
  method?: "GET" | "POST" | "PUT" | "DELETE" | "PATCH";
  query?: Record<string, unknown>;
  body?: TBody;
  auth?: boolean;
  retryAuth?: boolean;
  signal?: AbortSignal;
}

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

    const execute = async () => {
      const response = await $fetch.raw<ApiEnvelope<TResponse>>(path, {
        baseURL: normalizedBaseURL,
        method: options.method ?? "GET",
        query: options.query,
        body: options.body as RequestBody,
        headers,
        credentials: "include",
        retry: 0,
        signal: options.signal,
      });

      const payload = response._data as ApiEnvelope<TResponse>;
      if (payload.code !== 0) {
        throw new ApiBusinessError(payload.msg || "请求失败", payload.code, payload.data, response.status);
      }
      return payload;
    };

    try {
      return await execute();
    } catch (error) {
      if (options.auth === false || options.retryAuth === false || !isAuthLikeError(error)) {
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

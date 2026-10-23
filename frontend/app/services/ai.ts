import type {
  AiArticleMetainfo,
  AiArticleScoringRequest,
  AiArticleScoringResponseData,
  AiDiagnoseRequest,
  AiDiagnoseResponseData,
  AiOverwriteRequest,
  ApiEnvelope,
} from "~/types/api";

export interface AiConversationMessage {
  role: "user" | "assistant";
  content: string;
}

interface AiBaseResponseData {
  content?: string;
}

export interface StreamAiAssistantReplyOptions {
  history?: AiConversationMessage[];
  signal?: AbortSignal;
  onChunk?: (chunk: string) => void;
}

function normalizeApiBase(baseURL: string) {
  const normalized = String(baseURL || "/api").trim() || "/api";
  return normalized.endsWith("/") ? normalized.slice(0, -1) : normalized;
}

export function resolveAiRequestUrl(baseURL: string, path: string) {
  const normalizedBase = normalizeApiBase(baseURL);
  const baseSegments = normalizedBase.split("/").filter(Boolean);
  const pathSegments = String(path || "")
    .split("/")
    .filter(Boolean);

  if (baseSegments.length && pathSegments.length && baseSegments[baseSegments.length - 1] === pathSegments[0]) {
    pathSegments.shift();
  }

  const joinedPath = [...baseSegments, ...pathSegments].join("/");
  return joinedPath ? `/${joinedPath}` : "/";
}

export function buildAiConversationPrompt(history: AiConversationMessage[], latestUserInput: string) {
  const normalizedInput = latestUserInput.trim();
  if (!normalizedInput) {
    return "";
  }

  const normalizedHistory = history
    .map((item) => ({
      role: item.role,
      content: item.content.trim(),
    }))
    .filter((item) => item.content);

  if (!normalizedHistory.length) {
    return normalizedInput;
  }

  const recentHistory = normalizedHistory.slice(-6);
  const transcript = recentHistory
    .map((item) => `${item.role === "assistant" ? "助手" : "用户"}：${item.content}`)
    .join("\n");

  return [
    "下面是 BlogX 首页 AI 对话的最近上下文，请延续语境回答最后一个用户问题。",
    "如果用户是在找站内文章，请优先按文章搜索与推荐的方式回答。",
    transcript,
    `用户：${normalizedInput}`,
  ].join("\n\n");
}

export function parseAiSseDataBlock<TData>(block: string): ApiEnvelope<TData> | null {
  const lines = block
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);

  const dataLines = lines
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice(5).trim())
    .filter(Boolean);

  if (!dataLines.length) {
    return null;
  }

  return JSON.parse(dataLines.join("\n")) as ApiEnvelope<TData>;
}

async function consumeAiSseStream<TData>(
  stream: ReadableStream<Uint8Array>,
  onEvent: (payload: ApiEnvelope<TData>) => void,
) {
  const reader = stream.getReader();
  const decoder = new TextDecoder("utf-8");
  let buffer = "";

  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) {
        break;
      }

      buffer += decoder.decode(value, { stream: true });

      let delimiterMatch = buffer.match(/\r?\n\r?\n/);
      while (delimiterMatch && delimiterMatch.index !== undefined) {
        const delimiterLength = delimiterMatch[0].length;
        const delimiterIndex = delimiterMatch.index;
        const eventBlock = buffer.slice(0, delimiterIndex);
        buffer = buffer.slice(delimiterIndex + delimiterLength);

        const payload = parseAiSseDataBlock<TData>(eventBlock);
        if (payload) {
          onEvent(payload);
        }

        delimiterMatch = buffer.match(/\r?\n\r?\n/);
      }
    }

    buffer += decoder.decode();
    const finalPayload = parseAiSseDataBlock<TData>(buffer);
    if (finalPayload) {
      onEvent(finalPayload);
    }
  } finally {
    reader.releaseLock();
  }
}

interface StreamAiSseRequestOptions<TData> {
  signal?: AbortSignal;
  onEvent?: (payload: ApiEnvelope<TData>) => void;
}

async function streamAiSseRequest<TBody extends object, TData>(
  path: string,
  body: TBody,
  options: StreamAiSseRequestOptions<TData> = {},
) {
  if (import.meta.server) {
    throw new Error("AI 流式请求仅支持在浏览器环境中发起。");
  }

  const authStore = useAuthStore();
  await authStore.initializeSession();

  if (!authStore.accessToken) {
    throw new Error("请先登录后再使用 AI 辅助。");
  }

  const runtimeConfig = useRuntimeConfig();
  const requestUrl = resolveAiRequestUrl(runtimeConfig.public.apiBase, path);

  const execute = async () => {
    const headers = new Headers({
      "Content-Type": "application/json",
      Authorization: `Bearer ${authStore.accessToken}`,
    });

    return fetch(requestUrl, {
      method: "POST",
      headers,
      body: JSON.stringify(body),
      credentials: "include",
      signal: options.signal,
    });
  };

  let response = await execute();

  if ((response.status === 401 || response.status === 403) && (await authStore.refreshSession()) && authStore.accessToken) {
    response = await execute();
  }

  if (!response.ok) {
    const errorText = (await response.text()).trim();
    throw new Error(errorText || `AI 请求失败（${response.status}）`);
  }

  if (!response.body) {
    throw new Error("AI 响应为空。");
  }

  await consumeAiSseStream<TData>(response.body, (payload) => {
    if (payload.code !== 0) {
      throw new Error(payload.msg || "AI 请求失败");
    }

    options.onEvent?.(payload);
  });
}

export async function streamAiAssistantReply(
  content: string,
  options: StreamAiAssistantReplyOptions = {},
) {
  if (import.meta.server) {
    throw new Error("AI 首页对话仅支持在浏览器环境中发起。");
  }

  const authStore = useAuthStore();
  const runtimeConfig = useRuntimeConfig();
  const prompt = buildAiConversationPrompt(options.history || [], content);

  if (!prompt) {
    throw new Error("请输入要发送给 AI 的内容。");
  }

  const requestUrl = resolveAiRequestUrl(runtimeConfig.public.apiBase, "/api/ai/search/llm");
  let combinedContent = "";

  const execute = async () => {
    const headers = new Headers({
      "Content-Type": "application/json",
    });

    if (authStore.accessToken) {
      headers.set("Authorization", `Bearer ${authStore.accessToken}`);
    }

    return fetch(requestUrl, {
      method: "POST",
      headers,
      body: JSON.stringify({ content: prompt }),
      credentials: "include",
      signal: options.signal,
    });
  };

  let response = await execute();

  if ((response.status === 401 || response.status === 403) && (await authStore.refreshSession())) {
    response = await execute();
  }

  if (!response.ok) {
    const errorText = (await response.text()).trim();
    throw new Error(errorText || `AI 对话请求失败（${response.status}）`);
  }

  if (!response.body) {
    throw new Error("AI 对话响应为空。");
  }

  await consumeAiSseStream<AiBaseResponseData>(response.body, (payload) => {
    if (payload.code !== 0) {
      throw new Error(payload.msg || "AI 对话失败");
    }

    const chunk = payload.data?.content || "";
    if (!chunk) {
      return;
    }

    combinedContent += chunk;
    options.onChunk?.(chunk);
  });

  return combinedContent;
}

export interface StreamAiOverwriteOptions {
  signal?: AbortSignal;
  onChunk?: (chunk: string) => void;
}

export async function streamAiOverwrite(request: AiOverwriteRequest, options: StreamAiOverwriteOptions = {}) {
  let combinedContent = "";

  await streamAiSseRequest<AiOverwriteRequest, AiBaseResponseData>("/api/ai/overwrite", request, {
    signal: options.signal,
    onEvent(payload) {
      const chunk = payload.data?.content || "";
      if (!chunk) {
        return;
      }

      combinedContent += chunk;
      options.onChunk?.(chunk);
    },
  });

  return combinedContent;
}

export interface StreamAiDiagnoseOptions {
  signal?: AbortSignal;
  onResult?: (result: AiDiagnoseResponseData) => void;
}

export async function streamAiDiagnose(request: AiDiagnoseRequest, options: StreamAiDiagnoseOptions = {}) {
  let finalResult: AiDiagnoseResponseData | null = null;

  await streamAiSseRequest<AiDiagnoseRequest, AiDiagnoseResponseData>("/api/ai/diagnose", request, {
    signal: options.signal,
    onEvent(payload) {
      if (!payload.data) {
        return;
      }

      finalResult = payload.data;
      options.onResult?.(payload.data);
    },
  });

  if (!finalResult) {
    throw new Error("AI 诊断未返回结果。");
  }

  return finalResult;
}

export function generateArticleMetainfo(content: string) {
  return useNuxtApp().$api.request<AiArticleMetainfo>("/api/ai/metainfo", {
    method: "POST",
    body: {
      content,
    },
  });
}

export function getArticleScoreSummary(article_id: string) {
  return useNuxtApp().$api.request<AiArticleScoringResponseData>("/api/ai/scoring/article", {
    method: "POST",
    body: {
      type: 1,
      article_id,
    } satisfies AiArticleScoringRequest,
  });
}

export function getArticleScoreDetail(article_id: string) {
  return useNuxtApp().$api.request<AiArticleScoringResponseData>("/api/ai/scoring/article", {
    method: "POST",
    body: {
      type: 2,
      article_id,
    } satisfies AiArticleScoringRequest,
  });
}

export function regenerateArticleScore(request: Omit<AiArticleScoringRequest, "type">) {
  return useNuxtApp().$api.request<AiArticleScoringResponseData>("/api/ai/scoring/article", {
    method: "POST",
    body: {
      type: 3,
      ...request,
    } satisfies AiArticleScoringRequest,
  });
}

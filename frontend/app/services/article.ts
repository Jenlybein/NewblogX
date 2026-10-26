import type { RequiredSchema } from "~/types/contract";
import type { ArticleAuthorInfo, ArticleCreateResult, ArticleDetail, ArticleWritePayload } from "~/types/api";

export function getTopArticles() {
  return useNuxtApp().$api.request<
    RequiredSchema<"myblogx_internal_kernel.ListData-internal_article_adapter_http_ArticleTopCardResponse">
  >("/api/articles/top", {
    auth: false,
    query: {
      type: 2,
    },
  });
}

export function getArticleDetail(id: string | number) {
  return useNuxtApp().$api.request<ArticleDetail>(`/api/articles/${id}`);
}

export function getArticleAuthorInfo(authorId: string | number) {
  return useNuxtApp().$api.request<ArticleAuthorInfo>("/api/articles/author_info", {
    auth: false,
    query: {
      author_id: String(authorId),
    },
  });
}

export function markArticleViewed(id: string | number) {
  return useNuxtApp().$api.request("/api/articles/view", {
    method: "POST",
    body: {
      article_id: String(id),
    },
  });
}

export function toggleArticleDigg(id: string | number) {
  return useNuxtApp().$api.requestEnvelope<unknown>(`/api/articles/${id}/digg`, {
    method: "PUT",
  });
}

export function favoriteArticle(articleId: string | number, favorId?: string | number) {
  return useNuxtApp().$api.request("/api/articles/favorite", {
    method: "POST",
    body: {
      article_id: String(articleId),
      ...(favorId ? { favor_id: String(favorId) } : {}),
    },
  });
}

export function createArticle(payload: ArticleWritePayload) {
  return useNuxtApp().$api.request<ArticleCreateResult>("/api/articles", {
    method: "POST",
    body: payload,
  });
}

export function updateArticle(
  id: string | number,
  payload: Partial<Omit<ArticleWritePayload, "status">> & {
    status?: 1 | 2;
  },
) {
  return useNuxtApp().$api.request(`/api/articles/${id}`, {
    method: "PUT",
    body: payload,
  });
}

export function deleteOwnArticle(id: string | number) {
  return useNuxtApp().$api.request(`/api/articles/${id}`, {
    method: "DELETE",
  });
}

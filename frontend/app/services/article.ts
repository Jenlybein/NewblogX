import { useNuxtApp } from "#imports";
import type {
  ArticleAuthorInfo,
  ArticleCreateResult,
  ArticleDetail,
  ArticleHotTopListData,
  ArticleWritePayload,
} from "~/types/api";

/**
 * 首页「热门置顶」运营位（公开读）。
 *
 * 顺序由后端按管理员手动排的顺序返回，前端不排序。旧实现调 `/articles/top` 并塞
 * `type: 2`（v1 的全站运营置顶参数，v2 后端根本不认），所以这个区块一直是空的（#67）。
 */
export function getHotTopArticles() {
  return useNuxtApp().$api.request<ArticleHotTopListData>("/api/articles/hot-top", {
    auth: false,
  });
}

export function getArticleDetail(id: string | number) {
  return useNuxtApp().$api.request<ArticleDetail>(`/api/articles/${id}`);
}

/**
 * 文章详情的**匿名**取数（ADR-0019 决策 5 / 约束 2）。
 *
 * 返回体里可能带 `is_digg`（"我有没有点过赞"）。服务端渲染的 HTML 要进按 URL 键的
 * 共享缓存，所以 SSR 取数必须显式匿名（`auth: false`），再叠加
 * `withoutPrivateViewState` 把用户态字段整个剔掉；客户端首屏/跳转仍走
 * {@link getArticleDetail}，用真实登录态补齐。
 */
export function getArticleDetailAnonymous(id: string | number) {
  return useNuxtApp().$api.request<ArticleDetail>(`/api/articles/${id}`, { auth: false });
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

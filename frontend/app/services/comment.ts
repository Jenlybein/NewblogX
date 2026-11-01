import { useNuxtApp } from "#imports";
import type { CommentReplyListData, CommentRootListData } from "~/types/api";

export function getRootComments(articleId: string | number, page = 1, limit = 7) {
  return useNuxtApp().$api.request<CommentRootListData>("/api/comments", {
    query: {
      article_id: String(articleId),
      page,
      limit,
    },
  });
}

/**
 * 根评论的**匿名**取数（ADR-0019 决策 5 / 约束 2）。
 *
 * 评论条目里可能带 `is_digg`（"我有没有点过赞"）与 `relation`（"我和作者的关系"），
 * 都是登录用户私有状态；SSR 的 HTML 会进按 URL 键的 30s 共享缓存，所以服务端
 * 取评论必须显式匿名（`auth: false`），再叠加 `withoutPrivateFieldsFromItems`
 * 把私有字段剔掉。客户端（首屏 / 跳转）仍走带鉴权的 {@link getRootComments}，
 * 登录用户的点赞态由它自己补齐。
 */
export function getRootCommentsAnonymous(articleId: string | number, page = 1, limit = 7) {
  return useNuxtApp().$api.request<CommentRootListData>("/api/comments", {
    auth: false,
    query: {
      article_id: String(articleId),
      page,
      limit,
    },
  });
}

export function getReplyComments(articleId: string | number, rootId: string, page = 1, limit = 3) {
  return useNuxtApp().$api.request<CommentReplyListData>("/api/comments/replies", {
    query: {
      article_id: String(articleId),
      root_id: String(rootId),
      page,
      limit,
    },
  });
}

export function createComment(payload: { article_id: string; content: string; reply_id?: string }) {
  return useNuxtApp().$api.request("/api/comments", {
    method: "POST",
    body: payload,
  });
}

export function diggComment(id: string) {
  return useNuxtApp().$api.request(`/api/comments/${String(id)}/digg`, {
    method: "POST",
  });
}

import type { MessageSummary, UserBaseInfo, UserSelfDetail } from "~/types/api";

export function getUserBaseInfo(id: string | number) {
  return useNuxtApp().$api.request<UserBaseInfo>("/api/users/base", {
    query: {
      id: String(id),
    },
  });
}

/**
 * 取当前登录用户详情。
 *
 * `retryAuth: false` 用于"刷新令牌之后立刻校验新令牌"这一步：那时刷新流程本身
 * 还持有一个未完成的 Promise，若这里再触发一次自动刷新，就会 await 到自己。
 */
export function getSelfUserDetail(options: { retryAuth?: boolean } = {}) {
  return useNuxtApp().$api.request<UserSelfDetail>("/api/users/detail", {
    retryAuth: options.retryAuth,
  });
}

export function getMessageSummary() {
  return useNuxtApp().$api.request<MessageSummary>("/api/sitemsg/user");
}

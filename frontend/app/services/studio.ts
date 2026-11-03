import type {
  FanUserListData,
  FollowUserListData,
  MessagePreference,
  UserSessionListData,
  UserProfileUpdatePayload,
} from "~/types/api";

// 浏览历史已从创作区移出（`/history` 顶级页，见 #241）：它的服务与游标分页
// 一起落在 `services/history.ts`，这里不再导出 get/deleteHistoryArticles。

export function getFollowUsers(params: { page?: number; limit?: number }) {
  return useNuxtApp().$api.request<FollowUserListData>("/api/follow", {
    query: {
      page: params.page ?? 1,
      limit: params.limit ?? 20,
    },
  });
}

export function getFanUsers(params: { page?: number; limit?: number }) {
  return useNuxtApp().$api.request<FanUserListData>("/api/fans", {
    query: {
      page: params.page ?? 1,
      limit: params.limit ?? 20,
    },
  });
}

export function getUserSessions(params: { page?: number; limit?: number } = {}) {
  return useNuxtApp().$api.request<UserSessionListData>("/api/users/sessions", {
    query: {
      page: params.page ?? 1,
      limit: params.limit ?? 10,
    },
  });
}

export function revokeUserSession(id: string) {
  return useNuxtApp().$api.request(`/api/users/sessions/${id}`, {
    method: "DELETE",
  });
}

export function updateUserProfile(payload: UserProfileUpdatePayload) {
  return useNuxtApp().$api.request("/api/users/info", {
    method: "PUT",
    body: payload,
  });
}

export function bindUserEmail(payload: { email_id: string; email_code: string }) {
  return useNuxtApp().$api.request("/api/users/email/bind", {
    method: "PUT",
    body: payload,
  });
}

export function renewPasswordByEmail(payload: { old_password: string; new_password: string }) {
  return useNuxtApp().$api.request("/api/users/password/renewal/email", {
    method: "PUT",
    body: payload,
  });
}

export function getMessagePreference() {
  return useNuxtApp().$api.request<MessagePreference>("/api/sitemsg/conf");
}

export function updateMessagePreference(payload: MessagePreference) {
  return useNuxtApp().$api.request("/api/sitemsg/conf", {
    method: "PUT",
    body: payload,
  });
}

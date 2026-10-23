import type {
  FanUserListData,
  FollowUserListData,
  HistoryArticleListData,
  LoginLogListData,
  MessagePreference,
  UserSessionListData,
  UserProfileUpdatePayload,
} from "~/types/api";

export function getHistoryArticles(params: { page?: number; limit?: number; type?: 1 | 2 }) {
  return useNuxtApp().$api.request<HistoryArticleListData>("/api/articles/history", {
    query: {
      page: params.page ?? 1,
      limit: params.limit ?? 12,
      type: String(params.type ?? 1),
    },
  });
}

export function deleteHistoryArticles(idList: string[]) {
  return useNuxtApp().$api.request("/api/articles/history", {
    method: "DELETE",
    body: {
      id_list: idList,
    },
  });
}

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

export function getLoginLogs(params: { type?: 1 | 2 }) {
  return useNuxtApp().$api.request<LoginLogListData>("/api/users/login/log", {
    query: {
      ...(params.type ? { type: String(params.type) } : {}),
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

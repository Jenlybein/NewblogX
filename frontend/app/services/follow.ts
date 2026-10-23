import type { FanUserListData, FollowUserListData } from "~/types/api";

export function followUser(id: string | number) {
  return useNuxtApp().$api.request(`/api/follow/${id}`, {
    method: "POST",
  });
}

export function unfollowUser(id: string | number) {
  return useNuxtApp().$api.request(`/api/follow/${id}`, {
    method: "DELETE",
  });
}

export function getFollowList(params: { userId?: string | number; page?: number; limit?: number }) {
  return useNuxtApp().$api.request<FollowUserListData>("/api/follow", {
    query: {
      page: params.page ?? 1,
      limit: params.limit ?? 30,
      ...(params.userId ? { user_id: String(params.userId) } : {}),
    },
  });
}

export function getFansList(params: { userId?: string | number; page?: number; limit?: number }) {
  return useNuxtApp().$api.request<FanUserListData>("/api/fans", {
    query: {
      page: params.page ?? 1,
      limit: params.limit ?? 30,
      ...(params.userId ? { user_id: String(params.userId) } : {}),
    },
  });
}

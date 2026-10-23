import type { MessageSummary, UserBaseInfo, UserSelfDetail } from "~/types/api";

export function getUserBaseInfo(id: string | number) {
  return useNuxtApp().$api.request<UserBaseInfo>("/api/users/base", {
    query: {
      id: String(id),
    },
  });
}

export function getSelfUserDetail() {
  return useNuxtApp().$api.request<UserSelfDetail>("/api/users/detail");
}

export function getMessageSummary() {
  return useNuxtApp().$api.request<MessageSummary>("/api/sitemsg/user");
}

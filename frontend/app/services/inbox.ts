import type {
  ChatMessageListData,
  ChatSessionListData,
  ChatWsTicketData,
  GlobalNoticeListData,
  SiteMessageListData,
} from "~/types/api";

export function getSiteMessages(params: { group: 1 | 2 | 3; page?: number; limit?: number }) {
  return useNuxtApp().$api.request<SiteMessageListData>("/api/sitemsg", {
    query: {
      t: String(params.group),
      page: params.page ?? 1,
      limit: params.limit ?? 20,
    },
  });
}

export function markSiteMessageRead(payload: { id?: string; group?: 1 | 2 | 3 }) {
  return useNuxtApp().$api.request("/api/sitemsg", {
    method: "POST",
    body: {
      ...(payload.id ? { id: payload.id } : {}),
      ...(payload.group ? { t: payload.group } : {}),
    },
  });
}

export function deleteSiteMessage(payload: { id?: string; group?: 1 | 2 | 3 }) {
  return useNuxtApp().$api.request("/api/sitemsg", {
    method: "DELETE",
    body: {
      ...(payload.id ? { id: payload.id } : {}),
      ...(payload.group ? { t: payload.group } : {}),
    },
  });
}

export function getGlobalNotices(params: { page?: number; limit?: number; type?: 1 | 2 }) {
  return useNuxtApp().$api.request<GlobalNoticeListData>("/api/global_notif", {
    query: {
      type: String(params.type ?? 1),
      page: params.page ?? 1,
      limit: params.limit ?? 20,
    },
  });
}

export function markGlobalNoticeRead(idList: string[]) {
  return useNuxtApp().$api.request("/api/global_notif/read", {
    method: "POST",
    body: {
      id_list: idList,
    },
  });
}

export function deleteGlobalNoticeForCurrentUser(idList: string[]) {
  return useNuxtApp().$api.request("/api/global_notif/user", {
    method: "DELETE",
    body: {
      id_list: idList,
    },
  });
}

export function getChatSessions(params: { type?: 1 | 2 }) {
  return useNuxtApp().$api.request<ChatSessionListData>("/api/chat/sessions", {
    query: {
      type: String(params.type ?? 1),
    },
  });
}

export function deleteChatSessions(sessionIdList: string[]) {
  return useNuxtApp().$api.request("/api/chat/sessions", {
    method: "DELETE",
    body: {
      session_id_list: sessionIdList,
    },
  });
}

export function getChatMessages(params: { sessionId: string; userId?: string; type?: 1 | 2 }) {
  return useNuxtApp().$api.request<ChatMessageListData>("/api/chat/messages", {
    query: {
      session_id: params.sessionId,
      type: params.type ?? 1,
      ...(params.userId ? { user_id: params.userId } : {}),
    },
  });
}

export function markChatMessagesRead(msgIdList: string[]) {
  return useNuxtApp().$api.request("/api/chat/read", {
    method: "POST",
    body: {
      msg_id_list: msgIdList,
    },
  });
}

export function deleteChatMessages(msgIdList: string[]) {
  return useNuxtApp().$api.request("/api/chat/messages", {
    method: "DELETE",
    body: {
      msg_id_list: msgIdList,
    },
  });
}

export function getChatWsTicket() {
  return useNuxtApp().$api.request<ChatWsTicketData>("/api/chat/ws-ticket");
}

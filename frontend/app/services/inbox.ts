import { useNuxtApp } from "#imports";
import type {
  ChatMessageItem,
  ChatSendMessagePayload,
  ChatSessionListData,
  ChatWsTicketData,
  GlobalNoticeListData,
  SiteMessageListData,
} from "~/types/api";

/**
 * 私信消息分页的游标。后端 `GET /api/chat/messages` 按
 * `ORDER BY send_time DESC, id DESC` 稳定返回，翻页靠
 * `(before_send_time, before_id)` 这一对值，而不是页码 —— 聊天期间新消息不断
 * 插入，用 page 会重复/漏消息。两个值都是字符串（id 是十进制 ID 字符串）。
 */
export interface ChatMessageCursor {
  before_send_time: string;
  before_id: string;
}

/**
 * **手写**的分页响应形状。
 *
 * 为什么不像 `ChatMessageListData` 那样引用 `~/types/api`：这个接口正在与后端
 * 并行改造，`openapi.d.ts` 还要等 swagger 重新生成后才由上层刷新。这里先按
 * 后端契约把「我们实际消费的 data 形状」钉在本地，避免编译期依赖一份还没
 * 生成的类型；等契约刷新后再决定是否收回 `types/api.ts`。
 */
export interface ChatMessagePageData {
  list: ChatMessageItem[];
  count: number;
  has_more: boolean;
  next_cursor: ChatMessageCursor;
}

/** 老形状（或测试桩）允许缺字段：统一在服务层归一化，调用方不必到处判空。 */
interface RawChatMessagePageData {
  list?: ChatMessageItem[];
  count?: number;
  has_more?: boolean;
  next_cursor?: ChatMessageCursor | null;
}

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

/**
 * 拉一页私信消息。聊天窗口每页 7 条，缺省不传 `page`（读者站改走游标分页）。
 *
 * 响应里的 `list` 仍是 DESC（最新在最前），由 `toAscending` 在 composable 里
 * 转成界面要的升序；这里只负责请求形状与对旧形状的归一化。
 */
export function getChatMessages(params: {
  sessionId: string;
  userId?: string;
  type?: 1 | 2;
  limit?: number;
  cursor?: ChatMessageCursor;
}): Promise<ChatMessagePageData> {
  return useNuxtApp().$api.request<RawChatMessagePageData>("/api/chat/messages", {
    query: {
      session_id: params.sessionId,
      type: params.type ?? 1,
      ...(params.userId ? { user_id: params.userId } : {}),
      limit: params.limit ?? 7,
      ...(params.cursor?.before_send_time
        ? {
            before_send_time: params.cursor.before_send_time,
            before_id: params.cursor.before_id,
          }
        : {}),
    },
  }).then((payload) => {
    const list = payload.list ?? [];
    return {
      list,
      count: payload.count ?? list.length,
      has_more: payload.has_more ?? false,
      next_cursor: payload.next_cursor ?? { before_send_time: "", before_id: "" },
    };
  });
}

/**
 * 发送私信消息（HTTP 可靠落库，唯一发送通道）。
 *
 * WebSocket 只做通知（#201）：WS 断开/断网时消息照样能发出，
 * 对端重连后靠通知刷新拿到消息。
 *
 * `client_msg_id` 是幂等键：同一次发送的重试必须复用同一个值，
 * 服务端据此保证重复提交不产生第二条消息、不重复加未读。
 */
export function sendChatMessage(payload: ChatSendMessagePayload) {
  return useNuxtApp().$api.request<ChatMessageItem>("/api/chat/messages", {
    method: "POST",
    body: payload,
  });
}

/**
 * 整条会话标记已读（优先路径）。
 *
 * 后端这个模式是幂等的：第二次调用返回 0 affected，不是错误。前端因此可以在
 * 「进会话」和「当前会话收到 WS 通知」两处无条件调用，不必自己收集未读 id。
 */
export function markChatSessionRead(sessionId: string) {
  return useNuxtApp().$api.request("/api/chat/read", {
    method: "POST",
    body: {
      session_id: sessionId,
    },
  });
}

/** 按消息 id 标记已读：后端把 `msg_id_list` 绑定为字符串数组，别传数字。 */
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

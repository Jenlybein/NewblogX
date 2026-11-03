import type { ChatSessionItem } from "~/types/api";

export const CHAT_DRAFT_SESSION_PREFIX = "draft:";

export interface InboxDraftSessionSeed {
  receiverId: string;
  receiverNickname: string;
  receiverAvatar?: string | null;
  relation?: number | null;
}

export function buildDraftChatSessionId(receiverId: string) {
  return `${CHAT_DRAFT_SESSION_PREFIX}${receiverId}`;
}

export function isDraftChatSessionId(sessionId?: string | null) {
  return typeof sessionId === "string" && sessionId.startsWith(CHAT_DRAFT_SESSION_PREFIX);
}

export function buildPreparedChatSession(seed: InboxDraftSessionSeed): ChatSessionItem {
  return {
    session_id: buildDraftChatSessionId(seed.receiverId),
    receiver_id: seed.receiverId,
    receiver_nickname: seed.receiverNickname,
    receiver_avatar: seed.receiverAvatar || "",
    relation: seed.relation ?? 0,
    last_msg_content: "发送第一条消息后开始对话",
    last_msg_time: null,
    unread_count: 0,
    is_top: false,
    is_mute: false,
  };
}

/**
 * 生成一次发送的幂等键（client_msg_id）。
 *
 * 为什么需要它：HTTP 落库是唯一真相，但网络可能"请求已落库、响应丢了"。
 * 同一次发送的重试复用同一个键，服务端就会把重复提交识别成同一条消息
 * （返回第一次落库的那条），而不是再插一条、再给对端加一次未读。
 *
 * 依赖注入是为了可测：`random` 与 `now` 都由调用方给，测试里就能断言
 * "同一秒随机不同也必不同"与"格式稳定"；浏览器里传 crypto.randomUUID。
 */
export function buildChatClientMsgId(random: () => string, now: () => number) {
  const nonce = random().replace(/[^a-zA-Z0-9_-]/gu, "").slice(0, 24) || "nonce";
  return `ck-${now()}-${nonce}`.slice(0, 64);
}

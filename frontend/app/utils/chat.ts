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

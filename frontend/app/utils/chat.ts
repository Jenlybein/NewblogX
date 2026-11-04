import type { ChatNotificationPayload, ChatSessionItem } from "~/types/api";
import { firstQueryValue } from "~/utils/queryValue";

export const CHAT_DRAFT_SESSION_PREFIX = "draft:";

/** 已读回执的 `msg_type`（后端 domain.TypeRead）。它不是"又收到一条新消息"。 */
export const CHAT_READ_RECEIPT_MSG_TYPE = 8;

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
    last_msg_content: "",
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

/**
 * 生成「发私信」跳转的 query —— `draft_receiver_*` 跨页契约的**唯一写入口**。
 *
 * 以前 `users/[id].vue` 手写这四个字段，文章页却只弹一个「接入中」的 toast：
 * 两处入口只有一处接线，契约也就只有一个消费者。现在作者主页与文章页都走这里，
 * 第三个入口（评论区作者名等）也不会再抄错字段名。
 *
 * 字段名是跨页契约，不能改：旧链接（分享 / 书签）里的草稿靠它们复活会话。
 * 头像为空时不带这一项（路由会把 undefined 丢掉），`parseDraftSessionSeed` 对缺项宽进。
 */
export function buildDraftChatQuery(seed: InboxDraftSessionSeed): Record<string, string> {
  return {
    draft_receiver_id: seed.receiverId,
    draft_receiver_nickname: seed.receiverNickname,
    ...(seed.receiverAvatar ? { draft_receiver_avatar: seed.receiverAvatar } : {}),
    draft_relation: String(seed.relation ?? 0),
  };
}

/**
 * 解析作者主页「发私信」跳转带来的 `draft_receiver_*` query（票 #240）。
 *
 * 这是**跨页契约**：写入由 `buildDraftChatQuery` 单一来源负责，这里只读。字段名一旦
 * 改动，旧链接（已发出的分享 / 书签）里的草稿就会失效，所以宁可在这里做宽进
 * （数组取值、空白回落），也不要改契约。
 *
 * 缺接收方 id 就没有会话可开 -> 返回 null，页面照常展示普通会话列表。
 */
export function parseDraftSessionSeed(query: Record<string, unknown>): InboxDraftSessionSeed | null {
  const receiverId = firstQueryValue(query.draft_receiver_id).trim();
  if (!receiverId) {
    return null;
  }

  const nickname = firstQueryValue(query.draft_receiver_nickname).trim();
  const relation = Number.parseInt(firstQueryValue(query.draft_relation), 10);

  return {
    receiverId,
    receiverNickname: nickname || "新会话",
    receiverAvatar: firstQueryValue(query.draft_receiver_avatar),
    relation: Number.isFinite(relation) ? relation : 0,
  };
}

/**
 * 判断一条 WS 通知是不是「收到了新消息」。
 *
 * 为什么不能只看 `session_id`：已读回执（`msg_type=8`）同样带 `session_id`，
 * 它到达时是对方读掉了发送方发出的消息 —— 把它算成新消息会让顶栏未读越聊越大。
 * 新消息通知的形状是 `SentNotice`（带 `message_id` / `receiver_id`），回执没有
 * `message_id` 的语义，所以判据是「有 message_id 且 msg_type 不是回执」。
 */
export function isNewChatMessageNotification(
  payload: ChatNotificationPayload | null | undefined,
): boolean {
  if (!payload || typeof payload.session_id !== "string" || !payload.session_id) {
    return false;
  }
  if (payload.msg_type === CHAT_READ_RECEIPT_MSG_TYPE) {
    return false;
  }
  return typeof payload.message_id === "string" && payload.message_id !== "";
}


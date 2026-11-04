import { describe, expect, it } from "vitest";
import {
  CHAT_DRAFT_SESSION_PREFIX,
  buildChatClientMsgId,
  buildDraftChatQuery,
  buildDraftChatSessionId,
  buildPreparedChatSession,
  isDraftChatSessionId,
  isNewChatMessageNotification,
  parseDraftSessionSeed,
} from "~/utils/chat";

describe("chat utils", () => {
  it("builds a stable draft session id for prepared conversations", () => {
    expect(buildDraftChatSessionId("301827524126576640")).toBe(
      `${CHAT_DRAFT_SESSION_PREFIX}301827524126576640`,
    );
  });

  it("detects only prepared front-end draft sessions", () => {
    expect(isDraftChatSessionId("draft:receiver-1")).toBe(true);
    expect(isDraftChatSessionId("session-1")).toBe(false);
    expect(isDraftChatSessionId(null)).toBe(false);
  });

  it("creates a front-end prepared chat session without hitting the backend", () => {
    expect(
      buildPreparedChatSession({
        receiverId: "u-100",
        receiverNickname: "River",
        receiverAvatar: "https://image.example.com/avatar.png",
        relation: 1,
      }),
    ).toEqual({
      session_id: "draft:u-100",
      receiver_id: "u-100",
      receiver_nickname: "River",
      receiver_avatar: "https://image.example.com/avatar.png",
      relation: 1,
      last_msg_content: "发送第一条消息后开始对话",
      last_msg_time: null,
      unread_count: 0,
      is_top: false,
      is_mute: false,
    });
  });

  // 幂等键是 HTTP 发送的幂等前提：同一次发送的重试要复用同一个值。
  // 这里钉住它的形状：长度受后端 max=64 约束，且随机段不同就必须不同。
  it("builds a bounded, collision-resistant client message id", () => {
    const first = buildChatClientMsgId(() => "uuid-one", () => 1000);
    const second = buildChatClientMsgId(() => "uuid-two", () => 1000);

    expect(first).toBe("ck-1000-uuid-one");
    expect(second).not.toBe(first);
    expect(first.length).toBeLessThanOrEqual(64);

    // 随机段里的非法字符会被剔除（后端只接受一个普通字符串字段）。
    expect(buildChatClientMsgId(() => "a/b c", () => 1000)).toBe("ck-1000-abc");
  });
});

/**
 * `draft_receiver_*` 是作者主页「发私信」→ 会话页的**跨页契约**（票 #240 验收：
 * 从用户主页发私信仍带 draft 直接进会话）。解析放在纯函数里，页面只做接线。
 *
 * 契约原样保留：用户主页写的是 `draft_receiver_id / nickname / avatar` 与
 * `draft_relation`，字段名与含义都不许改（改了旧链接与旧页面就断）。
 */
describe("私信草稿 query 契约（draft_receiver_*）", () => {
  it("接收方 id 是必需项：缺 id 或空白一律视为没有草稿", () => {
    expect(parseDraftSessionSeed({})).toBeNull();
    expect(parseDraftSessionSeed({ draft_receiver_id: "" })).toBeNull();
    expect(parseDraftSessionSeed({ draft_receiver_id: "   " })).toBeNull();
  });

  it("完整 query 解析成会话种子（昵称 / 头像 / 关系）", () => {
    expect(
      parseDraftSessionSeed({
        draft_receiver_id: "u9",
        draft_receiver_nickname: "乙",
        draft_receiver_avatar: "https://image.example.com/b.png",
        draft_relation: "2",
      }),
    ).toEqual({
      receiverId: "u9",
      receiverNickname: "乙",
      receiverAvatar: "https://image.example.com/b.png",
      relation: 2,
    });
  });

  it("昵称缺失时回落到「新会话」，头像缺失按空串，关系非法按 0", () => {
    expect(parseDraftSessionSeed({ draft_receiver_id: "u9" })).toEqual({
      receiverId: "u9",
      receiverNickname: "新会话",
      receiverAvatar: "",
      relation: 0,
    });
    expect(parseDraftSessionSeed({ draft_receiver_id: "u9", draft_relation: "not-a-number" })?.relation).toBe(0);
  });

  // 路由 query 可能是数组（重复参数）：取第一个，别把数组拼进 id。
  it("query 值可能是数组时取第一个字符串", () => {
    expect(parseDraftSessionSeed({ draft_receiver_id: ["u9", "u10"] })?.receiverId).toBe("u9");
  });

  it("与路由无关的 query 被忽略，不污染种子", () => {
    expect(parseDraftSessionSeed({ from: "profile" })).toBeNull();
  });

  // 票 265：写入口收进 buildDraftChatQuery，作者主页与文章页不再各抄一份字段。
  it("写入口生成四个字段，且能被解析端原样读回（写读闭环）", () => {
    const query = buildDraftChatQuery({
      receiverId: "u9",
      receiverNickname: "乙",
      receiverAvatar: "https://image.example.com/b.png",
      relation: 2,
    });

    expect(query).toEqual({
      draft_receiver_id: "u9",
      draft_receiver_nickname: "乙",
      draft_receiver_avatar: "https://image.example.com/b.png",
      draft_relation: "2",
    });
    expect(parseDraftSessionSeed(query)).toEqual({
      receiverId: "u9",
      receiverNickname: "乙",
      receiverAvatar: "https://image.example.com/b.png",
      relation: 2,
    });
  });

  it("头像为空时不带该字段，relation 缺省按 0", () => {
    const query = buildDraftChatQuery({ receiverId: "u9", receiverNickname: "乙" });

    expect(query).not.toHaveProperty("draft_receiver_avatar");
    expect(query.draft_relation).toBe("0");
    expect(parseDraftSessionSeed(query)).toMatchObject({ receiverId: "u9", relation: 0 });
  });
});

/**
 * WS 通知分两类：新消息（`chat.message.sent`）与已读回执（`chat.message.read`）。
 * 未读 +1 只对前者成立 —— 已读回执也带 `session_id`，但它不是"又收到一条"。
 */
describe("WS 通知归类", () => {
  it("新消息通知：带 session_id + message_id，且不是已读回执", () => {
    expect(
      isNewChatMessageNotification({
        session_id: "chat:1:2",
        message_id: "m1",
        sender_id: "2",
        receiver_id: "1",
        msg_type: 1,
      }),
    ).toBe(true);
  });

  it("已读回执（msg_type=8）不算新消息，即使带 message_id", () => {
    expect(
      isNewChatMessageNotification({
        session_id: "chat:1:2",
        message_id: "m1",
        msg_type: 8,
        reader_id: "2",
      }),
    ).toBe(false);
  });

  it("缺 message_id / session_id 的帧不算新消息（心跳、拒绝壳）", () => {
    expect(isNewChatMessageNotification({ session_id: "chat:1:2" })).toBe(false);
    expect(isNewChatMessageNotification({ message_id: "m1" })).toBe(false);
    expect(isNewChatMessageNotification(null)).toBe(false);
  });
});

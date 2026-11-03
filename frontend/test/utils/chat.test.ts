import { describe, expect, it } from "vitest";
import {
  CHAT_DRAFT_SESSION_PREFIX,
  buildChatClientMsgId,
  buildDraftChatSessionId,
  buildPreparedChatSession,
  isDraftChatSessionId,
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

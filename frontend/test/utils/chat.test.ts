import { describe, expect, it } from "vitest";
import {
  CHAT_DRAFT_SESSION_PREFIX,
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
});

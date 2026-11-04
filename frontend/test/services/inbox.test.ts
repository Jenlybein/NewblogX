import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  deleteChatMessages,
  deleteChatSessions,
  getChatMessages,
  getChatSessions,
  getChatWsTicket,
  markChatMessagesRead,
  markChatSessionRead,
  sendChatMessage,
} from "~/services/inbox";

// 真实契约测试（#201）。
//
// 现状：私信链路的用例几乎全是 mock 的编排测试，`services/inbox.ts` 没有一条
// 校验真实请求形状的用例 —— 路径写错、方法写成 GET、载荷键名漂移，全都不会红，
// 只会在生产里 404/参数错误。
//
// 这里对 mock 的 `$api.request` 断言每个私信接口的**路径、方法、载荷形状**，
// 与后端 handler 的 swagger 注解 + 集成用例互为对账。
const requestMock = vi.fn();

function stubNuxtApp() {
  vi.stubGlobal("__useNuxtAppMock", () => ({
    $api: {
      request: requestMock,
    },
  }));
}

describe("私信 service：真实请求契约", () => {
  beforeEach(() => {
    stubNuxtApp();
    requestMock.mockReset();
    requestMock.mockResolvedValue({});
  });

  it("发送私信是 POST /api/chat/messages，body 带幂等键 client_msg_id", async () => {
    await sendChatMessage({
      receiver_id: "u2",
      msg_type: 1,
      content: "你好",
      client_msg_id: "ck-1",
    });

    expect(requestMock).toHaveBeenCalledTimes(1);
    expect(requestMock).toHaveBeenCalledWith("/api/chat/messages", {
      method: "POST",
      body: {
        receiver_id: "u2",
        msg_type: 1,
        content: "你好",
        client_msg_id: "ck-1",
      },
    });
  });

  // 游标分页改造：聊天窗口缺省 7 条，不再发 page；后端按 send_time/id DESC 返回。
  it("消息列表是 GET /api/chat/messages，query 带 session_id / type / limit=7", async () => {
    const page = await getChatMessages({ sessionId: "chat:1:2" });

    expect(requestMock).toHaveBeenCalledWith("/api/chat/messages", {
      query: {
        session_id: "chat:1:2",
        type: 1,
        limit: 7,
      },
    });
    expect(page).toEqual({
      list: [],
      count: 0,
      has_more: false,
      next_cursor: { before_send_time: "", before_id: "" },
    });
  });

  it("消息列表支持显式 limit、user_id 与复合游标", async () => {
    await getChatMessages({
      sessionId: "chat:1:2",
      userId: "9",
      type: 2,
      limit: 20,
      cursor: { before_send_time: "2026-11-05T01:07:00.123456789Z", before_id: "123456789" },
    });

    expect(requestMock).toHaveBeenCalledWith("/api/chat/messages", {
      query: {
        session_id: "chat:1:2",
        type: 2,
        user_id: "9",
        limit: 20,
        before_send_time: "2026-11-05T01:07:00.123456789Z",
        before_id: "123456789",
      },
    });
  });

  it("空游标的 before_send_time 不进 query（第一页不带游标参数）", async () => {
    await getChatMessages({
      sessionId: "chat:1:2",
      cursor: { before_send_time: "", before_id: "" },
    });

    expect(requestMock).toHaveBeenCalledWith("/api/chat/messages", {
      query: {
        session_id: "chat:1:2",
        type: 1,
        limit: 7,
      },
    });
  });

  it("分页响应原样归一化：has_more / next_cursor / count 都落到本地接口", async () => {
    const cursor = { before_send_time: "2026-11-05T01:07:00.123456789Z", before_id: "123456789" };
    requestMock.mockResolvedValue({
      list: [{ id: "1", send_time: "2026-11-05T01:07:00Z" }],
      count: 123,
      has_more: true,
      next_cursor: cursor,
    });

    const page = await getChatMessages({ sessionId: "chat:1:2" });

    expect(page.count).toBe(123);
    expect(page.has_more).toBe(true);
    expect(page.next_cursor).toEqual(cursor);
    expect(page.list).toHaveLength(1);
  });

  it("旧形状响应（缺 has_more / next_cursor）不炸，归一化成空游标", async () => {
    requestMock.mockResolvedValue({ list: [{ id: "1" }], count: 1 });

    const page = await getChatMessages({ sessionId: "chat:1:2" });

    expect(page.has_more).toBe(false);
    expect(page.next_cursor).toEqual({ before_send_time: "", before_id: "" });
    expect(page.list).toEqual([{ id: "1" }]);
  });

  it("会话列表是 GET /api/chat/sessions，query 带 type=1", async () => {
    await getChatSessions({});

    expect(requestMock).toHaveBeenCalledWith("/api/chat/sessions", {
      query: { type: "1" },
    });
  });

  it("整会话已读是 POST /api/chat/read，body 只有 session_id", async () => {
    await markChatSessionRead("chat:1:2");

    expect(requestMock).toHaveBeenCalledWith("/api/chat/read", {
      method: "POST",
      body: { session_id: "chat:1:2" },
    });
  });

  it("按消息已读是 POST /api/chat/read，body 是字符串数组 msg_id_list", async () => {
    await markChatMessagesRead(["m1", "m2"]);

    expect(requestMock).toHaveBeenCalledWith("/api/chat/read", {
      method: "POST",
      body: { msg_id_list: ["m1", "m2"] },
    });
  });

  it("删除消息是 DELETE /api/chat/messages，body 是 msg_id_list", async () => {
    await deleteChatMessages(["m1"]);

    expect(requestMock).toHaveBeenCalledWith("/api/chat/messages", {
      method: "DELETE",
      body: { msg_id_list: ["m1"] },
    });
  });

  it("删除会话是 DELETE /api/chat/sessions，body 是 session_id_list", async () => {
    await deleteChatSessions(["chat:1:2"]);

    expect(requestMock).toHaveBeenCalledWith("/api/chat/sessions", {
      method: "DELETE",
      body: { session_id_list: ["chat:1:2"] },
    });
  });

  it("WS 票据是 GET /api/chat/ws-ticket", async () => {
    await getChatWsTicket();

    expect(requestMock).toHaveBeenCalledWith("/api/chat/ws-ticket");
  });
});

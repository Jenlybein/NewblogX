import { flushPromises } from "@vue/test-utils";
import { effectScope, ref, shallowRef, toValue, watch } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatMessageItem, ChatNotificationPayload, ChatSessionItem } from "~/types/api";

/**
 * 私信会话 composable（票 #240 收窄后）。
 *
 * 范围只剩**私信**：评论 / 互动 / 系统 / 全局通知已由 useMessageCenter（#239）接管。
 * 所以这里的站点消息与全局通知桩全部删掉 —— 再出现就是死代码又长回来了。
 */

const getChatSessionsMock = vi.hoisted(() => vi.fn());
const getChatMessagesMock = vi.hoisted(() => vi.fn());
const sendChatMessageMock = vi.hoisted(() => vi.fn());
const refreshSummaryMock = vi.hoisted(() => vi.fn());

const messageSummaryMock = vi.hoisted(() => ({
  comment_msg_count: 1,
  digg_favor_msg_count: 6,
  private_msg_count: 0,
  system_msg_count: 2,
  global_msg_count: 1,
}));

// WS 通知现在是订阅制：composable 用 onNotification 注册自己的回调。
// 这里把订阅者收集起来，测试自己触发（等价于收到一条下行帧）。
const notificationHandlers = new Set<(notification: ChatNotificationPayload) => void>();

// 发送被拒原因（票 202）：useInboxCenter 发送失败时写它，界面据此就地提示。
const sendErrorState = shallowRef("");

vi.mock("~/services/inbox", () => ({
  deleteChatMessages: vi.fn(),
  deleteChatSessions: vi.fn(),
  getChatMessages: getChatMessagesMock,
  getChatSessions: getChatSessionsMock,
  markChatMessagesRead: vi.fn(),
  sendChatMessage: sendChatMessageMock,
}));

vi.mock("~/utils/chat", async (importOriginal) => {
  const actual = await importOriginal<typeof import("~/utils/chat")>();
  return {
    ...actual,
    buildChatClientMsgId: (random: () => string, now: () => number) => `ck-${now()}-${random()}`,
  };
});

const chatSession = {
  session_id: "chat:u1:u2",
  receiver_id: "u2",
  receiver_nickname: "乙",
  receiver_avatar: "",
  relation: 2,
  last_msg_content: "你好",
  last_msg_time: "2026-04-17T00:00:00.000+08:00",
  unread_count: 1,
  is_top: false,
  is_mute: false,
} satisfies ChatSessionItem;

// chat store 的可变桩：composable 会通过 setActiveSession 写它，
// 用普通对象即可（computed 在会话列表变化时重新求值）。
const chatStoreState = {
  activeSessionId: null as string | null,
  setActiveSession: (id: string | null) => {
    chatStoreState.activeSessionId = id;
  },
};

function pushNotification(payload: ChatNotificationPayload) {
  for (const handler of [...notificationHandlers]) {
    handler(payload);
  }
}

async function createInboxCenter(options: { draftSessionSeed?: ReturnType<typeof ref> } = {}) {
  vi.resetModules();
  const module = await import("~/composables/useInboxCenter");
  return module.useInboxCenter(options);
}

describe("useInboxCenter（私信）", () => {
  beforeEach(() => {
    getChatSessionsMock.mockReset();
    getChatMessagesMock.mockReset();
    sendChatMessageMock.mockReset();
    refreshSummaryMock.mockClear();
    refreshSummaryMock.mockResolvedValue(messageSummaryMock);
    notificationHandlers.clear();
    sendErrorState.value = "";
    chatStoreState.activeSessionId = null;
    messageSummaryMock.private_msg_count = 0;

    getChatSessionsMock.mockResolvedValue({ list: [] });
    getChatMessagesMock.mockResolvedValue({ list: [] });

    vi.stubGlobal("watch", watch);
    vi.stubGlobal("toValue", toValue);
    vi.stubGlobal("useAuthStore", () => ({
      isLoggedIn: true,
      profileId: "u1",
    }));
    vi.stubGlobal("useMessageStore", () => ({
      summary: messageSummaryMock,
      refreshSummary: refreshSummaryMock,
    }));
    vi.stubGlobal("useChatStore", () => chatStoreState);
    vi.stubGlobal("useChatSocket", () => ({
      socketStatus: shallowRef("error"),
      socketError: shallowRef("实时未连接"),
      sendError: sendErrorState,
      setSendError: (reason: string) => {
        sendErrorState.value = reason;
      },
      connect: vi.fn(),
      onNotification: (handler: (notification: ChatNotificationPayload) => void) => {
        notificationHandlers.add(handler);
        return () => {
          notificationHandlers.delete(handler);
        };
      },
    }));
  });

  // 收窄后的 composable 不再有 activeTab 开关：进入这个 composable 就等于进入私信子项。
  it("创建即拉会话，并自动选中第一条；随后拉它的消息", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [chatSession] });
    getChatMessagesMock.mockResolvedValue({ list: [] });

    const inbox = await createInboxCenter();
    await flushPromises();

    expect(getChatSessionsMock).toHaveBeenCalledWith({ type: 1 });
    expect(inbox.chatSessions.value).toHaveLength(1);
    expect(inbox.activeSessionId.value).toBe(chatSession.session_id);
    expect(getChatMessagesMock).toHaveBeenCalledWith({ sessionId: chatSession.session_id, type: 1 });
  });

  // ── 草稿直达会话（users/[id].vue 的 draft_receiver_* 契约）──
  it("草稿种子命中已有会话时直接选中它，不建预备会话", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [chatSession] });
    const seed = ref({
      receiverId: chatSession.receiver_id,
      receiverNickname: "乙",
      receiverAvatar: "",
      relation: 2,
    });

    const inbox = await createInboxCenter({ draftSessionSeed: seed });
    await flushPromises();

    expect(inbox.preparedSession.value).toBeNull();
    expect(inbox.activeSessionId.value).toBe(chatSession.session_id);
  });

  it("草稿种子没有历史会话时生成预备会话并选中（首条消息发出前不请求后端）", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [] });
    const seed = ref({
      receiverId: "u9",
      receiverNickname: "丙",
      receiverAvatar: "https://image.example.com/c.png",
      relation: 0,
    });

    const inbox = await createInboxCenter({ draftSessionSeed: seed });
    await flushPromises();

    const prepared = inbox.preparedSession.value;
    expect(prepared).not.toBeNull();
    expect(prepared?.receiver_id).toBe("u9");
    expect(inbox.activeSession.value?.receiver_nickname).toBe("丙");
    // 预备会话没有服务端消息可拉：不该为一个还不存在的会话打 messages 接口。
    expect(getChatMessagesMock).not.toHaveBeenCalled();
  });

  // 真实契约：发送必须走 HTTP（POST /api/chat/messages），且 body 形状与后端
  // DTO 一致（receiver_id / msg_type / content / client_msg_id）。
  // 路径与方法由 test/services/inbox.test.ts 对 $api.request 断言；
  // 这里钉住 composable 传给服务层的载荷 —— 两者一起才是完整契约。
  it("发送私信走 HTTP 且载荷带幂等键；WS 断开也照常发出", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [chatSession] });
    sendChatMessageMock.mockResolvedValue({
      id: "m1",
      sender_id: "u1",
      receiver_id: "u2",
      session_id: chatSession.session_id,
      content: "你好",
      send_time: "2026-04-17T00:00:00.000+08:00",
      msg_status: 1,
      msg_type: 1,
      is_self: true,
      is_read: false,
    } satisfies ChatMessageItem);

    const inbox = await createInboxCenter();
    await flushPromises();

    inbox.chatDraft.value = "你好";
    const result = await inbox.sendCurrentChatMessage();

    // socketStatus 桩固定是 error：WS 没连接也不影响发送（HTTP 是唯一发送通道）。
    expect(inbox.socketStatus.value).toBe("error");
    expect(sendChatMessageMock).toHaveBeenCalledTimes(1);
    const payload = sendChatMessageMock.mock.calls[0]?.[0];
    expect(payload).toMatchObject({
      receiver_id: "u2",
      msg_type: 1,
      content: "你好",
    });
    expect(payload.client_msg_id).toMatch(/^ck-/u);
    expect(result).toMatchObject({ usedPreparedSession: false, resolvedSessionId: chatSession.session_id });
  });

  // 幂等：同一次发送失败后重试，必须复用同一个 client_msg_id，
  // 否则"其实已落库、只是响应丢了"的重试会变成第二条消息。
  it("发送失败后重试复用同一个幂等键", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [chatSession] });
    sendChatMessageMock.mockRejectedValueOnce(new Error("网络中断")).mockResolvedValueOnce({
      id: "m1",
      session_id: chatSession.session_id,
      content: "你好",
    });

    const inbox = await createInboxCenter();
    await flushPromises();

    inbox.chatDraft.value = "你好";
    await expect(inbox.sendCurrentChatMessage()).rejects.toThrow("网络中断");
    expect(inbox.chatDraft.value).toBe("你好");

    const retried = await inbox.sendCurrentChatMessage();
    expect(retried).toMatchObject({ resolvedSessionId: chatSession.session_id });

    const firstKey = sendChatMessageMock.mock.calls[0]?.[0]?.client_msg_id;
    const retryKey = sendChatMessageMock.mock.calls[1]?.[0]?.client_msg_id;
    expect(retryKey).toBe(firstKey);
  });

  // ── WS 通知（订阅制，不再是"取走队列"）──
  it("收到 WS 通知后刷新会话列表与未读数", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [chatSession] });

    await createInboxCenter();
    await flushPromises();

    getChatSessionsMock.mockClear();
    refreshSummaryMock.mockClear();
    getChatMessagesMock.mockClear();

    pushNotification({ session_id: "chat:u1:u3", message_id: "m2", sender_id: "u3", msg_type: 1 });
    await flushPromises();

    expect(getChatSessionsMock).toHaveBeenCalledTimes(1);
    expect(refreshSummaryMock).toHaveBeenCalledTimes(1);
    // 通知属于另一个会话（不是当前打开的 chat:u1:u2），不该重拉当前会话消息。
    expect(getChatMessagesMock).not.toHaveBeenCalled();
  });

  // 通知命中当前会话时也要刷新消息（"即时到达"靠这一步）。
  it("WS 通知命中当前会话时刷新消息", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [chatSession] });

    await createInboxCenter();
    await flushPromises();

    getChatMessagesMock.mockClear();
    pushNotification({ session_id: chatSession.session_id, message_id: "m2", sender_id: "u2", msg_type: 1 });
    await flushPromises();

    expect(getChatMessagesMock).toHaveBeenCalledWith({ sessionId: chatSession.session_id, type: 1 });
  });

  // 票 #240："离开私信页不关闭连接"——连接归全局所有，但要退订本页的通知。
  // 这条钉住"退订"这一半（关连接那半由 useChatSocket / useMessageRealtime 负责）。
  it("所属作用域销毁时只退订通知，不再刷新会话", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [chatSession] });

    vi.resetModules();
    const module = await import("~/composables/useInboxCenter");
    // 作用域只影响"注册到谁身上"：onScopeDispose 是同步注册的，异步 bootstrap 不受影响。
    const scope = effectScope();
    const inbox = scope.run(() => module.useInboxCenter())!;

    await flushPromises();
    expect(notificationHandlers.size).toBe(1);

    scope.stop();
    expect(notificationHandlers.size).toBe(0);

    getChatSessionsMock.mockClear();
    pushNotification({ session_id: "chat:u1:u3", message_id: "m3", msg_type: 1 });
    await flushPromises();
    expect(getChatSessionsMock).not.toHaveBeenCalled();
    expect(inbox.chatSessions.value).toHaveLength(1);
  });

  // 拒绝原因透出（票 202 的界面出口）：HTTP 发送失败时后端文案要原样到达界面，
  // 用户才不会把"站点关闭陌生人私信"误认成断网。
  it("发送被站点策略拒绝时把后端文案透出到 chatSendError", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [chatSession] });
    const policyMessage = "站点当前不允许陌生人发私信（站点策略），请联系管理员开启";
    sendChatMessageMock.mockRejectedValueOnce(new Error(policyMessage));

    const inbox = await createInboxCenter();
    await flushPromises();

    inbox.chatDraft.value = "你好";
    await expect(inbox.sendCurrentChatMessage()).rejects.toThrow("站点");

    expect(inbox.chatSendError.value).toBe(policyMessage);
  });

  // 成功要清掉上一次的拒绝原因，否则界面上一直挂着旧错误。
  it("发送成功后清空上一次的拒绝原因", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [chatSession] });
    sendChatMessageMock.mockResolvedValue({
      id: "m1",
      session_id: chatSession.session_id,
      content: "你好",
    });
    sendErrorState.value = "上一次发送失败";

    const inbox = await createInboxCenter();
    await flushPromises();

    inbox.chatDraft.value = "你好";
    await inbox.sendCurrentChatMessage();

    expect(inbox.chatSendError.value).toBe("");
  });
});

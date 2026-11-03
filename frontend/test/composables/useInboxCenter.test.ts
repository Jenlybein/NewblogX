import { flushPromises } from "@vue/test-utils";
import { computed, nextTick, ref, shallowRef, toValue, watch } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatMessageItem, ChatSessionItem, GlobalNoticeItem, SiteMessageItem } from "~/types/api";

const getSiteMessagesMock = vi.hoisted(() => vi.fn());
const getGlobalNoticesMock = vi.hoisted(() => vi.fn());
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

// WS 下行帧的"收件箱"：真实 composable 用 shallowRef，这里用同一个 ref 模拟推送。
// 必须是真 ref —— useInboxCenter 靠 inbox.value.length 的 watch 感知通知。
const socketInbox = shallowRef<unknown[]>([]);

vi.mock("~/services/inbox", () => ({
  deleteChatMessages: vi.fn(),
  deleteChatSessions: vi.fn(),
  deleteGlobalNoticeForCurrentUser: vi.fn(),
  deleteSiteMessage: vi.fn(),
  getChatMessages: getChatMessagesMock,
  getChatSessions: getChatSessionsMock,
  getGlobalNotices: getGlobalNoticesMock,
  getSiteMessages: getSiteMessagesMock,
  markChatMessagesRead: vi.fn(),
  markGlobalNoticeRead: vi.fn(),
  markSiteMessageRead: vi.fn(),
  sendChatMessage: sendChatMessageMock,
}));

vi.mock("~/utils/chat", () => ({
  buildChatClientMsgId: (random: () => string, now: () => number) => `ck-${now()}-${random()}`,
  buildPreparedChatSession: vi.fn(),
  isDraftChatSessionId: (id: string) => id.startsWith("draft-"),
}));

const commentMessage = {
  id: "comment-1",
  created_at: "2026-04-17T00:00:00.000+08:00",
  updated_at: "2026-04-17T00:00:00.000+08:00",
  type: 1,
  receiver_id: "u1",
  action_user_id: "u2",
  action_user_nickname: "River",
  action_user_avatar: "",
  content: "评论了你的文章",
  article_id: "a1",
  comment_id: "c1",
  article_title: "文章",
  link_title: "",
  link_herf: "",
  is_read: false,
  read_at: null,
} satisfies SiteMessageItem;

const globalNotice = {
  id: "global-1",
  create_at: "2026-04-17T00:00:00.000+08:00",
  title: "全局通知",
  icon: "",
  content: "全局通知内容",
  herf: "",
  is_read: false,
} satisfies GlobalNoticeItem;

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

// takeNotifications 的桩：把 inbox 里的下行帧取走并返回 data。
function takeNotificationsStub() {
  const notifications = (socketInbox.value as Array<{ data?: Record<string, unknown> }>)
    .map((envelope) => envelope?.data)
    .filter((data): data is Record<string, unknown> => Boolean(data) && typeof data?.session_id === "string");
  socketInbox.value = [];
  return notifications;
}

async function createInboxCenter() {
  vi.resetModules();
  const module = await import("~/composables/useInboxCenter");
  const inbox = module.useInboxCenter();
  await flushPromises();
  await nextTick();
  return inbox;
}

describe("useInboxCenter", () => {
  beforeEach(() => {
    getSiteMessagesMock.mockReset();
    getGlobalNoticesMock.mockReset();
    getChatSessionsMock.mockReset();
    getChatMessagesMock.mockReset();
    sendChatMessageMock.mockReset();
    refreshSummaryMock.mockClear();
    refreshSummaryMock.mockResolvedValue(messageSummaryMock);
    socketInbox.value = [];
    chatStoreState.activeSessionId = null;
    messageSummaryMock.comment_msg_count = 1;
    messageSummaryMock.digg_favor_msg_count = 6;
    messageSummaryMock.private_msg_count = 0;
    messageSummaryMock.system_msg_count = 2;
    messageSummaryMock.global_msg_count = 1;

    getSiteMessagesMock.mockResolvedValue({ list: [], has_more: false });
    getGlobalNoticesMock.mockResolvedValue({ list: [], has_more: false });
    getChatSessionsMock.mockResolvedValue({ list: [] });
    getChatMessagesMock.mockResolvedValue({ list: [] });

    vi.stubGlobal("computed", computed);
    vi.stubGlobal("ref", ref);
    vi.stubGlobal("shallowRef", shallowRef);
    vi.stubGlobal("toValue", toValue);
    vi.stubGlobal("watch", watch);
    vi.stubGlobal("useAuthStore", () => ({
      isLoggedIn: true,
      profileId: "u1",
    }));
    vi.stubGlobal("useMessageStore", () => ({
      summary: messageSummaryMock,
      refreshSummary: refreshSummaryMock,
    }));    vi.stubGlobal("useChatStore", () => chatStoreState);
    vi.stubGlobal("useChatSocket", () => ({
      socketStatus: shallowRef("error"),
      socketError: shallowRef("实时未连接"),
      connect: vi.fn(),
      takeNotifications: takeNotificationsStub,
      inbox: socketInbox,
    }));
  });

  it("prefetches global notices on the site tab so the global badge is not stuck at zero", async () => {
    getSiteMessagesMock.mockResolvedValue({ list: [commentMessage], has_more: false });
    getGlobalNoticesMock.mockResolvedValue({ list: [globalNotice], has_more: false });

    const inbox = await createInboxCenter();

    expect(getGlobalNoticesMock).toHaveBeenCalledWith({ page: 1, limit: 9, type: 1 });
    expect(inbox.globalNotices.value).toEqual([globalNotice]);
  });

  it("uses the separated summary field for the system notification badge", async () => {
    getSiteMessagesMock.mockImplementation(({ group }: { group: 1 | 2 | 3 }) =>
      Promise.resolve({
        list: group === 3 ? [] : [commentMessage],
        has_more: false,
      }),
    );
    getGlobalNoticesMock.mockResolvedValue({ list: [globalNotice], has_more: false });

    const inbox = await createInboxCenter();

    expect(getSiteMessagesMock).not.toHaveBeenCalledWith({ group: 3, page: 1, limit: 9 });
    expect(inbox.siteCategories.value.find((item) => item.key === 3)?.count).toBe(2);
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
    inbox.activeTab.value = "chat";
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
    inbox.activeTab.value = "chat";
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

  // 真实契约：WS 只推通知，收到通知后刷新会话与未读（不依赖通知帧带正文）。
  it("收到 WS 通知后刷新会话列表与未读数", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [chatSession] });

    const inbox = await createInboxCenter();
    inbox.activeTab.value = "chat";
    await flushPromises();

    getChatSessionsMock.mockClear();
    refreshSummaryMock.mockClear();
    getChatMessagesMock.mockClear();
    socketInbox.value = [
      {
        code: 0,
        msg: "成功",
        data: { session_id: "chat:u1:u3", message_id: "m2", sender_id: "u3", receiver_id: "u1" },
      },
    ];
    await flushPromises();

    expect(getChatSessionsMock).toHaveBeenCalledTimes(1);
    expect(refreshSummaryMock).toHaveBeenCalledTimes(1);
    // 通知属于另一个会话（不是当前打开的 chat:u1:u2），不该重拉当前会话消息。
    expect(getChatMessagesMock).not.toHaveBeenCalled();
  });

  // 通知命中当前会话时也要刷新消息（"即时到达"靠这一步）。
  it("WS 通知命中当前会话时刷新消息", async () => {
    getChatSessionsMock.mockResolvedValue({ list: [chatSession] });

    const inbox = await createInboxCenter();
    inbox.activeTab.value = "chat";
    await flushPromises();

    getChatMessagesMock.mockClear();
    socketInbox.value = [
      {
        code: 0,
        msg: "成功",
        data: { session_id: chatSession.session_id, message_id: "m2", sender_id: "u2", receiver_id: "u1" },
      },
    ];
    await flushPromises();

    expect(getChatMessagesMock).toHaveBeenCalledWith({ sessionId: chatSession.session_id, type: 1 });
  });
});

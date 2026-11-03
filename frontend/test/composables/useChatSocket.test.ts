import { beforeEach, describe, expect, it, vi } from "vitest";

// useChatSocket 在构造时注册 onBeforeUnmount。这里没有真实组件实例，
// 直接把它换成空实现，避免每条用例都刷一遍 Vue 警告（警告本身没有信息量）。
vi.mock("vue", async (importOriginal) => {
  const actual = await importOriginal<typeof import("vue")>();
  return { ...actual, onBeforeUnmount: vi.fn() };
});

const getChatWsTicketMock = vi.hoisted(() => vi.fn());

vi.mock("~/services/inbox", () => ({
  getChatWsTicket: getChatWsTicketMock,
}));

// 最小 WebSocket 替身：只实现 useChatSocket 真正用到的表面
// （readyState / onopen / onmessage / onerror / onclose / close）。
class FakeWebSocket {
  static OPEN = 1;
  static CONNECTING = 0;
  static CLOSED = 3;
  static instances: FakeWebSocket[] = [];

  readyState = FakeWebSocket.CONNECTING;
  url: string;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;
  sent: string[] = [];

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }

  send(data: string) {
    this.sent.push(data);
  }

  close() {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.();
  }
}

const chatStoreState = {
  socketStatus: "idle",
  socketError: "",
  setSocketStatus: (status: string, error = "") => {
    chatStoreState.socketStatus = status;
    chatStoreState.socketError = error;
  },
  resetSocketState: () => {
    chatStoreState.socketStatus = "idle";
    chatStoreState.socketError = "";
  },
  markSocketMessage: vi.fn(),
};

async function createChatSocket() {
  vi.resetModules();
  const module = await import("~/composables/useChatSocket");
  return module.useChatSocket();
}

describe("useChatSocket（WS 只做通知，不做发送）", () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    chatStoreState.socketStatus = "idle";
    chatStoreState.socketError = "";
    getChatWsTicketMock.mockReset();
    getChatWsTicketMock.mockResolvedValue({ ticket: "tk-1" });

    vi.stubGlobal("WebSocket", FakeWebSocket);
    vi.stubGlobal("__useRuntimeConfigMock", () => ({
      public: { siteUrl: "http://localhost", wsPath: "/api/chat/ws" },
    }));
    vi.stubGlobal("useAuthStore", () => ({ isLoggedIn: true }));
    vi.stubGlobal("useChatStore", () => chatStoreState);
  });

  it("连接时先取票据，再用 ticket 打开 WS（不再有 sendMessage 出口）", async () => {
    const chat = await createChatSocket();
    const socket = await chat.connect();

    expect(getChatWsTicketMock).toHaveBeenCalledTimes(1);
    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(FakeWebSocket.instances[0]?.url).toContain("ticket=tk-1");
    expect(socket).toBe(FakeWebSocket.instances[0]);

    // 发送已经统一走 HTTP：composable 上不该再有 WS 发送能力。
    expect("sendMessage" in chat).toBe(false);
  });

  it("下行帧收窄成通知载荷：有 session_id 才算通知，取走即清空", async () => {
    const chat = await createChatSocket();
    await chat.connect();
    const ws = FakeWebSocket.instances[0]!;

    ws.onmessage?.({
      data: JSON.stringify({
        code: 0,
        msg: "成功",
        data: { session_id: "chat:1:2", message_id: "m1", sender_id: "2" },
      }),
    });

    const notifications = chat.takeNotifications();
    expect(notifications).toEqual([
      { session_id: "chat:1:2", message_id: "m1", sender_id: "2" },
    ]);
    expect(chat.inbox.value).toHaveLength(0);

    // 没有 session_id 的帧（心跳/错误）不算通知，不该触发刷新。
    ws.onmessage?.({ data: JSON.stringify({ code: 0, msg: "成功", data: { kind: "ping" } }) });
    expect(chat.takeNotifications()).toEqual([]);
  });
});

// 发送被拒原因的读者侧接线（票 202 的原意；#201 后拒绝原因来自 HTTP）。
//
// 服务端出站帧现在是响应壳（code / msg / data）：只有 code 非 0 才代表
// "这次请求被拒绝/出错"。把 code=0 的通知帧当成失败的话，每收到一条
// 新消息都会弹一次错误。
describe("useChatSocket：拒绝原因", () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    chatStoreState.socketStatus = "idle";
    chatStoreState.socketError = "";
    chatStoreState.markSocketMessage.mockClear();
    getChatWsTicketMock.mockReset();
    getChatWsTicketMock.mockResolvedValue({ ticket: "tk-1" });

    vi.stubGlobal("WebSocket", FakeWebSocket);
    vi.stubGlobal("__useRuntimeConfigMock", () => ({
      public: { siteUrl: "http://localhost", wsPath: "/api/chat/ws" },
    }));
    vi.stubGlobal("useAuthStore", () => ({ isLoggedIn: true }));
    vi.stubGlobal("useChatStore", () => chatStoreState);
  });

  it("拒绝帧的文案原样暴露给界面（不另造措辞）", async () => {
    const chat = await createChatSocket();
    await chat.connect();
    const ws = FakeWebSocket.instances[0]!;

    ws.onmessage?.({
      data: JSON.stringify({ code: 1001, msg: "站点当前不允许陌生人发私信（站点策略，不是网络问题）", data: null }),
    });

    expect(chat.sendError.value).toBe("站点当前不允许陌生人发私信（站点策略，不是网络问题）");
  });

  it("正常通知帧 code=0，不会被误判成失败", async () => {
    const chat = await createChatSocket();
    await chat.connect();
    const ws = FakeWebSocket.instances[0]!;

    ws.onmessage?.({
      data: JSON.stringify({
        code: 0,
        msg: "成功",
        data: { session_id: "chat:1:2", message_id: "m1", sender_id: "2" },
      }),
    });

    expect(chat.sendError.value).toBe("");
  });

  it("拒绝帧缺 msg 时给一句兜底文案，不显示空白错误", async () => {
    const chat = await createChatSocket();
    await chat.connect();
    const ws = FakeWebSocket.instances[0]!;

    ws.onmessage?.({ data: JSON.stringify({ code: 1002, msg: "", data: null }) });

    expect(chat.sendError.value).toBe("消息发送失败。");
  });

  it("发送成功后清空上一次的拒绝原因", async () => {
    const chat = await createChatSocket();

    chat.setSendError("站点当前不允许陌生人发私信");
    expect(chat.sendError.value).not.toBe("");

    // useInboxCenter 在 HTTP 发送成功后调用 setSendError("")。
    chat.setSendError("");
    expect(chat.sendError.value).toBe("");
  });
});

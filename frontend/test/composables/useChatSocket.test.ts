import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

// useChatSocket 现在是**全局单例**（票 #240）：连接不绑在某个页面的生命周期上。
// 这里不再需要桩掉 onBeforeUnmount —— 那正是本票删掉的东西（设计门禁会断言源码
// 里没有它，见 test/design/message-center.test.ts）。
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

  // 票 #240 的核心：连接是**进程内共享的**。页头（全局未读订阅）、消息中心
  // 私信页（会话刷新）各自调用 useChatSocket()，必须落到同一条连接上 ——
  // 否则同一个用户会开两条长连接，且离开页面时把别人的连接关掉（旧行为）。
  it("多次调用共享同一条连接（不是每个调用方各建一条）", async () => {
    const first = await createChatSocket();
    await first.connect();

    // 同一个模块实例里的第二次调用 = 页头 / 私信页的另一个消费方。
    const module = await import("~/composables/useChatSocket");
    const second = module.useChatSocket();
    const socket = await second.connect();

    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(socket).toBe(FakeWebSocket.instances[0]);
    expect(first.socketStatus.value).toBe(second.socketStatus.value);
  });

  // 通知用"订阅"而不是"取走队列"：消费方数量不固定（全局未读 + 当前私信页），
  // 谁先取走谁就把别人的通知吃掉（旧 takeNotifications 的真实缺陷）。
  it("下行帧收窄成通知载荷推给订阅者；非通知帧不推送", async () => {
    const chat = await createChatSocket();
    const received: Array<Record<string, unknown>> = [];
    const stop = chat.onNotification((notification) => received.push(notification));

    await chat.connect();
    const ws = FakeWebSocket.instances[0]!;

    ws.onmessage?.({
      data: JSON.stringify({
        code: 0,
        msg: "成功",
        data: { session_id: "chat:1:2", message_id: "m1", sender_id: "2", msg_type: 1 },
      }),
    });

    expect(received).toEqual([{ session_id: "chat:1:2", message_id: "m1", sender_id: "2", msg_type: 1 }]);

    // 没有 session_id 的帧（心跳/错误）不算通知，不该触发刷新。
    ws.onmessage?.({ data: JSON.stringify({ code: 0, msg: "成功", data: { kind: "ping" } }) });
    expect(received).toHaveLength(1);

    stop();
    ws.onmessage?.({
      data: JSON.stringify({ code: 0, msg: "成功", data: { session_id: "chat:1:3", message_id: "m2" } }),
    });
    expect(received).toHaveLength(1);
  });

  it("两个订阅者都收到同一条通知（没有被谁取走）", async () => {
    const chat = await createChatSocket();
    const a: unknown[] = [];
    const b: unknown[] = [];
    chat.onNotification((notification) => a.push(notification));
    chat.onNotification((notification) => b.push(notification));

    await chat.connect();
    FakeWebSocket.instances[0]!.onmessage?.({
      data: JSON.stringify({ code: 0, msg: "成功", data: { session_id: "chat:1:2", message_id: "m1" } }),
    });

    expect(a).toHaveLength(1);
    expect(b).toHaveLength(1);
  });

  // 取票是异步的：如果这期间用户登出（useMessageRealtime 已 close），
  // 票据回来后再建连就会留下一条"没人认领"的通道（且下一个登录用户共用它）。
  it("取票期间登出：不再建连", async () => {
    const loggedIn = ref(true);
    vi.stubGlobal("useAuthStore", () => ({
      get isLoggedIn() {
        return loggedIn.value;
      },
    }));

    let resolveTicket!: (value: { ticket: string }) => void;
    getChatWsTicketMock.mockReturnValue(
      new Promise<{ ticket: string }>((resolve) => {
        resolveTicket = resolve;
      }),
    );

    const chat = await createChatSocket();
    const pending = chat.connect();

    loggedIn.value = false;
    resolveTicket({ ticket: "tk-1" });

    expect(await pending).toBeNull();
    expect(FakeWebSocket.instances).toHaveLength(0);
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

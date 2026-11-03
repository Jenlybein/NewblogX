import { beforeEach, describe, expect, it, vi } from "vitest";
import { useChatSocket } from "~/composables/useChatSocket";

// useChatSocket 在构造时注册 onBeforeUnmount。这里没有真实组件实例，
// 直接把它换成空实现，避免每条用例都刷一遍 Vue 警告（警告本身没有信息量）。
vi.mock("vue", async (importOriginal) => {
  const actual = await importOriginal<typeof import("vue")>();
  return { ...actual, onBeforeUnmount: vi.fn() };
});

/**
 * WS 发送回执的读者侧接线（#202）。
 *
 * 服务端有两种出站帧：
 *   1. 正常推送 —— 消息实体（id / sender_id / content…），**没有 code**；
 *   2. 发送结局 —— 响应壳（code / msg / data）。
 * 只有第二种、且 code 非 0 才代表"这次发送被拒绝"。区分这两种帧是本文件的
 * 核心：把推送帧当成失败的话，每收到一条正常消息都会弹一次错误。
 */
class FakeWebSocket {
  static readonly OPEN = 1;
  static readonly CONNECTING = 0;

  readyState = FakeWebSocket.OPEN;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;
  sent: string[] = [];

  constructor(public readonly url: string) {}

  send(data: string) {
    this.sent.push(data);
  }

  close() {
    this.readyState = 3;
  }
}

const getChatWsTicket = vi.fn();
let socket: FakeWebSocket | null = null;
const chatStore = {
  socketStatus: "idle",
  socketError: "",
  setSocketStatus: vi.fn(),
  markSocketMessage: vi.fn(),
  resetSocketState: vi.fn(),
};

vi.mock("~/services/inbox", () => ({
  getChatWsTicket: () => getChatWsTicket(),
}));

function createSocket() {
  socket = null;
  vi.stubGlobal(
    "WebSocket",
    class extends FakeWebSocket {
      constructor(url: string) {
        super(url);
        // 测试要在"用例已创建 socket"之后拿到实例；这是测试脚手架，
        // 不是生产代码里的 this 别名（规则的本意是禁止业务代码这么写）。
        // eslint-disable-next-line @typescript-eslint/no-this-alias
        socket = this;
      }
    },
  );
  vi.stubGlobal("useRuntimeConfig", () => ({
    public: { siteUrl: "http://localhost:3000", wsPath: "/api/chat/ws" },
  }));
  vi.stubGlobal("useAuthStore", () => ({ isLoggedIn: true }));
  vi.stubGlobal("useChatStore", () => chatStore);
  return useChatSocket();
}

function pushFrame(payload: unknown) {
  if (!socket?.onmessage) throw new Error("连接尚未建立");
  socket.onmessage({ data: JSON.stringify(payload) });
}

describe("useChatSocket：发送被拒原因", () => {
  beforeEach(() => {
    getChatWsTicket.mockReset();
    getChatWsTicket.mockResolvedValue({ ticket: "ticket-1" });
    chatStore.setSocketStatus.mockReset();
    chatStore.markSocketMessage.mockReset();
  });

  it("拒绝回执的文案原样暴露给界面（不另造措辞）", async () => {
    const chat = createSocket();
    await chat.connect();

    pushFrame({ code: 1001, msg: "站点当前不允许陌生人发私信（站点策略，不是网络问题）", data: null });

    expect(chat.sendError.value).toBe("站点当前不允许陌生人发私信（站点策略，不是网络问题）");
  });

  it("正常消息推送没有 code，不会被误判成发送失败", async () => {
    const chat = createSocket();
    await chat.connect();

    pushFrame({
      id: "1",
      sender_id: "2",
      receiver_id: "1",
      session_id: "chat:1:2",
      content: "在吗",
      msg_type: 1,
      is_self: false,
      is_read: false,
      send_time: "2026-11-03T09:00:00+08:00",
    });

    expect(chat.sendError.value).toBe("");
  });

  it("重新发送时清掉上一次的拒绝原因", async () => {
    const chat = createSocket();
    await chat.connect();

    pushFrame({ code: 1001, msg: "站点当前不允许陌生人发私信（站点策略，不是网络问题）", data: null });
    expect(chat.sendError.value).not.toBe("");

    await chat.sendMessage({ receiver_id: "2", msg_type: 1, content: "再试一次" });

    expect(chat.sendError.value).toBe("");
    expect(socket?.sent.at(-1)).toContain("再试一次");
  });

  it("回执缺 msg 时给一句兜底文案，不显示空白错误", async () => {
    const chat = createSocket();
    await chat.connect();

    pushFrame({ code: 1002, msg: "", data: null });

    expect(chat.sendError.value).toBe("消息发送失败。");
  });
});

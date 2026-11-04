import { nextTick, ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatNotificationPayload } from "~/types/api";

/**
 * 全局未读实时订阅（票 #240）。
 *
 * 验收点：私信到达时 `useMessageStore` 的未读**实时 +1**（不刷新页面），
 * 且这件事发生在**登录后 / 非私信页**也成立 —— 旧实现把 WS 连接绑在私信页的
 * composable 生命周期上，离开页面就断开，顶栏数字只能等下次进页面才更新。
 *
 * 订阅与连接分开测：连接归 useChatSocket（单例），这里只验证"谁在登录后建连、
 * 登出后断开"以及"通知到达时的未读累积"。
 */

const connectMock = vi.hoisted(() => vi.fn());
const closeMock = vi.hoisted(() => vi.fn());
const bumpPrivateUnreadMock = vi.hoisted(() => vi.fn());
const refreshSummaryMock = vi.hoisted(() => vi.fn());

// 与真实 useChatSocket 一致：onNotification / onReconnected 注册回调，返回退订函数。
const notificationHandlers = new Set<(notification: ChatNotificationPayload) => void>();
const reconnectedHandlers = new Set<() => void>();

function pushNotification(payload: ChatNotificationPayload) {
  for (const handler of [...notificationHandlers]) {
    handler(payload);
  }
}

function pushReconnected() {
  for (const handler of [...reconnectedHandlers]) {
    handler();
  }
}

let loggedIn = ref(true);

async function installRealtime() {
  vi.resetModules();
  const module = await import("~/composables/useMessageRealtime");
  module.useMessageRealtime();
}

describe("useMessageRealtime：连接生命周期回到全局", () => {
  beforeEach(() => {
    loggedIn = ref(true);
    notificationHandlers.clear();
    reconnectedHandlers.clear();
    connectMock.mockReset();
    closeMock.mockReset();
    bumpPrivateUnreadMock.mockReset();
    refreshSummaryMock.mockReset();
    refreshSummaryMock.mockResolvedValue(undefined);

    vi.stubGlobal("useAuthStore", () => ({
      get isLoggedIn() {
        return loggedIn.value;
      },
    }));
    vi.stubGlobal("useMessageStore", () => ({
      bumpPrivateUnread: bumpPrivateUnreadMock,
      refreshSummary: refreshSummaryMock,
    }));
    vi.stubGlobal("useChatSocket", () => ({
      connect: connectMock,
      close: closeMock,
      onNotification: (handler: (notification: ChatNotificationPayload) => void) => {
        notificationHandlers.add(handler);
        return () => {
          notificationHandlers.delete(handler);
        };
      },
      onReconnected: (handler: () => void) => {
        reconnectedHandlers.add(handler);
        return () => {
          reconnectedHandlers.delete(handler);
        };
      },
    }));
  });

  it("已登录时一装上就建连（不再等用户进入私信页）", async () => {
    await installRealtime();

    expect(connectMock).toHaveBeenCalledTimes(1);
  });

  it("未登录不建连；登录后才建连", async () => {
    loggedIn.value = false;
    await installRealtime();

    expect(connectMock).not.toHaveBeenCalled();

    loggedIn.value = true;
    await nextTick();

    expect(connectMock).toHaveBeenCalledTimes(1);
  });

  it("登出时关闭连接（连接不属于某个页面，只属于登录会话）", async () => {
    await installRealtime();
    expect(closeMock).not.toHaveBeenCalled();

    loggedIn.value = false;
    await nextTick();

    expect(closeMock).toHaveBeenCalledTimes(1);
  });

  it("私信通知到达：未读立刻 +1，并触发一次汇总刷新校准", async () => {
    await installRealtime();

    pushNotification({ session_id: "chat:1:2", message_id: "m1", sender_id: "2", msg_type: 1 });
    await nextTick();

    expect(bumpPrivateUnreadMock).toHaveBeenCalledTimes(1);
    expect(refreshSummaryMock).toHaveBeenCalledTimes(1);
  });

  // #247：断线期间漏掉的未读只能靠重连后的一次汇总刷新补上。
  // 通知订阅负责"到达即 +1"，重连事件负责"把断线期间的缺口校准"。
  it("重连成功：立刻刷新一次未读汇总（补上断线期间漏掉的）", async () => {
    await installRealtime();

    pushReconnected();
    await nextTick();

    expect(refreshSummaryMock).toHaveBeenCalledTimes(1);
  });

  // 已读回执也带 session_id，但它不是"又收到一条消息"：
  // 拿它 +1 的话，顶栏未读会随着对方阅读越聊越大。
  it("已读回执（msg_type=8）不增加未读，也不触发校准", async () => {
    await installRealtime();

    pushNotification({ session_id: "chat:1:2", message_id: "m1", msg_type: 8, reader_id: "2" });
    await nextTick();

    expect(bumpPrivateUnreadMock).not.toHaveBeenCalled();
    expect(refreshSummaryMock).not.toHaveBeenCalled();
  });

  // 多处都会调用 useMessageRealtime（插件 + 测试/页面）；重复安装不能把未读 +2。
  it("重复调用只安装一份订阅（未读不会 +2）", async () => {
    vi.resetModules();
    const module = await import("~/composables/useMessageRealtime");
    module.useMessageRealtime();
    module.useMessageRealtime();

    expect(connectMock).toHaveBeenCalledTimes(1);
    expect(notificationHandlers.size).toBe(1);

    pushNotification({ session_id: "chat:1:2", message_id: "m1", msg_type: 1 });
    expect(bumpPrivateUnreadMock).toHaveBeenCalledTimes(1);
  });
});

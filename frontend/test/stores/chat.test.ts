import { createPinia, setActivePinia } from "pinia";
import { ref } from "vue";
import { beforeEach, describe, expect, it, vi } from "vitest";

// 和 message 一样：登录态用真 ref，才能测"登出时自动重置"。
// 每个用例换一份新 ref —— 共用会让前面用例留下的 watch 被同一赋值唤醒（见 message.test.ts）。
let loggedIn = ref(true);

async function createChatStore() {
  vi.resetModules();
  setActivePinia(createPinia());
  const module = await import("~/stores/chat");
  return module.useChatStore();
}

describe("chat store", () => {
  beforeEach(() => {
    loggedIn = ref(true);
    vi.stubGlobal("useAuthStore", () => ({
      get isLoggedIn() {
        return loggedIn.value;
      },
    }));
  });

  it("初始是未连接、无错误", async () => {
    const store = await createChatStore();

    expect(store.socketStatus).toBe("idle");
    expect(store.socketError).toBe("");
    expect(store.lastSocketMessageAt).toBeNull();
    expect(store.activeSessionId).toBeNull();
  });

  it("记录连接状态与错误信息", async () => {
    const store = await createChatStore();

    store.setSocketStatus("error", "连接断开");
    expect(store.socketStatus).toBe("error");
    expect(store.socketError).toBe("连接断开");

    store.setSocketStatus("connected");
    // 成功后错误信息必须被清掉，否则界面上会一直挂着上一次的报错
    expect(store.socketError).toBe("");
  });

  it("收到消息时记下时间戳", async () => {
    const store = await createChatStore();

    store.markSocketMessage();

    expect(store.lastSocketMessageAt).not.toBeNull();
  });

  it("resetSocketState 只清连接状态，不动当前打开的会话", async () => {
    const store = await createChatStore();
    store.setActiveSession("session-1");
    store.setSocketStatus("error", "断线");

    store.resetSocketState();

    expect(store.socketStatus).toBe("idle");
    expect(store.socketError).toBe("");
    expect(store.lastSocketMessageAt).toBeNull();
    // 会话选择是用户的操作结果，不该被连接层的重置牵连
    expect(store.activeSessionId).toBe("session-1");
  });

  // 这条测的是新增的 watch（baseline §42）：以前是 auth store 登出时来调 resetSocketState，
  // 现在本 store 自己盯 isLoggedIn。
  it("登出时自动重置连接状态", async () => {
    const store = await createChatStore();
    store.setSocketStatus("connected");
    store.markSocketMessage();

    loggedIn.value = false;

    // flush:"sync"，所以不需要 await
    expect(store.socketStatus).toBe("idle");
    expect(store.lastSocketMessageAt).toBeNull();
  });
});

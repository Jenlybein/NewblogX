// 显式 import 而不是靠 Nuxt 自动导入：这个 store 原来两个都没写，
// 结果在 vitest 里 import 它直接 `defineStore is not defined` —— 也就一直没测试。
import { shallowRef, watch } from "vue";
import { defineStore } from "pinia";

export type ChatSocketStatus = "idle" | "connecting" | "connected" | "error";

export const useChatStore = defineStore("chat", () => {
  const activeSessionId = shallowRef<string | null>(null);
  const socketStatus = shallowRef<ChatSocketStatus>("idle");
  const lastSocketMessageAt = shallowRef<string | null>(null);
  const socketError = shallowRef("");

  function setActiveSession(id: string | null) {
    activeSessionId.value = id;
  }

  function setSocketStatus(status: ChatSocketStatus, error = "") {
    socketStatus.value = status;
    socketError.value = error;
  }

  function markSocketMessage() {
    lastSocketMessageAt.value = new Date().toISOString();
  }

  function resetSocketState() {
    socketStatus.value = "idle";
    socketError.value = "";
    lastSocketMessageAt.value = null;
  }

  // 登出时把连接状态归零，理由同 message store（见 baseline §42）：
  // 会话没了，"已连接 / 连接出错"这些状态就不再成立，必须清掉，
  // 否则下次登录前界面上会挂着上一个账号的连接状态。
  watch(
    () => useAuthStore().isLoggedIn,
    (loggedIn) => {
      if (!loggedIn) {
        resetSocketState();
      }
    },
    { flush: "sync" },
  );

  return {
    activeSessionId,
    socketStatus,
    lastSocketMessageAt,
    socketError,
    setActiveSession,
    setSocketStatus,
    markSocketMessage,
    resetSocketState,
  };
});

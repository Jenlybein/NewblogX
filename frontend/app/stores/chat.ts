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

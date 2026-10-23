import { computed, onBeforeUnmount, shallowRef } from "vue";
import { getChatWsTicket } from "~/services/inbox";
import type { ChatMessageItem, ChatSocketEnvelope, ChatSocketOutgoingMessage } from "~/types/api";

function resolveWebSocketUrl(siteUrl: string, wsPath: string) {
  const origin = import.meta.client ? window.location.origin : siteUrl;
  const endpoint = new URL(wsPath, origin);
  if (endpoint.protocol === "https:") {
    endpoint.protocol = "wss:";
  } else if (endpoint.protocol === "http:") {
    endpoint.protocol = "ws:";
  }
  return endpoint.toString();
}

function normalizeSocketData(payload: unknown) {
  if (!payload || typeof payload !== "object") {
    return null;
  }

  return payload as Partial<ChatMessageItem> & Record<string, unknown>;
}

export function useChatSocket() {
  const config = useRuntimeConfig();
  const authStore = useAuthStore();
  const chatStore = useChatStore();
  const socket = shallowRef<WebSocket | null>(null);
  const inbox = shallowRef<Array<ChatSocketEnvelope>>([]);

  const wsUrl = computed(() => resolveWebSocketUrl(config.public.siteUrl, config.public.wsPath));
  const canConnect = computed(() => import.meta.client && authStore.isLoggedIn);

  function pushEnvelope(envelope: ChatSocketEnvelope) {
    inbox.value = [...inbox.value.slice(-19), envelope];
    chatStore.markSocketMessage();
  }

  async function connect() {
    if (!canConnect.value) {
      return null;
    }

    if (socket.value && socket.value.readyState === WebSocket.OPEN) {
      return socket.value;
    }

    chatStore.setSocketStatus("connecting");

    try {
      const ticketPayload = await getChatWsTicket();
      const ticket = typeof ticketPayload.ticket === "string" ? ticketPayload.ticket : "";
      if (!ticket) {
        throw new Error("后端未返回可用的 WebSocket ticket。");
      }

      const instance = new WebSocket(`${wsUrl.value}?ticket=${encodeURIComponent(ticket)}`);

      instance.onopen = () => {
        chatStore.setSocketStatus("connected");
      };

      instance.onmessage = (event) => {
        try {
          const envelope = JSON.parse(String(event.data)) as ChatSocketEnvelope;
          pushEnvelope(envelope);
        } catch {
          pushEnvelope({
            code: -1,
            msg: "收到无法解析的实时消息。",
            data: event.data,
          });
        }
      };

      instance.onerror = () => {
        chatStore.setSocketStatus("error", "实时连接发生异常。");
      };

      instance.onclose = () => {
        socket.value = null;
        if (chatStore.socketStatus !== "error") {
          chatStore.setSocketStatus("idle");
        }
      };

      socket.value = instance;
      return instance;
    } catch (error) {
      chatStore.setSocketStatus("error", error instanceof Error ? error.message : "实时连接初始化失败。");
      return null;
    }
  }

  async function sendMessage(payload: ChatSocketOutgoingMessage) {
    const instance = socket.value?.readyState === WebSocket.OPEN ? socket.value : await connect();
    if (!instance || instance.readyState !== WebSocket.OPEN) {
      throw new Error(chatStore.socketError || "实时连接尚未建立。");
    }

    instance.send(JSON.stringify(payload));
  }

  function consumeIncomingMessages(sessionId: string) {
    const matched = inbox.value.filter((item) => {
      const data = normalizeSocketData(item.data);
      return data?.session_id === sessionId;
    });

    if (!matched.length) {
      return [];
    }

    inbox.value = inbox.value.filter((item) => {
      const data = normalizeSocketData(item.data);
      return data?.session_id !== sessionId;
    });

    return matched;
  }

  function close() {
    socket.value?.close();
    socket.value = null;
    chatStore.resetSocketState();
  }

  onBeforeUnmount(close);

  return {
    socketStatus: computed(() => chatStore.socketStatus),
    socketError: computed(() => chatStore.socketError),
    inbox,
    connect,
    close,
    sendMessage,
    consumeIncomingMessages,
  };
}

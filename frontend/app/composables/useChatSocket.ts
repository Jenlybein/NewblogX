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
  // 最近一次发送被服务端拒绝的原因（票 202）。
  //
  // 服务端在拒绝时回的是 `{code:非0, msg:"..."}` 响应壳；而正常推送的是消息
  // 实体，没有 code 字段。两种帧都进 inbox，所以必须先按 code 是否真是数字
  // 区分响应壳，否则每条正常消息都会被当成失败。
  const sendError = shallowRef("");

  const wsUrl = computed(() => resolveWebSocketUrl(config.public.siteUrl, config.public.wsPath));
  // 服务端不连：`import.meta.client` 在真实客户端构建里是 true，在 SSR 里是 false。
  // 测试环境没有注入这个宏（undefined），但 happy-dom 提供了 window，
  // 所以用 window 作为兜底：SSR 仍然不连，而 WS 行为能在单测里被验证。
  const isClientRuntime = Boolean(import.meta.client) || typeof window !== "undefined";
  const canConnect = computed(() => isClientRuntime && authStore.isLoggedIn);

  function pushEnvelope(envelope: ChatSocketEnvelope) {
    const code = (envelope as { code?: unknown }).code;
    if (typeof code === "number" && code !== 0) {
      const reason = typeof envelope.msg === "string" ? envelope.msg.trim() : "";
      // 文案由后端错误码体系给出（如「站点当前不允许陌生人发私信（站点策略…）」），
      // 前端不另造措辞，缺失时才退回一句兜底。
      sendError.value = reason || "消息发送失败。";
    }
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

    // 每次发送先清掉上一次的拒绝原因：否则第二次发送成功时，界面上还挂着
    // 上一次「陌生人私信被站点关闭」的提示。
    sendError.value = "";
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
    sendError,
    inbox,
    connect,
    close,
    sendMessage,
    consumeIncomingMessages,
  };
}

import { computed, onBeforeUnmount, shallowRef } from "vue";
import { useRuntimeConfig } from "#imports";
import { getChatWsTicket } from "~/services/inbox";
import type { ChatNotificationPayload, ChatSocketEnvelope } from "~/types/api";

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

/**
 * isClientRuntime 判断是否在浏览器环境。
 *
 * 优先用 Nuxt 的 `import.meta.client`；再兜一层 `window.WebSocket`：
 * vitest 环境下 `import.meta.client` 不保证被注入，而"有没有 WebSocket 实现"
 * 才是建连这件事真正依赖的前提（SSR 的 Node 进程里没有 window）。
 */
function isClientRuntime() {
  if (import.meta.client) {
    return true;
  }
  return typeof window !== "undefined" && typeof window.WebSocket !== "undefined";
}

/**
 * 把下行帧的 data 收窄成通知载荷。
 *
 * WS 已降级为通知通道（#201）：帧里只有"有新消息/对方已读"的标识，
 * 没有正文。缺少 session_id 的帧（比如心跳/错误帧）不算通知，直接丢弃。
 */
function normalizeNotification(payload: unknown): ChatNotificationPayload | null {
  if (!payload || typeof payload !== "object") {
    return null;
  }
  const record = payload as Record<string, unknown>;
  const sessionId = record.session_id;
  if (typeof sessionId !== "string" || !sessionId) {
    return null;
  }
  return record as unknown as ChatNotificationPayload;
}

export function useChatSocket() {
  const config = useRuntimeConfig();
  const authStore = useAuthStore();
  const chatStore = useChatStore();
  const socket = shallowRef<WebSocket | null>(null);
  const inbox = shallowRef<Array<ChatSocketEnvelope>>([]);
  // 最近一次发送被服务端拒绝的原因（票 202 的界面出口）。
  //
  // #201 之后发送走 HTTP，拒绝原因是 HTTP 抛出的后端错误码文案（由
  // useInboxCenter 写入）。WS 这一侧也会收到异常响应壳（例如"请改用 HTTP 发送"），
  // 所以 pushEnvelope 里同样按 code 是否为非 0 数字区分"拒绝"与"通知"：
  // 通知帧的 code 是 0，当成失败的话每收一条消息都会弹错误。
  const sendError = shallowRef("");

  const wsUrl = computed(() => resolveWebSocketUrl(config.public.siteUrl, config.public.wsPath));
  const canConnect = computed(() => isClientRuntime() && authStore.isLoggedIn);

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

  /**
   * 取走本轮收到的通知（取走即清空）。
   *
   * 调用方（useInboxCenter）据此刷新会话列表、未读数，命中当前会话时再刷新消息。
   * 不再按会话过滤：对方发来的新消息可能属于一个**当前没打开**的会话，
   * 那种情况也要刷新会话列表，否则左侧列表的红点不出现。
   */
  function takeNotifications(): ChatNotificationPayload[] {
    if (!inbox.value.length) {
      return [];
    }

    const notifications = inbox.value
      .map((envelope) => normalizeNotification(envelope.data))
      .filter((item): item is ChatNotificationPayload => Boolean(item));
    inbox.value = [];
    return notifications;
  }

  /**
   * 记录/清空最近一次发送的拒绝原因（票 202 的界面出口）。
   *
   * #201 之后发送走 HTTP：拒绝原因是 HTTP 请求抛出的后端错误码文案，
   * 由 useInboxCenter 在发送失败时写进来；成功时清空。
   * WS 上的非 0 响应壳（例如"请改用 HTTP 发送"）也会走这里。
   */
  function setSendError(reason: string) {
    sendError.value = reason;
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
    setSendError,
    inbox,
    connect,
    close,
    takeNotifications,
  };
}

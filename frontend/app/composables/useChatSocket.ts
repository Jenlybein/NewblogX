import { computed, shallowRef } from "vue";
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

// ── 模块级单例（票 #240）────────────────────────────────────────────────────
//
// 为什么不再是"每个调用方一份"：私信连接曾经绑在**私信页面**的生命周期上
// （composable 里 onBeforeUnmount 关连接），于是"离开消息中心"= 断开实时通道，
// 顶栏未读就得等下一次进页面才更新。现在连接属于整个登录会话：
//   - 页头（全局未读订阅）与私信页（会话/消息刷新）共享同一条连接；
//   - 离开任何页面都不关；只有登出才关（由 useMessageRealtime 负责）。
//
// 通知改成**订阅制**而不是"取走队列"：消费方数量不固定（全局未读 + 当前私信页），
// 旧 takeNotifications 谁先取走谁就把别人的通知吃掉。
const socket = shallowRef<WebSocket | null>(null);
// 最近一次发送被服务端拒绝的原因（票 202 的界面出口）。
//
// #201 之后发送走 HTTP，拒绝原因是 HTTP 抛出的后端错误码文案（由
// useInboxCenter 写入）。WS 这一侧也会收到异常响应壳（例如"请改用 HTTP 发送"），
// 所以 pushEnvelope 里同样按 code 是否为非 0 数字区分"拒绝"与"通知"：
// 通知帧的 code 是 0，当成失败的话每收一条消息都会弹错误。
const sendError = shallowRef("");
const notificationHandlers = new Set<(notification: ChatNotificationPayload) => void>();
// 建连是异步的（先取票据）：并发调用必须复用同一个 Promise，否则会开两条连接。
let connectPromise: Promise<WebSocket | null> | null = null;

function dispatchEnvelope(envelope: ChatSocketEnvelope) {
  useChatStore().markSocketMessage();

  const notification = normalizeNotification(envelope.data);
  if (!notification) {
    return;
  }

  for (const handler of [...notificationHandlers]) {
    try {
      handler(notification);
    } catch {
      // 单个订阅者的异常不该打断其它订阅者，也不该让 WS 的 onmessage 冒泡。
    }
  }
}

function pushEnvelope(envelope: ChatSocketEnvelope) {
  const code = (envelope as { code?: unknown }).code;
  if (typeof code === "number" && code !== 0) {
    const reason = typeof envelope.msg === "string" ? envelope.msg.trim() : "";
    // 文案由后端错误码体系给出（如「站点当前不允许陌生人发私信（站点策略…）」），
    // 前端不另造措辞，缺失时才退回一句兜底。
    sendError.value = reason || "消息发送失败。";
    return;
  }
  dispatchEnvelope(envelope);
}

function closeSocket() {
  socket.value?.close();
  socket.value = null;
  // 关连接时可能还有 in-flight 的取票 Promise，它回来后会再开一条连接；
  // 清掉登记即可让下一次 connect 重新走完整流程。
  connectPromise = null;
  useChatStore().resetSocketState();
}

export function useChatSocket() {
  const config = useRuntimeConfig();
  const authStore = useAuthStore();
  const chatStore = useChatStore();

  const wsUrl = computed(() => resolveWebSocketUrl(config.public.siteUrl, config.public.wsPath));
  const canConnect = computed(() => isClientRuntime() && authStore.isLoggedIn);

  async function connect(): Promise<WebSocket | null> {
    if (!canConnect.value) {
      return null;
    }

    // 连接已存在（含"正在握手"）就复用：第二个消费方不该再开一条。
    if (
      socket.value &&
      (socket.value.readyState === WebSocket.OPEN || socket.value.readyState === WebSocket.CONNECTING)
    ) {
      return socket.value;
    }

    if (connectPromise) {
      return connectPromise;
    }

    chatStore.setSocketStatus("connecting");

    connectPromise = (async () => {
      try {
        const ticketPayload = await getChatWsTicket();
        // 取票是异步的：期间可能刚好登出（useMessageRealtime 会 close）。
        // 这里再确认一次登录态，避免登出后又悄悄建起一条连接。
        if (!canConnect.value) {
          return null;
        }
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
          // 只清理自己：重连期间旧连接的 onclose 不能把新连接抹掉。
          if (socket.value === instance) {
            socket.value = null;
          }
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
    })().finally(() => {
      connectPromise = null;
    });

    return connectPromise;
  }

  /**
   * 订阅下行通知（返回取消订阅函数）。
   *
   * 订阅与连接的生命周期解耦：连接是全局的，订阅属于消费方（全局未读 / 私信页）。
   */
  function onNotification(handler: (notification: ChatNotificationPayload) => void) {
    notificationHandlers.add(handler);
    return () => {
      notificationHandlers.delete(handler);
    };
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

  return {
    socketStatus: computed(() => chatStore.socketStatus),
    socketError: computed(() => chatStore.socketError),
    sendError,
    setSendError,
    connect,
    close: closeSocket,
    onNotification,
  };
}

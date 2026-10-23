import WebSocket from "crossws/websocket";

type SocketData = string | ArrayBuffer | Blob | Uint8Array;
type UpstreamSocket = {
  readyState: number;
  on(event: string, callback: (...args: unknown[]) => void): void;
  send(data: SocketData): void;
  close(code?: number, reason?: string): void;
};
type RelayPeer = {
  request: { url: string };
  send(data: SocketData): void;
  close(code?: number, reason?: string): void;
  terminate(): void;
};

const UpstreamWebSocket = WebSocket as unknown as {
  new (url: string): UpstreamSocket;
  OPEN: number;
  CONNECTING: number;
};

interface RelayState {
  upstream: UpstreamSocket;
  queue: SocketData[];
  closed: boolean;
}

const relayStates = new WeakMap<object, RelayState>();

function trimTrailingSlash(value: string) {
  return value.endsWith("/") ? value.slice(0, -1) : value;
}

function normalizeBasePath(value: unknown) {
  const path = String(value || "/api").trim() || "/api";
  return path.startsWith("/") ? path.replace(/\/+$/u, "") || "/" : `/${path.replace(/\/+$/u, "")}`;
}

function resolveUpstreamWsUrl(request: { url: string }) {
  const config = useRuntimeConfig();
  const requestUrl = new URL(request.url);
  const apiBase = normalizeBasePath(config.public.apiBase);
  const upstreamUrl = new URL(trimTrailingSlash(String(config.apiUpstream || "http://127.0.0.1:8080")));
  const suffix = requestUrl.pathname.startsWith(`${apiBase}/`)
    ? requestUrl.pathname.slice(apiBase.length)
    : requestUrl.pathname;

  upstreamUrl.protocol = upstreamUrl.protocol === "https:" ? "wss:" : "ws:";
  upstreamUrl.pathname = `${apiBase}${suffix}`.replace(/\/{2,}/gu, "/");
  upstreamUrl.search = requestUrl.search;
  return upstreamUrl.toString();
}

function toCloseReason(reason: unknown) {
  if (typeof reason === "string") return reason;
  if (reason instanceof Uint8Array) return new TextDecoder().decode(reason);
  return "";
}

function toCloseCode(code: unknown) {
  return typeof code === "number" && code >= 1000 && code <= 4999 ? code : 1011;
}

function toSocketData(data: unknown): SocketData {
  if (typeof data === "string" || data instanceof ArrayBuffer || data instanceof Blob || data instanceof Uint8Array) {
    return data;
  }
  return String(data ?? "");
}

function toRelayPeer(peer: unknown) {
  return peer as RelayPeer;
}

function closePeer(peer: RelayPeer, code = 1011, reason = "WebSocket relay closed") {
  try {
    peer.close(toCloseCode(code), reason.slice(0, 120));
  } catch {
    peer.terminate();
  }
}

export default defineWebSocketHandler({
  open(peer) {
    const relayPeer = toRelayPeer(peer);
    const upstream = new UpstreamWebSocket(resolveUpstreamWsUrl(relayPeer.request));
    const state: RelayState = {
      upstream,
      queue: [],
      closed: false,
    };
    relayStates.set(relayPeer, state);

    upstream.on("open", () => {
      for (const data of state.queue.splice(0)) {
        upstream.send(data);
      }
    });

    upstream.on("message", (data: unknown) => {
      relayPeer.send(toSocketData(data));
    });

    upstream.on("close", (code: unknown, reason: unknown) => {
      state.closed = true;
      closePeer(relayPeer, toCloseCode(code), toCloseReason(reason));
    });

    upstream.on("error", () => {
      state.closed = true;
      closePeer(relayPeer, 1011, "Upstream WebSocket error");
    });

    upstream.on("unexpected-response", (_request: unknown, rawResponse: unknown) => {
      const response = rawResponse as { statusCode?: number; resume?: () => void };
      state.closed = true;
      closePeer(relayPeer, 1008, `Upstream rejected WebSocket: ${response.statusCode ?? "unknown"}`);
      response.resume?.();
    });
  },

  message(peer, message) {
    const relayPeer = toRelayPeer(peer);
    const rawData = toSocketData(message.rawData);
    const state = relayStates.get(relayPeer);
    if (!state || state.closed) return;

    if (state.upstream.readyState === UpstreamWebSocket.OPEN) {
      state.upstream.send(rawData);
      return;
    }

    state.queue.push(rawData);
  },

  close(peer, details) {
    const relayPeer = toRelayPeer(peer);
    const state = relayStates.get(relayPeer);
    relayStates.delete(relayPeer);
    if (!state || state.closed) return;

    state.closed = true;
    if (state.upstream.readyState === UpstreamWebSocket.OPEN || state.upstream.readyState === UpstreamWebSocket.CONNECTING) {
      state.upstream.close(toCloseCode(details.code), details.reason);
    }
  },

  error(peer) {
    const relayPeer = toRelayPeer(peer);
    const state = relayStates.get(relayPeer);
    relayStates.delete(relayPeer);
    if (!state || state.closed) return;

    state.closed = true;
    state.upstream.close(1011, "Client WebSocket error");
  },
});

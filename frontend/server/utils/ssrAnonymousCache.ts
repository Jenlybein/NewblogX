/**
 * 匿名 SSR 的 30s 内存缓存策略与实现（ADR-0019 决策 5 / 约束 2、3）。
 *
 * 为什么不用 routeRules 的 swr：Nitro 只按**注册 handler 的 route** 给 handler 套
 * cachedEventHandler（匹配时把动态段与通配段换成下划线），而 Nuxt 渲染器注册在
 * 通配路由上（构建产物里是 `{ route: '/**', handler: renderer }`），永远匹配不上
 * `/` 与 `/article/**` 这两条规则 —— 在 node-server 预设下写 swr 不生效，还会给出
 * "已经开了缓存"的假信号。所以这里自己实现最小的一层：Nitro 的 render:before /
 * render:response 钩子 + 按 URL 键的内存表。
 *
 * 三条硬约束（判据见 test/gates/ssr-anonymous-cache.test.ts）：
 *   1. 只缓存 SSR 白名单 `/` 与 `/article/**`（白名单的唯一来源是
 *      `shared/readerRenderingBoundary.ts`，这里只派生、不重复写表）；
 *   2. 缓存键只由 URL 决定，不含任何请求身份（cookie / Authorization / UA）；
 *   3. TTL 固定 30 秒，只存 200 + HTML + 不带 Set-Cookie 的匿名整页响应。
 *
 * 这个文件不依赖 Nuxt/Nitro 的任何自动导入，是纯逻辑 + 一个内存表，方便门禁直接单测。
 * 门禁夹具（样例路由与策略判据）刻意放在测试里：生产模块只留运行时纯函数
 * （键 / 可存性 / TTL / 上限），不背测试常量。
 */

import { isSsrPagePath } from "../../shared/readerRenderingBoundary";

/** 缓存 TTL：ADR-0019 决策 5 写死的 30 秒（内容最多滞后 30 秒，这是已知代价）。 */
export const SSR_ANONYMOUS_CACHE_TTL_MS = 30_000;

/** 内存表条数上限：爬虫可能带来无数个文章 id，过期条目又只在读时清理，必须封顶。 */
export const SSR_ANONYMOUS_CACHE_MAX_ENTRIES = 500;

/** 缓存键只接受 URL 成分 —— 请求身份在类型上就传不进来（约束 3 的第一道保险）。 */
export interface AnonymousCacheUrl {
  pathname: string;
  search?: string;
}

/** 一次请求里缓存策略关心的信息（方法只为排除写请求；没有任何身份字段）。 */
export interface AnonymousCacheRequest extends AnonymousCacheUrl {
  method?: string;
}

/** 去掉 baseURL 前缀，得到路由规则匹配用的 pathname（baseURL 通常是 `/`）。 */
export function withoutBasePath(pathname: string, baseURL: string) {
  if (baseURL === "/" || !pathname.startsWith(baseURL)) {
    return pathname;
  }
  const rest = pathname.slice(baseURL.length);
  return rest.startsWith("/") ? rest : `/${rest}`;
}

/** 路由是否属于 SSR 白名单且是整页 HTML 端点（`/` 与 `/article/<id>`）。 */
export function isCacheableSsrPath(pathname: string) {
  return isSsrPagePath(pathname);
}

/** 一次请求是否允许查/写匿名缓存：读请求 + SSR 白名单路由。 */
export function isCacheableSsrRequest(request: AnonymousCacheRequest) {
  const method = (request.method ?? "GET").toUpperCase();
  if (method !== "GET" && method !== "HEAD") {
    return false;
  }
  return isCacheableSsrPath(request.pathname);
}

/**
 * 归一化一次请求后返回缓存用的 URL 形状；不缓存时返回 null。
 *
 * 纯函数：插件只把 URL 与方法喂进来，cookie/Authorization/UA 在调用点就没被读过。
 */
export function cacheRequestShape(
  pathname: string,
  search: string,
  method: string,
  baseURL = "/",
): AnonymousCacheRequest | null {
  const normalizedPathname = withoutBasePath(pathname, baseURL);
  const request: AnonymousCacheRequest = { pathname: normalizedPathname, search, method };
  return isCacheableSsrRequest(request) ? request : null;
}

/**
 * 缓存键：`pathname + search`，别的什么都没有（约束 3）。
 *
 * 签名只收 URL 成分，是"不按用户区分键"的第一道保险；第二道是一个判据
 * （`ssrAnonymousCachePolicyViolations`）会把人造的身份键抓出来。
 */
export function anonymousCacheKey(request: AnonymousCacheUrl) {
  return `${request.pathname}${request.search ?? ""}`;
}

/** 可缓存的匿名整页响应（`render:response` 钩子收到的形状）。 */
export interface AnonymousSsrResponseShape {
  statusCode?: number;
  headers?: Record<string, string>;
  body?: unknown;
}

/** 缓存表里的一条：渲染结果 + 到期时间（`expiresAt` 是绝对时间戳）。 */
export interface CachedAnonymousSsrHtml {
  statusCode: number;
  headers: Record<string, string>;
  body: string;
  expiresAt: number;
}

/** 大小写不敏感地取一个响应头（渲染器返回的是小写键，但这里不依赖这一点）。 */
function readHeader(headers: Record<string, string>, name: string) {
  const match = Object.keys(headers).find((key) => key.toLowerCase() === name);
  return match === undefined ? undefined : headers[match];
}

/**
 * 只缓存"可以共享给所有人"的整页响应：200 + text/html + 没有 Set-Cookie。
 *
 * Set-Cookie 是最后一道闸：带 cookie 的响应一定含请求身份相关的东西（登录态、
 * A/B 分流等），把它缓存下来会在读者之间串号。
 */
export function isStorableAnonymousSsrResponse(
  response: AnonymousSsrResponseShape,
): response is { statusCode: number; headers: Record<string, string>; body: string } {
  if (response.statusCode !== 200) {
    return false;
  }
  if (typeof response.body !== "string" || response.body.length === 0) {
    return false;
  }
  const headers = response.headers ?? {};
  const contentType = (readHeader(headers, "content-type") ?? "").toLowerCase();
  if (!contentType.includes("text/html")) {
    return false;
  }
  if (readHeader(headers, "set-cookie") !== undefined) {
    return false;
  }
  return true;
}

export interface SsrAnonymousCacheStoreOptions {
  ttlMs?: number;
  maxEntries?: number;
  /** 时钟可注入：门禁要能"快进 30 秒"而不真的睡。 */
  now?: () => number;
}

/** 建一个按 URL 键、固定 TTL 的内存缓存表（进程内单例由 Nitro 插件持有）。 */
export function createSsrAnonymousCacheStore(options: SsrAnonymousCacheStoreOptions = {}) {
  const ttlMs = options.ttlMs ?? SSR_ANONYMOUS_CACHE_TTL_MS;
  const maxEntries = options.maxEntries ?? SSR_ANONYMOUS_CACHE_MAX_ENTRIES;
  const now = options.now ?? (() => Date.now());
  const entries = new Map<string, CachedAnonymousSsrHtml>();

  function purgeExpired() {
    const timestamp = now();
    for (const [key, entry] of entries) {
      if (timestamp >= entry.expiresAt) {
        entries.delete(key);
      }
    }
  }

  return {
    read(key: string): CachedAnonymousSsrHtml | null {
      const entry = entries.get(key);
      if (!entry) {
        return null;
      }
      if (now() >= entry.expiresAt) {
        entries.delete(key);
        return null;
      }
      return entry;
    },
    write(key: string, response: Omit<CachedAnonymousSsrHtml, "expiresAt">) {
      purgeExpired();
      if (!entries.has(key) && entries.size >= maxEntries) {
        // Map 按插入顺序迭代：删掉最老的条目，保证内存有上界。
        const oldestKey = entries.keys().next().value;
        if (oldestKey !== undefined) {
          entries.delete(oldestKey);
        }
      }
      entries.set(key, { ...response, expiresAt: now() + ttlMs });
    },
    clear() {
      entries.clear();
    },
    size() {
      purgeExpired();
      return entries.size;
    },
  };
}

export type SsrAnonymousCacheStore = ReturnType<typeof createSsrAnonymousCacheStore>;


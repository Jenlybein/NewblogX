import {
  anonymousCacheKey,
  cacheRequestShape,
  createSsrAnonymousCacheStore,
  isStorableAnonymousSsrResponse,
} from "../utils/ssrAnonymousCache";

/**
 * 匿名 SSR 的 30s 服务端缓存（ADR-0019 决策 5 / 约束 2、3）。
 *
 * 为什么不写 `routeRules` 的 `swr`：Nitro 只按**注册 handler 的 route** 给 handler 套
 * 缓存（匹配时把动态段与通配段换成下划线），而 Nuxt 渲染器注册在通配路由上（构建产物
 * 里是 `{ route: '/**', handler: renderer }`），匹配不上 `/` 与 `/article/**` 这两条规则。
 * 在 node-server 预设下，写 `swr` 不会生效 —— 这比不缓存更糟：它会给出"缓存已经开了"
 * 的假信号。所以这里按票面允许的兜底做法，在 Nitro 层做最小实现：
 *
 *   - `render:before`：命中就**直接给出缓存的匿名 HTML**（跳过整次渲染与服务端取数）；
 *   - `render:response`：未命中时把刚刚渲染出的这一份整页 HTML 按 URL 存 30 秒。
 *
 * 为什么这一层不会串号：
 *   1. SSR 取数已匿名化（services/article 的 `getArticleDetailAnonymous` +
 *      `app/utils/readerPrivateFields` 剔除 is_digg / is_favor / relation），HTML 里没有用户态；
 *   2. 缓存键只有 `pathname + search`（见 `anonymousCacheKey`），cookie/Authorization/UA
 *      在调用点根本没被读到；
 *   3. 只存 200 + text/html + 不带 Set-Cookie 的响应（`isStorableAnonymousSsrResponse`）。
 *
 * 响应上的 `x-ssr-cache: HIT/MISS` 是验收与线上排障用的（读者阅读不受影响）。
 */
const CACHE_HIT_MARKER = "hit";

export default defineNitroPlugin((nitroApp) => {
  // dev 不缓存：本地改一行代码后看到 30 秒前的旧页面会浪费排查时间。
  if (import.meta.dev) {
    return;
  }

  const cache = createSsrAnonymousCacheStore();
  const baseURL = String(useRuntimeConfig().app.baseURL || "/");

  nitroApp.hooks.hook("render:before", (context) => {
    const url = getRequestURL(context.event);
    const request = cacheRequestShape(url.pathname, url.search, context.event.method, baseURL);
    if (!request) {
      return;
    }

    const cached = cache.read(anonymousCacheKey(request));
    if (!cached) {
      setResponseHeader(context.event, "x-ssr-cache", "MISS");
      return;
    }

    // 标记这次是"缓存命中"：否则下面的 render:response 会把同一份 HTML 再存一遍，
    // 命中一次就顺延一次 TTL，30 秒的失效语义会被热门页面悄悄改掉。
    context.event.context.ssrAnonymousCache = CACHE_HIT_MARKER;
    setResponseHeader(context.event, "x-ssr-cache", "HIT");
    context.response = {
      body: cached.body,
      statusCode: cached.statusCode,
      headers: { ...cached.headers },
    };
  });

  nitroApp.hooks.hook("render:response", (response, context) => {
    if (context.event.context.ssrAnonymousCache === CACHE_HIT_MARKER) {
      return;
    }

    const url = getRequestURL(context.event);
    const request = cacheRequestShape(url.pathname, url.search, context.event.method, baseURL);
    if (!request || !isStorableAnonymousSsrResponse(response)) {
      return;
    }

    cache.write(anonymousCacheKey(request), {
      statusCode: response.statusCode,
      headers: response.headers,
      body: response.body,
    });
  });
});

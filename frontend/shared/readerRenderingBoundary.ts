/**
 * 读者站渲染边界（ADR-0019 决策 1 / 约束 1）的**唯一来源**。
 *
 * 三张表：
 *   1. `SSR_WHITELIST_PATTERNS` —— 允许服务端渲染的读者站路由（首页门面 + 文章页分享卡片/收录）；
 *   2. `CLIENT_ONLY_PATTERNS` —— 必须显式 `ssr: false` 的路由（个人主页、搜索页、创作区、合集）；
 *   3. `SERVER_REDIRECT_ROUTES` —— 历史入口必须由服务端 301 承担（`/articles` 已并入 `/search`）。
 *
 * 消费方（任一消费方擅自扩张/收缩白名单都会让门禁变红）：
 *   - `frontend/nuxt.config.ts`：`routeRules` 由 {@link CLIENT_ONLY_PATTERNS} /
 *     {@link SERVER_REDIRECT_ROUTES} 派生；
 *   - `frontend/server/utils/ssrAnonymousCache.ts`：匿名缓存的缓存范围 = 白名单 ∩ 整页 HTML；
 *   - `frontend/test/gates/rendering-boundary.test.ts`：白名单双向门禁；
 *   - `frontend/test/gates/ssr-anonymous-cache.test.ts`：缓存范围门禁。
 *
 * 新增 SSR 页面必须先改 ADR-0019 再改这里，不得在消费方按页面随手加。
 * 本文件不依赖 Nuxt/Nitro 的任何自动导入（`nuxt.config` 与 Nitro 服务端都要能直接 import）。
 */

/** 服务端重定向路由（301：爬虫/无 JS 客户端仍拿到权重合并信号）。 */
export interface ServerRedirectRoute {
  route: string;
  to: string;
  /** 永久重定向：目前只有 301 一种用途（`routeRules` 要求状态码是字面量，不能是宽泛 number）。 */
  statusCode: 301;
}

/** 允许服务端渲染的读者站路由（ADR-0019 决策 1 / 约束 1）。 */
export const SSR_WHITELIST_PATTERNS = ["/", "/article/**"] as const;

/**
 * 必须显式 `ssr: false` 的读者站路由（ADR-0019 决策 1）。
 *
 * `/collections/**` 与 `/projects/**` 都是运营编排列表：内容随管理员的操作变化，
 * 不参与 SSR 首屏，也不进匿名缓存。
 * `/history` 是私人数据（登录用户自己的浏览记录）：既不该被索引，也不该进
 * 匿名共享的整页 HTML 缓存 —— 后者是跨用户泄漏面。
 * `/messages/**` 是消息中心（#239）：私人数据、不索引，整段（含 `/messages` 入口）
 * 都走 SPA，由客户端中间件重定向到默认子项。
 */
export const CLIENT_ONLY_PATTERNS = [
  "/users/**",
  "/search",
  "/studio/**",
  "/history",
  "/collections/**",
  "/projects/**",
  "/messages/**",
] as const;

/**
 * 必须由服务端 301 承担的历史入口。
 *
 * `/articles` 是并入 `/search` 的旧列表入口；`/studio/history` 是历史记录从创作区
 * 提升为顶级页 `/history` 之后的旧链接（见 #234 决策 7 / #241）。
 * 写成普通 `ssr: false` 会让爬虫/无 JS 客户端只拿到 200 空壳。
 */
export const SERVER_REDIRECT_ROUTES = [
  { route: "/articles", to: "/search", statusCode: 301 },
  { route: "/studio/history", to: "/history", statusCode: 301 },
  // 关注与粉丝合成一页（票 327 `/studio/relations`，页内顶栏切换关注/粉丝/互关）。
  // 旧的两条入口保留 301：它们是个人中心里的链接，历史收藏与浏览器书签可能还留着。
  { route: "/studio/follows", to: "/studio/relations?tab=follow", statusCode: 301 },
  { route: "/studio/fans", to: "/studio/relations?tab=fans", statusCode: 301 },
] as const satisfies readonly ServerRedirectRoute[];

/** 切分路由（丢掉空段）。 */
function splitRoute(value: string): string[] {
  return value.split("/").filter(Boolean);
}

/**
 * 路由规则模式是否覆盖某个路由（`**` 匹配零到多段，`*` 匹配一段）。
 *
 * 参数既可以是具体请求路径（`/article/a1`），也可以是页面文件派生的路由模式
 * （`/article/*`）；两者都按"段"匹配，所以同一套判据能同时服务配置、缓存与门禁。
 */
export function routePatternMatches(pattern: string, route: string): boolean {
  const patternSegments = splitRoute(pattern);
  const routeSegments = splitRoute(route);

  function matchesFrom(patternIndex: number, routeIndex: number): boolean {
    if (patternIndex === patternSegments.length) return routeIndex === routeSegments.length;
    const segment = patternSegments[patternIndex];
    if (segment === "**") {
      for (let next = routeIndex; next <= routeSegments.length; next += 1) {
        if (matchesFrom(patternIndex + 1, next)) return true;
      }
      return false;
    }
    if (routeIndex >= routeSegments.length) return false;
    if (segment === "*") return matchesFrom(patternIndex + 1, routeIndex + 1);
    return segment === routeSegments[routeIndex] && matchesFrom(patternIndex + 1, routeIndex + 1);
  }

  return matchesFrom(0, 0);
}

/** 该路由是否落在 SSR 白名单里（白名单是唯一判据，不按页面随手加）。 */
export function isWhitelistedSsrRoute(route: string): boolean {
  return SSR_WHITELIST_PATTERNS.some((pattern) => routePatternMatches(pattern, route));
}

/**
 * 请求路径是否是可进匿名 SSR 缓存的**整页 HTML**：白名单内，且深度不超过
 * "白名单模式允许的那一层页面"（`/article/<id>` 是页面，`/article/<id>/_payload.json` 是数据端点）。
 *
 * 缓存范围从渲染边界派生；"能不能缓存"还受匿名 / TTL / 整页 HTML 三条约束，
 * 见 `server/utils/ssrAnonymousCache.ts`。
 */
export function isSsrPagePath(pathname: string): boolean {
  const segments = splitRoute(pathname);
  return SSR_WHITELIST_PATTERNS.some((pattern) => {
    if (!routePatternMatches(pattern, pathname)) return false;
    // `**` 是白名单里"页面路由"的通配段：具体请求路径只允许比固定前缀多一段。
    if (!pattern.includes("**")) return segments.length === splitRoute(pattern).length;
    return segments.length === splitRoute(pattern.replace(/\*\*/gu, "")).length + 1;
  });
}


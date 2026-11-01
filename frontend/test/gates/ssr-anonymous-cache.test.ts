import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import nuxtConfig from "../../nuxt.config";
import {
  CLIENT_ONLY_PATTERNS,
  SERVER_REDIRECT_ROUTES,
  SSR_WHITELIST_PATTERNS,
} from "../../shared/readerRenderingBoundary";
import {
  SSR_ANONYMOUS_CACHE_TTL_MS,
  anonymousCacheKey,
  createSsrAnonymousCacheStore,
  isCacheableSsrPath,
  isCacheableSsrRequest,
  isStorableAnonymousSsrResponse,
  type AnonymousCacheUrl,
} from "../../server/utils/ssrAnonymousCache";

/**
 * 匿名 SSR 缓存门禁（ADR-0019 决策 5 / 约束 2、3）。
 *
 * 缓存是**按 URL 键**的共享缓存：同一份 HTML 会发给所有读者。所以这张票有两条
 * 不能松的约束，这里逐条钉住：
 *   1. 只缓存 SSR 白名单（`/` 与 `/article/**`）—— 列表页、个人主页、搜索页、
 *      创作区、API 一律不进缓存；
 *   2. 缓存键里**没有**请求身份（cookie / Authorization / UA）—— 一旦有人
 *      "顺手把它们也当 key 一部分"，登录用户就会各拿一份私有 HTML，缓存失去意义。
 *
 * 判据写成纯函数 + 人造违规用例（canary），而不是只断言当前实现：只断言现状的
 * 门禁是恒绿的，判据被改坏时没人会发现。风格参照 test/gates/rendering-boundary.test.ts。
 *
 * 样例路由与策略判据是**门禁夹具**（只在本文件用），刻意不放进生产模块
 * `server/utils/ssrAnonymousCache.ts`：那边只留运行时纯函数（键 / 可存性 / TTL / 上限）。
 */
const projectDir = process.cwd();

/** 可缓存的样例路由：从渲染边界的 SSR 白名单派生（`/**` → 一个具体 id）。 */
const SSR_SAMPLE_PATHS = SSR_WHITELIST_PATTERNS.map(pattern => pattern.replace("/**", "/a1"));

/** 绝不能进缓存的样例路由：SPA/301 路由从渲染边界派生，再补上数据端点与错误兜底。 */
const NON_SSR_SAMPLE_PATHS = [
  ...CLIENT_ONLY_PATTERNS.map(pattern => pattern.replace("/**", "/u1")),
  ...SERVER_REDIRECT_ROUTES.map(route => route.route),
  "/api/articles/a1",
  "/article/a1/_payload.json",
  "/__nuxt_error",
];

/** 缓存策略的判据入参：真实实现与"人造违规"都塞进来跑同一个判据。 */
interface SsrAnonymousCachePolicy {
  ttlMs: number;
  pathPredicate: (pathname: string) => boolean;
  key: (request: AnonymousCacheUrl) => string;
}

type SsrAnonymousCachePolicyViolation =
  | { kind: "ttl-not-30s"; actual: number }
  | { kind: "caches-non-ssr-path"; pathname: string }
  | { kind: "ssr-path-not-cached"; pathname: string }
  | { kind: "key-varies-by-request-identity"; pathname: string };

/**
 * 缓存策略判据：返回全部违规（空数组 = 策略符合 ADR-0019 决策 5 / 约束 3）。
 * 判据里包含两条"人造"检查：不同请求身份必须拿到同一个键、非白名单路由必须被拒绝。
 * 只断言当前实现的门禁是恒绿的，判据被改坏时没人会发现。
 */
function ssrAnonymousCachePolicyViolations(
  policy: SsrAnonymousCachePolicy,
): SsrAnonymousCachePolicyViolation[] {
  const violations: SsrAnonymousCachePolicyViolation[] = [];

  if (policy.ttlMs !== SSR_ANONYMOUS_CACHE_TTL_MS) {
    violations.push({ kind: "ttl-not-30s", actual: policy.ttlMs });
  }

  for (const pathname of SSR_SAMPLE_PATHS) {
    if (!policy.pathPredicate(pathname)) {
      violations.push({ kind: "ssr-path-not-cached", pathname });
    }
  }

  for (const pathname of NON_SSR_SAMPLE_PATHS) {
    if (policy.pathPredicate(pathname)) {
      violations.push({ kind: "caches-non-ssr-path", pathname });
    }
  }

  // 请求身份只差这几样（cookie / Authorization / UA）；键必须一样，否则就是 per-user 缓存。
  const anonymousRequest: AnonymousCacheUrl = { pathname: "/article/a1", search: "" };
  const loggedInRequest = {
    pathname: "/article/a1",
    search: "",
    cookie: "refresh=token",
    authorization: "Bearer token",
    "user-agent": "facebookexternalhit/1.1",
  } as AnonymousCacheUrl;

  if (policy.key(anonymousRequest) !== policy.key(loggedInRequest)) {
    violations.push({ kind: "key-varies-by-request-identity", pathname: anonymousRequest.pathname });
  }

  return violations;
}

const LOCAL_POLICY: SsrAnonymousCachePolicy = {
  ttlMs: SSR_ANONYMOUS_CACHE_TTL_MS,
  pathPredicate: isCacheableSsrPath,
  key: anonymousCacheKey,
};

function routeRulesOf(config: { routeRules?: unknown }): Record<string, Record<string, unknown>> {
  const rules: Record<string, Record<string, unknown>> = {};
  for (const [pattern, rule] of Object.entries((config.routeRules ?? {}) as Record<string, unknown>)) {
    rules[pattern] = typeof rule === "object" && rule !== null ? { ...(rule as Record<string, unknown>) } : {};
  }
  return rules;
}

describe("匿名 SSR 缓存：TTL 与路由边界（ADR-0019 决策 5）", () => {
  it("TTL 是 30 秒", () => {
    expect(SSR_ANONYMOUS_CACHE_TTL_MS).toBe(30_000);
  });

  it("只缓存 SSR 白名单：/ 与 /article/**；其余读者站路由一律不进缓存", () => {
    for (const pathname of SSR_SAMPLE_PATHS) {
      expect(isCacheableSsrPath(pathname)).toBe(true);
    }
    for (const pathname of NON_SSR_SAMPLE_PATHS) {
      expect(isCacheableSsrPath(pathname)).toBe(false);
    }
  });

  it("文章页的数据端点 / 嵌套路径不是 HTML，不缓存", () => {
    expect(isCacheableSsrPath("/article/a1/_payload.json")).toBe(false);
    expect(isCacheableSsrPath("/article/")).toBe(false);
    expect(isCacheableSsrPath("/article/a1/extra")).toBe(false);
  });

  it("只处理读请求：POST/非 GET 的 HTML 不缓存", () => {
    expect(isCacheableSsrRequest({ pathname: "/", method: "GET" })).toBe(true);
    expect(isCacheableSsrRequest({ pathname: "/article/a1", method: "HEAD" })).toBe(true);
    expect(isCacheableSsrRequest({ pathname: "/", method: "POST" })).toBe(false);
    expect(isCacheableSsrRequest({ pathname: "/search", method: "GET" })).toBe(false);
  });

  it("缓存策略与 nuxt.config 的渲染边界一致（同一条共享表：SPA 与 301 路由都不进缓存）", () => {
    const rules = routeRulesOf(nuxtConfig);

    expect(rules["/"]?.ssr).not.toBe(false);
    // 渲染边界的唯一来源就是 shared/readerRenderingBoundary.ts；这里删掉任何一个
    // 白名单项都会让断言（以及 rendering-boundary 门禁）变红，不是恒绿。
    expect([...SSR_WHITELIST_PATTERNS]).toEqual(["/", "/article/**"]);
    for (const pattern of CLIENT_ONLY_PATTERNS) {
      expect(isCacheableSsrPath(pattern.replace(/\*\*/g, "u1"))).toBe(false);
    }
    for (const route of SERVER_REDIRECT_ROUTES) {
      expect(isCacheableSsrPath(route.route)).toBe(false);
    }
  });
});

describe("匿名 SSR 缓存：键只由 URL 决定（ADR-0019 约束 3）", () => {
  it("换 cookie / Authorization / UA 拿到的缓存键相同", () => {
    // 这正是"不按用户区分键"的判据：请求身份压根不参与 key。
    const anonymous = { pathname: "/article/a1", search: "" };
    const loggedIn = {
      pathname: "/article/a1",
      search: "",
      cookie: "refresh=token",
      authorization: "Bearer token",
      "user-agent": "facebookexternalhit/1.1",
    };

    expect(anonymousCacheKey(anonymous)).toBe(anonymousCacheKey(loggedIn));
  });

  it("同一个 URL 才是同一个键：不同文章 / 不同查询串互不命中", () => {
    expect(anonymousCacheKey({ pathname: "/article/a1" })).not.toBe(anonymousCacheKey({ pathname: "/article/a2" }));
    expect(anonymousCacheKey({ pathname: "/", search: "" })).not.toBe(anonymousCacheKey({ pathname: "/", search: "?page=2" }));
  });
});

describe("匿名 SSR 缓存：只存匿名且可共享的响应", () => {
  it("只缓存 200 + text/html + 不带 Set-Cookie 的整页响应", () => {
    expect(
      isStorableAnonymousSsrResponse({
        statusCode: 200,
        body: "<html></html>",
        headers: { "content-type": "text/html;charset=utf-8" },
      }),
    ).toBe(true);
  });

  it("404 / 非 HTML / 带 Set-Cookie / 非字符串 body 都不进缓存", () => {
    expect(
      isStorableAnonymousSsrResponse({
        statusCode: 404,
        body: "<html></html>",
        headers: { "content-type": "text/html;charset=utf-8" },
      }),
    ).toBe(false);
    expect(
      isStorableAnonymousSsrResponse({
        statusCode: 200,
        body: '{"data":1}',
        headers: { "content-type": "application/json" },
      }),
    ).toBe(false);
    expect(
      isStorableAnonymousSsrResponse({
        statusCode: 200,
        body: "<html></html>",
        headers: { "content-type": "text/html;charset=utf-8", "set-cookie": "refresh=x" },
      }),
    ).toBe(false);
    expect(isStorableAnonymousSsrResponse({ statusCode: 200, body: { html: "" }, headers: {} })).toBe(false);
  });

  it("30 秒内命中、到期后失效、清空后不再命中", () => {
    let now = 1_000;
    const store = createSsrAnonymousCacheStore({ now: () => now });
    const key = anonymousCacheKey({ pathname: "/article/a1" });
    const entry = {
      statusCode: 200,
      headers: { "content-type": "text/html;charset=utf-8" },
      body: "<html>匿名</html>",
    };

    store.write(key, entry);
    expect(store.read(key)?.body).toBe("<html>匿名</html>");

    now += SSR_ANONYMOUS_CACHE_TTL_MS - 1;
    expect(store.read(key)?.body).toBe("<html>匿名</html>");

    now += 1;
    expect(store.read(key)).toBeNull();

    store.clear();
    expect(store.read(key)).toBeNull();
  });
});

describe("门禁不能恒绿：人造的缓存违规必须被抓到", () => {
  it("TTL 不是 30 秒会红", () => {
    expect(ssrAnonymousCachePolicyViolations({ ...LOCAL_POLICY, ttlMs: 300 })).toContainEqual({
      kind: "ttl-not-30s",
      actual: 300,
    });
  });

  it("把 /search 放进缓存会红", () => {
    const policy = {
      ...LOCAL_POLICY,
      pathPredicate: (pathname: string) => pathname === "/" || pathname === "/search",
    };

    expect(ssrAnonymousCachePolicyViolations(policy)).toContainEqual({
      kind: "caches-non-ssr-path",
      pathname: "/search",
    });
  });

  it("把 cookie 掺进缓存键（退化成 per-user 缓存）会红", () => {
    const policy = {
      ...LOCAL_POLICY,
      key: (request: { pathname: string; search?: string }) =>
        `${request.pathname}${request.search ?? ""}#${String((request as Record<string, unknown>).cookie ?? "")}`,
    };

    expect(ssrAnonymousCachePolicyViolations(policy)).toContainEqual({
      kind: "key-varies-by-request-identity",
      pathname: "/article/a1",
    });
  });

  it("当前实现本身通过判据（判据不是只对 canary 生效）", () => {
    expect(ssrAnonymousCachePolicyViolations(LOCAL_POLICY)).toEqual([]);
  });
});

describe("实现接线：Nitro 钩子做缓存，键只从 URL 取", () => {
  it("缓存落在 render:before / render:response 钩子上（不是 routeRules 的 swr）", () => {
    // routeRules 的缓存只按"注册 handler 的 route"套（渲染器注册在 /**），在 node-server
    // 预设下永远匹配不上 / 与 /article/** —— 写 swr 只会给出"已经开了缓存"的假信号。
    const pluginSource = readFileSync(join(projectDir, "server/plugins/ssrAnonymousCache.ts"), "utf8");

    expect(pluginSource).toContain("render:before");
    expect(pluginSource).toContain("render:response");
    expect(pluginSource).toContain("getRequestURL");
    expect(pluginSource).toContain("anonymousCacheKey");
    expect(pluginSource).not.toContain("getRequestHeader");
  });

  it("渲染边界配置里没有 routeRules 的缓存声明（避免假信号）", () => {
    const rules = routeRulesOf(nuxtConfig);

    for (const rule of Object.values(rules)) {
      expect(rule.swr).toBeUndefined();
      expect(rule.cache).toBeUndefined();
    }
  });
});

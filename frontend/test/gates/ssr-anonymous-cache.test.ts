import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import nuxtConfig from "../../nuxt.config";
import {
  NON_SSR_SAMPLE_PATHS,
  SSR_ANONYMOUS_CACHE_TTL_MS,
  SSR_SAMPLE_PATHS,
  anonymousCacheKey,
  createSsrAnonymousCacheStore,
  isCacheableSsrPath,
  isCacheableSsrRequest,
  isStorableAnonymousSsrResponse,
  ssrAnonymousCachePolicyViolations,
  type SsrAnonymousCachePolicy,
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
 */
const projectDir = process.cwd();

/** 允许进缓存的读者站路由（与 rendering-boundary 的 SSR 白名单同一张表）。 */
const SSR_WHITELIST = ["/", "/article/**"];

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

  it("缓存策略与 nuxt.config 的渲染边界一致（/articles 是 301，不进缓存；ssr:false 的页面不进缓存）", () => {
    const rules = routeRulesOf(nuxtConfig);

    expect(rules["/"]?.ssr).not.toBe(false);
    expect(SSR_WHITELIST).toEqual(["/", "/article/**"]);
    for (const pattern of ["/articles", "/search", "/users/**", "/studio/**"]) {
      expect(isCacheableSsrPath(pattern.replace(/\*\*/g, "u1"))).toBe(false);
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

import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import nuxtConfig from "../../nuxt.config";

/**
 * 渲染边界门禁（ADR-0019 决策 1 / 约束 1）。
 *
 * Nuxt 默认**全站 SSR**："关掉 SSR"是逐条显式声明，漏写一行没有任何信号，
 * 页面就悄悄带着服务端渲染上线 —— 每次直达都白付一次服务端渲染代价。所以
 * 边界必须是一张**双向白名单**：
 *   1. 允许服务端渲染的只有 `/` 与 `/article/**`（分享卡片与搜索引擎靠它，
 *      ADR-0019 决策 1）；新增 SSR 页面必须先改 ADR 再加进白名单；
 *   2. 其余读者站路由必须在 `routeRules` 里显式 `ssr: false`；
 *   3. `/articles` 例外：它是历史列表入口，已并入 `/search`，必须由**服务端
 *      301**（`routeRules.redirect`）在渲染前把权重信号合并过去。写成普通
 *      `ssr: false` 会让爬虫/无 JS 客户端只拿到 200 空壳，属于 SEO 退化；
 *   4. `app/pages` 下**每个**页面文件都要落在上面三类之一 —— 新加一个页面却
 *      不分类，默认就是 SSR，门禁必须把它抓出来。这正是"随手给列表页或详情
 *      页加 SSR"会漏网的地方。
 *
 * 判据写成纯函数 + 人造违规用例（canary），而不是只断言当前配置：只断言
 * 现状的门禁是恒绿的，判据被改坏时没有人会发现。
 */
const projectDir = process.cwd();

/** 允许服务端渲染的读者站路由（ADR-0019 决策 1 / 约束 1：只能改 ADR，不能按页面随手加）。 */
const SSR_WHITELIST = ["/", "/article/**"];

/** 必须显式关掉 SSR 的读者站路由（ADR-0019 决策 1：个人主页、搜索页、创作区）。 */
const CLIENT_ONLY_ROUTES = ["/users/**", "/search", "/studio/**"];

/** 必须由服务端 301 承担的历史入口（不能退化成 200 空壳或普通 SPA）。 */
const SERVER_REDIRECT_ROUTES = [{ route: "/articles", to: "/search", statusCode: 301 }];

/** 归一化后的 routeRules：模式 → 规则对象（字符串简写视为"没有 ssr/redirect 声明"）。 */
type RouteRules = Record<string, Record<string, unknown>>;

type Redirect = { to: string; statusCode: number };

type Violation =
  | { kind: "ssr-enabled-off-whitelist"; route: string }
  | { kind: "whitelist-route-disabled"; route: string }
  | { kind: "client-only-not-registered"; route: string }
  | { kind: "client-only-missing"; route: string }
  | { kind: "server-redirect-missing"; route: string; expected: Redirect; actual: Redirect | null }
  | { kind: "page-route-unclassified"; route: string };

function routeRulesOf(config: { routeRules?: unknown }): RouteRules {
  const rules: RouteRules = {};
  for (const [pattern, rule] of Object.entries((config.routeRules ?? {}) as Record<string, unknown>)) {
    rules[pattern] = typeof rule === "object" && rule !== null ? { ...(rule as Record<string, unknown>) } : {};
  }
  return rules;
}

/** 取一条规则里的重定向声明（Nuxt 允许字符串简写，这里按 302 归一化）。 */
function redirectOf(rule: Record<string, unknown> | undefined): Redirect | null {
  const redirect = rule?.redirect;
  if (typeof redirect === "string") return { to: redirect, statusCode: 302 };
  if (typeof redirect !== "object" || redirect === null) return null;
  const value = redirect as { to?: unknown; statusCode?: unknown };
  if (typeof value.to !== "string") return null;
  return { to: value.to, statusCode: typeof value.statusCode === "number" ? value.statusCode : 302 };
}

/** `app/pages` 下的文件路径 → 路由模式（动态段统一写成 `*`，与 routeRules 的 `**` 通配对齐）。 */
function pageRoutes(): string[] {
  function routeOf(segments: string[]): string {
    const normalized = segments.map((segment) => {
      if (/^\[\[?\.\.\./u.test(segment)) return "**";
      if (/^\[.*\]$/u.test(segment)) return "*";
      return segment;
    });
    return normalized.length > 0 ? `/${normalized.join("/")}` : "/";
  }

  function walk(dir: string, segments: string[]): string[] {
    const routes: string[] = [];
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      if (entry.isDirectory()) {
        routes.push(...walk(join(dir, entry.name), [...segments, entry.name]));
        continue;
      }
      if (!entry.name.endsWith(".vue")) continue;
      const name = entry.name.slice(0, -".vue".length);
      routes.push(routeOf(name === "index" ? segments : [...segments, name]));
    }
    return routes;
  }

  return walk(join(projectDir, "app/pages"), []).sort();
}

/** 路由规则模式是否覆盖某个具体路由（`**` 匹配零到多段，`*` 匹配一段）。 */
function patternMatches(pattern: string, route: string): boolean {
  const split = (value: string) => value.split("/").filter(Boolean);
  const patternSegments = split(pattern);
  const routeSegments = split(route);

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

function clientOnlyPatterns(rules: RouteRules): string[] {
  return Object.entries(rules)
    .filter(([, rule]) => rule.ssr === false)
    .map(([pattern]) => pattern);
}

/** 渲染边界判据：返回全部违规（空数组 = 边界符合白名单）。 */
function renderingBoundaryViolations(rules: RouteRules, pages: string[] = pageRoutes()): Violation[] {
  const violations: Violation[] = [];
  const clientOnly = clientOnlyPatterns(rules);
  const redirectPatterns = Object.entries(rules)
    .filter(([, rule]) => redirectOf(rule) !== null)
    .map(([pattern]) => pattern);

  for (const pattern of clientOnly) {
    if (SSR_WHITELIST.includes(pattern)) {
      violations.push({ kind: "whitelist-route-disabled", route: pattern });
    }
    if (!CLIENT_ONLY_ROUTES.includes(pattern)) {
      // 覆盖 `/articles` 被写成普通 SPA 的情况：重定向路由不许混进 ssr:false 清单。
      violations.push({ kind: "client-only-not-registered", route: pattern });
    }
  }

  for (const pattern of Object.keys(rules)) {
    if (rules[pattern]?.ssr === true && !SSR_WHITELIST.includes(pattern)) {
      violations.push({ kind: "ssr-enabled-off-whitelist", route: pattern });
    }
  }

  for (const pattern of CLIENT_ONLY_ROUTES) {
    if (!clientOnly.includes(pattern)) {
      violations.push({ kind: "client-only-missing", route: pattern });
    }
  }

  for (const expected of SERVER_REDIRECT_ROUTES) {
    const actual = redirectOf(rules[expected.route]);
    const matches =
      actual !== null &&
      actual.to === expected.to &&
      actual.statusCode === expected.statusCode &&
      rules[expected.route]?.ssr !== false;
    if (!matches) {
      violations.push({
        kind: "server-redirect-missing",
        route: expected.route,
        expected: { to: expected.to, statusCode: expected.statusCode },
        actual,
      });
    }
  }

  for (const page of pages) {
    const ssrWhitelisted = SSR_WHITELIST.some((pattern) => patternMatches(pattern, page));
    const explicitlyClientOnly = clientOnly.some((pattern) => patternMatches(pattern, page));
    const redirected = redirectPatterns.some((pattern) => patternMatches(pattern, page));
    if (!ssrWhitelisted && !explicitlyClientOnly && !redirected) {
      violations.push({ kind: "page-route-unclassified", route: page });
    }
  }

  return violations;
}

describe("渲染边界白名单（ADR-0019 决策 1）", () => {
  it("当前配置满足白名单：只有 / 与 /article/** 服务端渲染，其余读者站路由显式关掉 SSR", () => {
    expect(renderingBoundaryViolations(routeRulesOf(nuxtConfig))).toEqual([]);
  });

  it("白名单就是首页与文章页：只允许这两条改 ADR 之后再扩张", () => {
    expect(SSR_WHITELIST).toEqual(["/", "/article/**"]);
  });

  it("/users/** 显式 ssr: false（个人主页改 SPA）", () => {
    expect(routeRulesOf(nuxtConfig)["/users/**"]?.ssr).toBe(false);
  });

  it("/articles 是服务端 301 到 /search，不是普通 SPA（爬虫/无 JS 客户端仍拿到 301）", () => {
    const rule = routeRulesOf(nuxtConfig)["/articles"];

    expect(redirectOf(rule)).toEqual({ to: "/search", statusCode: 301 });
    expect(rule?.ssr).not.toBe(false);
  });

  it("搜索页与创作区维持现状：ssr: false（本票不改它们）", () => {
    const rules = routeRulesOf(nuxtConfig);

    expect(rules["/search"]?.ssr).toBe(false);
    expect(rules["/studio/**"]?.ssr).toBe(false);
  });

  it("app/pages 下每个页面都被分类过：新增页面默认会 SSR，未分类即违规", () => {
    // 光看 routeRules 抓不到"新加了一个页面但没人分类"——默认 SSR 是隐式的。
    // 先把页面清单读出来（下面的人造违规用例用它证明对账不是空转）。
    const pages = pageRoutes();

    expect(pages).toEqual(expect.arrayContaining(["/", "/article/*", "/articles", "/users/*", "/search"]));
    expect(pages.some((page) => page.startsWith("/studio/"))).toBe(true);
    expect(renderingBoundaryViolations(routeRulesOf(nuxtConfig), pages)).toEqual([]);
  });
});

describe("门禁不能恒绿：人造违规必须被抓到", () => {
  it("删掉 /articles 的服务端 301（直接删规则）会红", () => {
    const rules = routeRulesOf(nuxtConfig);
    delete rules["/articles"];

    expect(renderingBoundaryViolations(rules, [])).toContainEqual({
      kind: "server-redirect-missing",
      route: "/articles",
      expected: { to: "/search", statusCode: 301 },
      actual: null,
    });
  });

  it("把 /articles 写成普通 SPA（ssr: false，301 丢了）会红", () => {
    const rules = { ...routeRulesOf(nuxtConfig), "/articles": { ssr: false } };

    expect(renderingBoundaryViolations(rules, [])).toEqual(
      expect.arrayContaining([
        { kind: "server-redirect-missing", route: "/articles", expected: { to: "/search", statusCode: 301 }, actual: null },
        { kind: "client-only-not-registered", route: "/articles" },
      ]),
    );
  });

  it("重定向目标或状态码被改掉会红", () => {
    const rules = { ...routeRulesOf(nuxtConfig), "/articles": { redirect: { to: "/search", statusCode: 302 } } };

    expect(renderingBoundaryViolations(rules, [])).toContainEqual({
      kind: "server-redirect-missing",
      route: "/articles",
      expected: { to: "/search", statusCode: 301 },
      actual: { to: "/search", statusCode: 302 },
    });
  });

  it("给列表页开 SSR（/users/** 从 ssr: false 改成 ssr: true）会红", () => {
    const rules = { ...routeRulesOf(nuxtConfig), "/users/**": { ssr: true } };

    expect(renderingBoundaryViolations(rules, [])).toEqual(
      expect.arrayContaining([
        { kind: "ssr-enabled-off-whitelist", route: "/users/**" },
        { kind: "client-only-missing", route: "/users/**" },
      ]),
    );
  });

  it("漏掉 /users/** 的 ssr: false（默认重新变回 SSR）会红", () => {
    const rules = routeRulesOf(nuxtConfig);
    delete rules["/users/**"];

    expect(renderingBoundaryViolations(rules, [])).toContainEqual({
      kind: "client-only-missing",
      route: "/users/**",
    });
  });

  it("新增一个未分类页面（默认 SSR）会红", () => {
    expect(renderingBoundaryViolations(routeRulesOf(nuxtConfig), [...pageRoutes(), "/tags"])).toContainEqual({
      kind: "page-route-unclassified",
      route: "/tags",
    });
  });

  it("给文章页关掉 SSR（分享卡片就没了）会红", () => {
    const rules = { ...routeRulesOf(nuxtConfig), "/article/**": { ssr: false } };

    expect(renderingBoundaryViolations(rules, [])).toContainEqual({
      kind: "whitelist-route-disabled",
      route: "/article/**",
    });
  });
});

describe("分享卡片契约：文章页的 og:* 仍由服务端输出", () => {
  it("文章页继续用 useSeoMeta 声明 ogTitle / ogDescription（ADR-0019 约束 8）", () => {
    // 渲染边界改了，但"分享卡片必须有服务端 og:*"没变；这条锁住文章页别被顺手 SPA 化。
    const source = readFileSync(join(projectDir, "app/pages/article/[id].vue"), "utf8");

    expect(source).toContain("useSeoMeta");
    expect(source).toContain("ogTitle");
    expect(source).toContain("ogDescription");
  });

  it("首页仍是服务端渲染（站点门面不在 SPA 白名单里）", () => {
    expect(routeRulesOf(nuxtConfig)["/"]?.ssr).not.toBe(false);
  });
});

describe("/articles 客户端侧兜底：页面中间件跳转，而不是 setup 里的导航", () => {
  it("跳转放在页面中间件里，页面 setup 不 await 导航（后者会把客户端整页卡成空白）", () => {
    // 服务端 301 只覆盖文档请求；站内 router.push('/articles') 走的是客户端路由，
    // 由这个页面中间件兜底。改前的证据：改成 ssr: false 后，页面 setup 里 await
    // 一次路由替换会打断客户端初始挂起，URL 变成 /search 但 #__nuxt 始终为空
    // （#83 实测，无异常、无报错）。所以跳转搬到
    // app/middleware/articles-redirect.ts，在组件创建之前改路由。这条锁住别再写回 setup 里。
    const pageSource = readFileSync(join(projectDir, "app/pages/articles.vue"), "utf8");
    const middlewareSource = readFileSync(join(projectDir, "app/middleware/articles-redirect.ts"), "utf8");

    expect(pageSource).not.toMatch(/await navigateTo/u);
    expect(pageSource).toContain('middleware: ["articles-redirect"]');
    expect(middlewareSource).toContain('navigateTo("/search"');
    expect(middlewareSource).toContain("redirectCode: 301");
  });
});

import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import nuxtConfig from "../../nuxt.config";
import {
  CLIENT_ONLY_PATTERNS,
  SERVER_REDIRECT_ROUTES,
} from "../../shared/readerRenderingBoundary";
import { stripComments } from "../helpers/designFiles";

/**
 * 历史记录独立页的路由与入口（#241）。
 *
 * 与渲染边界门禁（rendering-boundary.test.ts）的分工：那道门禁守"每个页面都被
 * 分过类"，这道门禁守**这张票的具体承诺**：
 *   1. `/history` 是私人数据 → `ssr: false`（不进 SSR、不进匿名整页缓存）；
 *   2. 旧路径 `/studio/history` 必须 301 到 `/history`（服务端 + 客户端兜底）；
 *   3. 个人中心（侧栏与数据概览）不再有历史入口，旧页面的数据获取也已移除；
 *   4. 历史页的 `middleware: "auth"` 不能丢 —— 丢了就是"未登录也能看到空列表"，
 *      而接口会 403，页面显示成失败态。
 *
 * 判据全部从唯一来源（shared/readerRenderingBoundary.ts）读，不重复写清单。
 */
const projectDir = process.cwd();

function read(relativePath: string): string {
  const absolute = join(projectDir, relativePath);
  if (!existsSync(absolute)) throw new Error(`找不到文件 ${relativePath}`);
  return readFileSync(absolute, "utf8");
}

function routeRules(): Record<string, Record<string, unknown>> {
  const rules: Record<string, Record<string, unknown>> = {};
  for (const [pattern, rule] of Object.entries(
    (nuxtConfig.routeRules ?? {}) as Record<string, unknown>,
  )) {
    rules[pattern] = typeof rule === "object" && rule !== null ? { ...(rule as Record<string, unknown>) } : {};
  }
  return rules;
}

describe("历史独立页：渲染边界", () => {
  it("/history 是 client-only 且 routeRules 派生出 ssr: false", () => {
    expect(CLIENT_ONLY_PATTERNS).toContain("/history");
    expect(routeRules()["/history"]?.ssr).toBe(false);
  });

  it("/history 不在 SSR 白名单里（私人数据不进服务端渲染与匿名缓存）", () => {
    // 直接看 routeRules：白名单里只有 "/" 与 "/article/**"，写成 ssr:true 会红。
    expect(routeRules()["/history"]?.ssr).not.toBe(true);
  });

  it("/studio/history 是服务端 301 到 /history（不是普通 SPA、也不是停留的旧页面）", () => {
    expect(SERVER_REDIRECT_ROUTES).toContainEqual({
      route: "/studio/history",
      to: "/history",
      statusCode: 301,
    });

    const rule = routeRules()["/studio/history"];
    expect(rule?.redirect).toEqual({ to: "/history", statusCode: 301 });
    expect(rule?.ssr).not.toBe(false);
  });
});

describe("历史独立页：页面文件与守卫", () => {
  it("新页面带 auth 中间件，且不再依赖创作区的服务模块", () => {
    const page = stripComments(read("app/pages/history.vue"));

    expect(page).toContain("middleware: \"auth\"");
    expect(page).toContain("~/services/history");
    expect(page).not.toContain("~/services/studio");
  });

  it("旧 /studio/history 页面只剩重定向：不再取数据、不再渲染旧版式", () => {
    const legacy = stripComments(read("app/pages/studio/history.vue"));

    expect(legacy).toContain("history-redirect");
    expect(legacy).not.toContain("getHistoryArticles");
    expect(legacy).not.toContain("article-history-grid");
    // 旧页面是 studio 布局 + 数据加载；重定向页不该再声明布局。
    expect(legacy).not.toContain("layout: \"studio\"");
  });

  it("客户端兜底中间件与服务端 301 目标一致", () => {
    const middleware = read("app/middleware/history-redirect.ts");

    expect(middleware).toContain('navigateTo("/history"');
    expect(middleware).toContain("redirectCode: 301");
  });
});

describe("个人中心不再有历史入口", () => {
  it("侧栏导航里没有 /studio/history，也没有浏览历史文案", () => {
    const sidebar = stripComments(read("app/components/studio/StudioSidebarNav.vue"));

    expect(sidebar).not.toContain("/studio/history");
    expect(sidebar).not.toContain("浏览历史");
  });

  it("数据概览不再把浏览历史当成一张指标卡，也不再请求历史接口", () => {
    const dashboard = stripComments(read("app/pages/studio/dashboard.vue"));

    expect(dashboard).not.toContain("getHistoryArticles");
    expect(dashboard).not.toContain("浏览历史");
  });

  it("创作区服务模块不再导出历史接口（历史有自己的 services/history.ts）", () => {
    const studioService = stripComments(read("app/services/studio.ts"));

    expect(studioService).not.toContain("articles/history");
    expect(studioService).not.toContain("HistoryArticleListData");
  });
});

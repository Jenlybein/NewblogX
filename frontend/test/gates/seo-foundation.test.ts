import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { ROBOTS_DISALLOW_PATHS, ROBOTS_SITEMAP_PATH, buildRobotsTxt } from "../../shared/robotsTxt";

/**
 * #314 交付 2：robots.txt 的两件事 —— ① sitemap 在哪；② 哪些目录别来爬。
 *
 * 这里测的是**策略本身**（纯函数），不是"某个文件被部署了"：robots.txt 由
 * `server/routes/robots.txt.get.ts` 按运行时 `siteUrl` 现拼，因为生产域名来自
 * 环境变量（local / test / production 三个域名），静态文件写不出正确的那一个。
 *
 * 关键取舍（见 docs/notes/2026-11-07-publish-gate-and-moderation-grill.md §6.2③）：
 * 私密**页面**用页面级 `noindex`，而**不**在这里 Disallow —— 被 Disallow 的页面
 * 爬虫根本进不去，也就读不到页面里的 noindex；若别处有链接指向它，它仍可能以
 * "裸 URL 无标题"出现在结果里。所以 robots 只挡"没有页面形态、没有标签可看"的
 * 接口面。
 */

describe("buildRobotsTxt：门口指示牌", () => {
  const siteUrl = "https://blogx.gentlybeing.cn";

  it("sitemap 给的是**绝对地址**（爬虫要能直接取到，相对地址不被接受）", () => {
    expect(buildRobotsTxt(siteUrl)).toContain(`Sitemap: ${siteUrl}${ROBOTS_SITEMAP_PATH}`);
  });

  it("siteUrl 带结尾斜杠也拼不出双斜杠", () => {
    expect(buildRobotsTxt(`${siteUrl}/`)).toContain(`Sitemap: ${siteUrl}${ROBOTS_SITEMAP_PATH}`);
  });

  it("默认全站放行，只 Disallow 接口面（/api/）", () => {
    const txt = buildRobotsTxt(siteUrl);

    expect(txt).toContain("User-agent: *");
    expect(txt).toContain("Disallow: /api/");
    expect(ROBOTS_DISALLOW_PATHS).toEqual(["/api/"]);
  });

  it("**不** Disallow 私密页面：它们要靠页面里的 noindex 才有效", () => {
    const txt = buildRobotsTxt(siteUrl);

    for (const privatePath of ["/studio", "/messages", "/history", "/search"]) {
      expect(txt).not.toContain(`Disallow: ${privatePath}`);
    }
  });

  it("不 Disallow 内容面：首页与文章页必须可抓", () => {
    const txt = buildRobotsTxt(siteUrl);

    expect(txt).not.toContain("Disallow: /article");
    expect(txt).not.toContain("Disallow: /$");
  });

  it("文件以换行结尾（robots.txt 是逐行协议，最后一行不闭合会被部分爬虫忽略）", () => {
    expect(buildRobotsTxt(siteUrl).endsWith("\n")).toBe(true);
  });

  it("没有配 siteUrl 时退化成站内相对路径，不输出 ://", () => {
    expect(buildRobotsTxt("")).toContain(`Sitemap: ${ROBOTS_SITEMAP_PATH}`);
  });
});

/**
 * robots.txt 的落点必须是**能读运行时配置**的地方。
 *
 * 钉住这条是因为"静态文件放 public/"这个写法在本站是错的：域名来自
 * `NUXT_PUBLIC_SITE_URL`（local / test / production 三个不同域名），静态文件
 * 只能写死一个 —— 测试环境会把爬虫指向生产站的 sitemap。
 */
describe("robots.txt 的落点", () => {
  const projectDir = process.cwd();

  it("由 Nitro 路由按运行时 siteUrl 生成，public/ 下不再有静态副本", () => {
    const routeSource = readFileSync(join(projectDir, "server/routes/robots.txt.get.ts"), "utf8");

    expect(routeSource).toContain("buildRobotsTxt");
    expect(routeSource).toContain("siteUrl");
    expect(() => readFileSync(join(projectDir, "public/robots.txt"), "utf8")).toThrow();
  });
});

/* ------------------------------------------------------------------------- *
 * 页面级 noindex（#314 交付 3）
 * ------------------------------------------------------------------------- */

const projectDir = process.cwd();
const PAGES_DIR = join(projectDir, "app/pages");

/** 必须声明 noindex 的路由前缀 / 整路由（票面清单）。 */
const PRIVATE_ROUTES = ["/studio/**", "/messages/**", "/history", "/search"];

interface PageEntry {
  route: string;
  file: string;
}

/** 枚举 `app/pages`：动态段归一成 `*`，`index.vue` 归一到父路由。 */
function pageEntries(): PageEntry[] {
  function routeOf(segments: string[]): string {
    const normalized = segments.map((segment) => (/^\[.*\]$/u.test(segment) ? "*" : segment));
    return normalized.length > 0 ? `/${normalized.join("/")}` : "/";
  }

  function walk(dir: string, segments: string[]): PageEntry[] {
    const entries: PageEntry[] = [];
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const file = join(dir, entry.name);
      if (entry.isDirectory()) {
        entries.push(...walk(file, [...segments, entry.name]));
        continue;
      }
      if (!entry.name.endsWith(".vue")) continue;
      const name = entry.name.slice(0, -".vue".length);
      entries.push({ route: routeOf(name === "index" ? segments : [...segments, name]), file });
    }
    return entries;
  }

  return walk(PAGES_DIR, []);
}

/** 路由是否落在私密清单里（`/studio/**` 覆盖它自己与所有子页）。 */
function matchesPrivateRoute(route: string): boolean {
  return PRIVATE_ROUTES.some((pattern) => {
    if (!pattern.endsWith("/**")) return route === pattern;
    const prefix = pattern.slice(0, -"/**".length);
    return route === prefix || route.startsWith(`${prefix}/`);
  });
}

/**
 * 逐页钉而不是"信约定"：私密页是**新增页面时最容易漏**的一类 —— 新加一个
 * `/studio/xxx.vue` 不会有人想起来加 robots，直到它出现在搜索结果里。
 * 所以这里按路由清单枚举 `app/pages`，任何一个私密页缺 `useNoIndexPage()` 都会红。
 */
describe("私密页的 noindex（页面级，不是 robots.txt Disallow）", () => {
  it("清单非空：/search、/history、/messages、/studio 都在页面树里（否则这条门禁是空转）", () => {
    const routes = pageEntries()
      .map((entry) => entry.route)
      .filter(matchesPrivateRoute);

    expect(routes).toEqual(
      expect.arrayContaining(["/search", "/history", "/messages", "/messages/*", "/studio/write"]),
    );
  });

  it("每个私密页都在 setup 里调用 useNoIndexPage()", () => {
    const missing = pageEntries()
      .filter((entry) => matchesPrivateRoute(entry.route))
      .filter((entry) => !readFileSync(entry.file, "utf8").includes("useNoIndexPage()"))
      .map((entry) => entry.route);

    expect(missing).toEqual([]);
  });

  it("公开页**不**声明 noindex（首页与文章页要被收录）", () => {
    for (const file of ["app/pages/index.vue", "app/pages/article/[id].vue"]) {
      expect(readFileSync(join(projectDir, file), "utf8")).not.toContain("useNoIndexPage()");
    }
  });

  it("noindex 的指令值只有一个来源（useNoIndexPage → utils/seo 的常量），不在页面里手写字面量", () => {
    const composable = readFileSync(join(projectDir, "app/composables/useNoIndexPage.ts"), "utf8");

    expect(composable).toContain("PRIVATE_PAGE_ROBOTS");
    expect(composable).toContain("useSeoMeta");
  });
});

/* ------------------------------------------------------------------------- *
 * 文章页：canonical / 分享卡片 / 结构化数据（#314 交付 4–6）
 * ------------------------------------------------------------------------- */

const articlePageSource = () => readFileSync(join(projectDir, "app/pages/article/[id].vue"), "utf8");
const homePageSource = () => readFileSync(join(projectDir, "app/pages/index.vue"), "utf8");

describe("文章页的 canonical：改 head，不改地址栏与路由", () => {
  it("link rel=canonical 指向 canonicalArticleUrl 的产物", () => {
    const source = articlePageSource();

    expect(source).toContain('rel: "canonical"');
    expect(source).toContain("canonicalArticleUrl(");
  });

  it("规范地址从 id 拼出来，**不**从当前地址（route.fullPath / useRequestURL）反推", () => {
    const source = articlePageSource();

    // `?from=` 的来路参数不参与 canonical 的构造 —— 页面只传 id 与 siteUrl。
    expect(source).not.toContain("route.fullPath");
    expect(source).not.toContain("useRequestURL");
  });

  it("地址栏与路由行为原样保留：左侧抽屉照旧读 route.query.from", () => {
    const source = articlePageSource();

    expect(source).toContain("route.query.from");
    // canonical 是 head 里的一行，不是一次跳转：页面里不该出现"为了洗掉参数"的
    // 路由替换（那样会丢掉合集来路，抽屉会空）。
    expect(source).not.toContain("router.replace");
    expect(source).not.toContain("navigateTo(`/article/");
  });
});

describe("分享卡片：声明成「一篇文章」而不是「一个网站」", () => {
  it("og:type=article、og:url=规范地址", () => {
    const source = articlePageSource();

    expect(source).toContain('ogType: "article"');
    expect(source).toContain("ogUrl:");
  });

  it("发布时间、作者、标签都进 article:* 字段", () => {
    const source = articlePageSource();

    expect(source).toContain("articlePublishedTime:");
    expect(source).toContain("articleAuthor:");
    expect(source).toContain("articleTag:");
  });
});

describe("结构化数据（JSON-LD）", () => {
  it("文章页产出 Article 与 BreadcrumbList（两条 script，独立 key 便于水合替换）", () => {
    const source = articlePageSource();

    expect(source).toContain("articleJsonLd(");
    expect(source).toContain("breadcrumbJsonLd(");
    expect(source).toContain('type: "application/ld+json"');
    expect(source).toContain('key: "seo-article-jsonld"');
    expect(source).toContain('key: "seo-breadcrumb-jsonld"');
  });

  it("首页产出 WebSite + SearchAction", () => {
    const source = homePageSource();

    expect(source).toContain("webSiteJsonLd(");
    expect(source).toContain('type: "application/ld+json"');
    expect(source).toContain('key: "seo-website-jsonld"');
  });

  it("JSON-LD 用 JSON.stringify 序列化，且不把对象直接塞进模板（会变成 [object Object]）", () => {
    expect(articlePageSource()).toContain("JSON.stringify(");
    expect(homePageSource()).toContain("JSON.stringify(");
  });
});

/**
 * 本票的两条硬约束：SSR 白名单不动、不加 RSS/Atom。
 *
 * 白名单已经有一个独立的门禁（rendering-boundary），这里再钉一次是因为本票
 * 恰好是最容易起"顺手把合集也 SSR 了吧"念头的一张票 —— 那条决定的代价（缓存与
 * 性能重估）不在本票预算内。
 */
describe("硬约束：渲染边界不动、不加订阅源", () => {
  it("SSR 白名单仍然只有 / 与 /article/**", async () => {
    const boundary = await import("../../shared/readerRenderingBoundary");

    expect([...boundary.SSR_WHITELIST_PATTERNS]).toEqual(["/", "/article/**"]);
  });

  it("没有 RSS/Atom：public 下没有 feed 文件，依赖里没有 feed 模块", () => {
    const publicFiles = readdirSync(join(projectDir, "public")).map((name) => name.toLowerCase());
    const pkg = JSON.parse(readFileSync(join(projectDir, "package.json"), "utf8")) as {
      dependencies?: Record<string, string>;
      devDependencies?: Record<string, string>;
    };
    const modules = Object.keys({ ...pkg.dependencies, ...pkg.devDependencies });

    for (const forbidden of ["rss.xml", "atom.xml", "feed.xml"]) {
      expect(publicFiles).not.toContain(forbidden);
    }
    expect(modules.filter((name) => /feed|rss|atom/iu.test(name))).toEqual([]);
  });
});


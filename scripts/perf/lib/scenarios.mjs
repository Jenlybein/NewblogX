// 测量场景（每次运行返回原始记录，聚合与判定在 report.mjs）：
//   firstScreen.*  文档级直达的首屏内容可见 + 字体字节（① ③；含 SPA 个人主页、补充参考）
//   transition.*   点击站内链接后的切页内容可见（②）
//   backCache.*    浏览器返回上一页是否命中缓存（④）
//   redirect.*     历史入口 /articles 是否由服务端 301 到 /search（渲染边界，ADR-0019 决策 1）
import { DEFAULT_OPTIONS, openContext, settle, collectResources, fontFilesOf, apiResourcesOf, sumBytes, apiWallMs, sleep } from "./browser.mjs";

/** ① ③：文档级直达（冷 = 全新上下文；热 = 同上下文先访问一次再测）。 */
export async function runFirstScreen({ browser, base, mode, target, articleUrl, userUrl, options }) {
  const opts = { ...DEFAULT_OPTIONS, ...options };
  const url =
    target === "home" ? new URL("/", base).href : target === "article" ? articleUrl : userUrl;
  if (!url) {
    throw new Error(`firstScreen.${target} 的目标 URL 未发现`);
  }
  const cold = mode === "cold";
  const startedAt = Date.now();
  const { ctx, page, pageErrors } = await openContext(browser, {
    cold,
    options: opts,
    contentSelectors: opts.selectors[target],
  });
  try {
    if (!cold) {
      await page.goto(url, { waitUntil: "load", timeout: opts.navigationTimeoutMs });
      await settle(page, opts);
    }
    await page.goto(url, { waitUntil: "load", timeout: opts.navigationTimeoutMs });
    const content = await page.evaluate(timeout => window.__perf.waitContent(timeout), opts.visibleTimeoutMs);
    await settle(page, opts);
    const resources = await collectResources(page);
    const fontFiles = fontFilesOf(resources);
    return {
      scenario: `firstScreen.${target}`,
      mode,
      ok: true,
      elapsedMs: Date.now() - startedAt,
      metrics: {
        contentVisibleMs: content.contentVisibleMs,
        contentSelector: content.selector,
        fontBytes: sumBytes(fontFiles),
        fontFiles,
        requestCount: resources.length,
        totalBytes: sumBytes(resources),
        apiCount: apiResourcesOf(resources).length,
        pageErrors: pageErrors.length,
      },
    };
  } finally {
    await ctx.close().catch(() => {});
  }
}

/** ②：从首页点击第一篇文章，量"点击 → 目标内容可见"。 */
export async function runTransition({ browser, base, mode, articleUrl, options }) {
  const opts = { ...DEFAULT_OPTIONS, ...options };
  const home = new URL("/", base).href;
  const cold = mode === "cold";
  const startedAt = Date.now();
  const { ctx, page, pageErrors } = await openContext(browser, {
    cold,
    options: opts,
    contentSelectors: opts.selectors.home,
  });
  try {
    if (!cold) {
      await page.goto(home, { waitUntil: "load", timeout: opts.navigationTimeoutMs });
      await settle(page, opts);
    }
    await page.goto(home, { waitUntil: "load", timeout: opts.navigationTimeoutMs });
    await page.evaluate(timeout => window.__perf.waitContent(timeout), opts.visibleTimeoutMs);
    await settle(page, opts);
    const snapshot = await page.evaluate(() => window.__perf.backSnapshot());
    const click = await page.evaluate(
      ([linkSelector, contentSelectors, timeoutMs]) =>
        window.__perf.clickWatch(linkSelector, contentSelectors, timeoutMs),
      [opts.transitionLink, opts.selectors.article, opts.clickTimeoutMs],
    );
    await settle(page, opts);
    const resources = (await collectResources(page)).filter(entry => entry.startTime >= snapshot.now);
    const api = apiResourcesOf(resources);
    return {
      scenario: "transition.homeToArticle",
      mode,
      ok: true,
      elapsedMs: Date.now() - startedAt,
      metrics: {
        contentVisibleMs: click.missing ? null : click.contentVisibleMs,
        clickToUrlMs: click.missing ? null : click.clickToUrlMs,
        clickToErrorMs: click.missing ? null : click.errorAtMs,
        errorMsg: click.missing ? null : click.errorMsg,
        timeoutMs: opts.clickTimeoutMs,
        apiCount: api.length,
        apiWallMs: apiWallMs(api),
        pageErrors: pageErrors.length,
        pageErrorMsg: pageErrors.length > 0 ? pageErrors[0] : null,
      },
    };
  } finally {
    await ctx.close().catch(() => {});
  }
}

/**
 * ④：站内点击进文章 →（浏览器返回）→ 首页，量是否命中缓存（文档级重载 + 数据请求数/耗时）。
 *
 * 口径修订（#87，P0 修复后）：返回场景改测**真实用户路径**——在首页点击文章链接（客户端跳转），
 * 再按浏览器「返回」。这时 `performance.timeOrigin` 不变、期望 Vue Router 走 SPA 返回并命中
 * 30s 客户端 SWR 缓存（0 个数据请求）。原来的实现用 `page.goto(articleUrl)` 进文章页，
 * 于是「返回」必然是文档级重载——那只测到无头浏览器的口径坑，测不到客户端缓存是否命中。
 *
 * P0（#80）未修时站内点击根本进不去文章页：此时退化为旧口径（文档级 goto 文章页再返回），
 * 报告用 `backFlow=document` 标注。改前基线就是这一形态，因此不把「测不到」误判成「没命中」。
 */
export async function runBackCache({ browser, base, mode, articleUrl, options }) {
  const opts = { ...DEFAULT_OPTIONS, ...options };
  const home = new URL("/", base).href;
  const cold = mode === "cold";
  const startedAt = Date.now();
  const { ctx, page, pageErrors } = await openContext(browser, {
    cold,
    options: opts,
    contentSelectors: opts.selectors.home,
  });

  const visitHome = async () => {
    await page.goto(home, { waitUntil: "load", timeout: opts.navigationTimeoutMs });
    await page.evaluate(timeout => window.__perf.waitContent(timeout), opts.visibleTimeoutMs);
    await settle(page, opts);
  };
  const visitArticleDocument = async () => {
    await page.goto(articleUrl, { waitUntil: "load", timeout: opts.navigationTimeoutMs });
    await page.evaluate(
      ([selectors, timeoutMs]) => window.__perf.waitVisibleAny(selectors, timeoutMs),
      [opts.selectors.article, opts.visibleTimeoutMs],
    );
    await settle(page, opts);
  };
  // 站内点击进文章：能进就记 spa（真实路径）；进不去（P0 崩溃 / 链接缺失）就交给旧口径。
  // 这里的等待上限比切页场景短：崩溃时不必等满 12s，测的是"点了有没有反应"。
  const enterArticle = async () => {
    const click = await page.evaluate(
      ([linkSelector, contentSelectors, timeoutMs]) =>
        window.__perf.clickWatch(linkSelector, contentSelectors, timeoutMs),
      [opts.transitionLink, opts.selectors.article, Math.min(opts.clickTimeoutMs, 4000)],
    );
    if (!click.missing && click.contentVisibleMs != null) {
      await settle(page, opts);
      return { flow: "spa", clickToArticleMs: click.contentVisibleMs };
    }
    await visitArticleDocument();
    return { flow: "document", clickToArticleMs: null };
  };
  const backOnce = async (flow) => {
    if (flow === "spa") {
      // SPA 返回：不会产生文档导航，等 URL 回到首页、首页内容重新可见即可（不跑 waitForNavigation，省 5s）。
      await page.evaluate(() => history.back()).catch(() => {});
      const deadline = Date.now() + opts.clickTimeoutMs;
      while (Date.now() < deadline) {
        const href = await page.evaluate(() => location.href).catch(() => null);
        if (href === home) break;
        await sleep(100);
      }
      await page
        .evaluate(
          ([selectors, timeoutMs]) => window.__perf.waitVisibleAny(selectors, timeoutMs),
          [opts.selectors.home, opts.visibleTimeoutMs],
        )
        .catch(() => undefined);
      return false;
    }
    const navPromise = page
      .waitForNavigation({ waitUntil: "load", timeout: opts.backNavigationWaitMs })
      .then(() => true)
      .catch(() => false);
    await page.evaluate(() => history.back()).catch(() => {});
    const navigated = await navPromise;
    if (!navigated) {
      // 非文档导航：等 URL 回到首页（或超时），再停稳。
      const deadline = Date.now() + opts.clickTimeoutMs;
      while (Date.now() < deadline) {
        const href = await page.evaluate(() => location.href).catch(() => null);
        if (href === home) break;
        await sleep(100);
      }
    }
    return navigated;
  };

  try {
    let flow = null;
    if (!cold) {
      // 热返回：先完整走一轮（不计入测量），让 HTTP 缓存、路由与客户端 SWR 都热起来。
      await visitHome();
      const warm = await enterArticle();
      await backOnce(warm.flow);
    }
    await visitHome();
    const entry = await enterArticle();
    flow = entry.flow;
    const snapshot = await page.evaluate(() => window.__perf.backSnapshot());
    await backOnce(flow);
    if (flow === "document") {
      await page.waitForLoadState("load", { timeout: opts.navigationTimeoutMs }).catch(() => {});
    }
    await settle(page, opts);

    let after = null;
    for (let attempt = 0; attempt < 10 && after == null; attempt++) {
      try {
        const timeOrigin = await page.evaluate(() => performance.timeOrigin);
        const resources = await collectResources(page);
        after = { timeOrigin, resources };
      } catch {
        await sleep(300);
      }
    }
    const timeOrigin = after ? after.timeOrigin : null;
    const allResources = after ? after.resources : [];
    const documentNavigation = timeOrigin == null ? true : timeOrigin !== snapshot.timeOrigin;
    const relevant = documentNavigation ? allResources : allResources.filter(entry => entry.startTime >= snapshot.now);
    const api = apiResourcesOf(relevant);
    return {
      scenario: "backCache.articleToHome",
      mode,
      ok: true,
      elapsedMs: Date.now() - startedAt,
      metrics: {
        documentNavigation,
        // backFlow 记「返回」本身是文档级重载还是 SPA 返回；entryFlow 记「进文章」是点击还是退化。
        // 两者分开：P0 时代进文章只能文档级 goto，但返回仍可能被客户端路由接管（SPA popstate）。
        backFlow: documentNavigation ? "document" : "spa",
        entryFlow: flow,
        dataRequestCount: api.length,
        dataWallMs: api.length ? apiWallMs(api) : 0,
        allRequestCount: relevant.length,
        pageErrors: pageErrors.length,
        clickToArticleMs: entry.clickToArticleMs,
      },
    };
  } finally {
    await ctx.close().catch(() => {});
  }
}

/**
 * redirect.*：历史入口是否由**服务端 301**承担（ADR-0019 决策 1 / #83 口径）。
 *
 * `/articles` 是并入 `/search` 的旧列表入口：必须在渲染前 301（把权重合并过去），
 * 不能写成普通 `ssr: false`（那会给爬虫/无 JS 客户端一个 200 空壳，属 SEO 退化）。
 * 判据看两件事：**首个响应状态码是 301**、最终落点是 `/search`。
 * 顺带把重定向链长度与墙钟记下来（重定向本身要尽可能便宜）。
 */
export async function runRedirect({ browser, base, mode, options }) {
  const opts = { ...DEFAULT_OPTIONS, ...options };
  const cold = mode === "cold";
  const startedAt = Date.now();
  const { ctx, page, pageErrors } = await openContext(browser, {
    cold,
    options: opts,
  });
  try {
    if (!cold) {
      await page.goto(new URL("/", base).href, { waitUntil: "load", timeout: opts.navigationTimeoutMs });
      await settle(page, opts);
    }

    // 只量"访问 /articles 到落在 /search"这段：预热（热模式的首页访问）不计入墙钟，
    // 否则同一场景冷/热两行的数字没有可比性（热模式会把预热整段算进去）。
    const redirectStartedAt = Date.now();
    const finalResponse = await page.goto(new URL(opts.redirectPath, base).href, {
      waitUntil: "load",
      timeout: opts.navigationTimeoutMs,
    });
    const redirectWallMs = Date.now() - redirectStartedAt;
    // Playwright 会跟随重定向：从最终请求倒着找回整条链，链头就是原始 /articles 请求。
    const chain = [];
    let request = finalResponse?.request() ?? null;
    while (request) {
      chain.unshift(request);
      request = request.redirectedFrom();
    }
    // 第一个请求对应的响应就是 301（finalResponse 是最终那次 200）。
    const firstResponse = chain.length > 0 ? await chain[0].response().catch(() => null) : null;

    return {
      scenario: "redirect.articles",
      mode,
      ok: true,
      elapsedMs: Date.now() - startedAt,
      metrics: {
        firstStatusCode: firstResponse?.status() ?? null,
        finalStatusCode: finalResponse?.status() ?? null,
        redirectCount: Math.max(0, chain.length - 1),
        finalPath: new URL(page.url()).pathname,
        wallMs: redirectWallMs,
        pageErrors: pageErrors.length,
      },
    };
  } finally {
    await ctx.close().catch(() => {});
  }
}

// 四个测量场景（每次运行返回原始记录，聚合与判定在 report.mjs）：
//   firstScreen.*  文档级直达的首屏内容可见 + 字体字节（① ③）
//   transition.*   点击站内链接后的切页内容可见（②）
//   backCache.*    浏览器返回上一页是否命中缓存（④）
import { DEFAULT_OPTIONS, openContext, settle, collectResources, fontFilesOf, apiResourcesOf, sumBytes, apiWallMs, sleep } from "./browser.mjs";

/** ① ③：文档级直达（冷 = 全新上下文；热 = 同上下文先访问一次再测）。 */
export async function runFirstScreen({ browser, base, mode, target, articleUrl, options }) {
  const opts = { ...DEFAULT_OPTIONS, ...options };
  const url = target === "home" ? new URL("/", base).href : articleUrl;
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

/** ④：文章页 →（浏览器返回）→ 首页，量是否命中缓存（文档级重载 + 数据请求数/耗时）。 */
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
  const visitArticle = async () => {
    await page.goto(articleUrl, { waitUntil: "load", timeout: opts.navigationTimeoutMs });
    await page.evaluate(
      ([selectors, timeoutMs]) => window.__perf.waitVisibleAny(selectors, timeoutMs),
      [opts.selectors.article, opts.visibleTimeoutMs],
    );
    await settle(page, opts);
  };
  const backOnce = async () => {
    const navPromise = page
      .waitForNavigation({ waitUntil: "load", timeout: opts.backNavigationWaitMs })
      .then(() => true)
      .catch(() => false);
    await page.evaluate(() => history.back()).catch(() => {});
    const navigated = await navPromise;
    if (!navigated) {
      // SPA 返回：等 URL 回到首页（或超时），再停稳。
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
    if (!cold) {
      // 热返回：先完整走一轮（不计入测量），让 HTTP 缓存与路由都热起来。
      await visitHome();
      await visitArticle();
      await backOnce();
    }
    await visitHome();
    await visitArticle();
    const snapshot = await page.evaluate(() => window.__perf.backSnapshot());
    await backOnce();
    await page.waitForLoadState("load", { timeout: opts.navigationTimeoutMs }).catch(() => {});
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
        dataRequestCount: api.length,
        dataWallMs: api.length ? apiWallMs(api) : 0,
        allRequestCount: relevant.length,
        pageErrors: pageErrors.length,
      },
    };
  } finally {
    await ctx.close().catch(() => {});
  }
}

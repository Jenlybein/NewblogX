// 浏览器测量层：上下文与缓存模式、内容可见性判定、点击/返回观察器、资源汇总。
// 只收集"浏览器里能观察到的事实"（毫秒 / 字节 / 次数），判定与汇总在 report.mjs。
//
// 口径（与 docs/notes/2026-11-01-reader-site-performance-probe.md 一致）：
// - 冷 = 全新浏览器上下文 + CDP 禁用 HTTP 缓存（相当于第一次访问）；
// - 热 = 同一上下文先访问一次再测（相当于回访）；
// - "内容可见" = 目标内容元素存在、链路上无 opacity:0/visibility:hidden/display:none、
//   在视口内、且文本非空；首屏时间从文档起点（performance.now）算起。

// 内容选择器必须指向"页面内容本身"，不能是站点标题这类全站可见元素：
// 首屏/切页都在等"目标内容出现"，把站点 h1 当内容会让 P0 崩溃也被误测成 3ms 可见。
// 页面结构变化时只改这里；fixture 自检用的也是同一组选择器。
export const DEFAULT_SELECTORS = {
  home: [".feed-entry"],
  article: [".article-page h1"],
};

export const DEFAULT_OPTIONS = {
  runs: 3,
  modes: ["cold", "warm"],
  scenarios: ["firstScreen", "transition", "backCache"],
  viewport: { width: 1440, height: 900 },
  locale: "zh-CN",
  timezoneId: "Asia/Shanghai",
  navigationTimeoutMs: 45000,
  visibleTimeoutMs: 20000,
  clickTimeoutMs: 12000,
  backNavigationWaitMs: 5000,
  settleQuietMs: 900,
  settleMaxMs: 8000,
  minContentChars: 1,
  selectors: DEFAULT_SELECTORS,
  transitionLink: 'a[href^="/article/"]',
  onRun: null,
};

const INIT_TEMPLATE = `(() => {
  if (window.__perf) return;
  const CONFIG = __CONFIG__;
  const state = {
    marks: { contentVisibleMs: null, selector: null, contentTimedOut: false },
    errors: [],
  };
  window.__perf = state;

  function recordError(message) {
    state.errors.push({ at: performance.now(), msg: String(message == null ? "error" : message).slice(0, 300) });
  }
  window.addEventListener("error", function (event) { recordError(event.message); }, true);
  window.addEventListener("unhandledrejection", function (event) {
    const reason = event.reason;
    recordError(reason && reason.message ? reason.message : reason);
  }, true);

  function isVisible(element) {
    if (!element) return false;
    let node = element;
    while (node && node.nodeType === 1) {
      const style = getComputedStyle(node);
      if (style.opacity === "0" || style.visibility === "hidden" || style.display === "none") return false;
      node = node.parentElement;
    }
    const rect = element.getBoundingClientRect();
    if (rect.width < 2 || rect.height < 2) return false;
    return rect.bottom > 0 && rect.top < (window.innerHeight || 0) + 400;
  }

  function hasEnoughText(element) {
    const text = String(element.innerText || element.textContent || "").trim();
    return text.length >= CONFIG.minContentChars;
  }

  function findVisible(selectors) {
    for (let i = 0; i < selectors.length; i++) {
      const element = document.querySelector(selectors[i]);
      if (element && isVisible(element) && hasEnoughText(element)) return selectors[i];
    }
    return null;
  }

  const watchStarted = performance.now();
  function watchContent() {
    if (state.marks.contentVisibleMs != null || state.marks.contentTimedOut) return;
    const matched = findVisible(CONFIG.contentSelectors);
    if (matched) {
      state.marks.contentVisibleMs = Math.round(performance.now());
      state.marks.selector = matched;
      return;
    }
    if (performance.now() - watchStarted > CONFIG.visibleTimeoutMs) {
      state.marks.contentTimedOut = true;
      return;
    }
    requestAnimationFrame(watchContent);
  }
  requestAnimationFrame(watchContent);

  state.waitContent = function (timeoutMs) {
    return new Promise(function (resolve) {
      const began = performance.now();
      function tick() {
        if (state.marks.contentVisibleMs != null || state.marks.contentTimedOut) {
          resolve({ contentVisibleMs: state.marks.contentVisibleMs, selector: state.marks.selector, timedOut: state.marks.contentTimedOut });
          return;
        }
        if (performance.now() - began > timeoutMs) {
          resolve({ contentVisibleMs: null, selector: null, timedOut: true });
          return;
        }
        requestAnimationFrame(tick);
      }
      tick();
    });
  };

  state.waitVisibleAny = function (selectors, timeoutMs) {
    return new Promise(function (resolve) {
      const began = performance.now();
      function tick() {
        const matched = findVisible(selectors);
        if (matched) {
          resolve({ visibleMs: Math.round(performance.now() - began), selector: matched });
          return;
        }
        if (performance.now() - began > timeoutMs) {
          resolve({ visibleMs: null, selector: null });
          return;
        }
        requestAnimationFrame(tick);
      }
      tick();
    });
  };

  state.clickWatch = function (linkSelector, contentSelectors, timeoutMs) {
    return new Promise(function (resolve) {
      const link = document.querySelector(linkSelector);
      if (!link) { resolve({ missing: true }); return; }
      // 点击前已经存在的元素（站点标题、当前页内容）不能算"切页后的目标内容"：
      // 记录点击前的节点引用，只有节点被换掉或新出现才计入。
      const before = {};
      for (let index = 0; index < contentSelectors.length; index++) {
        before[contentSelectors[index]] = document.querySelector(contentSelectors[index]);
      }
      const startUrl = location.href;
      const t0 = performance.now();
      let urlChangedMs = null;
      let visibleMs = null;
      let matchedSelector = null;
      function findChangedVisible() {
        for (let index = 0; index < contentSelectors.length; index++) {
          const selector = contentSelectors[index];
          const element = document.querySelector(selector);
          if (!element) continue;
          if (before[selector] === element) continue;
          if (isVisible(element) && hasEnoughText(element)) return selector;
        }
        return null;
      }
      link.click();
      function tick() {
        if (urlChangedMs == null && location.href !== startUrl) urlChangedMs = Math.round(performance.now() - t0);
        if (visibleMs == null) {
          const matched = findChangedVisible();
          if (matched) { visibleMs = Math.round(performance.now() - t0); matchedSelector = matched; }
        }
        const firstError = state.errors.length > 0 ? state.errors[0] : null;
        if (visibleMs != null || performance.now() - t0 > timeoutMs) {
          resolve({
            missing: false,
            contentVisibleMs: visibleMs,
            clickToUrlMs: urlChangedMs,
            contentSelector: matchedSelector,
            errorAtMs: firstError ? Math.round(firstError.at - t0) : null,
            errorMsg: firstError ? firstError.msg : null,
            finalUrl: location.href,
            timedOut: visibleMs == null,
          });
          return;
        }
        requestAnimationFrame(tick);
      }
      requestAnimationFrame(tick);
    });
  };

  state.backSnapshot = function () {
    return {
      timeOrigin: performance.timeOrigin,
      now: performance.now(),
      resourceCount: performance.getEntriesByType("resource").length,
      href: location.href,
    };
  };
})();`;

export function buildInitScript({ selectors, minContentChars, visibleTimeoutMs }) {
  const config = { contentSelectors: selectors, minContentChars, visibleTimeoutMs };
  return INIT_TEMPLATE.replace("__CONFIG__", JSON.stringify(config));
}

export async function openContext(browser, { cold, options, contentSelectors }) {
  const ctx = await browser.newContext({
    viewport: options.viewport,
    locale: options.locale,
    timezoneId: options.timezoneId,
  });
  const page = await ctx.newPage();
  const pageErrors = [];
  page.on("pageerror", error => pageErrors.push(String((error && error.message) || error).slice(0, 300)));
  if (cold) {
    const cdp = await ctx.newCDPSession(page);
    await cdp.send("Network.enable");
    await cdp.send("Network.setCacheDisabled", { cacheDisabled: true });
  }
  await page.addInitScript(
    buildInitScript({
      selectors: contentSelectors,
      minContentChars: options.minContentChars,
      visibleTimeoutMs: options.visibleTimeoutMs,
    }),
  );
  return { ctx, page, pageErrors };
}

export async function settle(page, options) {
  const quietMs = options.settleQuietMs;
  const maxMs = options.settleMaxMs;
  const started = Date.now();
  let lastCount = -1;
  let lastChange = Date.now();
  while (Date.now() - lastChange < quietMs && Date.now() - started < maxMs) {
    await sleep(150);
    const count = await page
      .evaluate(() => performance.getEntriesByType("resource").length)
      .catch(() => lastCount);
    if (count !== lastCount) {
      lastCount = count;
      lastChange = Date.now();
    }
  }
}

export async function collectResources(page) {
  const raw = await page.evaluate(() =>
    performance.getEntriesByType("resource").map(entry => ({
      name: entry.name,
      initiatorType: entry.initiatorType,
      startTime: entry.startTime,
      duration: entry.duration,
      responseStart: entry.responseStart,
      responseEnd: entry.responseEnd,
      encodedBodySize: entry.encodedBodySize,
      transferSize: entry.transferSize,
      protocol: entry.nextHopProtocol,
    })),
  );
  return raw.map(entry => ({
    ...entry,
    path: pathOf(entry.name),
    file: fileOf(entry.name),
    isApi: isApiPath(entry.name),
    isFont: isFontPath(entry.name),
  }));
}

export async function navigationTiming(page) {
  return page.evaluate(() => {
    const entry = performance.getEntriesByType("navigation")[0];
    if (!entry) return null;
    return {
      type: entry.type,
      ttfb: Math.round(entry.responseStart),
      domContentLoaded: Math.round(entry.domContentLoadedEventEnd),
      load: Math.round(entry.loadEventEnd),
      protocol: entry.nextHopProtocol,
      docEncodedBodySize: entry.encodedBodySize,
    };
  });
}

export function fontFilesOf(resources) {
  const byPath = new Map();
  for (const resource of resources.filter(entry => entry.isFont)) {
    const bytes = resource.encodedBodySize || 0;
    const existing = byPath.get(resource.path);
    // 同一字体 URL 可能被 preload 与 CSS 各记一条；按 URL 去重取最大值，避免重复计数。
    if (!existing || existing.bytes < bytes) byPath.set(resource.path, { file: resource.file, bytes });
  }
  return [...byPath.values()].sort((a, b) => b.bytes - a.bytes || a.file.localeCompare(b.file));
}

export function apiResourcesOf(resources) {
  return resources.filter(entry => entry.isApi);
}

export function sumBytes(entries) {
  return entries.reduce((sum, entry) => sum + (entry.bytes != null ? entry.bytes : entry.encodedBodySize || 0), 0);
}

export function apiWallMs(apiResources) {
  if (apiResources.length === 0) return 0;
  const starts = apiResources.map(entry => entry.startTime);
  const ends = apiResources.map(entry => (entry.responseEnd || entry.startTime + entry.duration));
  return Math.round(Math.max(...ends) - Math.min(...starts));
}

export function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function pathOf(url) {
  try {
    return new URL(url).pathname;
  } catch {
    return url;
  }
}

function fileOf(url) {
  const path = pathOf(url);
  return path.split("/").pop() || path;
}

function isApiPath(url) {
  return /\/api\//.test(pathOf(url));
}

function isFontPath(url) {
  return /\.woff2?(\?|$)/i.test(pathOf(url));
}

// 编排层：发现文章页 / 个人主页 URL + 按「模式 × 场景 × 次数」跑完整套测量。
import { DEFAULT_OPTIONS, openContext } from "./browser.mjs";
import { runFirstScreen, runTransition, runBackCache, runRedirect } from "./scenarios.mjs";

export const DEFAULT_RUN_OPTIONS = DEFAULT_OPTIONS;
const FIRST_SCREEN_TARGETS = ["home", "article", "userProfile"];

/** 从首页找第一条文章链接（不写死文章 id，内容换了也能跑）。 */
export async function discoverArticleUrl({ browser, base, options = {} }) {
  const opts = { ...DEFAULT_OPTIONS, ...options };
  const { ctx, page } = await openContext(browser, {
    cold: false,
    options: opts,
    contentSelectors: opts.selectors.home,
  });
  try {
    await page.goto(new URL("/", base).href, { waitUntil: "load", timeout: opts.navigationTimeoutMs });
    const href = await page.evaluate(() => {
      const link = document.querySelector('a[href^="/article/"]');
      return link ? link.getAttribute("href") : null;
    });
    return href ? new URL(href, base).href : null;
  } finally {
    await ctx.close().catch(() => {});
  }
}

/**
 * 发现一个**匿名可访问**的个人主页 URL（`/users/<id>`）。
 *
 * 优先从首页找作者链接（侧栏/列表都会链到作者主页）；首页没有就退到文章页找。
 * 找不到就返回 null —— 调用方会跳过 `firstScreen.userProfile`，不会把缺失当失败。
 */
export async function discoverUserUrl({ browser, base, articleUrl, options = {} }) {
  const opts = { ...DEFAULT_OPTIONS, ...options };
  const { ctx, page } = await openContext(browser, {
    cold: false,
    options: opts,
    contentSelectors: opts.selectors.home,
  });
  const findOnPage = async (url) => {
    await page.goto(url, { waitUntil: "load", timeout: opts.navigationTimeoutMs });
    await page.waitForTimeout(1200);
    return page.evaluate(linkSelector => {
      const link = document.querySelector(linkSelector);
      return link ? link.getAttribute("href") : null;
    }, opts.userLink);
  };
  try {
    const fromHome = await findOnPage(new URL("/", base).href);
    if (fromHome) return new URL(fromHome, base).href;
    if (articleUrl) {
      const fromArticle = await findOnPage(articleUrl);
      if (fromArticle) return new URL(fromArticle, base).href;
    }
    return null;
  } finally {
    await ctx.close().catch(() => {});
  }
}

/** 跑完整套：返回逐次原始记录（排序固定为 模式 → 场景 → 次数）。 */
export async function runSuite({ browser, base, articleUrl, userUrl, options = {} }) {
  const opts = { ...DEFAULT_OPTIONS, ...options };
  const runs = [];

  const record = async (descriptor, measure) => {
    let run;
    try {
      run = await measure();
    } catch (error) {
      run = {
        scenario: descriptor.scenario,
        mode: descriptor.mode,
        run: descriptor.run,
        ok: false,
        error: String((error && error.message) || error),
      };
    }
    run.run = descriptor.run;
    runs.push(run);
    if (typeof opts.onRun === "function") opts.onRun(run);
  };

  for (const mode of opts.modes) {
    if (opts.scenarios.includes("firstScreen")) {
      for (const target of FIRST_SCREEN_TARGETS) {
        if (target === "article" && !articleUrl) throw new Error("文章页 URL 未发现：请用 --article 显式指定");
        // 个人主页是可选补充场景：没发现匿名可访问的 /users/<id> 就跳过（fixture 也没有）。
        if (target === "userProfile" && !userUrl) continue;
        for (let index = 1; index <= opts.runs; index++) {
          await record({ scenario: `firstScreen.${target}`, mode, run: index }, () =>
            runFirstScreen({ browser, base, mode, target, articleUrl, userUrl, options: opts }),
          );
        }
      }
    }
    if (opts.scenarios.includes("transition")) {
      if (!articleUrl) throw new Error("文章页 URL 未发现：请用 --article 显式指定");
      for (let index = 1; index <= opts.runs; index++) {
        await record({ scenario: "transition.homeToArticle", mode, run: index }, () =>
          runTransition({ browser, base, mode, articleUrl, options: opts }),
        );
      }
    }
    if (opts.scenarios.includes("backCache")) {
      if (!articleUrl) throw new Error("文章页 URL 未发现：请用 --article 显式指定");
      for (let index = 1; index <= opts.runs; index++) {
        await record({ scenario: "backCache.articleToHome", mode, run: index }, () =>
          runBackCache({ browser, base, mode, articleUrl, options: opts }),
        );
      }
    }
    if (opts.scenarios.includes("redirect")) {
      for (let index = 1; index <= opts.runs; index++) {
        await record({ scenario: "redirect.articles", mode, run: index }, () =>
          runRedirect({ browser, base, mode, options: opts }),
        );
      }
    }
  }
  return runs;
}

// 编排层：发现文章页 URL + 按「模式 × 场景 × 次数」跑完整套测量。
import { DEFAULT_OPTIONS, openContext } from "./browser.mjs";
import { runFirstScreen, runTransition, runBackCache } from "./scenarios.mjs";

export const DEFAULT_RUN_OPTIONS = DEFAULT_OPTIONS;
const FIRST_SCREEN_TARGETS = ["home", "article"];

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

/** 跑完整套：返回逐次原始记录（排序固定为 模式 → 场景 → 次数）。 */
export async function runSuite({ browser, base, articleUrl, options = {} }) {
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
        for (let index = 1; index <= opts.runs; index++) {
          await record({ scenario: `firstScreen.${target}`, mode, run: index }, () =>
            runFirstScreen({ browser, base, mode, target, articleUrl, options: opts }),
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
  }
  return runs;
}

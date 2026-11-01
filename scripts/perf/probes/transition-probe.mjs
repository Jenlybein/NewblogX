// 收尾用探针（#87）：默认不进 CI，只用于量化"路由过渡固定开销"。
// 拆解「首页点击 → 文章内容可见」的时间构成：点击→URL / 点击→骨架可见 / 点击→正文 h1 可见，
// 以及把路由过渡关掉后同一组数字，用来量化"路由过渡固定开销"到底占多少。
import { chromium } from "playwright";

const base = process.argv[2] || "http://127.0.0.1:3000";
const runs = Number(process.argv[3] || 3);
const onlyWarm = process.argv[4] === "warm";

const browser = await chromium.launch();

function median(values) {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.floor(sorted.length / 2)];
}

async function measure({ cold, disableTransition }) {
  const ctx = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    locale: "zh-CN",
    timezoneId: "Asia/Shanghai",
  });
  const page = await ctx.newPage();
  try {
    if (cold) {
      const cdp = await ctx.newCDPSession(page);
      await cdp.send("Network.enable");
      await cdp.send("Network.setCacheDisabled", { cacheDisabled: true });
    } else {
      await page.goto(base, { waitUntil: "load", timeout: 30000 });
      await page.waitForTimeout(1500);
    }
    await page.goto(base, { waitUntil: "load", timeout: 30000 });
    await page.waitForFunction(() => document.querySelector(".feed-entry") !== null, null, { timeout: 30000 });
    await page.waitForTimeout(1200);
    if (disableTransition) {
      await page.addStyleTag({
        content:
          ".page-enter-active,.page-leave-active{transition-duration:0ms !important;transition-delay:0ms !important;}",
      });
    }
    return await page.evaluate(() => {
      return new Promise((resolve) => {
        const link = document.querySelector('a[href^="/article/"]');
        const beforeH1 = document.querySelector(".article-page h1");
        const t0 = performance.now();
        const marks = { clickToUrl: null, skeleton: null, h1: null };
        function visible(element) {
          if (!element) return false;
          let node = element;
          while (node && node.nodeType === 1) {
            const style = getComputedStyle(node);
            if (style.opacity === "0" || style.visibility === "hidden" || style.display === "none") return false;
            node = node.parentElement;
          }
          const rect = element.getBoundingClientRect();
          return rect.width >= 2 && rect.height >= 2 && rect.bottom > 0 && rect.top < innerHeight + 400;
        }
        link.click();
        function tick() {
          const elapsed = performance.now() - t0;
          if (marks.clickToUrl == null && location.pathname.startsWith("/article/")) {
            marks.clickToUrl = Math.round(elapsed);
          }
          if (marks.skeleton == null) {
            const skeleton = document.querySelector('[data-role="article-page-skeleton"]');
            if (visible(skeleton)) marks.skeleton = Math.round(elapsed);
          }
          if (marks.h1 == null) {
            const h1 = document.querySelector(".article-page h1");
            if (h1 && h1 !== beforeH1 && visible(h1)) marks.h1 = Math.round(elapsed);
          }
          if (marks.h1 != null || elapsed > 8000) {
            resolve(marks);
            return;
          }
          requestAnimationFrame(tick);
        }
        requestAnimationFrame(tick);
      });
    });
  } finally {
    await ctx.close().catch(() => {});
  }
}

for (const cold of onlyWarm ? [false] : [true, false]) {
  for (const disableTransition of [false, true]) {
    const rows = [];
    for (let index = 0; index < runs; index++) {
      rows.push(await measure({ cold, disableTransition }));
    }
    const field = (name) => median(rows.map((row) => row[name]).filter((value) => value != null));
    console.log(
      [
        cold ? "冷" : "热",
        disableTransition ? "过渡关闭" : "当前构建过渡",
        `URL=${field("clickToUrl")}ms`,
        `骨架=${field("skeleton")}ms`,
        `h1=${field("h1")}ms`,
        `runs=${JSON.stringify(rows)}`,
      ].join(" | "),
    );
  }
}
await browser.close();

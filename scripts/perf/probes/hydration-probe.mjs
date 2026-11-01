// 收尾用探针（#87）：默认不进 CI，只用于观察 hydration mismatch 是否仍存在。
// 观察首页/文章页的 hydration 是否仍报 "Hydration completed but contains mismatches"
// （spec #79 的 Out of Scope 里登记的独立观察项）。同时记录未捕获异常与关键 console 警告。
import { chromium } from "playwright";

const base = process.argv[2] || "http://127.0.0.1:3000";

const browser = await chromium.launch();
const context = await browser.newContext({
  viewport: { width: 1440, height: 900 },
  locale: "zh-CN",
  timezoneId: "Asia/Shanghai",
});
const page = await context.newPage();
const messages = [];
page.on("console", (message) => messages.push({ type: message.type(), text: message.text() }));
page.on("pageerror", (error) => messages.push({ type: "pageerror", text: String(error && error.message) }));

async function visit(path) {
  messages.length = 0;
  await page.goto(new URL(path, base).href, { waitUntil: "load", timeout: 30000 });
  await page.waitForTimeout(1500);
  const interesting = messages.filter(
    (message) =>
      /hydrat|mismatch|hydration/iu.test(message.text) ||
      message.type === "pageerror" ||
      message.type === "error",
  );
  console.log(`${path}: ${interesting.length === 0 ? "无 hydration mismatch / 无页面错误" : JSON.stringify(interesting)}`);
}

await visit("/");
const articleUrl = await page.evaluate(() => {
  const link = document.querySelector('a[href^="/article/"]');
  return link ? link.getAttribute("href") : null;
});
if (articleUrl) {
  await visit(articleUrl);
} else {
  console.log("首页未发现文章链接，跳过文章页");
}
await context.close();
await browser.close();

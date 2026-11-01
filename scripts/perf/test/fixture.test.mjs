// fixture 自检：不依赖真实站点，用本地静态页验证"尺子"本身能测出四类指标，
// 并按契约产出可 diff 的 JSON / Markdown 两份报告。
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, readFile, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { startFixtureSite } from "../lib/fixture-site.mjs";
import { runSuite, discoverArticleUrl, DEFAULT_RUN_OPTIONS } from "../lib/runner.mjs";
import { buildReport, renderJson, renderMarkdown } from "../lib/report.mjs";

let chromium = null;
try {
  ({ chromium } = await import("playwright"));
} catch {
  chromium = null;
}

const FONT_FILE = new URL("../fixture/fonts/fixture.woff2", import.meta.url);
const SKIP = chromium ? false : "未安装 playwright：先在 scripts/perf 下执行 pnpm install";

test("fixture 自检：本地静态页能测出首屏/切页/字体字节/返回缓存", { skip: SKIP }, async () => {
  const fontBytes = (await stat(FONT_FILE)).size;
  const site = await startFixtureSite();
  const browser = await chromium.launch();
  try {
    const articleUrl = await discoverArticleUrl({ browser, base: site.base, options: DEFAULT_RUN_OPTIONS });
    assert.ok(
      articleUrl && articleUrl.endsWith("/article/fixture-1"),
      `应能从首页发现文章链接，实际：${articleUrl}`,
    );

    const options = { ...DEFAULT_RUN_OPTIONS, runs: 2 };
    const runs = await runSuite({ browser, base: site.base, articleUrl, options });
    // 2 个模式 × （首屏 2 目标（fixture 没有 /users 链接，个人主页跳过）+ 切页 1 路径 +
    // 返回 1 路径 + 重定向 1 路径）× 2 次 = 20
    assert.equal(runs.length, 20);

    const report = buildReport({
      meta: {
        base: site.base,
        label: "fixture-selfcheck",
        runs: options.runs,
        viewport: `${options.viewport.width}x${options.viewport.height}`,
        chromium: browser.version(),
        generatedAt: new Date().toISOString(),
      },
      runs,
    });

    // ① 首屏：fixture 首屏内容确实可见，且时间是从文档起点算出的正数
    const homeCold = report.scenarios["firstScreen.home"].modes.cold;
    assert.equal(homeCold.aggregates.contentVisibleMs.measured, 2);
    assert.equal(homeCold.runs[0].contentSelector, ".feed-entry");
    assert.ok(homeCold.aggregates.contentVisibleMs.median > 0);
    assert.ok(homeCold.aggregates.contentVisibleMs.median < 5000, "fixture 首屏应在 5 秒内可见");

    // ③ 字体：首屏字体下载字节应等于 fixture 字体文件大小
    assert.equal(report.fonts.home.cold.totalBytes.median, fontBytes);
    assert.ok(report.fonts.home.cold.files.some(file => file.file === "fixture.woff2"));

    // ② 切页：fixture 的站内跳转是可用的，因此必须测出「可见」而不是不可测
    const transitionCold = report.scenarios["transition.homeToArticle"].modes.cold;
    assert.equal(transitionCold.outcome, "visible");
    assert.ok(transitionCold.aggregates.contentVisibleMs.median > 0);
    assert.ok(transitionCold.aggregates.contentVisibleMs.median < 5000, "fixture 切页应在 5 秒内可见");
    assert.equal(transitionCold.runs[0].contentVisibleMs !== null, true);

    // ④ 返回：数据请求数必须与「是否文档级重载」自洽（重载 = 新文档会再发一次 /api/fixture）
    for (const mode of ["cold", "warm"]) {
      const back = report.scenarios["backCache.articleToHome"].modes[mode];
      assert.equal(back.runs.length, 2);
      for (const run of back.runs) {
        assert.equal(
          run.dataRequestCount,
          run.documentNavigation ? 1 : 0,
          `返回场景数据请求数应与文档级重载一致：${JSON.stringify(run)}`,
        );
      }
    }

    // 报告落盘：JSON 可解析、Markdown 六节齐全、Markdown 不含时间戳因而两次渲染逐字节一致
    const dir = await mkdtemp(path.join(tmpdir(), "blogx-perf-selfcheck-"));
    await writeFile(path.join(dir, "report.json"), renderJson(report));
    await writeFile(path.join(dir, "report.md"), renderMarkdown(report));

    const parsed = JSON.parse(await readFile(path.join(dir, "report.json"), "utf8"));
    assert.equal(parsed.schemaVersion, 1);
    assert.equal(parsed.tool, "blogx-perf");

    const md = await readFile(path.join(dir, "report.md"), "utf8");
    for (const heading of [
      "## ① 首屏内容可见（文档级直达）",
      "## ② 切页内容可见（点击站内链接）",
      "## ③ 首屏字体下载字节",
      "## ④ 返回上一页是否命中缓存",
      "## ⑤ 个人主页首屏（SPA，补充参考）",
      "## ⑥ 历史入口 `/articles`：服务端 301 → `/search`",
    ]) {
      assert.ok(md.includes(heading), `Markdown 缺少小节：${heading}`);
    }
    // fixture 没有 /users 链接 → 个人主页场景被跳过（⑤ 写"未运行"而不是伪造数据）
    assert.equal(report.scenarios["firstScreen.userProfile"], undefined);
    // fixture 不是真实站点，/articles 是 404 而不是 301：重定向判定必须是 fail，不能误判成 pass
    assert.equal(report.scenarios["redirect.articles"].modes.cold.verdicts.redirect, "fail");
    const again = renderMarkdown(
      buildReport({ meta: { ...report.meta, generatedAt: "another-run" }, runs }),
    );
    assert.equal(md, again, "Markdown 不应随时间戳变化");
  } finally {
    await browser.close();
    await site.close();
  }
});

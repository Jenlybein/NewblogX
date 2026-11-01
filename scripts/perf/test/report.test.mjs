// 报告格式的单元测试：聚合结果、判定、JSON/Markdown 渲染、以及"两次运行可直接 diff"。
// 所有期望值来自设计口径（ADR-0019 预算 + 实测笔记的测法），手算写死；不调用被测代码反推。
import { test } from "node:test";
import assert from "node:assert/strict";
import { buildReport, renderMarkdown, renderJson, BUDGETS } from "../lib/report.mjs";

const META = {
  base: "https://example.test",
  label: "fixture-selfcheck",
  runs: 3,
  viewport: "1440x900",
  chromium: "fixture",
};

const FONT_FILES_HOME = [
  { file: "noto-sans-sc-400.woff2", bytes: 1142552 },
  { file: "noto-sans-sc-700.woff2", bytes: 1172244 },
  { file: "sora-latin-400.woff2", bytes: 12345 },
];

function homeRun(run, contentVisibleMs, fontBytes) {
  return {
    scenario: "firstScreen.home",
    mode: "cold",
    run,
    ok: true,
    metrics: {
      contentVisibleMs,
      contentSelector: ".feed-entry",
      requestCount: 66,
      totalBytes: 2900000,
      fontBytes,
      fontFiles: FONT_FILES_HOME,
    },
  };
}

const RAW = [
  // ① 首屏 · 首页（冷）：1200 / 1644 / 2000 -> 中位 1644，最差 2000
  homeRun(1, 1200, 2407904),
  homeRun(2, 1644, 2407904),
  homeRun(3, 2000, 2410000),
  // ① 首屏 · 首页（热）：297 / 299 / 301 -> 中位 299，最差 301；字体已被缓存
  { scenario: "firstScreen.home", mode: "warm", run: 1, ok: true, metrics: { contentVisibleMs: 297, contentSelector: ".feed-entry", requestCount: 66, totalBytes: 2900000, fontBytes: 0, fontFiles: [] } },
  { scenario: "firstScreen.home", mode: "warm", run: 2, ok: true, metrics: { contentVisibleMs: 301, contentSelector: ".feed-entry", requestCount: 66, totalBytes: 2900000, fontBytes: 0, fontFiles: [] } },
  { scenario: "firstScreen.home", mode: "warm", run: 3, ok: true, metrics: { contentVisibleMs: 299, contentSelector: ".feed-entry", requestCount: 66, totalBytes: 2900000, fontBytes: 0, fontFiles: [] } },
  // ① 首屏 · 文章页（冷）：1572 / 2211 / 1900 -> 中位 1900，最差 2211
  { scenario: "firstScreen.article", mode: "cold", run: 1, ok: true, metrics: { contentVisibleMs: 1572, contentSelector: ".article-page h1", requestCount: 68, totalBytes: 1800000, fontBytes: 180000, fontFiles: [{ file: "inter-latin-400.woff2", bytes: 100000 }, { file: "sora-latin-400.woff2", bytes: 80000 }] } },
  { scenario: "firstScreen.article", mode: "cold", run: 2, ok: true, metrics: { contentVisibleMs: 2211, contentSelector: ".article-page h1", requestCount: 68, totalBytes: 1800000, fontBytes: 180000, fontFiles: [{ file: "inter-latin-400.woff2", bytes: 100000 }, { file: "sora-latin-400.woff2", bytes: 80000 }] } },
  { scenario: "firstScreen.article", mode: "cold", run: 3, ok: true, metrics: { contentVisibleMs: 1900, contentSelector: ".article-page h1", requestCount: 68, totalBytes: 1800000, fontBytes: 180000, fontFiles: [{ file: "inter-latin-400.woff2", bytes: 100000 }, { file: "sora-latin-400.woff2", bytes: 80000 }] } },
  { scenario: "firstScreen.article", mode: "warm", run: 1, ok: true, metrics: { contentVisibleMs: 200, contentSelector: ".article-page h1", requestCount: 68, totalBytes: 1800000, fontBytes: 0, fontFiles: [] } },
  { scenario: "firstScreen.article", mode: "warm", run: 2, ok: true, metrics: { contentVisibleMs: 210, contentSelector: ".article-page h1", requestCount: 68, totalBytes: 1800000, fontBytes: 0, fontFiles: [] } },
  { scenario: "firstScreen.article", mode: "warm", run: 3, ok: true, metrics: { contentVisibleMs: 220, contentSelector: ".article-page h1", requestCount: 68, totalBytes: 1800000, fontBytes: 0, fontFiles: [] } },
  // ② 切页 · 首页点文章（冷/热）：P0 未修，内容从不出现 -> 不可测
  ...["cold", "warm"].flatMap(mode => [1, 2, 3].map(run => ({
    scenario: "transition.homeToArticle",
    mode,
    run,
    ok: true,
    metrics: {
      contentVisibleMs: null,
      clickToUrlMs: 24,
      clickToErrorMs: 225,
      errorMsg: "TypeError: n.off is not a function",
      timeoutMs: 12000,
      pageErrors: 1,
    },
  }))),
  // ④ 返回 · 文章页返回首页：P0 修复前站内点击进不去文章，退化为文档级往返，数据请求重新发起 -> 未命中
  ...["cold", "warm"].flatMap(mode => [
    { scenario: "backCache.articleToHome", mode, run: 1, ok: true, metrics: { documentNavigation: true, dataRequestCount: 3, dataWallMs: 271, allRequestCount: 66, backFlow: "document", entryFlow: "document", clickToArticleMs: null } },
    { scenario: "backCache.articleToHome", mode, run: 2, ok: true, metrics: { documentNavigation: true, dataRequestCount: 3, dataWallMs: 271, allRequestCount: 66, backFlow: "document", entryFlow: "document", clickToArticleMs: null } },
    { scenario: "backCache.articleToHome", mode, run: 3, ok: true, metrics: { documentNavigation: true, dataRequestCount: 3, dataWallMs: 271, allRequestCount: 66, backFlow: "document", entryFlow: "document", clickToArticleMs: null } },
  ]),
];

const report = buildReport({
  meta: { ...META, generatedAt: "2026-11-01T12:00:00.000Z" },
  runs: RAW,
});

test("报告骨架：schemaVersion / tool / meta / budgets 都在", () => {
  assert.equal(report.schemaVersion, 1);
  assert.equal(report.tool, "blogx-perf");
  assert.equal(report.meta.base, "https://example.test");
  assert.equal(report.meta.runs, 3);
  assert.deepEqual(report.budgets, BUDGETS);
  assert.equal(report.scenarios["firstScreen.home"].modes.cold.runs.length, 3);
  assert.equal(report.scenarios["firstScreen.home"].modes.warm.runs.length, 3);
});

test("聚合：中位/最差/不可测次数按 3 次运行如实计算", () => {
  const cold = report.scenarios["firstScreen.home"].modes.cold;
  assert.deepEqual(cold.aggregates.contentVisibleMs, { measured: 3, unmeasured: 0, median: 1644, worst: 2000 });
  assert.deepEqual(cold.aggregates.fontBytes, { measured: 3, unmeasured: 0, median: 2407904, worst: 2410000 });

  const articleCold = report.scenarios["firstScreen.article"].modes.cold;
  assert.deepEqual(articleCold.aggregates.contentVisibleMs, { measured: 3, unmeasured: 0, median: 1900, worst: 2211 });
});

test("判定：按 ADR-0019 预算给 pass / fail / unmeasurable", () => {
  assert.equal(report.scenarios["firstScreen.home"].modes.cold.verdicts.contentVisibleMs, "fail");   // 1644 > 800
  assert.equal(report.scenarios["firstScreen.home"].modes.warm.verdicts.contentVisibleMs, "pass");   // 299 < 800
  assert.equal(report.fonts.home.cold.verdicts.totalBytes, "fail");                                  // 2.4 MB > 100 KB
  assert.equal(report.fonts.home.warm.verdicts.totalBytes, "pass");                                  // 0 B
  assert.equal(report.scenarios["backCache.articleToHome"].modes.cold.verdicts.cacheHit, "fail");
});

test("切页不可测：内容从不出现时记为 outcome=never、中位为 null，而不是 0", () => {
  const cold = report.scenarios["transition.homeToArticle"].modes.cold;
  assert.equal(cold.outcome, "never");
  assert.equal(cold.aggregates.contentVisibleMs.measured, 0);
  assert.equal(cold.aggregates.contentVisibleMs.median, null);
  assert.equal(cold.aggregates.contentVisibleMs.worst, null);
  assert.equal(cold.aggregates.contentVisibleMs.unmeasured, 3);
  assert.equal(cold.verdicts.contentVisibleMs, "unmeasurable");
});

test("字体：从各次运行的字体文件明细聚合出总字节、中文字体字节与文件表", () => {
  const cold = report.fonts.home.cold;
  assert.equal(cold.totalBytes.median, 2407904);
  assert.equal(cold.notoBytes.median, 2314796); // 1172244 + 1142552
  assert.deepEqual(cold.files, FONT_FILES_HOME.slice().sort((a, b) => b.bytes - a.bytes));
  assert.equal(report.fonts.article.cold.totalBytes.median, 180000);
});

test("返回：命中缓存按「无数据请求或数据耗时 < 50 ms」判定", () => {
  const cold = report.scenarios["backCache.articleToHome"].modes.cold;
  assert.equal(cold.aggregates.dataRequestCount.median, 3);
  assert.equal(cold.aggregates.dataWallMs.median, 271);
  assert.equal(cold.documentNavigation, "yes");
  assert.equal(cold.backFlow, "document");
  assert.equal(cold.entryFlow, "document");
  assert.equal(cold.cacheHitRuns, 0);
});

test("返回：站内点击后 SPA 返回命中 30s 客户端缓存（#87 口径修订）", () => {
  const spaRuns = ["cold", "warm"].flatMap(mode =>
    [1, 2, 3].map(run => ({
      scenario: "backCache.articleToHome",
      mode,
      run,
      ok: true,
      metrics: {
        documentNavigation: false,
        dataRequestCount: 0,
        dataWallMs: 0,
        allRequestCount: 2,
        pageErrors: 0,
        backFlow: "spa",
        entryFlow: "spa",
        clickToArticleMs: 120,
      },
    })),
  );
  const spaReport = buildReport({ meta: { ...META, generatedAt: "2026-11-01T12:00:00.000Z" }, runs: spaRuns });
  const cold = spaReport.scenarios["backCache.articleToHome"].modes.cold;

  assert.equal(cold.backFlow, "spa");
  assert.equal(cold.verdicts.cacheHit, "pass");
  assert.ok(
    renderMarkdown(spaReport).includes(
      "| 首页 → 文章 → 返回首页 | 冷 | SPA 返回 | 否 | 0 | 0 ms | 命中 |\n",
    ),
  );
  // 不能把「测不到」当成「没命中」：SPA 口径下不应出现文档级重载的备注。
  assert.equal(renderMarkdown(spaReport).includes("表现为文档级重载"), false);
});

test("json 报告只保留标量指标（不把每次运行的字体明细塞进 runs，避免 diff 噪音）", () => {
  const run = report.scenarios["firstScreen.home"].modes.cold.runs[0];
  assert.equal(run.contentVisibleMs, 1200);
  assert.equal(run.fontBytes, 2407904);
  assert.equal("fontFiles" in run, false);
});

test("markdown 报告：四节标题与表格逐行可预期", () => {
  const md = renderMarkdown(report);
  assert.ok(md.startsWith("# 读者站性能基线报告\n"));
  assert.ok(md.includes("- 站点：https://example.test\n"));
  assert.ok(md.includes("- 标签：fixture-selfcheck\n"));
  assert.ok(md.includes("- 每模式次数：3（取中位与最差）\n"));
  assert.ok(md.includes("## ① 首屏内容可见（文档级直达）\n"));
  assert.ok(md.includes("| 首页 `/` | 冷 | 1644 ms | 2000 ms | 不达标 | 66 | 2900000 B |\n"));
  assert.ok(md.includes("| 首页 `/` | 热 | 299 ms | 301 ms | 达标 | 66 | 2900000 B |\n"));
  assert.ok(md.includes("| 文章页 | 冷 | 1900 ms | 2211 ms | 不达标 | 68 | 1800000 B |\n"));
  assert.ok(md.includes("## ② 切页内容可见（点击站内链接）\n"));
  assert.ok(md.includes("| 首页 → 文章 | 冷 | — | — | 不可测（从不出现） | 24 ms | 225 ms |\n"));
  assert.ok(md.includes("## ③ 首屏字体下载字节\n"));
  assert.ok(md.includes("| 首页 | 冷 | 2407904 B | 2410000 B | 102400 B | 不达标 |\n"));
  assert.ok(md.includes("| noto-sans-sc-700.woff2 | 1172244 B |\n"));
  assert.ok(md.includes("| noto-sans-sc-400.woff2 | 1142552 B |\n"));
  assert.ok(md.includes("## ④ 返回上一页是否命中缓存\n"));
  assert.ok(md.includes("| 首页 → 文章 → 返回首页 | 冷 | 文档级重载 | 是 | 3 | 271 ms | 未命中 |\n"));
});

test("markdown 报告：不可测与无头返回重载都在备注里如实标注", () => {
  const md = renderMarkdown(report);
  assert.ok(md.includes("## 备注\n"));
  assert.ok(md.includes("3/3 次点击后内容在 12000 ms 内从未出现"));
  assert.ok(md.includes("首次异常：TypeError: n.off is not a function"));
  assert.ok(md.includes("无头 Chromium 的「返回」表现为文档级重载"));
  // P0 时代站内点击进不去文章：报告必须说明④退化为旧口径，不能把「测不到」当成「没命中」。
  assert.ok(md.includes("进文章退化为文档级 goto（entryFlow=document）"));
});

test("可 diff：两次运行只有生成时间不同，JSON 去掉时间戳后逐字节一致", () => {
  const a = renderJson(buildReport({ meta: { ...META, generatedAt: "2026-11-01T12:00:00.000Z" }, runs: RAW }));
  const b = renderJson(buildReport({ meta: { ...META, generatedAt: "2026-11-02T00:00:00.000Z" }, runs: RAW }));
  const strip = s => s.split("\n").filter(line => !line.includes('"generatedAt"')).join("\n");
  assert.equal(strip(a), strip(b));
});

test("可 diff：Markdown 不含时间戳，两次渲染逐字节一致", () => {
  const a = renderMarkdown(buildReport({ meta: { ...META, generatedAt: "2026-11-01T12:00:00.000Z" }, runs: RAW }));
  const b = renderMarkdown(buildReport({ meta: { ...META, generatedAt: "2026-11-02T00:00:00.000Z" }, runs: RAW }));
  assert.equal(a, b);
});

test("热模式字体体积非零时，报告自动标注「体积不代表新增下载量」", () => {
  const runs = RAW.map(run =>
    run.scenario === "firstScreen.home" && run.mode === "warm"
      ? { ...run, metrics: { ...run.metrics, fontBytes: 2407904 } }
      : run,
  );
  const shared = buildReport({ meta: { ...META, generatedAt: "2026-11-01T12:00:00.000Z" }, runs });
  assert.ok(renderMarkdown(shared).includes("热模式即便命中 HTTP 缓存"));
});

test("未跑的场景不伪造数据，表格里写「未运行」", () => {
  const partial = buildReport({
    meta: { ...META, generatedAt: "2026-11-01T12:00:00.000Z" },
    runs: RAW.filter(r => r.scenario === "firstScreen.home"),
  });
  assert.equal(partial.scenarios["transition.homeToArticle"], undefined);
  const md = renderMarkdown(partial);
  assert.ok(md.includes("| （未运行） |  |  |  |  |  |  |\n"));
});

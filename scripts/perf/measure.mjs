#!/usr/bin/env node
// 读者站性能尺子：一条命令跑完四个测量场景，输出 report.json + report.md。
//
// 用法示例：
//   pnpm --dir scripts/perf run measure -- --base https://gentlybeing.cn --label 2026-11-01-pre-p0 \
//       --out baselines/2026-11-01-pre-p0 --note "P0（#80）未修复：线上站内跳转崩溃，切页内容可见在本基线中不可测"
//
// 默认不进 CI，按需手动跑。
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { buildReport, renderJson, renderMarkdown, BUDGETS } from "./lib/report.mjs";
import { DEFAULT_RUN_OPTIONS, discoverArticleUrl, runSuite } from "./lib/runner.mjs";

const SCRIPT_DIR = fileURLToPath(new URL(".", import.meta.url));
const DEFAULT_BASE = "https://gentlybeing.cn";
const SCENARIO_IDS = ["firstScreen", "transition", "backCache"];

const HELP = `读者站性能尺子（Playwright）

用法：
  node measure.mjs [选项]

选项：
  --base <url>              被测站点根地址，默认 ${DEFAULT_BASE}（也可指向本地构建，如 http://127.0.0.1:3000）
  --article <url>           文章页 URL；默认从首页第一条 /article/ 链接自动发现
  --fixture                 端到端自检：启动本地 fixture 站点并测量（不需要网络，报告结构相同）
  --runs <n>                每个场景每种模式跑几次，默认 3（冷/热各 3 次取中位与最差）
  --modes <cold,warm>       要跑的模式，默认 cold,warm
  --only <ids>              只跑部分场景：firstScreen,transition,backCache（fonts 由 firstScreen 派生，总会输出）
  --out <dir>               报告输出目录（相对 scripts/perf），默认 results/<时间戳>
  --label <name>            报告标签（基线存档名），可省略
  --note <text>             报告备注，可重复；基线存档时用它标注"P0 修复前不可测"
  --raw <path>              另外保存逐次原始记录（含字体文件明细与全部资源计数）
  --click-timeout <ms>      切页等待上限，默认 ${DEFAULT_RUN_OPTIONS.clickTimeoutMs}（站在 P0 未修时不要设太长）
  --visible-timeout <ms>    首屏/返回内容可见等待上限，默认 ${DEFAULT_RUN_OPTIONS.visibleTimeoutMs}
  --font-budget <bytes>     首屏字体预算，默认 ${BUDGETS.firstScreenFontBytes}
  --first-screen-budget <ms> 首屏内容可见预算，默认 ${BUDGETS.firstScreenContentVisibleMs}
  --transition-budget <ms>    切页内容可见预算，默认 ${BUDGETS.transitionContentVisibleMs}
  --back-budget <ms>          返回命中缓存的数据耗时上限，默认 ${BUDGETS.backCacheDataWallMs}
  --help                    打印本帮助

输出口径：report.json 机器可读、report.md 人类可读；两者除 meta.generatedAt 外格式固定，可逐次 diff 对比。
`;

function parseArgs(argv) {
  const args = { notes: [] };
  for (let index = 0; index < argv.length; index++) {
    const token = argv[index];
    if (!token.startsWith("--")) throw new Error(`无法识别的参数：${token}`);
    const body = token.slice(2);
    const equalsAt = body.indexOf("=");
    const key = equalsAt === -1 ? body : body.slice(0, equalsAt);
    const inline = equalsAt === -1 ? undefined : body.slice(equalsAt + 1);
    if (key === "help") {
      args.help = true;
      continue;
    }
    if (key === "fixture") {
      args.fixture = true;
      continue;
    }
    const value = inline !== undefined ? inline : argv[++index];
    if (value === undefined) throw new Error(`参数 --${key} 缺值`);
    switch (key) {
      case "base":
        args.base = value;
        break;
      case "article":
        args.article = value;
        break;
      case "runs":
        args.runs = toPositiveInt(key, value);
        break;
      case "modes":
        args.modes = value;
        break;
      case "only":
        args.only = value;
        break;
      case "out":
        args.out = value;
        break;
      case "label":
        args.label = value;
        break;
      case "note":
        args.notes.push(value);
        break;
      case "raw":
        args.raw = value;
        break;
      case "click-timeout":
        args.clickTimeoutMs = toPositiveInt(key, value);
        break;
      case "visible-timeout":
        args.visibleTimeoutMs = toPositiveInt(key, value);
        break;
      case "font-budget":
        args.fontBudget = toPositiveInt(key, value);
        break;
      case "first-screen-budget":
        args.firstScreenBudget = toPositiveInt(key, value);
        break;
      case "transition-budget":
        args.transitionBudget = toPositiveInt(key, value);
        break;
      case "back-budget":
        args.backBudget = toPositiveInt(key, value);
        break;
      default:
        throw new Error(`未知参数：--${key}`);
    }
  }
  return args;
}

function toPositiveInt(key, value) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) throw new Error(`--${key} 需要正整数，收到 ${value}`);
  return Math.round(number);
}

function splitList(value, fallback) {
  if (value == null || value === "") return fallback;
  return value
    .split(",")
    .map(item => item.trim())
    .filter(item => item.length > 0);
}

function normalizeScenarios(value) {
  const requested = splitList(value, DEFAULT_RUN_OPTIONS.scenarios).map(id => (id === "fonts" ? "firstScreen" : id));
  const scenarios = requested.filter((id, index) => requested.indexOf(id) === index);
  for (const id of scenarios) {
    if (!SCENARIO_IDS.includes(id)) throw new Error(`--only 只接受 ${SCENARIO_IDS.join(",")}（fonts 由 firstScreen 派生）`);
  }
  return scenarios;
}

function budgetsOf(args) {
  const budgets = { ...BUDGETS };
  if (args.fontBudget != null) budgets.firstScreenFontBytes = args.fontBudget;
  if (args.firstScreenBudget != null) budgets.firstScreenContentVisibleMs = args.firstScreenBudget;
  if (args.transitionBudget != null) budgets.transitionContentVisibleMs = args.transitionBudget;
  if (args.backBudget != null) budgets.backCacheDataWallMs = args.backBudget;
  return budgets;
}

function timestampName() {
  return new Date().toISOString().replace(/[:.]/g, "-").replace("T", "_").replace("Z", "");
}

async function loadPlaywright() {
  try {
    return await import("playwright");
  } catch {
    throw new Error(
      "未安装 playwright：请在 scripts/perf 下执行 `pnpm install`；若浏览器未下载，再执行 `pnpm run install:browsers`。",
    );
  }
}

function runSummary(run) {
  if (!run.ok) return `FAILED ${run.error}`;
  const keys = [
    "contentVisibleMs",
    "clickToUrlMs",
    "clickToErrorMs",
    "fontBytes",
    "requestCount",
    "dataRequestCount",
    "dataWallMs",
    "documentNavigation",
  ];
  return keys
    .filter(key => Object.prototype.hasOwnProperty.call(run.metrics, key))
    .map(key => `${key}=${run.metrics[key]}`)
    .join(" ");
}

function summaryLine(report) {
  const parts = [];
  const firstHome = report.scenarios["firstScreen.home"];
  if (firstHome) {
    for (const mode of ["cold", "warm"]) {
      const data = firstHome.modes[mode];
      if (!data) continue;
      const aggregate = data.aggregates.contentVisibleMs;
      parts.push(`firstScreen.home.${mode}=${aggregate.median}/${aggregate.worst}ms`);
    }
  }
  const firstArticle = report.scenarios["firstScreen.article"];
  if (firstArticle && firstArticle.modes.cold) {
    const aggregate = firstArticle.modes.cold.aggregates.contentVisibleMs;
    parts.push(`firstScreen.article.cold=${aggregate.median}/${aggregate.worst}ms`);
  }
  const transition = report.scenarios["transition.homeToArticle"];
  if (transition) {
    for (const mode of ["cold", "warm"]) {
      const data = transition.modes[mode];
      if (!data) continue;
      const aggregate = data.aggregates.contentVisibleMs;
      parts.push(`transition.${mode}=${data.outcome}${aggregate.median == null ? "" : `(${aggregate.median}ms)`}`);
    }
  }
  if (report.fonts.home) {
    for (const mode of ["cold", "warm"]) {
      const data = report.fonts.home[mode];
      if (!data) continue;
      parts.push(`fonts.home.${mode}=${data.totalBytes.median}B`);
    }
  }
  const back = report.scenarios["backCache.articleToHome"];
  if (back) {
    for (const mode of ["cold", "warm"]) {
      const data = back.modes[mode];
      if (!data) continue;
      parts.push(`back.${mode}=dataRequests:${data.aggregates.dataRequestCount.median},cacheHit:${data.verdicts.cacheHit}`);
    }
  }
  return `summary: ${parts.join(" | ")}`;
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    console.log(HELP);
    return;
  }
  const base = args.base || DEFAULT_BASE;
  let fixtureSite = null;
  let effectiveBase = base;
  if (args.fixture) {
    const { startFixtureSite } = await import("./lib/fixture-site.mjs");
    fixtureSite = await startFixtureSite();
    effectiveBase = fixtureSite.base;
  }
  const runs = args.runs || DEFAULT_RUN_OPTIONS.runs;
  const modes = splitList(args.modes, DEFAULT_RUN_OPTIONS.modes);
  for (const mode of modes) {
    if (!["cold", "warm"].includes(mode)) throw new Error(`--modes 只接受 cold,warm，收到 ${mode}`);
  }
  const scenarios = normalizeScenarios(args.only);
  const options = {
    ...DEFAULT_RUN_OPTIONS,
    runs,
    modes,
    scenarios,
    onRun: run => console.log(`[${run.mode}] ${run.scenario} #${run.run} ${runSummary(run)}`),
  };
  if (args.clickTimeoutMs != null) options.clickTimeoutMs = args.clickTimeoutMs;
  if (args.visibleTimeoutMs != null) options.visibleTimeoutMs = args.visibleTimeoutMs;
  const budgets = budgetsOf(args);

  const { chromium } = await loadPlaywright();
  const browser = await chromium.launch();
  const startedAt = new Date();
  try {
    const articleUrl = args.article
      ? new URL(args.article, effectiveBase).href
      : await discoverArticleUrl({ browser, base: effectiveBase, options });
    console.log(`base=${effectiveBase}`);
    console.log(`article=${articleUrl || "（首页未找到 /article/ 链接）"}`);
    const rawRuns = await runSuite({ browser, base: effectiveBase, articleUrl, options });
    const report = buildReport({
      meta: {
        base: effectiveBase,
        label: args.label || "",
        runs,
        modes,
        viewport: `${options.viewport.width}x${options.viewport.height}`,
        chromium: browser.version(),
        generatedAt: startedAt.toISOString(),
      },
      runs: rawRuns,
      notes: args.notes,
      budgets,
    });
    const outDir = path.resolve(SCRIPT_DIR, args.out || path.join("results", timestampName()));
    await mkdir(outDir, { recursive: true });
    await writeFile(path.join(outDir, "report.json"), renderJson(report));
    await writeFile(path.join(outDir, "report.md"), renderMarkdown(report));
    if (args.raw) {
      const rawPath = path.resolve(SCRIPT_DIR, args.raw);
      await mkdir(path.dirname(rawPath), { recursive: true });
      await writeFile(rawPath, JSON.stringify({ meta: report.meta, runs: rawRuns }, null, 2) + "\n");
      console.log(`raw=${rawPath}`);
    }
    console.log(`report=${outDir}`);
    console.log(summaryLine(report));
    console.log(`wrote ${path.join(outDir, "report.json")} + ${path.join(outDir, "report.md")}`);
  } finally {
    await browser.close().catch(() => {});
    if (fixtureSite) await fixtureSite.close().catch(() => {});
  }
}

main().catch(error => {
  console.error(`FAILED: ${error && error.message ? error.message : error}`);
  process.exit(1);
});

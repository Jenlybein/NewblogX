// 固定格式报告：聚合各场景的运行结果，渲染成机器可读的 JSON 与人类可读的 Markdown 表。
//
// 稳定性约定（这是"两次运行可直接 diff"的依据）：
// - 报告只含标量指标（毫秒/字节/计数/布尔/枚举），每次运行的字体明细只在 fonts 段按文件名聚合一次；
// - 场景、模式、表格行、字体文件、备注的顺序全部写死；数字一律整数；
// - Markdown 不含任何时间戳；JSON 的易变字段只有 meta.generatedAt 一行。
import { median, summarize } from "./aggregate.mjs";

export const BUDGETS = {
  firstScreenContentVisibleMs: 800,
  transitionContentVisibleMs: 300,
  firstScreenFontBytes: 102400,
  backCacheDataWallMs: 50,
};

// 场景定义：顺序即报告顺序。metrics 进 JSON 的每次运行记录并参与聚合；extraFields 只透传到运行记录。
export const SCENARIO_SPECS = {
  "firstScreen.home": {
    group: "firstScreen",
    label: "首页 `/`",
    target: "首页 `/`",
    metrics: ["contentVisibleMs", "fontBytes", "requestCount", "totalBytes", "apiCount", "pageErrors"],
    extraFields: ["contentSelector"],
  },
  "firstScreen.article": {
    group: "firstScreen",
    label: "文章页",
    target: "文章页",
    metrics: ["contentVisibleMs", "fontBytes", "requestCount", "totalBytes", "apiCount", "pageErrors"],
    extraFields: ["contentSelector"],
  },
  "transition.homeToArticle": {
    group: "transition",
    label: "首页 → 文章",
    metrics: ["contentVisibleMs", "clickToUrlMs", "clickToErrorMs", "timeoutMs", "apiCount", "apiWallMs", "pageErrors"],
    extraFields: ["errorMsg", "pageErrorMsg"],
  },
  "backCache.articleToHome": {
    group: "backCache",
    label: "首页 → 文章 → 返回首页",
    metrics: ["dataRequestCount", "dataWallMs", "allRequestCount", "pageErrors"],
    // backFlow：返回本身是 SPA 还是文档级重载；entryFlow：进文章是站内点击（spa）还是退化为文档级 goto（document）。
    extraFields: ["backFlow", "entryFlow", "clickToArticleMs"],
    flags: ["documentNavigation"],
  },
};

const MODE_ORDER = ["cold", "warm"];
export const MODE_LABELS = { cold: "冷", warm: "热" };
const VERDICT_TEXT = { pass: "达标", fail: "不达标", partial: "部分不可测", unmeasurable: "不可测" };
const TRANSITION_VERDICT_TEXT = { pass: "达标", fail: "不达标", partial: "部分不可测", unmeasurable: "不可测（从不出现）" };

export function buildReport({ meta, runs, notes = [], budgets = BUDGETS }) {
  const index = new Map();
  for (const run of runs) {
    if (!SCENARIO_SPECS[run.scenario]) continue;
    if (!index.has(run.scenario)) index.set(run.scenario, []);
    index.get(run.scenario).push(run);
  }

  const scenarios = {};
  for (const [id, spec] of Object.entries(SCENARIO_SPECS)) {
    const scenarioRuns = index.get(id);
    if (!scenarioRuns || scenarioRuns.length === 0) continue;
    const modes = {};
    for (const mode of MODE_ORDER) {
      const modeRuns = scenarioRuns.filter(r => r.mode === mode);
      if (modeRuns.length === 0) continue;
      modes[mode] = buildMode(spec, modeRuns, budgets);
    }
    scenarios[id] = { group: spec.group, label: spec.label, modes };
  }

  const fonts = buildFonts(index, budgets);
  const auto = autoNotes(scenarios, fonts);
  return {
    schemaVersion: 1,
    tool: "blogx-perf",
    meta,
    budgets,
    scenarios,
    fonts,
    notes: [...notes, ...auto],
  };
}

function buildMode(spec, modeRuns, budgets) {
  const runs = modeRuns
    .slice()
    .sort((a, b) => a.run - b.run)
    .map(run => emitRun(spec, run));

  const aggregates = {};
  for (const key of spec.metrics) {
    aggregates[key] = summarize(modeRuns.map(run => (run.ok ? numberOrNull(run.metrics[key]) : null)));
  }

  const mode = { runs, aggregates };
  if (spec.group === "backCache") {
    mode.documentNavigation = flagOf(modeRuns, "documentNavigation");
    mode.cacheHitRuns = modeRuns.filter(run => run.ok && cacheHit(run, budgets)).length;
    mode.backFlow = flowOf(modeRuns, "backFlow");
    mode.entryFlow = flowOf(modeRuns, "entryFlow");
  }
  if (spec.group === "transition") {
    const measured = aggregates.contentVisibleMs.measured;
    mode.outcome = measured === 0 ? "never" : measured === modeRuns.length ? "visible" : "partial";
  }
  mode.verdicts = verdictsOf(spec, mode, budgets);
  return mode;
}

function emitRun(spec, run) {
  if (!run.ok) {
    return { run: run.run, ok: false, error: run.error == null ? "unknown" : String(run.error) };
  }
  const out = { run: run.run, ok: true };
  for (const key of spec.metrics) out[key] = scalarOrNull(run.metrics[key]);
  for (const key of spec.extraFields) out[key] = scalarOrNull(run.metrics[key]);
  for (const key of spec.flags || []) out[key] = run.metrics[key] === true;
  return out;
}

function verdictsOf(spec, mode, budgets) {
  if (spec.group === "firstScreen") {
    return { contentVisibleMs: judge(mode.aggregates.contentVisibleMs.worst, budgets.firstScreenContentVisibleMs) };
  }
  if (spec.group === "transition") {
    if (mode.outcome === "never") return { contentVisibleMs: "unmeasurable" };
    if (mode.outcome === "partial") return { contentVisibleMs: "partial" };
    return { contentVisibleMs: judge(mode.aggregates.contentVisibleMs.worst, budgets.transitionContentVisibleMs) };
  }
  if (spec.group === "backCache") {
    const passed = mode.runs.length > 0 && mode.cacheHitRuns === mode.runs.length;
    return { cacheHit: passed ? "pass" : "fail" };
  }
  return {};
}

function judge(worstValue, budget) {
  if (worstValue == null) return "unmeasurable";
  return worstValue <= budget ? "pass" : "fail";
}

// 命中缓存口径（ADR-0019）：返回上一页后没有数据请求，或数据请求耗时 < 50 ms，
// 且不能是文档级重载（整篇文档重新加载本身就是缓存未命中的最强证据）。
function cacheHit(run, budgets) {
  if (run.metrics.documentNavigation === true) return false;
  const requests = numberOrNull(run.metrics.dataRequestCount);
  if (requests === 0) return true;
  const wall = numberOrNull(run.metrics.dataWallMs);
  return wall != null && wall < budgets.backCacheDataWallMs;
}

function flowOf(modeRuns, key) {
  const flows = modeRuns.filter(run => run.ok).map(run => run.metrics[key]);
  if (flows.length === 0) return "unknown";
  if (flows.every(flow => flow === "spa")) return "spa";
  if (flows.every(flow => flow === "document")) return "document";
  return "mixed";
}

function flagOf(modeRuns, key) {
  const values = modeRuns.filter(run => run.ok).map(run => run.metrics[key] === true);
  if (values.length === 0) return "unknown";
  if (values.every(Boolean)) return "yes";
  if (values.every(v => !v)) return "no";
  return "mixed";
}

const FONT_TARGETS = [
  ["home", "firstScreen.home", "首页"],
  ["article", "firstScreen.article", "文章页"],
];

function buildFonts(index, budgets) {
  const fonts = {};
  for (const [key, scenarioId, label] of FONT_TARGETS) {
    const scenarioRuns = index.get(scenarioId);
    if (!scenarioRuns || scenarioRuns.length === 0) continue;
    const entry = { label };
    for (const mode of MODE_ORDER) {
      const modeRuns = scenarioRuns.filter(r => r.mode === mode);
      if (modeRuns.length === 0) continue;
      const totalBytes = summarize(modeRuns.map(run => (run.ok ? numberOrNull(run.metrics.fontBytes) : null)));
      const notoBytes = summarize(modeRuns.map(run => (run.ok ? notoBytesOf(run.metrics.fontFiles) : null)));
      entry[mode] = {
        totalBytes,
        notoBytes,
        files: aggregateFontFiles(modeRuns),
        verdicts: { totalBytes: judge(totalBytes.worst, budgets.firstScreenFontBytes) },
      };
    }
    fonts[key] = entry;
  }
  return fonts;
}

function aggregateFontFiles(modeRuns) {
  const byFile = new Map();
  for (const run of modeRuns) {
    if (!run.ok) continue;
    for (const file of run.metrics.fontFiles || []) {
      if (!byFile.has(file.file)) byFile.set(file.file, []);
      byFile.get(file.file).push(numberOrNull(file.bytes));
    }
  }
  return [...byFile.entries()]
    .map(([file, values]) => ({ file, bytes: median(values) }))
    .filter(entry => entry.bytes != null && entry.bytes > 0)
    .sort((a, b) => b.bytes - a.bytes || a.file.localeCompare(b.file));
}

function notoBytesOf(fontFiles) {
  if (fontFiles == null) return null;
  return fontFiles
    .filter(file => String(file.file).toLowerCase().includes("noto-sans-sc"))
    .reduce((sum, file) => sum + (numberOrNull(file.bytes) || 0), 0);
}

function autoNotes(scenarios, fonts) {
  const notes = [];
  const transition = scenarios["transition.homeToArticle"];
  if (transition) {
    for (const mode of MODE_ORDER) {
      const data = transition.modes[mode];
      if (!data || data.outcome !== "never") continue;
      const timeout = data.aggregates.timeoutMs.median;
      const total = data.runs.length;
      const firstError = data.runs
        .map(run => run.errorMsg || run.pageErrorMsg)
        .find(msg => typeof msg === "string" && msg.length > 0);
      const timeoutText = timeout == null ? "等待上限" : `${timeout} ms 内`;
      notes.push(
        `切页「${transition.label}」· ${MODE_LABELS[mode]}：${total}/${total} 次点击后内容在 ${timeoutText}从未出现，` +
          `结论记作不可测（从不出现）${firstError ? `；首次异常：${firstError}` : ""}。`,
      );
    }
  }
  const back = scenarios["backCache.articleToHome"];
  if (back) {
    for (const mode of MODE_ORDER) {
      const data = back.modes[mode];
      if (!data) continue;
      if (data.entryFlow === "document") {
        notes.push(
          `返回「${back.label}」· ${MODE_LABELS[mode]}：站内点击没能进入文章页（点击导航不可用，P0 #80 时代的形态），` +
            `进文章退化为文档级 goto（entryFlow=document）；返回本身是${data.backFlow === "spa" ? "SPA 返回" : "文档级重载"}。` +
            "该行只作改前对照，不代表客户端缓存的最终形态。",
        );
      }
      if (data.documentNavigation !== "yes") continue;
      notes.push(
        `返回「${back.label}」· ${MODE_LABELS[mode]}：无头 Chromium 的「返回」表现为文档级重载；` +
          "与真实浏览器 bfcache 对比时不要把该差异当成回归（口径见 README）。",
      );
    }
  }
  const warmFontBytes = ["home", "article"]
    .map(key => (fonts[key] && fonts[key].warm ? fonts[key].warm.totalBytes.worst : null))
    .filter(value => value != null && value > 0);
  if (warmFontBytes.length > 0) {
    notes.push(
      "字体字节是资源计时里的体积（encodedBodySize）：热模式即便命中 HTTP 缓存，该体积仍会被报告，" +
        "它不代表热启动的新增下载量；字体预算判定以冷模式为准（新增下载量看 transferSize，本报告未单列）。",
    );
  }
  return notes;
}

/* ---------------------------------- 渲染 ---------------------------------- */

export function renderJson(report) {
  return JSON.stringify(report, null, 2) + "\n";
}

export function renderMarkdown(report) {
  const { meta, budgets } = report;
  const lines = [];
  lines.push("# 读者站性能基线报告", "");
  lines.push(`- 站点：${meta.base || "（未填）"}`);
  lines.push(`- 标签：${meta.label || "（无）"}`);
  lines.push(`- 每模式次数：${meta.runs}（取中位与最差）`);
  lines.push("- 模式：冷 = 全新浏览器上下文 + 禁用 HTTP 缓存；热 = 同一上下文先访问一次再测");
  lines.push(`- 视口：${meta.viewport || "（未填）"}`);
  lines.push(`- Chromium：${meta.chromium || "（未填）"}`);
  lines.push(
    `预算（ADR-0019 硬指标）：首屏内容可见 < ${budgets.firstScreenContentVisibleMs} ms；` +
      `切页内容可见 < ${budgets.transitionContentVisibleMs} ms（3 次取最差作为 P95 保守代理）；` +
      `首屏字体下载 < ${budgets.firstScreenFontBytes} B；` +
      `返回上一页命中缓存（数据请求数为 0 或数据耗时 < ${budgets.backCacheDataWallMs} ms）。`,
    "",
  );

  // ① 首屏内容可见
  lines.push("## ① 首屏内容可见（文档级直达）", "", "| 目标 | 模式 | 中位 | 最差 | 达标 | 请求数（中位） | 总字节（中位） |", "| --- | --- | --- | --- | --- | --- | --- |");
  const firstScreenRows = [];
  for (const id of ["firstScreen.home", "firstScreen.article"]) {
    const scenario = report.scenarios[id];
    if (!scenario) continue;
    const target = SCENARIO_SPECS[id].target;
    for (const mode of modesOf(scenario)) {
      const data = scenario.modes[mode];
      firstScreenRows.push(
        `| ${target} | ${MODE_LABELS[mode]} | ${fmtMs(data.aggregates.contentVisibleMs.median)} | ` +
          `${fmtMs(data.aggregates.contentVisibleMs.worst)} | ${VERDICT_TEXT[data.verdicts.contentVisibleMs]} | ` +
          `${fmtNum(data.aggregates.requestCount.median)} | ${fmtBytes(data.aggregates.totalBytes.median)} |`,
      );
    }
  }
  lines.push(...(firstScreenRows.length ? firstScreenRows : [PLACEHOLDER_ROW_7]), "");

  // ② 切页内容可见
  lines.push("## ② 切页内容可见（点击站内链接）", "", "| 路径 | 模式 | 中位 | 最差 | 结论 | 点击→URL（中位） | 点击→异常（中位） |", "| --- | --- | --- | --- | --- | --- | --- |");
  const transitionRows = [];
  const transition = report.scenarios["transition.homeToArticle"];
  if (transition) {
    for (const mode of modesOf(transition)) {
      const data = transition.modes[mode];
      transitionRows.push(
        `| ${transition.label} | ${MODE_LABELS[mode]} | ${fmtMs(data.aggregates.contentVisibleMs.median)} | ` +
          `${fmtMs(data.aggregates.contentVisibleMs.worst)} | ${TRANSITION_VERDICT_TEXT[data.verdicts.contentVisibleMs]} | ` +
          `${fmtMs(data.aggregates.clickToUrlMs.median)} | ${fmtMs(data.aggregates.clickToErrorMs.median)} |`,
      );
    }
  }
  lines.push(...(transitionRows.length ? transitionRows : [PLACEHOLDER_ROW_7]), "");

  // ③ 首屏字体下载字节
  lines.push("## ③ 首屏字体下载字节", "", `| 目标 | 模式 | 中位 | 最差 | 预算 | 达标 |`, "| --- | --- | --- | --- | --- | --- |");
  const fontRows = [];
  for (const key of ["home", "article"]) {
    const entry = report.fonts[key];
    if (!entry) continue;
    for (const mode of MODE_ORDER) {
      const data = entry[mode];
      if (!data) continue;
      fontRows.push(
        `| ${entry.label} | ${MODE_LABELS[mode]} | ${fmtBytes(data.totalBytes.median)} | ${fmtBytes(data.totalBytes.worst)} | ` +
          `${budgets.firstScreenFontBytes} B | ${VERDICT_TEXT[data.verdicts.totalBytes]} |`,
      );
    }
  }
  lines.push(...(fontRows.length ? fontRows : [PLACEHOLDER_ROW_6]), "");
  const notoLine = notoSummaryLine(report);
  if (notoLine) lines.push(notoLine, "");
  const homeCold = report.fonts.home && report.fonts.home.cold;
  lines.push("### 字体文件（首页 · 冷，按字节降序）", "", "| 文件 | 字节 |", "| --- | --- |");
  if (homeCold && homeCold.files.length > 0) {
    for (const file of homeCold.files) lines.push(`| ${file.file} | ${file.bytes} B |`);
  } else {
    lines.push("| （无） |  |");
  }
  lines.push("");

  // ④ 返回上一页是否命中缓存
  lines.push(
    "## ④ 返回上一页是否命中缓存",
    "",
    "| 路径 | 模式 | 返回方式 | 文档级重载 | 数据请求数（中位） | 数据耗时（中位） | 命中缓存 |",
    "| --- | --- | --- | --- | --- | --- | --- |",
  );
  const backRows = [];
  const back = report.scenarios["backCache.articleToHome"];
  if (back) {
    for (const mode of modesOf(back)) {
      const data = back.modes[mode];
      backRows.push(
        `| ${back.label} | ${MODE_LABELS[mode]} | ${backFlowText(data.backFlow)} | ${flagText(data.documentNavigation)} | ` +
          `${fmtNum(data.aggregates.dataRequestCount.median)} | ${fmtMs(data.aggregates.dataWallMs.median)} | ` +
          `${data.verdicts.cacheHit === "pass" ? "命中" : "未命中"} |`,
      );
    }
  }
  lines.push(...(backRows.length ? backRows : [PLACEHOLDER_ROW_6]), "");

  // 备注
  lines.push("## 备注", "");
  if (report.notes.length === 0) {
    lines.push("- （无）");
  } else {
    for (const note of report.notes) lines.push(`- ${note}`);
  }
  lines.push("");
  return lines.join("\n");
}

const PLACEHOLDER_ROW_7 = "| （未运行） |  |  |  |  |  |  |";
const PLACEHOLDER_ROW_6 = "| （未运行） |  |  |  |  |  |";

function modesOf(scenario) {
  return MODE_ORDER.filter(mode => scenario.modes[mode]);
}

function notoSummaryLine(report) {
  const home = report.fonts.home;
  if (!home) return null;
  const parts = [];
  for (const mode of MODE_ORDER) {
    const data = home[mode];
    if (!data) continue;
    parts.push(`${MODE_LABELS[mode]} ${fmtBytes(data.notoBytes.median)}`);
  }
  if (parts.length === 0) return null;
  return `其中中文字体（noto-sans-sc）合计：${parts.join("；")}。`;
}

function fmtMs(value) {
  return value == null ? "—" : `${value} ms`;
}

function fmtBytes(value) {
  return value == null ? "—" : `${value} B`;
}

function fmtNum(value) {
  return value == null ? "—" : String(value);
}

function flagText(flag) {
  if (flag === "yes") return "是";
  if (flag === "no") return "否";
  if (flag === "mixed") return "混合";
  return "—";
}

function backFlowText(flow) {
  if (flow === "spa") return "SPA 返回";
  if (flow === "document") return "文档级重载";
  return "—";
}

function numberOrNull(value) {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function scalarOrNull(value) {
  if (value == null) return null;
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return value;
  return null;
}

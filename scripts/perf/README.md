# 读者站性能尺子（`scripts/perf`）

用真实 Chromium（Playwright）测量读者站的四类指标，输出**固定格式**的两份报告（`report.json` 机器可读、`report.md` 人类可读），作为性能改动的验收依据与基线存档。测量口径来自实测笔记 [读者站性能实测](../../docs/notes/2026-11-01-reader-site-performance-probe.md)，预算口径来自 [ADR-0019](../../docs/adr/0019-reader-rendering-and-cache-strategy.md)。

**默认不进 CI 门禁**：它要打真实站点、耗时几分钟、结果受网络抖动影响，只按需手动跑。依赖也不放进 `frontend` 的依赖树，单独声明在本目录的 `package.json`。

## 安装（干净检出后一次性）

```bash
cd scripts/perf
pnpm install                 # 只装 playwright（版本固定在 package.json / pnpm-lock.yaml）
pnpm run install:browsers    # 若本机还没有 Chromium 缓存
```

## 跑一次基线

```bash
cd scripts/perf
pnpm run measure -- --base https://gentlybeing.cn --label 2026-11-01-pre-p0 \
  --out baselines/2026-11-01-pre-p0 \
  --note "P0（#80）未修复：线上站内跳转崩溃，切页内容可见不可测"
```

也可以指向本地构建（`--base http://127.0.0.1:3000`）。文章页 URL 默认从首页第一条 `/article/` 链接自动发现，内容换了也不用改脚本；要指定就加 `--article`。

输出：

- `report.json`：聚合后的标量报告（每场景每模式的 3 次运行、中位/最差、判定、字体文件表）；
- `report.md`：同样的内容的人类可读表格；
- 可加 `--raw <path>` 另存逐次原始记录（含字体文件明细与请求计数）。

**可 diff**：场景/模式/表格行/字体文件/备注的顺序全部固定，数字一律整数，Markdown 不含时间戳；JSON 里唯一的易变字段是 `meta.generatedAt` 一行。所以两次运行可以直接 `diff`，关注点只会落在数字上。

## 量什么（四个场景）

| 编号 | 场景 | 指标 | 预算（ADR-0019） |
| --- | --- | --- | --- |
| ① | `firstScreen.home` / `firstScreen.article` | 文档级直达的「首屏内容可见」 | < 800 ms |
| ② | `transition.homeToArticle` | 首页点击文章链接的「切页内容可见」（点击 → 目标内容可见） | < 300 ms（3 次取最差作 P95 保守代理） |
| ③ | `fonts`（由 ① 派生） | 首屏字体下载字节（按 URL 去重，含中文字体单独合计） | < 100 KB（102400 B） |
| ④ | `backCache.articleToHome` | 浏览器返回上一页是否命中缓存（文档级重载？数据请求数 / 数据耗时） | 无数据请求或数据耗时 < 50 ms，且不能是文档级重载 |

「内容可见」的判定是：目标元素存在、其祖先链上没有 `opacity:0` / `visibility:hidden` / `display:none`、在视口内、文本非空（选择器候选见 `lib/browser.mjs` 的 `DEFAULT_SELECTORS`）。首屏时间从文档起点（`performance.now`）算，切页时间从点击时刻算。

内容选择器必须指向**页面内容本身**（首页第一条 `.feed-entry` 卡片、文章页 `.article-page h1`），不能是站点标题这类全站可见元素——否则"点了没反应"的崩溃也会被误测成几毫秒可见。切页场景还会排除"点击前就在的同一个元素"（记录点击前节点引用，节点换了或新出现才计入），避免把当前页内容当成目标页内容。页面结构改版时只需改 `DEFAULT_SELECTORS`。

冷/热各跑 3 次（`--runs` 可改），报告给中位与最差：

- **冷** = 全新浏览器上下文 + CDP 禁用 HTTP 缓存（第一次访问）；
- **热** = 同一上下文先访问一次再测（回访）。

## 自检（不需要真实站点）

```bash
cd scripts/perf
pnpm run selfcheck          # 全部：纯逻辑单测 + 本地 fixture 浏览器端到端
pnpm run selfcheck:pure     # 只跑聚合/报告格式单测（秒级，无浏览器）
pnpm run measure -- --fixture --runs 1 --out results/fixture-smoke   # 用本地 fixture 跑一遍 CLI
```

fixture 是 `fixture/` 下的两个静态页（模拟"能用的"站内跳转、一条数据请求、一个字体文件）。自检验证：尺子能测出四类指标、报告四节齐全、聚合口径（中位/最差/不可测）正确、两次渲染逐字节一致。`fixture/fonts/fixture.woff2` 是占位二进制（内容不是合法 WOFF2），只用于触发一次真实的字体下载并校验字节统计；`fixture/` 不是生产站点，不用于基线。

## 已知口径坑

1. **无头 Chromium 的「返回」表现为文档级重载**（实测见上述笔记）。真实浏览器可能走 bfcache；对比改前/改后数据时不要把这一差异当成回归。报告会在备注里自动标注。
2. **P0（#80）未修复前，② 切页内容可见不可测**：反复点击后目标视图从不出现，报告如实记为 `outcome=never` / `不可测（从不出现）`，并记录点击→URL 时间与首次异常；等待上限由 `--click-timeout` 控制（默认 12000 ms，站在崩溃状态时不要调长）。等 P0 合入后用同一命令补一次正常基线。
3. **冷/热各 3 次仍会抖动**：中位/最差是保守口径，不要拿单次极端值做结论。
4. 结果与网络、机器负载相关；基线存档必须带上 `--label` 与 `--note`，说明测的是哪个站点/哪个阶段。
5. **字体字节是资源计时里的体积（`encodedBodySize`）**：热模式即便命中 HTTP 缓存，这个体积仍会被报告（`transferSize` 会归零），所以"热"那一行的字体字节不代表新增下载量。字体预算判定以冷模式为准；报告在字体热体积非零时会自动加一条备注。
6. **内容选择器是测量定义的一部分**：`DEFAULT_SELECTORS` 指错（例如指到站点标题）会把"崩溃"测成"3 ms 可见"。改版后若选择器失效，报告会显示"不可测（从不出现）"而不是误报成功——先核对选择器再下结论。

## 目录

```
scripts/perf/
  measure.mjs            CLI 入口（参数、编排、写报告）
  lib/browser.mjs        浏览器测量层（上下文/冷热、可见性、点击/返回观察器、资源汇总）
  lib/scenarios.mjs      四个场景的测量实现
  lib/runner.mjs         发现文章页 URL + 套件编排
  lib/aggregate.mjs      中位/最差/不可测次数
  lib/report.mjs         固定格式 JSON / Markdown 报告与判定
  lib/fixture-site.mjs   自检用本地静态站点（不用于生产测量）
  fixture/              自检夹具
  test/                 自检用例（node --test）
  baselines/            基线存档（report.json + report.md）
```

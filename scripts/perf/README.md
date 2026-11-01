# 读者站性能尺子（`scripts/perf`）

用真实 Chromium（Playwright）测量读者站的**四项硬指标**（首屏内容可见 / 切页内容可见 / 首屏字体下载 / 返回是否命中缓存）并补两个渲染边界场景（个人主页首屏参考值、`/articles` 服务端 301），输出**固定格式**的两份报告（`report.json` 机器可读、`report.md` 人类可读），作为性能改动的验收依据与基线存档。测量口径来自实测笔记 [读者站性能实测](../../docs/notes/2026-11-01-reader-site-performance-probe.md)，预算口径来自 [ADR-0019](../../docs/adr/0019-reader-rendering-and-cache-strategy.md)。

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

也可以指向本地构建（`--base http://127.0.0.1:3000`）。文章页 URL 默认从首页第一条 `/article/` 链接自动发现；个人主页 URL（`/users/<id>`）默认从首页或文章页的作者链接自动发现（首页没有就退到文章页），要指定就加 `--article` / `--user`。找不到匿名可访问的 `/users/<id>` 时跳过个人主页场景（报告里 ⑤ 写「未运行」，不伪造数据）。

输出：

- `report.json`：聚合后的标量报告（每场景每模式的 3 次运行、中位/最差、判定、字体文件表）；
- `report.md`：同样的内容的人类可读表格；
- 可加 `--raw <path>` 另存逐次原始记录（含字体文件明细与请求计数）。

**可 diff**：场景/模式/表格行/字体文件/备注的顺序全部固定，数字一律整数，Markdown 不含时间戳；JSON 里唯一的易变字段是 `meta.generatedAt` 一行。所以两次运行可以直接 `diff`，关注点只会落在数字上。

## 量什么（六个场景）

| 编号 | 场景 | 指标 | 预算（ADR-0019） |
| --- | --- | --- | --- |
| ① | `firstScreen.home` / `firstScreen.article` | 文档级直达的「首屏内容可见」 | < 800 ms |
| ② | `transition.homeToArticle` | 首页点击文章链接的「切页内容可见」（点击 → 目标内容可见） | < 300 ms（3 次取最差作 P95 保守代理） |
| ③ | `fonts`（由 ① 派生） | 首屏字体下载字节（按 URL 去重，含中文字体单独合计） | < 100 KB（102400 B） |
| ④ | `backCache.articleToHome` | 站内点击进文章后按浏览器「返回」首页是否命中 30s 客户端缓存（文档级重载？数据请求数 / 数据耗时） | 无数据请求或数据耗时 < 50 ms，且不能是文档级重载 |
| ⑤ | `firstScreen.userProfile` | `/users/<id>` 直达的「首屏内容可见」 | **无预算**：个人主页是 SPA（`/users/**` 显式 `ssr:false`），这行只是"渲染边界收窄后 SPA 页的代价"参考值，不参与硬指标判定 |
| ⑥ | `redirect.articles` | `/articles` 首个响应是否**服务端 301**、最终路径是否 `/search` | 301 + `/search`（渲染边界回归门禁） |

⑤ 的内容锚点是 `.profile-hero-name`（作者名出现才算可见；骨架 `data-role="skeleton"` 不算内容）。
⑥ 不做内容可见测量：它看的是 Playwright 跟随重定向后，**原始请求的响应用状态码**（`page.goto` 会跟随 301，所以要从请求链的链头取状态码）与最终落点；`/articles` 若退化成 200 空壳或 302，判定直接 `不达标` 并在备注里点出来。

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

fixture 是 `fixture/` 下的两个静态页（模拟"能用的"站内跳转、一条数据请求、一个字体文件）。自检验证：尺子能测出六类场景、报告六节齐全、聚合口径（中位/最差/不可测）正确、两次渲染逐字节一致；fixture 没有 `/users` 链接（个人主页场景按"未运行"跳过）、`/articles` 是 404（重定向判定必须 fail，不能误判成 pass）。`fixture/fonts/fixture.woff2` 是占位二进制（内容不是合法 WOFF2），只用于触发一次真实的字体下载并校验字节统计；`fixture/` 不是生产站点，不用于基线。

## 收尾用的两个探针（`probes/`，默认不进 CI）

#87 收口时用它们做了两件"尺子之外的确认"，需要时可直接复跑（同样复用本目录的 Playwright 依赖，只写 stdout、不落文件）：

```bash
node probes/transition-probe.mjs http://127.0.0.1:3000 3        # 冷/热 × 正常过渡/过渡关闭
node probes/transition-probe.mjs http://127.0.0.1:3000 3 warm   # 只跑热
node probes/hydration-probe.mjs http://127.0.0.1:3000           # 首页与文章页的 hydration 警告
```

- `transition-probe`：同一构建上分别测「正常过渡」与「注入 `.page-enter-active/.page-leave-active { transition-duration: 0 }`」的
  点击→URL / 点击→骨架 / 点击→正文可见。用来量化 `pageTransition` 的固定开销，决定 ADR-0012 三档里页面过渡取哪一档。
- `hydration-probe`：抓 `Hydration completed but contains mismatches`、`console.error` 与未捕获异常（spec #79 已登记的独立观察项）。

## 已知口径坑

1. **④ 返回场景走的是真实用户路径（#87 口径修订）**：先在首页点击文章链接（客户端跳转）、再按浏览器「返回」。`performance.timeOrigin` 不变时才计入 SPA 返回；这样才能测到 30s 客户端 SWR 缓存是否命中。旧的实现用 `page.goto(articleUrl)` 进文章页，于是「返回」必然是文档级重载——那只是无头浏览器的口径坑，测不到客户端缓存。
   **P0（#80）未修时**站内点击进不去文章页，此时 `entryFlow=document`：进文章退化为文档级 `goto`（报告「备注」会标注），返回本身仍可能是 SPA 返回或文档级重载（看 `backFlow` / 「返回方式」列）。改前基线就是这一形态，所以那行只作对照，不能直接当成「30s 客户端缓存未命中」的证据。
2. **无头 Chromium 的「返回」仍可能表现为文档级重载**（真实浏览器可能走 bfcache）；报告在备注里自动标注。对比改前/改后数据时不要把这一差异当成回归。
3. **P0（#80）未修复前，② 切页内容可见不可测**：反复点击后目标视图从不出现，报告如实记为 `outcome=never` / `不可测（从不出现）`，并记录点击→URL 时间与首次异常；等待上限由 `--click-timeout` 控制（默认 12000 ms，站在崩溃状态时不要调长）。等 P0 合入后用同一命令补一次正常基线。
4. **冷/热各 3 次仍会抖动**：中位/最差是保守口径，不要拿单次极端值做结论。
5. 结果与网络、机器负载相关；基线存档必须带上 `--label` 与 `--note`，说明测的是哪个站点/哪个阶段。
6. **字体字节是资源计时里的体积（`encodedBodySize`）**：热模式即便命中 HTTP 缓存，这个体积仍会被报告（`transferSize` 会归零），所以"热"那一行的字体字节不代表新增下载量。字体预算判定以冷模式为准；报告在字体热体积非零时会自动加一条备注。
7. **内容选择器是测量定义的一部分**：`DEFAULT_SELECTORS` 指错（例如指到站点标题）会把"崩溃"测成"3 ms 可见"。改版后若选择器失效，报告会显示"不可测（从不出现）"而不是误报成功——先核对选择器再下结论。
8. **⑤ 个人主页是 SPA 参考值，不是硬指标**：它的首屏数字包含"客户端取数 + 渲染"（`/users/**` 显式 `ssr:false`），和首页/文章页的 SSR 首屏不是同一种成本；不要拿它去判定 ADR-0019 的 `< 800 ms`，它只回答"SPA 页首屏要多久"。
9. **⑥ 看重定向链的链头状态码**：`page.goto` 会自动跟随 301，拿到的 `finalResponse` 是最终 `/search` 的 200；脚本从 `response.request().redirectedFrom()` 倒推整条链，取链头请求的响应状态码。若以后有人把 `/articles` 改成 client-side 跳转，链会变短、链头会是 `/articles` 的 200 → 判定 fail（这正是要抓的 SEO 退化）。

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
  probes/               收尾用的过渡/水合探针（默认不进 CI）
  test/                 自检用例（node --test）
  baselines/            基线存档（report.json + report.md）
```

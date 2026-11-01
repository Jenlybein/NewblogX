# ADR-0019：读者站渲染与缓存策略（SSR 边界、取数并行与两层缓存）

- 状态：已交付
- 日期：2026-11-01
- 来源：生产环境性能实测（[读者站性能实测](../notes/2026-11-01-reader-site-performance-probe.md)）· spec [#79](https://github.com/Jenlybein/NewblogX/issues/79) · 修订 [ADR-0012](./0012-neutral-glass-visual-system.md) 决策 8 / 约束 10 中的中文字体部分

## 背景与问题

用户反馈：读者站从首页点进文章、进搜索页、再切回首页"都很慢"，而后台各种切换都很快。生产环境实测（详见 [性能实测](../notes/2026-11-01-reader-site-performance-probe.md)）得到了一个**比"慢"更严重**的结论，以及三条结构性原因。

**0. 站内客户端跳转当前是坏的，不是慢（P0，独立缺陷）**。点击站内链接后 URL 在 24 ms 内就变了，但目标视图**从不出现**（15 秒后仍停在原页），并在 225 ms 抛未捕获异常 `TypeError: n.off is not a function`——滚动视差组合式在路由切换卸载时调用了 motion-v 的 MotionValue 上并不存在的 `off` 方法。该调用自提交 `2bd7931`（#23 滚动进场与视差）引入，属长期缺陷，与本 ADR 的取舍无关；但它意味着**切页耗时的真实基线目前无法测量**，必须先修复才能谈优化。它单独开票跟踪，不在本 ADR 的决策范围内（本 ADR 只规定"修好之后切页应该长什么样"）。

在此前提下，三条结构性原因：

1. **取数是串行水车**。首页 setup 里连续 `await useAsyncData`（banners → hot-top → 最新文章），文章页连续 `await`（正文 → 作者资料 → 评论 → 评分）。崩溃前实测：客户端导航一次打 **5 个 `/api/*`（全部 `fetch`）**，`articles/{id}` → `author_info` → `comments` → `users/base` 严格首尾相接，墙钟 271–348 ms；期间 0 个文档请求、0 个 `_payload`——**SSR 成果对站内点击不起作用**。
2. **没有跨导航复用**。`usePagedResourceCache` 每次初始化都 `loadPage(..., force: true)`；全站没有 `getCachedData` / `useNuxtData`。于是从文章返回首页，首页那 3 个接口照样重新走一遍。
3. **首屏扛着 2.4 MB 字体**。首页冷启动内容可见 **1644 ms**（中位）、总下载 2.9 MB / 66 请求；其中 Noto Sans SC 两个整包字重合计 **2,314,796 B，占子资源 77.7%**。全站 HTTP/1.1、同域 6 连接，字体与 CSS/JS 从 228 ms 起排队。

后台之所以快，不是因为它被优化过，而是**它的模型里没有"切页重新渲染文档 + 重新取数"**：全站 `ssr: false`，首屏 HTML 只有 2.3 KB、2 个预加载 chunk，切页只换组件并打 1–2 个接口。

因此本 ADR 要同时回答两个问题：**哪些页面值得付服务端渲染的代价**，以及**切页路径怎么在不牺牲分享/收录的前提下变快**。

## 决策

1. **渲染边界收窄为一张白名单**：读者站 SSR 只保留 **`/`** 与 **`/article/**`**；`/articles` 与 `/users/**` 新增 `ssr: false` 走 SPA；`/search`、`/studio/**` 维持 SPA；后台维持全 SPA。
   理由：SSR 真正买到的是"不执行 JS 的抓取器能看到内容"——文章页靠它生成分享卡片（微信/QQ 预览的 `og:*`，来自 `useSeoMeta`）与被搜索引擎收录；首页是站点门面。列表页与个人主页的抓取价值不足以抵消每次直达的服务端渲染代价，用户主页更是以登录后使用为主。
2. **中文字体不再自托管**：删除 Noto Sans SC 的 `@font-face`，中文回落到系统字体栈（`-apple-system`、`PingFang SC`、`Hiragino Sans GB`、`Microsoft YaHei`、`Noto Sans SC`、`sans-serif`，末位保留真装了 Noto 的机器）；Sora（品牌/字标/大标题）与 Inter（拉丁正文）保留自托管。首屏字体预算 **< 100 KB**。
   **本条取代 [ADR-0012](./0012-neutral-glass-visual-system.md) 决策 8 与约束 10 中"中文继续 Noto Sans SC"的部分**；其余字体分工（Sora = 字标/大标题、Inter = 正文/UI、全站无衬线）继续生效。
3. **取数并行化 + 骨架优先**：页面不得用阻塞式串行 `await` 链取数；正文优先渲染，作者资料、评论、评分等次要数据并行发起或延后；路由切换时先出骨架，不再用 Suspense 等"数据齐了才切页"。
4. **客户端缓存：30s SWR**。跨导航复用同一路由的数据（返回、重进命中），过期后后台静默刷新；缓存键含路由参数（文章 id、分页、搜索词），不做 per-user 持久化。
5. **服务端缓存：匿名 SSR 30s**。SSR 的 HTML **只允许包含匿名内容**——`is_digg`、收藏/关注状态等用户私有字段改为客户端获取；因此登录用户与匿名用户命中同一份缓存，不需要按 cookie 分流。
6. **验证口径**：把探测脚本整理进仓库，改前/改后各跑一次并对比；硬指标见「当前生效约束」第 7 条。

## 理由与已否决方案

- **全站改 SPA**：切页会立刻接近后台的体感，但微信/QQ 分享卡片会退化成"站点名"，搜索引擎也拿不到文章内容——用户明确会往微信群发链接，这条不可接受。
- **全站保留 SSR 并直接加服务端缓存**：文章页 SSR 里含 `is_digg`（"我有没有点过赞"），共享缓存会把 A 的状态发给 B。必须先解决"SSR 只输出匿名内容"，否则不能缓存。
- **只给未登录请求做服务端缓存**：登录用户拿不到收益，且要多一层 cookie 分流与两套缓存路径；相比之下"SSR 匿名化"更彻底。
- **只做客户端缓存**：切页/返回会变快，但分享链接的首字节（别人点开链接那一刻）完全不变。
- **保留 Noto Sans SC 全量字体**：2.3 MB，首屏预算不可能达标；只裁字重仍剩 1 MB 以上。
- **构建期字体子集化**：需要引入字体子集工具链，而中文字符集随文章内容变化，子集边界不稳定，收益与维护成本不成比。
- **引入 CDN / 边缘缓存**：当前没有 CDN，运维与成本代价远大于本阶段收益；留待将来（本 ADR 不排除）。
- **静态预渲染文章页**：文章内容动态、需要按 id 取数，且分享卡片要求服务端输出 `og:*`。
- **保留"数据齐了再切页"**：不闪，但总时长永远等于最慢的那个接口；与"骨架优先"相比是纯粹的体感损失。

## 代价 / 不做什么

- **内容最多滞后 30 秒**：两层缓存都取 30s，运营改完内容后读者可能短暂看到旧版本。
- **登录态数据会跳变**：点赞/收藏状态先按未登录渲染，客户端补齐后才显示出真实状态（这与"SSR 只输出匿名内容"是同一枚硬币的两面）。
- **中文字形变化**：从自托管 Noto 变为系统黑体（苹方/雅黑/思源），不同操作系统下字形略有差异；这是用"视觉一致性"换"2.3 MB"。ADR-0012 的排版立场因此在字体一项上被修订。
- **骨架屏改变观感**：点一下立刻跳页，但会先看到骨架而不是完整内容。
- **不做什么**：不改后端接口与 OpenAPI 契约；不改后台（它已经是对的模型）；不做 CDN / 边缘渲染；不做 ISR 持久化；不做实时推送/WebSocket 推送更新；不为列表页与个人主页保留 SSR。

## 当前生效约束

1. 读者站 SSR 白名单只有 `/` 与 `/article/**`；新增 SSR 页面必须改本 ADR，不得按页面随手加。
2. SSR 输出不得包含用户私有数据（点赞、收藏、关注、登录身份等）；用户态一律客户端获取。违反这条等于把共享缓存变成串号缓存。
3. 服务端缓存只缓存匿名内容，TTL 30s；**不得**引入按用户区分的缓存键（那会退化成 per-user 缓存并失去意义）。
4. 客户端缓存 TTL 30s；返回同一路由必须命中缓存，不得重新发起串行请求。
5. 首屏字体预算 < 100 KB；中文字体不得以自托管 webfont 形式加载；Sora/Inter 保留自托管。
6. 页面 setup 不得使用阻塞式串行 `await` 链取数（多于一个 `await useAsyncData` 时要么并行、要么走骨架）。
7. 验收指标由入库脚本测量：**切页内容可见 P95 < 300 ms、首屏内容可见 < 800 ms、首屏字体下载 < 100 KB**；改前基线见 [性能实测](../notes/2026-11-01-reader-site-performance-probe.md)。
8. 分享卡片是硬需求：文章页的 `og:*` 必须由服务端输出，任何改动导致微信/QQ 预览退化都视为回归。
9. 不改接口与 OpenAPI 契约；不改后台渲染模式。
10. 字体与渲染边界变更必须同步更新设计门禁：`frontend/test/design/fonts.test.ts`、`tokens.test.ts`、`foundation-tokens.test.ts`、`frontend/test/utils/readerTheme.test.ts` 里锁定"Noto 优先于系统字体"的断言锁的是旧立场，需按其新口径改写而不是删除断言。

## 相关代码与文档

- 渲染边界：[frontend/nuxt.config.ts](../../frontend/nuxt.config.ts)（`routeRules`）
- 字体：[fonts.css](../../frontend/app/assets/css/fonts.css) · [tokens.css](../../frontend/app/assets/css/tokens.css)（`--font-cjk` / `--font-sans`）· [readerTheme.ts](../../frontend/app/utils/readerTheme.ts) · [tailwind.config.ts](../../frontend/tailwind.config.ts)
- 取数与缓存：[index.vue](../../frontend/app/pages/index.vue) · [article/[id].vue](../../frontend/app/pages/article/[id].vue) · [usePagedResourceCache.ts](../../frontend/app/composables/usePagedResourceCache.ts) · `frontend/app/services/*`
- 字体门禁：[fonts.test.ts](../../frontend/test/design/fonts.test.ts) · [tokens.test.ts](../../frontend/test/design/tokens.test.ts) · [foundation-tokens.test.ts](../../frontend/test/design/foundation-tokens.test.ts) · [readerTheme.test.ts](../../frontend/test/utils/readerTheme.test.ts)
- 上游：[ADR-0012](./0012-neutral-glass-visual-system.md)（视觉系统；本 ADR 取代其决策 8 / 约束 10 的中文字体部分）· [ADR-0016](./0016-deco-segment-and-cursor-charm.md) / [ADR-0018](./0018-charm-frame-cap-and-pointer-offset.md)（挂件，与本 ADR 无交集）
- 实测与基线：[2026-11-01 读者站性能实测](../notes/2026-11-01-reader-site-performance-probe.md)

## 实施记录（2026-11-01）

决策与约束正文一字未改（本目录规则：已交付的 ADR 不改结论）。落地由分票 **#80（P0 站内跳转崩溃）· #81（性能尺子）· #82（字体瘦身）· #83（渲染边界收窄）· #84（取数并行 + 骨架）· #85（客户端 30s SWR）· #86（匿名 SSR 30s 缓存）** 完成，**#87 收尾**（改前/改后对比、四项硬指标判定、文档口径与门禁收口）。下面记录最终形态、实际数字、与提议的偏离和回退方式，供之后读本 ADR 的人对照代码。

### 一、逐条落地结果

1. **决策 1（渲染边界白名单）**：`routeRules` 只有四条显式声明——`/articles` 服务端 301 → `/search`、`/users/**` 与 `/search` 与 `/studio/**` 为 `ssr: false`；SSR 白名单 `/` 与 `/article/**` 由 [rendering-boundary.test.ts](../../frontend/test/gates/rendering-boundary.test.ts) 双向守着（`app/pages` 下每个页面文件都必须落进 SSR / SPA / 301 三类之一，新增页面不分类即红）。落地数字：`/users/**` 首屏 HTML 40,757 → 1,503 B（#83）。
   **偏离**：`/articles` 最初按票面写成 `ssr: false`，#83 复盘中改判为**服务端 301**——`ssr: false` 会让爬虫/无 JS 客户端拿到 200 空壳（SEO 退化的另一种形态），而 301 在渲染前完成、不产生 SSR 成本。门禁从"非白名单不许开 SSR"扩成"重定向路由必须 301"。
2. **决策 2（中文字体不自托管）**：删除 Noto Sans SC 两个 `@font-face`；`--font-cjk` 改为系统字体栈（`-apple-system` / `PingFang SC` / `Hiragino Sans GB` / `Microsoft YaHei` / `Noto Sans SC` / `sans-serif`，末位 Noto 只是"系统装了就有"的名字、不再下载）；Sora / Inter 继续自托管，首屏只预加载这 4 个 woff2。**实测首屏字体下载 2,407,904 B → 93,108 B（中文字体 0 B），预算 102,400 B，达标**（#82）。
   **门禁改写**：[fonts.test.ts](../../frontend/test/design/fonts.test.ts)（零 CJK `@font-face` + preload 字节 < 100 KB）、[tokens.test.ts](../../frontend/test/design/tokens.test.ts)、[foundation-tokens.test.ts](../../frontend/test/design/foundation-tokens.test.ts)、[readerTheme.test.ts](../../frontend/test/utils/readerTheme.test.ts) 里锁定"Noto 排最前"的断言按新口径改写（系统栈顺序、Sora/Inter 保留），未删除断言。
3. **决策 3（取数并行 + 骨架优先）**：[useHomePageSources.ts](../../frontend/app/composables/useHomePageSources.ts) 三个数据源先全部发起、再按渲染环境决定等谁（SSR 三个都等；客户端跳转一个都不等）；[useArticlePageSources.ts](../../frontend/app/composables/useArticlePageSources.ts) 正文/评论/评分第一波并行，作者资料依赖正文 `author_id` 作第二波；新增 [LoadingSkeleton.vue](../../frontend/app/components/common/LoadingSkeleton.vue) 与 [ArticlePageSkeleton.vue](../../frontend/app/components/article/ArticlePageSkeleton.vue)（骨架可观察锚点 `feed-skeleton` / `article-page-skeleton`）；[readerRenderMode.ts](../../frontend/app/utils/readerRenderMode.ts) 区分 `server` / `hydration` / `navigation`。`usePagedResourceCache` 增加 `initialLoad: block | background`（默认 `block` 保持旧语义）与 `ready` 信号。实测客户端跳转首波 3 个请求在 0–1 ms 内并行（#84）。
4. **决策 4（客户端 30s SWR）**：[readerSwrCache.ts](../../frontend/app/utils/readerSwrCache.ts)（TTL `30_000`、`fresh` / `stale` 两态、时钟可注入）+ [useReaderSwrAsyncData.ts](../../frontend/app/composables/useReaderSwrAsyncData.ts)（`getCachedData` 命中即不跑 handler；stale 先渲染再后台 `refresh()` 不闪；服务端完全不碰客户端缓存；首屏 hydration 用 SSR payload 预热）。缓存键含路由参数（文章 id / 分页 / 搜索词）；写缓存前用 [readerPrivateFields.ts](../../frontend/app/utils/readerPrivateFields.ts) 剔除 `is_digg` / `is_favor` / `relation` 等私有字段（#85）。
5. **决策 5（匿名 SSR 30s 缓存）**：[ssrAnonymousCache.ts](../../frontend/server/utils/ssrAnonymousCache.ts) + [Nitro 插件](../../frontend/server/plugins/ssrAnonymousCache.ts)；SSR 取数匿名化（文章详情走 `getArticleDetailAnonymous`，点赞/收藏/关注交客户端）。
   **偏离（重要）**：**没有用 `routeRules` 的 `swr`**。Nitro 只给"注册 handler 的路由"套 `cachedEventHandler`，而 Nuxt 渲染器注册在通配路由 `/**`（构建产物里是 `{ route: '/**', handler: renderer }`），永远匹配不上 `/` 与 `/article/**`——在 node-server 预设下写 `swr` 不会生效，还会给出"缓存已开"的假信号。改为在 Nitro 层做最小实现：`render:before` 命中直接返回缓存的匿名 HTML（跳过整次渲染与服务端取数），`render:response` 未命中时按 URL 存 30 s。缓存键只有 `pathname + search`（cookie / Authorization / UA 在类型上就传不进来），只存 200 + `text/html` + 无 `Set-Cookie` 的响应，内存表上限 500 条（过期条目读时清理，最老条目在写满时淘汰），响应头 `x-ssr-cache: HIT/MISS` 供验收与排障。门禁 [ssr-anonymous-cache.test.ts](../../frontend/test/gates/ssr-anonymous-cache.test.ts) / [ssr-anonymity.test.ts](../../frontend/test/gates/ssr-anonymity.test.ts) 锁 TTL、路径、键与"SSR 输出不含用户态"（#86）。
   **实测（本地构建，curl，2026-11-01）**：首页 TTFB 200.5 ms（MISS）→ 1.5 ms（HIT）；文章页 221.3 ms → 1.9 ms（#86 自测口径为首页 107.3 → 2.1 ms、文章 735.9 → 2.8 ms；差异来自本机与当时上游状态）。
6. **决策 6（验证口径）**：探测脚本整理入库为 [scripts/perf](../../scripts/perf/README.md)（Playwright，依赖单独声明、**默认不进 CI**），改前/改后各跑一次，报告存档在 `scripts/perf/baselines/`。

### 二、硬指标（改前 vs 改后，同脚本同口径：1440×900，冷/热各 3 次取中位/最差）

改前有两份参照：**生产基线**（[`2026-11-01-pre-p0`](../../scripts/perf/baselines/2026-11-01-pre-p0/report.md)，生产站旧构建、含 P0 崩溃）与**本地改前构建**（[`2026-11-01-pre-local`](../../scripts/perf/baselines/2026-11-01-pre-local/report.md)，`main` `5530bb8` 的生产构建，与改后同一机器 / 同一脚本 / 同一 API 上游）。生产站当时仍是旧构建，改后无法在生产上直接测，故对比表以"本地改前 vs 本地改后"为主、生产基线作第二参照。改后见 [`2026-11-01-post-local`](../../scripts/perf/baselines/2026-11-01-post-local/report.md)。

| 指标（口径） | 改前·生产基线（中位/最差） | 改前·本地构建（中位/最差） | 改后·本地构建（中位/最差） | 预算 | 判定 |
| --- | --- | --- | --- | --- | --- |
| 首屏内容可见 · 首页冷 | 1588 / 1998 ms | 515 / 962 ms | **314 / 435 ms** | < 800 ms | PASS |
| 首屏内容可见 · 首页热 | 345 / 620 ms | 378 / 440 ms | **161 / 172 ms** | < 800 ms | PASS |
| 首屏内容可见 · 文章页冷 | 2318 / 2471 ms | 583 / 1375 ms | **298 / 592 ms** | < 800 ms | PASS |
| 首屏内容可见 · 文章页热 | 276 / 292 ms | 446 / 448 ms | **167 / 184 ms** | < 800 ms | PASS |
| 切页内容可见 · 首页→文章冷 | 不可测（从不出现） | 不可测（从不出现） | **226 / 230 ms** | P95 < 300 ms（3 次最差作保守代理） | PASS |
| 切页内容可见 · 首页→文章热 | 不可测（从不出现） | 不可测（从不出现） | **232 / 233 ms** | 同上 | PASS |
| 首屏字体下载 · 首页冷 | 2,407,904 B | 2,407,904 B | **93,108 B**（中文 0 B） | < 102,400 B | PASS |
| 返回上一页命中缓存（数据请求数 / 耗时） | 未命中（文档级重载） | 未命中（3 请求 / 194 ms，冷） | **命中（0 请求 / 0 ms，SPA 返回）** | 0 请求或 < 50 ms | PASS |
| 首屏总下载（首页冷，中位） | 2,978,885 B | 3,941,461 B | **1,634,564 B** | —（参考） | 下降 58%（对生产基线） |

四点说明：① 生产基线与本地改前的差异来自网络路径（生产站 HTTP/1.1、公网往返；本地直连），"生产基线"用于说明用户体感，`本地改前 → 本地改后` 才是同口径对比。② 改前本地构建的首屏冷存在一次抖动（首页 962 ms、文章页 1375 ms），判定按"3 次最差"的口径记不达标；改后最差已全部落在预算内。③ 切页在改前的正确记录是"不可测"——P0 崩溃导致目标视图从不出现，不是"很慢"（见实测笔记）。④ 返回场景做了**口径修订**（见下节第 2 条），改前那一行是 P0 时代"进文章退化为文档级 goto"的兜底形态。

### 三、与提议的偏离与调整

1. **`routeRules.swr` → Nitro render 钩子**（上面第 5 条已详述）。表现层看，仍然是"匿名 SSR 30s、键只含 URL、不按用户分流"，但落点是 Nitro 钩子而不是 Nitro 的路由缓存。
2. **页面过渡时长 200 ms → 120 ms（#87 收尾调档）**。先纠正一个前提：ADR-0012 的三档预算是 `120 / 200 / 320ms`（`MOTION_DURATIONS_MS` 保留这三档；CSS 侧现存 `--motion-fast` 与 `--motion-base` 两个变量），**路由过渡本来用的是 base 档 200 ms，不是 320 ms**。`mode: 'out-in'` 下旧页要先完整离场才挂新页，这段离场是用户可感知的固定开销。为量化它，#87 在同一构建上用探针（[transition-probe.mjs](../../scripts/perf/probes/transition-probe.mjs)，默认不进 CI）把 `.page-enter-active/.page-leave-active` 的过渡时长临时改成 0 再测：
   - 过渡 200 ms：点击→URL 24–32 ms，点击→正文可见 **291–318 ms**；
   - 过渡关闭：点击→正文可见 **120–171 ms**；
   - 差值 ≈ 170–180 ms ≈ 离场时长。取数不是这段的主要成本（骨架/正文都在 120–170 ms 内到达）。
   因此按 ADR-0012 允许的三档把路由过渡下调到 **fast 档 120 ms**（[tailwind.css](../../frontend/app/assets/css/tailwind.css) 的 `.page-enter-active, .page-leave-active` 由 `var(--motion-base)` 改为 `var(--motion-fast)`）；档位仍在 120 / 200 / 320 内，`mode: 'out-in'`、只动 `opacity`/`transform`、`prefers-reduced-motion` 直接关掉的立场都不变。设计门禁 [motion-foundation.test.ts](../../frontend/test/design/motion-foundation.test.ts) 同步改判为断言 `var(--motion-fast)` 且不再出现 `var(--motion-base)`（**改写，不是删除**）。实测切页内容可见从 302 / 305（冷）、299 / 308（热）（改后第一版存档 [`2026-11-01-post-local-200ms`](../../scripts/perf/baselines/2026-11-01-post-local-200ms/report.md)）降到 226 / 230、232 / 233 ms，达标。**切页 P95 < 300 ms 这一项的达标依赖本调档**：不调时最差 305 ms、已经压线超标（复审补记，口径与回退方式不变）。
   **回退方式**：把该规则的 `var(--motion-fast)` 改回 `var(--motion-base)`，并把 `motion-foundation.test.ts` 的断言改回 base（或另开 ADR 记录新的档位）；不涉及缓存/取数代码。
3. **SSR 匿名取数**：文章详情的 SSR 只走匿名接口（不传 token），`is_digg` / `is_favor` / `relation` 由客户端在 hydration 后补齐；登录用户与匿名用户共享同一份缓存，不做按 cookie 分流。
4. **首屏字体预加载**：只保留 Sora ×3 + Inter ×1（合计 93,108 B）；中文不再产生任何 webfont 下载。
5. **首屏 hydration 预热客户端缓存**：hydration 时把 SSR payload（剔除私有字段后）预热进 30 s SWR，这样"首页→文章→返回首页"第一次返回就能命中（#85 在票面之外的实现补充，目的是让决策 4 的第一天就生效）。
6. **后台字体一并删除 Noto（复审补记）**：`frontend_admin` 的 `fonts.css` 与 `readerTheme` 里的字体栈也一并删掉 Noto Sans SC 的自托管声明。理由：两端 `tokens.css` 必须**逐字节同源**（`scripts/check-token-parity.mjs`），而 `--font-cjk` 已改成系统字体栈；后台若还保留 2.3 MB 的自托管 Noto，它既没有任何消费点，又会让两端字体口径漂移。后台没有自己的字体预算，故本项不改变任何验收指标；回退方式与读者站同一条（恢复 `@font-face` 与系统栈定义），但必须两端同步、走同源门禁。

### 四、门禁与回归（#87 收口）

新增/改写门禁全部在 `frontend` 的 `pnpm exec vitest run` 全量里（`vitest.config.ts` 的 `include: test/**/*.{test,spec}.ts`），CI 的 `frontend-test` job 跑 `pnpm test:coverage` 即覆盖，**没有新增 CI 接线**：
- [rendering-boundary.test.ts](../../frontend/test/gates/rendering-boundary.test.ts)：SSR 白名单双向 + 每个页面归类 + `/articles` 301；
- [ssr-anonymous-cache.test.ts](../../frontend/test/gates/ssr-anonymous-cache.test.ts)：TTL 30 s、键不随请求身份变化、非白名单路由不进缓存；
- [ssr-anonymity.test.ts](../../frontend/test/gates/ssr-anonymity.test.ts)：SSR 输出不得含用户私有字段；
- [fonts.test.ts](../../frontend/test/design/fonts.test.ts)：零 CJK `@font-face` + 首屏字体 ≤ 102400 B；
- [motion-foundation.test.ts](../../frontend/test/design/motion-foundation.test.ts)：路由过渡使用 fast 档。
- **`scripts/perf` 不进 CI**：要打真实站点/本地构建、耗时几分钟、结果受网络抖动影响；依赖只在本目录的 `package.json`，不进 `frontend` 依赖树。
- 收尾实跑（#87）：`frontend` 与 `frontend_admin` 全量测试绿、两端 `eslint` 绿、`node scripts/check-token-parity.mjs` 绿、`node scripts/check-design-tokens.mjs frontend frontend_admin` 绿（数字见 [实测笔记「改后复测」](../notes/2026-11-01-reader-site-performance-probe.md)）。

### 五、实测笔记与遗留

改后复测、过渡拆解与分享卡片验收见 [性能实测 · 改后复测](../notes/2026-11-01-reader-site-performance-probe.md)。已知遗留：
- **首页仍有 `Hydration completed but contains mismatches`**（文章页无）：属 spec #79 的"Out of Scope、独立观察项"；P0 修复与本轮改造后仍存在，**建议另开票**，不属本 ADR 范围。
- **返回场景的测量口径已修订**（#87）：无头 Chromium 在"先 `page.goto` 进文章再返回"的语义下必然是文档级重载，测不到客户端缓存；脚本改为真实用户路径（首页点击进文章 → 浏览器返回），P0 未修时保留"进文章退化为文档级 `goto`"的兜底（`entryFlow=document`）。改前基线就是兜底形态，见 [scripts/perf/README](../../scripts/perf/README.md) 的口径说明。
- 30 s 内容滞后、登录态"先默认后跳变"、骨架屏观感变化、SSR 缓存是**进程内内存表**（重启即空、多实例不共享，多实例部署时每台各自 30 s）——均为「代价 / 不做什么」里已接受的代价，本阶段不处理。

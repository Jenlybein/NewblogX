# ADR-0019：读者站渲染与缓存策略（SSR 边界、取数并行与两层缓存）

- 状态：提议
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

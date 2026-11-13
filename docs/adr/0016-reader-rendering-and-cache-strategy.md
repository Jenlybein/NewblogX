# ADR-0016：读者站渲染、缓存、用户态与计数真相

- 状态：已交付
- 日期：2026-11-01
- 来源：生产环境性能实测（出处：2026-11-01 读者站性能实测，纯文本记录）· 读者反馈「点赞后刷新失效、按钮不显示已赞、计数要等 30 秒」· spec [#79](https://github.com/Jenlybein/NewblogX/issues/79) · [#234](https://github.com/Jenlybein/NewblogX/issues/234) · [#272](https://github.com/Jenlybein/NewblogX/issues/272) · 上游视觉约束 [ADR-0011](./0011-neutral-glass-visual-system.md)

## 背景与问题

读者站从首页点进文章、进搜索页、再切回首页的体感明显慢于后台。生产环境实测给出三条结构性原因：

1. **取数是串行水车**。首页 setup 里连续 `await useAsyncData`（banners → hot-top → 最新文章），文章页连续 `await`（正文 → 作者资料 → 评论 → 评分）；一次客户端导航里 `articles/{id}` → `author_info` → `comments` → `users/base` 严格首尾相接，墙钟时间由最慢的接口决定，SSR 成果对站内点击不起作用。
2. **没有跨导航复用**。每次初始化都强制重新拉第一页，全站没有 `getCachedData` / `useNuxtData`；从文章返回首页，首页那几个接口照样重走一遍。
3. **首屏扛着中文字体整包**。自托管 Noto Sans SC 两个整包字重远超 100 KB 预算，字体与 CSS/JS 在同域连接上排队，首屏内容可见被拖长。

后台之所以快，不是因为它被优化过，而是**它的模型里没有"切页重新渲染文档 + 重新取数"**：全站 `ssr: false`，首屏 HTML 很小，切页只换组件并打一两个接口。

因此本 ADR 要同时回答两个问题：**哪些页面值得付服务端渲染的代价**，以及**切页路径怎么在不牺牲分享/收录的前提下变快**。

用户态与计数真相是同一条链路上的第二组问题。文章计数不是 `COUNT(关系行)` 直接算出来的，而是一本独立维护的账：持久值是 `article_models` 的计数列（并账已入账的主值），实时差是 Redis 哈希里尚未入账的增量，由一条调度任务并进库。点赞、收藏这些计数是**派生值**，曾经存在三处会同时出错的接缝：

- hydration 把**匿名** payload 预热进客户端 30 秒 SWR 缓存后，若任何 `cause` 的 fresh 命中都短路 `refresh()`，登录用户页面 `onMounted` 的私有态补齐请求就一个都不会发，硬刷新后按钮永远显示「未赞」；而旧的 toggle 接口会把「显示未赞」变成一次真实的取消点赞，这是数据损坏而不只是显示问题。
- 写路径只把增量写进账本、响应不返回最新计数，前端只能等下一次 30 秒刷新才看到变化。
- 读路径曾有两套口径并存：一套「每个读点各自把 Redis 增量加到聚合上」，一套「卡片只读库里的列」。结果同一篇文章在同一时刻可以有两个数（首页卡片一个值、详情另一个值），差异恰好出现在"有增量还没并账"、用户最容易看到的时间段。并账周期还写死在代码里，想调就要发版。

## 决策

1. **渲染边界收窄为一张白名单**：读者站 SSR 只保留 **`/`** 与 **`/article/**`**；`/articles` 走**服务端 301 → `/search`**（在渲染前完成、不产生 SSR 成本，也不会给爬虫一个 200 空壳），`/users/**`、`/search`、`/studio/**` 为 `ssr: false` 走 SPA，后台维持全 SPA。理由：SSR 真正买到的是"不执行 JS 的抓取器能看到内容"——文章页靠它生成分享卡片（微信/QQ 预览的 `og:*`，来自 `useSeoMeta`）与被搜索引擎收录；首页是站点门面。列表页与个人主页的抓取价值不足以抵消每次直达的服务端渲染代价。每个页面文件必须落入 SSR / SPA / 301 三类之一，新增页面不分类即红（[rendering-boundary.test.ts](../../frontend/test/gates/rendering-boundary.test.ts) 双向门禁）。
2. **中文字体使用系统字体栈**：`-apple-system` / `PingFang SC` / `Hiragino Sans GB` / `Microsoft YaHei` / `Noto Sans SC` / `sans-serif`（末位只是"系统装了就有"的名字、不再下载）；Sora（品牌/字标/大标题）与 Inter（拉丁正文）保留自托管，首屏只预加载这 4 个 woff2，首屏字体预算 **< 100 KB**。两端 `tokens.css` 必须逐字节同源，后台字体同样删除 Noto 的自托管声明。[ADR-0011](./0011-neutral-glass-visual-system.md) 的其余字体分工（Sora = 字标/大标题、Inter = 正文/UI、全站无衬线）与动效预算继续生效。
3. **取数并行化 + 骨架优先**：页面不得用阻塞式串行 `await` 链取数；正文优先渲染，作者资料、评论、评分等次要数据并行发起或延后（作者资料依赖正文 `author_id` 作第二波）；路由切换时先出骨架，不再用 Suspense 等"数据齐了才切页"。路由过渡按 [ADR-0011](./0011-neutral-glass-visual-system.md) 允许的三档由 200ms 的 base 档下调到 **120ms 的 fast 档**，`mode: 'out-in'`、只动 `opacity` / `transform`、`prefers-reduced-motion` 直接关掉。理由：`out-in` 下旧页要先完整离场才挂新页，这段离场是用户可感知的固定开销，取数不是这段的主要成本。
4. **客户端缓存：30s SWR**。跨导航复用同一路由的数据（返回、重进命中），过期后后台静默刷新；`fresh` / `stale` 两态由可注入时钟判定；缓存键含路由参数（文章 id、分页、搜索词），不做 per-user 持久化；写缓存前剔除 `is_digg` / `is_favor` / `relation` 等私有字段；首屏 hydration 用剔除私有字段后的 SSR payload 预热，让"首页→文章→返回首页"第一次返回就能命中。`useReaderSwrAsyncData` **只允许 `cause === "initial"` 复用 fresh 缓存**；手工 `refresh()` / 钩子刷新 / `watch` 一律返回 `undefined`、让 handler 真发请求（stale 仍先渲染旧值、后台静默重取）。「30 秒内重进请求数为 0」的验收由内部第二波初始化走 `refreshIfNeeded`（fresh 直接落数据、不发请求）承担，不靠把 `refresh()` 也钉成缓存命中。
5. **服务端缓存：匿名 SSR 30s**。SSR 的 HTML **只允许包含匿名内容**——`is_digg`、收藏/关注状态等用户私有字段改为客户端获取，文章详情的 SSR 只走匿名接口（不传 token）；登录用户与匿名用户命中同一份缓存，不按 cookie 分流。`routeRules.swr` 在 node-server 预设下**不生效**（Nitro 只给"注册 handler 的路由"套 `cachedEventHandler`，而 Nuxt 渲染器注册在通配路由 `/**`，永远匹配不上 `/` 与 `/article/**`，还会给出"缓存已开"的假信号），改由 **Nitro 钩子**实现：`render:before` 命中直接返回缓存的匿名 HTML（跳过整次渲染与服务端取数），`render:response` 未命中时按 URL 存 30s。缓存键只有 `pathname + search`（cookie / Authorization / UA 在类型上就传不进来），只存 200 + `text/html` + 无 `Set-Cookie` 的响应，内存表上限 500 条（过期条目读时清理、写满时淘汰最老条目），响应头 `x-ssr-cache: HIT/MISS` 供验收与排障。
6. **用户私有态独立取数**：新增 `article-private-<id>` 源，`server: false` + `getCachedData: () => undefined` + `immediate: false` + 带鉴权取数，只投影四字段 `{is_digg, is_favor, digg_count, favor_count}`（详情接口不返回 `relation`，作者关系由 `authorProfile` 源负责）。它**永不进 SSR payload、永不进客户端 30 秒 SWR 缓存**；权威计数随私有态一起覆盖本次渲染、不写回缓存。登录初始化完成后显式触发一次，登录态 `false→true`（页内登录成功）时再补一次，两者共用单飞闸门（一次登录只发一次）；用户已手动操作过点赞（`articleDiggTouched`）时不覆盖乐观结果。
7. **点赞改幂等目标态**：`PUT /articles/{id}/digg` 请求体必填 `{liked: boolean}`，服务端按目标收敛；重复同态即无操作（不改关系行、不改计数、不发通知），响应仍返回当前权威 `is_digg` / `digg_count`。前端传「目标态 = 当前状态取反」，不再发 toggle。理由：显示未赞但服务端其实已赞的窗口一旦存在，toggle 就会取消真实点赞，数据损坏必须从接口语义上消除。
8. **写响应回填权威计数 + 主动失效，统一收进阅读页取数模块** `useArticlePageSources.ts` 的 `reportArticleWrite({kind, result})`：写响应回填权威计数与私有态（覆盖本次渲染）、空响应的判空防御、`readerArticleCacheKey(id)` 的键拼接、`readerSwrCache.delete(...)` 主动失效客户端 30 秒 SWR 缓存的正文键，四件事只此一处。`digg` 返回 `{is_digg, digg_count}`，收藏添加返回 `{is_favor, favor_count}`，批量移除返回 `{is_favor, favor_count, removed_count, failed_count}`（批量接口要能让调用方分辨部分失败，全部失败返回非 0 业务码）。计数是「库内快照 + 当前账本增量」的绝对值，**不是这次 ±1**；不得用本地 ±1 冒充权威值。页面 `[id].vue` 只上报「读者做了什么 + 写响应」，**不再 import `readerSwrCache` 单例或 `readerArticleCacheKey`**；写响应为空时同样失效、但不 patch、不造脏值。**只失效客户端 30 秒 SWR 缓存**；匿名服务端渲染缓存、私有态独立源、首页与评论分页缓存、渲染边界一律不动，也不做跨进程 purge（多实例下 Nitro 缓存是进程内 Map）。
9. **收藏移除走 `DELETE /articles/favorite/contents`**，`POST /articles/favorite` 只用于添加；取消收藏只删目标收藏夹，`is_favor` 统一为「是否还在任一收藏夹」；收藏计数同样走写响应回填。
10. **计数是派生值，按关系行重算**：并账时 `digg` / `favor` 必须用 `COUNT(关系行)` **覆盖整列**；`view` / `comment` 没有等价关系行，仍是 `列 += 增量`。并账瞬间同一次事件可能被「COUNT」与「增量」各算一次，不累积（下一轮覆盖即自愈）。
11. **并账周期是运行期配置**：唯一来源是 `site.reconcile.interval_minutes`（单位分钟，默认 30，缺省/负数回落 30，上限一天），换算只有一处 `site.Reconcile.Interval()`，代码里不再有第二份默认周期常量。它是**调度注册参数**：改完要重启才生效（换周期必须重新注册任务），不假装热更新。理由：它是扫表的运维动作，秒级周期只放大写放大。
12. **仍只有一个并账调度任务**：文章计数刷盘（增量驱动，四个指标共享一个任务体与一把锁）与私信未读重算（全量重算，私信没有 Redis 增量源）跑在**同一个 tick 的同一个任务体**里；一个失败只记日志并继续另一个，最后把错误交给调度器记一条失败——失败彼此隔离，不新增调度器，也不拆成两个 tick。
13. **读路径只有唯一权威入口 `app.CountLedger`**：权威值 = 持久列 + 待并账增量；增量只从 `CounterPort` 读（`CountLedger.Pending`），合成只发生在账本（`CountLedger.Authoritative`），所有对外可见的计数都经它写回：详情、列表、点赞/收藏写响应、收藏内容列表、首页热门置顶卡片、作者置顶卡片。首页卡片与详情不得各说一个数。前端可见的计数**永不等并账**，刷新后的权威值来自写响应回填或私有态取数。
14. **号段与派生主键收进具名模块**：私信「幂等键 → 消息主键」的派生规则收进 `backend/internal/chat/domain/msgid`，号段与哈希算法都是该模块的**实现细节**（不再导出常量）。由此明确：**排序、水位、分页一律不读 ID 大小**——派生主键的大小与消息先后无关，ID 只作同一时刻的确定性并列裁决与扫描游标；消息先后只由 `send_time` 决定。私信未读的可见集 / 未读数 / 清空界线由 chat 模块的唯一实现回答，本 ADR 只规定它必须与文章计数并账共用同一个 tick。
15. **门禁改为行为层**：真跑 `useArticlePageSources`，一条行为用例覆盖点赞 / 收藏 / 写响应为空三种情形，验「返回重进读到权威数字、私有字段不进客户端 30 秒 SWR 缓存、缓存里不留脏值」。**不再断言源码里是否出现删除动作**，也不新增同类源码文本断言；私有态独立源自身的 `server:false` / 恒 undefined `getCachedData` 等结构判据与 canary 保留。理由：删除动作的位置是实现细节，从页面挪进取数模块、或换一种失效写法，行为没变却会误报红。
16. **验证口径**：切页内容可见 P95 < 300 ms、首屏内容可见 < 800 ms、首屏字体下载 < 100 KB，由仓库内 Playwright 探测脚本按同口径测量（依赖单独声明、默认不进 CI）；分享卡片是硬需求，文章页的 `og:*` 必须由服务端输出，任何改动导致微信/QQ 预览退化都视为回归。渲染与缓存层不新增接口；互动写接口的请求体与响应字段按决策 7–9 执行，契约产物同批重生成（[ADR-0003](./0003-generated-contract-single-source.md)）。

## 理由与已否决方案

- **全站改 SPA**：切页会立刻接近后台的体感，但微信/QQ 分享卡片会退化成"站点名"，搜索引擎也拿不到文章内容——用户明确会往微信群发链接，这条不可接受。已否决。
- **全站保留 SSR 并直接加服务端缓存**：文章页 SSR 里含 `is_digg`（"我有没有点过赞"），共享缓存会把 A 的状态发给 B。必须先解决"SSR 只输出匿名内容"，否则不能缓存。已否决。
- **只给未登录请求做服务端缓存 / 按用户区分缓存键**：登录用户拿不到收益，且退化成 per-user 缓存，命中率与"登录共享"的收益一起消失。已否决。
- **只做客户端缓存 / 保留"数据齐了再切页"**：前者不改分享链接的首字节，后者总时长永远等于最慢的那个接口。已否决。
- **保留 Noto Sans SC 全量字体 / 构建期字体子集化**：全量远超首屏预算；子集化要引入字体子集工具链，中文字符集随文章内容变化、子集边界不稳定。已否决。
- **引入 CDN / 边缘缓存 / 静态预渲染文章页**：当前没有 CDN，运维与成本代价远大于本阶段收益；文章内容动态、需要按 id 取数，且分享卡片要求服务端输出 `og:*`。已否决（本 ADR 不排除将来引入 CDN）。
- **用 `routeRules.swr` 做服务端缓存**：在 node-server 预设下对 Nuxt 渲染器的通配路由永远不生效，还会给出"缓存已开"的假信号。已否决；改由 Nitro 钩子实现。
- **只修按钮显示（不换接口语义）**：显示未赞与服务端其实已赞之间永远存在窗口，只要窗口在，toggle 就还会取消真实点赞。已否决。
- **把点赞态放回 SSR payload**：匿名服务端渲染缓存会变成串号缓存，登录用户与匿名用户也不能再共用一份 HTML。已否决。
- **点赞用「先 GET 详情再决定发 POST 还是 DELETE」 / 前端本地 ±1 回填计数**：前者多一次往返且两请求之间状态仍可能变，后者在并发、重复点击、他人同时点赞时都会让本地值与库内值漂移。已否决；目标态接口原子按目标收敛天然幂等，写响应回填是唯一权威来源。
- **继续让页面自己 `delete('article-<id>')` / 把所有缓存模块合成一个「统一缓存」**：前者每加一种写动作就要重复一遍 patch + 拼键 + 失效，漏一处就留下 30 秒脏值，且页面必须 import 缓存单例与键函数；后者为整齐合并会顺带动到首页、评论分页与渲染边界。已否决。
- **断言源码里出现 `delete(...)`**：删除动作的位置是实现细节，行为没变却会误报红。已否决；改为行为层用例。
- **收藏移除时给 `Favorite` 加「已存在就转 removes」的兜底**：把误发 POST 的缺陷藏进后端语义，会让 POST 的幂等性变得不可推理。已否决；前端发对的动词。
- **全局 purge Nitro 匿名缓存 / 把 30 秒 TTL 调小甚至去掉**：多实例进程内 Map 无法跨进程失效，只做单实例可用的假能力比不做更糟；调 TTL 会同时推翻切页/返回的性能取舍，计数滞后用写响应回填 + 主动失效解决。已否决。
- **保持 ±1 并账、另加每日全量对账 / 只给 CLI 或后台按钮手动对账**：前者账本仍可能错 24 小时且多一条任务，后者没人会去点，等于"有人发现才修"。已否决。
- **本批直接上 CDC 投影 / 读路径改成每次 `COUNT`**：CDC 链路准但票数与周期翻倍，与"先修可见缺陷"的目标不符，属**非目标**；每次 `COUNT` 会让列表页 20 篇文章打 20 次 COUNT，且「按点赞数排序」失去廉价路径。已否决。
- **把并账周期做成秒级 / 新增第二个调度器 / 周期放进新配置段**：秒级只放大写放大；拆调度器会让"文章计数失败挡住私信未读"重新变成可能；运行期配置的段清单不含"并账"专属段，`site` 段本来就是自由键值块（深度合并会把老库缺的键按默认值补齐），不新开段、不加表、不做数据迁移。已否决。
- **让每个读面继续各自合并增量，只靠 code review 对齐 / 改并账频率来"让数字更快准"**：前者正是要修的病（卡片路径甚至不在同一个合并函数里），后者解决不了任何可见问题、只增写放大。已否决。

## 代价 / 不做什么

- **内容最多滞后 30 秒**：两层缓存都取 30s，运营改完内容后读者可能短暂看到旧版本；跨用户看到的计数仍可能滞后 30 秒（不 purge 共享 HTML 缓存是明确取舍），作者自己的写操作立即正确。
- **登录态数据会跳变**：点赞/收藏状态先按未登录渲染，客户端补齐后才显示出真实状态（这与"SSR 只输出匿名内容"是同一枚硬币的两面）；登录用户每次进文章多一次带鉴权请求，匿名用户零成本。
- **中文字形变化 / 骨架屏改变观感**：从自托管 Noto 变为系统黑体（苹方/雅黑/思源），不同操作系统下字形略有差异；点一下立刻跳页，但会先看到骨架而不是完整内容；路由过渡 120ms 是用户可感知的固定开销。
- **SSR 缓存是进程内内存表**：重启即空、多实例不共享，多实例部署时每台各自 30s。
- **每次并账多做一次 `COUNT`**：只对「这一批有增量的文章 id」做，`article_id` 有索引、批内通常几十条，可忽略；置顶卡片每页多一次 Redis 批量读（`HMGET` 管道即可）换来"首页与详情同一时刻同一个数"。
- **改并账周期要重启**：它是调度注册参数，不做热重载（热重载需要"监听配置 + 重建任务"的第二套机制，收益不抵复杂度）。
- **计数不能再有"非关系行来源"**：一旦引入迁移来的历史点赞数（不落关系行），`COUNT` 覆盖就会把它们抹掉。
- **开放接口契约**：`PUT /digg` 的请求体必填 `liked`，旧式空请求体收到 400（`binding:"required"`），不会静默按 `false` 取消点赞；响应新增 `digg_count` / `favor_count` 对旧前端是纯增量。批量移除收藏的 `is_favor` / `favor_count` 按「最后一条成功移除的文章」返回，只有单篇消费方该依赖；批量场景另给 `removed_count` / `failed_count`。写响应为空只失效、不回填（宁可让下一次取数回答，也不本地猜一个 ±1）。零 DDL、本批无新表无 ALTER。
- **字体与渲染边界变更必须同步更新设计门禁**：`fonts.test.ts`、`tokens.test.ts`、`foundation-tokens.test.ts`、`readerTheme.test.ts`、`motion-foundation.test.ts` 等断言按现行口径改写而不是删除。
- **不做什么**：不改后台渲染模式；不做 CDN / 边缘渲染；不做 ISR 持久化；不做实时推送 / WebSocket 推送计数；不做点赞状态的服务端缓存；不做 per-user 缓存键、跨进程缓存失效；不把收藏改成 toggle；不为整齐把所有缓存合成一个模块；不把并账拆成两个调度器、不把周期做成秒级、不给任何既有表加列；**CDC 实时并账为非目标**。

## 当前生效约束

1. 读者站 SSR 白名单只有 `/` 与 `/article/**`；新增 SSR 页面必须改本 ADR，不得按页面随手加。`/articles` 是服务端 301 → `/search`；`/users/**`、`/search`、`/studio/**` 为 `ssr: false`；后台全 SPA。`app/pages` 下每个页面文件必须落入 SSR / SPA / 301 三类之一，新增页面不分类即红（[rendering-boundary.test.ts](../../frontend/test/gates/rendering-boundary.test.ts) 双向门禁，含"重定向路由必须 301"）。
2. SSR 输出不得包含用户私有数据（点赞、收藏、关注、登录身份等）；用户态一律客户端获取；文章详情的 SSR 只走匿名接口（不传 token），`is_digg` / `is_favor` / `relation` 由客户端在 hydration 后补齐。违反这条等于把共享缓存变成串号缓存。
3. 服务端缓存只缓存匿名内容，TTL 30s；**不得**引入按用户区分的缓存键。缓存键只有 `pathname + search`，只存 200 + `text/html` + 无 `Set-Cookie` 的响应，内存表上限 500 条，响应头 `x-ssr-cache: HIT/MISS`。
4. 匿名 SSR 30s 缓存必须走 Nitro `render:before` / `render:response` 钩子：**不得**用 `routeRules.swr`（node-server 预设下对 Nuxt 渲染器的通配路由不生效）。
5. 客户端缓存 TTL 30s；返回同一路由必须命中缓存，不得重新发起串行请求；缓存键含路由参数（文章 id / 分页 / 搜索词），不做 per-user 持久化；写缓存前用 `readerPrivateFields.ts` 剔除 `is_digg` / `is_favor` / `relation` 等私有字段；首屏 hydration 用剔除私有字段后的 SSR payload 预热。
6. `useReaderSwrAsyncData` 只允许 `cause === "initial"` 复用 fresh 缓存；手工 `refresh()` / `refresh:manual` / `refresh:hook` / `watch` 必须落到 handler（真发请求）；stale 仍先渲染旧值、后台静默重取。内部第二波初始化若需要「fresh 不发请求」，用 `refreshIfNeeded`，不得回头把 `refresh()` 钉成缓存命中。
7. 用户私有态走独立键 `article-private-<id>`，必须同时满足：`server: false`、`getCachedData: () => undefined`、`immediate: false`、带鉴权取数、投影四字段 `{is_digg, is_favor, digg_count, favor_count}`。私有态不得进 SSR payload、客户端 30 秒 SWR 缓存或任何共享缓存；权威计数随私有态一起覆盖本次渲染，不写回缓存。
8. 登录初始化完成后，文章页必须发出**一次**私有态请求；登录态从 `false→true`（页内登录成功）同样补一次（初始化与 watch 共用单飞闸门，一次登录只发一次）。用户已手动操作过点赞（`articleDiggTouched`）时不覆盖乐观结果。
9. `PUT /articles/{id}/digg` 的请求体必须显式给目标态 `{liked: boolean}`；服务端按目标收敛，重复同态是无操作（关系行、计数、通知都不动），响应返回权威 `is_digg` / `digg_count`；旧式空请求体必须 400，不得静默按 `false` 取消点赞。
10. 写响应必须回填权威计数：`digg` → `is_digg` / `digg_count`；收藏添加 → `is_favor` / `favor_count`；批量移除 → `is_favor` / `favor_count` + `removed_count` / `failed_count`（全部失败返回非 0 业务码）。计数是「库内快照 + 当前账本增量」的绝对值，**不得**用本地 ±1 冒充权威值。
11. 写响应回填与主动失效统一由阅读页取数模块 `useArticlePageSources.ts` 的 `reportArticleWrite` 管（键拼接 `readerArticleCacheKey(id)` + `readerSwrCache.delete(...)`）；页面只上报读者写动作，**不再 import 缓存单例或键函数**；写响应为空时也要清缓存但不 patch、不造脏值。
12. **只失效客户端 30 秒 SWR 缓存**的正文键。匿名服务端渲染缓存、私有态独立源、首页与评论分页缓存、渲染边界一律不动；服务端渲染不碰客户端缓存；不做跨进程 purge Nitro 匿名缓存。
13. 收藏的「已收藏 → 移除」分支必须走 `DELETE /articles/favorite/contents`；`POST /articles/favorite` 只用于添加。取消收藏只删目标收藏夹，`is_favor` 统一为「是否还在任一收藏夹」。
14. 门禁以**行为层**为准：一条行为用例覆盖点赞 / 收藏 / 写响应为空三种情形，断言返回重进读到权威数字、私有键不进客户端 30 秒 SWR 缓存、缓存里不留脏值。**不再断言源码里是否出现删除动作**，不新增同类源码文本断言；私有态独立源的结构判据与 canary 继续保留。
15. 点赞 / 收藏数是派生值：并账写库时 `digg` / `favor` **必须**按关系行 `COUNT` 重算覆盖整列，禁止 `列 += 增量`；`view` / `comment` 保持 `列 += 增量`。
16. 并账周期**只能**来自运行期配置 `site.reconcile.interval_minutes`（默认 30 分钟、缺省 / 负数回落 30、上限一天）；换算只有一处 `site.Reconcile.Interval()`；代码里不得再出现第二份默认周期常量；改完要重启才生效（调度注册参数，不做热重载）。
17. 文章计数刷盘与私信未读重算**必须**挂在同一个调度任务、同一个 tick 上；任一失败不得跳过另一个（失败彼此隔离）；不得新增调度器，也不得拆成两个 tick；周期不得做成秒级。
18. 读路径**只认** `app.CountLedger`：权威值 = 持久列 + 待并账增量，增量只从 `CounterPort` 读、合成只发生在账本；详情、列表、点赞 / 收藏写响应、收藏内容列表、首页热门置顶卡片、作者置顶卡片一律经它写回；首页卡片与详情不得各说一个数；任何新增读面都必须经它取权威计数，禁止自己拼「列 + 增量」。
19. 前端可见的计数**永不等并账**；刷新后的权威值来自写响应回填或私有态取数。不做点赞/收藏的实时推送或 WebSocket 广播。
20. 私信「幂等键 → 消息主键」派生**只允许**在 `backend/internal/chat/domain/msgid` 模块里实现，号段与哈希算法是该模块的实现细节；**排序、水位、分页不读 ID 大小**，消息先后只由 `send_time` 决定。ID 只作同一时刻的确定性并列裁决与扫描游标。
21. 首屏字体预算 < 100 KB；中文字体不得以自托管 webfont 形式加载，使用系统字体栈 `-apple-system` / `PingFang SC` / `Hiragino Sans GB` / `Microsoft YaHei` / `Noto Sans SC` / `sans-serif`；Sora / Inter 保留自托管；两端 `tokens.css` 逐字节同源。
22. 页面 setup 不得使用阻塞式串行 `await` 链取数（多于一个 `await useAsyncData` 时要么并行、要么走骨架）；路由过渡使用 fast 档 120ms（`.page-enter-active/.page-leave-active` 用 `var(--motion-fast)`）、`mode: 'out-in'`、只动 `opacity` / `transform`、`prefers-reduced-motion` 直接关掉。
23. 验收指标：**切页内容可见 P95 < 300 ms、首屏内容可见 < 800 ms、首屏字体下载 < 100 KB**，由仓库内 Playwright 探测脚本按同口径测量（依赖单独声明、默认不进 CI）；分享卡片是硬需求，文章页 `og:*` 必须由服务端输出，微信/QQ 预览退化视为回归。
24. 字体与渲染边界变更必须同步更新设计门禁：[fonts.test.ts](../../frontend/test/design/fonts.test.ts)（零 CJK `@font-face` + preload 字节 < 100 KB）、[tokens.test.ts](../../frontend/test/design/tokens.test.ts)、[foundation-tokens.test.ts](../../frontend/test/design/foundation-tokens.test.ts)、[readerTheme.test.ts](../../frontend/test/utils/readerTheme.test.ts)、[motion-foundation.test.ts](../../frontend/test/design/motion-foundation.test.ts) 断言 `var(--motion-fast)` 且不出现 `var(--motion-base)`；按现行口径改写而不是删除断言。
25. 匿名 SSR 缓存门禁锁 TTL 30s、键不随请求身份变化、非白名单路由不进缓存（[ssr-anonymous-cache.test.ts](../../frontend/test/gates/ssr-anonymous-cache.test.ts)）与"SSR 输出不含用户态"（[ssr-anonymity.test.ts](../../frontend/test/gates/ssr-anonymity.test.ts)）。
26. 不做：CDN / 边缘渲染、ISR 持久化、per-user 缓存键、跨进程缓存失效、点赞状态的服务端缓存、把收藏改成 toggle、把所有缓存合成一个统一模块、CDC 实时并账（非目标）、给既有表加列（零 DDL、本批无新表无 ALTER）。

## 相关代码与文档

- 渲染边界：[frontend/nuxt.config.ts](../../frontend/nuxt.config.ts)（`routeRules`）· [rendering-boundary.test.ts](../../frontend/test/gates/rendering-boundary.test.ts)
- 字体：[fonts.css](../../frontend/app/assets/css/fonts.css) · [tokens.css](../../frontend/app/assets/css/tokens.css)（`--font-cjk` / `--font-sans`）· [readerTheme.ts](../../frontend/app/utils/readerTheme.ts) · [tailwind.config.ts](../../frontend/tailwind.config.ts) · [tailwind.css](../../frontend/app/assets/css/tailwind.css)（路由过渡）· [fonts.test.ts](../../frontend/test/design/fonts.test.ts) · [tokens.test.ts](../../frontend/test/design/tokens.test.ts) · [foundation-tokens.test.ts](../../frontend/test/design/foundation-tokens.test.ts) · [readerTheme.test.ts](../../frontend/test/utils/readerTheme.test.ts) · [motion-foundation.test.ts](../../frontend/test/design/motion-foundation.test.ts)
- 取数与客户端缓存：[index.vue](../../frontend/app/pages/index.vue) · [article/[id].vue](../../frontend/app/pages/article/[id].vue) · [useHomePageSources.ts](../../frontend/app/composables/useHomePageSources.ts) · [useArticlePageSources.ts](../../frontend/app/composables/useArticlePageSources.ts)（`reportArticleWrite` / `applyArticlePrivateState` / `refreshArticlePrivate`）· [useReaderSwrAsyncData.ts](../../frontend/app/composables/useReaderSwrAsyncData.ts) · [readerSwrCache.ts](../../frontend/app/utils/readerSwrCache.ts) · [readerCacheKeys.ts](../../frontend/app/utils/readerCacheKeys.ts) · [readerPrivateFields.ts](../../frontend/app/utils/readerPrivateFields.ts) · [usePagedResourceCache.ts](../../frontend/app/composables/usePagedResourceCache.ts) · [useArticlePrivateSession.ts](../../frontend/app/composables/useArticlePrivateSession.ts) · [readerRenderMode.ts](../../frontend/app/utils/readerRenderMode.ts) · [LoadingSkeleton.vue](../../frontend/app/components/common/LoadingSkeleton.vue) · [ArticlePageSkeleton.vue](../../frontend/app/components/article/ArticlePageSkeleton.vue) · `frontend/app/services/*`
- 匿名 SSR 缓存：[server/utils/ssrAnonymousCache.ts](../../frontend/server/utils/ssrAnonymousCache.ts) · [server/plugins/ssrAnonymousCache.ts](../../frontend/server/plugins/ssrAnonymousCache.ts) · [ssr-anonymous-cache.test.ts](../../frontend/test/gates/ssr-anonymous-cache.test.ts) · [ssr-anonymity.test.ts](../../frontend/test/gates/ssr-anonymity.test.ts)
- 写路径与行为门禁：[useArticleDigg.ts](../../frontend/app/composables/useArticleDigg.ts) · [articleFavorite.ts](../../frontend/app/utils/articleFavorite.ts) · [FavoriteFolderModal.vue](../../frontend/app/components/favorite/FavoriteFolderModal.vue) · [useArticlePageSources.test.ts](../../frontend/test/composables/useArticlePageSources.test.ts) · [reader-private-state.test.ts](../../frontend/test/gates/reader-private-state.test.ts) · [reader-page-sources.test.ts](../../frontend/test/design/reader-page-sources.test.ts)
- 后端写响应与契约：[interaction.go](../../backend/internal/article/app/interaction.go) · [handler.go](../../backend/internal/article/adapter/http/handler.go) · [handler_sub.go](../../backend/internal/article/adapter/http/handler_sub.go) · [swagger.json](../../backend/docs/swagger.json) · [frontend openapi.d.ts](../../frontend/app/types/openapi.d.ts) · [frontend_admin openapi.d.ts](../../frontend_admin/app/types/openapi.d.ts) · [契约](../architecture/contracts.md)
- 并账配置与调度：[backend/conf/site/enter.go](../../backend/conf/site/enter.go)（`Reconcile.Interval()`，默认值与边界在此一处）· [site_default_settings.yaml](../../backend/config/site_default_settings.yaml) · [cron_wiring.go](../../backend/app/cron_wiring.go)（单一任务体）· [配置分层](../architecture/configuration.md)
- 并账写库与读路径唯一入口：[counter_flush.go](../../backend/internal/article/adapter/mysql/counter_flush.go) · [unread_reconcile.go](../../backend/internal/chat/adapter/mysql/unread_reconcile.go) · [ledger.go](../../backend/internal/article/app/ledger.go)（`CountLedger`）· [query.go](../../backend/internal/article/app/query.go) · [subdomain.go](../../backend/internal/article/app/subdomain.go) · [hottop.go](../../backend/internal/article/app/hottop.go) · [counters.go](../../backend/internal/article/adapter/mysql/counters.go) · [redis_article/enter.go](../../backend/internal/platform/redis/redis_article/enter.go)
- 派生主键：[msgid.go](../../backend/internal/chat/domain/msgid/msgid.go)
- 上游：[ADR-0011](./0011-neutral-glass-visual-system.md)（视觉系统与动效预算；中文字体按本 ADR 现行口径）· [ADR-0003](./0003-generated-contract-single-source.md)（开放接口契约以代码生成为单一来源）· [ADR-0014](./0014-deco-segment-and-cursor-charm.md)（挂件，与本 ADR 无交集）
- 词汇与部署：[GLOSSARY · 30s SWR / 匿名 SSR 缓存 / 渲染边界 / 计数账本 / 零 DDL](../../GLOSSARY.md) · [README](../../README.md) · [docs/TODO.md](../TODO.md)

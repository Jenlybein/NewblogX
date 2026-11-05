# ADR-0024：阅读页用户态与计数真相（私有态独立取数、幂等目标态、写响应收口）

- 状态：已交付
- 日期：2026-11-04（2026-11-06 按 #282 正文整体重写，显式覆盖规则 3「已交付 ADR 不改结论」；初版判断全部保留，只把写响应与主动失效的**落点**从页面改为阅读页取数模块）
- 来源：读者反馈「点赞后刷新失效、30 秒后计数才 +1、按钮不显示已赞」· spec [#234](https://github.com/Jenlybein/NewblogX/issues/234) · 落地票 [#235](https://github.com/Jenlybein/NewblogX/issues/235) · 收尾票 [#247](https://github.com/Jenlybein/NewblogX/issues/247) · 评审修复 [#248](https://github.com/Jenlybein/NewblogX/issues/248) · 计数修订 [#251](https://github.com/Jenlybein/NewblogX/issues/251) / [#252](https://github.com/Jenlybein/NewblogX/issues/252) · 收拢票 [#282](https://github.com/Jenlybein/NewblogX/issues/282) · [决策清单](../notes/2026-11-04-reading-interactions-and-message-center-decisions.md)（第 0、1 节）· 收窄 [ADR-0019](./0019-reader-rendering-and-cache-strategy.md) 的决策 4 / 约束 4 口径

## 背景与问题

ADR-0019 定下了「渲染边界只放行 `/` 与 `/article/**`、SSR 只输出匿名内容 + 客户端补齐用户态」与「客户端 30 秒 SWR 缓存 + 匿名服务端渲染缓存」两条规则。落地后两者撞在一起，形成三个可复现的缺陷：

1. **硬刷新后按钮永远显示「未赞」**。hydration 把**匿名** payload 预热进客户端 30 秒 SWR 缓存（`useReaderSwrAsyncData.ts` 的 payload 分支），而同一 composable 对**任何** `cause` 的 fresh 命中也短路 `refresh()`。于是页面 `onMounted` 里那句「登录后 `refreshArticle()` 补齐用户态」实际一个请求都不发：30 秒内拿到的永远是 hydration 时那份匿名数据。ADR-0019 承诺的「客户端补齐」在刷新/重进路径上根本不成立。
2. **陈旧状态会把自己真实的点赞取消掉**。旧接口 `PUT /articles/{id}/digg` 是 **toggle**（无请求体）。按钮因为上一条显示「未赞」时，用户再点一下 → 前端发 toggle → 后端发现关系已存在 → `Remove`。这不是显示问题，是**数据损坏**。
3. **计数要等 30 秒**。写路径只把增量写进账本，响应不返回最新计数；前端只能等下一次 30 秒刷新（匿名服务端渲染缓存 + 客户端 30 秒 SWR 缓存两层）才看到变化。

附带两个既有缺陷：`FavoriteFolderModal.vue` 的「已收藏 → 移除」分支仍发 `POST /articles/favorite`，后端返回 `ErrItemExists`；批量移除收藏一条都没成功时也回「成功 + 空 data」，前端读 `payload.is_favor` 直接 TypeError。

还有一层**结构性问题**（#282 要治的病）：写响应回填权威计数、`readerSwrCache.delete(readerArticleCacheKey(id))` 主动失效、私有态覆盖、缓存键拼接四件事散在页面 `[id].vue` 的两处回调里。页面既 import 缓存单例又拼缓存键，等于每个写动作都要页面自己记得「清哪份缓存」；漏一处就会让「返回/重进」先渲染写之前的旧数字再等 TTL。页面应该只上报「读者做了什么」，不需要知道缓存存在。

## 决策

1. **SWR 语义收窄：只有首屏 `cause === 'initial'` 允许 fresh 命中**。手工 `refresh()` / 钩子刷新一律返回 `undefined`，让 handler 真发请求（stale 仍先渲染旧值、后台静默重取）。「30 秒内重进请求数为 0」的既有验收改由**内部第二波初始化**（作者资料）走 `refreshIfNeeded`（fresh 直接落数据、不发请求），不靠把 `refresh()` 也钉成缓存命中。
2. **用户私有态独立取数**：新增 `article-private-<id>` 源，`server: false` + `getCachedData: () => undefined` + `immediate: false`；页面在 `initializeSession()` 之后显式触发一次，并且**登录态 false→true（页内登录成功）时补一次**。它复用**带鉴权**的详情接口，只投影 `{is_digg, is_favor, digg_count, favor_count}` 四字段（详情接口不返回 `relation`，作者关系由 `authorProfile` 源负责；曾经的 `relation` 是永为 null 的死字段，已删除）。永不进 payload、永不进客户端 30 秒 SWR 缓存。ADR-0019 约束 2（SSR 只出匿名内容）**保持不变**。
3. **点赞改幂等目标态**：`PUT /articles/{id}/digg` 带 `{liked: boolean}`，服务端按目标收敛；重复同态即无操作（不改关系行、不改计数、不发通知），响应仍返回当前权威 `is_digg`。前端调用点传「目标态 = 当前状态取反」，不再传 toggle。
4. **写响应回填权威计数 + 主动失效，统一收进阅读页取数模块**（#282 重写此条）：`PUT /digg` 返回 `{is_digg, digg_count}`，`POST /articles/favorite` 返回 `{is_favor, favor_count}`；`DELETE /articles/favorite/contents` 返回 `{is_favor, favor_count, removed_count, failed_count}`（批量接口要能让调用方分辨部分失败，全部失败返回非 0 业务码）。计数是「库内快照 + 当前账本增量」的绝对值，**不是这次 ±1**。`useArticlePageSources.ts` 以 `reportArticleWrite({kind, result})` 一个入口统一管四件事：写响应回填权威计数与私有态（覆盖本次渲染）、空响应的判空防御、`readerArticleCacheKey(id)` 的键拼接、`readerSwrCache.delete(...)` 主动失效客户端 30 秒 SWR 缓存的正文键。页面 `[id].vue` 只上报「读者做了什么 + 写响应」，**不再 import `readerSwrCache` 单例或 `readerArticleCacheKey`**。写响应为空时同样失效、但不 patch、不造脏值。只失效客户端 30 秒 SWR 缓存；匿名服务端渲染缓存、私有态独立源、首页与评论分页缓存、渲染边界一律不动。
5. **不加大范围 purge 共享 SSR HTML 缓存**：接受「别人最多滞后 30 秒」。多实例下 Nitro 缓存是进程内 Map，跨进程失效无法做到；宁可不做，也不做一个只在单实例成立的假能力。
6. **收藏附带修复**：「已收藏 → 移除」走 `DELETE /articles/favorite/contents`，不再发 POST；取消收藏只删目标收藏夹，`is_favor` 统一为「是否还在任一收藏夹」；收藏计数同样走写响应回填。
7. **门禁改为行为层**：真跑 `useArticlePageSources`，一条行为用例覆盖点赞 / 收藏 / 写响应为空三种情形，验「返回重进读到权威数字、私有字段不进客户端 30 秒 SWR 缓存、缓存里不留脏值」。**不再断言源码里是否出现删除动作**，也不新增同类源码文本断言；私有态独立源自身的 `server:false` / 恒 undefined `getCachedData` 等结构判据与 canary 保留。

## 理由与已否决方案

- **只修按钮显示（不换接口语义）**：看起来改一行 UI 就够了，但「显示未赞」与「服务端其实已赞」之间永远存在窗口；只要窗口在，toggle 就还会取消真实点赞。数据损坏必须从接口语义上消除。
- **把点赞态放回 SSR payload**：直接违反 ADR-0019 约束 2，匿名服务端渲染缓存会变成串号缓存；登录用户与匿名用户也不能再共用一份 HTML。
- **按用户区分服务端/客户端缓存键**：退化成 per-user 缓存，命中率与「登录共享」的收益一起消失（ADR-0019 约束 3 已经否决过一次）。
- **点赞用「先 GET 详情再决定发 POST 还是 DELETE」**：多一次往返，且两个请求之间状态仍可能变；目标态接口是原子的「按目标收敛」，天然幂等，重试也安全。
- **前端本地 ±1 回填计数**：并发点赞、重复点击、他人同时点赞都会让本地值与库内值漂移；写响应回填是唯一权威来源。`useArticleDigg.ts` 与 `articleFavorite.ts` 都只采用写响应里的权威值。
- **继续让页面自己 `delete('article-<id>')`**（#282 否决）：页面每加一种写动作就要重复一遍「patch + 拼键 + 失效」，漏一处就留下 30 秒脏值；而且页面因此必须 import 缓存单例与键函数，取数模块反而管不住自己的缓存。收进阅读页取数模块后，页面不需要知道缓存存在。
- **把所有缓存模块合成一个「统一缓存」**（#282 范围外）：两层缓存的失效边界不同（匿名服务端渲染缓存是进程内整页 HTML、客户端 30 秒 SWR 缓存是跨导航内存数据），为整齐合并会顺带动到首页、评论分页与渲染边界；本票只收拢**阅读页写路径**这一处接缝。
- **断言源码里出现 `delete(...)`**：删除动作的位置是实现细节；从页面挪进取数模块、或换一种失效写法，行为没变却会误报红。#282 改为行为层用例（写响应 → 重进 → 权威数字 + 缓存无脏值）。
- **收藏移除时给 `Favorite` 加「已存在就转 removes」的兜底**：把「误发 POST」的缺陷藏进后端语义，会让 POST 的幂等性变得不可推理（收藏与取消收藏是两个不同的写）。正确做法是前端发对的动词。
- **全局 purge Nitro 匿名缓存**：多实例进程内 Map 无法跨进程失效；只做单实例可用的假能力比不做更糟。
- **把 30 秒 TTL 调小甚至去掉**：会同时推翻 ADR-0019 的性能取舍（切页/返回的体感就是靠它）；计数滞后用写响应回填 + 主动失效解决，不需要动 TTL。

## 代价 / 不做什么

- **登录用户每次进文章多一次带鉴权请求**。这是「SSR 只出匿名内容」的直接代价；只在登录后、且 `immediate:false` 由页面触发，匿名用户零成本。
- **登录态仍然「先默认、后跳变」**：匿名 HTML 先渲染未赞，客户端补齐后按钮跳变。ADR-0019 已接受这个观感，本 ADR 不改变它。
- **别人（跨用户）看到的计数仍可能滞后 30 秒**：不 purge 共享 HTML 缓存是明确取舍；作者自己的写操作是立即正确的。
- **开放接口契约是破坏性小改**：`PUT /digg` 的请求体从「可省略」变成**必填 `liked`**。仓库内唯一的调用点（读者站）同批更新；旧式空请求体会收到 400（`binding:"required"`），不会静默按 `false` 取消点赞。响应新增 `digg_count` / `favor_count` 字段，对旧前端是纯增量。#282 不改契约（基线 119/159/288 保持不变）、不改后端、**零 DDL**。
- **批量移除收藏的计数语义按「最后一条成功移除的文章」返回**：`is_favor` / `favor_count` 只有单篇消费方（收藏夹弹窗）该依赖；批量场景另给 `removed_count` / `failed_count`，全部失败返回非 0 业务码，不再有「成功 + 空 data」。
- **写响应为空只失效、不回填**：服务端状态可能已经变了，但响应里没有可采用的权威值；宁可让下一次取数回答，也不本地猜一个 ±1。
- **不做什么**：不做点赞/收藏的实时推送或 WebSocket 广播；不做点赞状态的服务端缓存；不改 ADR-0019 的渲染边界与两层 30 秒结构；不把收藏改成 toggle；不为了整齐把所有缓存合成一个模块；不动 #278–#281 私信批。

## 当前生效约束

1. `useReaderSwrAsyncData` 只允许 `cause === "initial"` 复用 fresh 缓存；`refresh:manual` / `refresh:hook` / `watch` 必须落到 handler（真发请求）。内部第二波初始化若需要「fresh 不发请求」，用 `refreshIfNeeded`，不得回头把 `refresh()` 钉成缓存命中。
2. 用户私有态走独立键 `article-private-<id>`，必须同时满足：`server: false`、`getCachedData: () => undefined`、`immediate: false`、带鉴权取数、投影四字段 `{is_digg, is_favor, digg_count, favor_count}`。私有态不得进 SSR payload，也不得进客户端 30 秒 SWR 缓存 / 共享缓存；权威计数随私有态一起覆盖本次渲染，不写回缓存。
3. 登录初始化完成后，文章页必须发出**一次**私有态请求；**登录态从 false→true（页内登录成功）同样补一次**（初始化与 watch 共用单飞闸门，一次登录只发一次）。用户已手动操作过点赞（`articleDiggTouched`）时不覆盖乐观结果。
4. `PUT /articles/{id}/digg` 的请求体必须显式给目标态 `{liked: boolean}`；服务端按目标收敛，重复同态是无操作（关系行、计数、通知都不动），响应返回权威 `is_digg` / `digg_count`。
5. 写响应必须回填权威计数：`digg` → `digg_count`；收藏添加 → `is_favor` / `favor_count`；批量移除 → `is_favor` / `favor_count` + `removed_count` / `failed_count`（全部失败返回非 0 业务码）。**由阅读页取数模块 `useArticlePageSources.ts` 的 `reportArticleWrite` 统一管写响应回填与主动失效**（键拼接 `readerArticleCacheKey(id)` + `readerSwrCache.delete(...)`）；页面只上报读者写动作，不再 import 缓存单例或键函数；不得用本地 ±1 冒充权威值。写响应为空时也要清缓存但不造脏值。
6. 只失效**客户端 30 秒 SWR 缓存**的正文键。匿名服务端渲染缓存、私有态独立源、首页与评论分页缓存、渲染边界一律不动；服务端渲染不碰客户端缓存。
7. 收藏的「已收藏 → 移除」分支必须走 `DELETE /articles/favorite/contents`；`POST /articles/favorite` 只用于添加。
8. 门禁以**行为层**为准：一条行为用例覆盖点赞 / 收藏 / 写响应为空三种情形，断言返回重进读到权威数字、私有键不进客户端 30 秒 SWR 缓存、缓存里不留脏值。**不再断言源码里是否出现删除动作**，不新增同类源码文本断言；私有态独立源的结构判据与 canary 继续保留。
9. ADR-0019 的 SSR 白名单、匿名渲染边界、两层 30 秒结构继续生效；本 ADR 只收窄「refresh 也遵守 SWR」这一条口径，不改缓存分层。

## 相关代码与文档

- 阅读页取数模块（#282 收口落点）：[useArticlePageSources.ts](../../frontend/app/composables/useArticlePageSources.ts)（`reportArticleWrite` / `applyArticlePrivateState` / `refreshArticlePrivate`）· [useArticlePageSources.test.ts](../../frontend/test/composables/useArticlePageSources.test.ts)（三情形行为用例）· [article/[id].vue](../../frontend/app/pages/article/[id].vue)（只上报写动作）
- 客户端 30 秒 SWR 缓存：[useReaderSwrAsyncData.ts](../../frontend/app/composables/useReaderSwrAsyncData.ts) · [readerSwrCache.ts](../../frontend/app/utils/readerSwrCache.ts) · [readerCacheKeys.ts](../../frontend/app/utils/readerCacheKeys.ts) · [usePagedResourceCache.ts](../../frontend/app/composables/usePagedResourceCache.ts)
- 私有态与净化：[readerPrivateFields.ts](../../frontend/app/utils/readerPrivateFields.ts) · [article.ts（services）](../../frontend/app/services/article.ts) · [useArticlePrivateSession.ts](../../frontend/app/composables/useArticlePrivateSession.ts)
- 写响应纯函数：[useArticleDigg.ts](../../frontend/app/composables/useArticleDigg.ts) · [articleFavorite.ts](../../frontend/app/utils/articleFavorite.ts) · [FavoriteFolderModal.vue](../../frontend/app/components/favorite/FavoriteFolderModal.vue)
- 后端与开放接口契约：[interaction.go](../../backend/internal/article/app/interaction.go) · [handler.go](../../backend/internal/article/adapter/http/handler.go) · [handler_sub.go](../../backend/internal/article/adapter/http/handler_sub.go) · [契约](../architecture/contracts.md)（基线 119/159/288）
- 门禁：[reader-private-state.test.ts](../../frontend/test/gates/reader-private-state.test.ts) · [ssr-anonymity.test.ts](../../frontend/test/gates/ssr-anonymity.test.ts) · [reader-page-sources.test.ts](../../frontend/test/design/reader-page-sources.test.ts)
- 上游：[ADR-0019](./0019-reader-rendering-and-cache-strategy.md)（渲染边界与两层缓存；本 ADR 收窄其决策 4 / 约束 4 的 refresh 口径）· [ADR-0003](./0003-generated-contract-single-source.md)（开放接口契约以代码生成为单一来源）· [ADR-0025](./0025-count-ledger-and-reconciliation.md)（计数账本与单一权威读路径）· [GLOSSARY](../../GLOSSARY.md)（30s SWR / 匿名 SSR 缓存 / 渲染边界 / 零 DDL）
- 决策来源：[2026-11-04 阅读页互动、消息中心与历史记录：决策清单](../notes/2026-11-04-reading-interactions-and-message-center-decisions.md) · spec [#234](https://github.com/Jenlybein/NewblogX/issues/234) · 落地票 [#235](https://github.com/Jenlybein/NewblogX/issues/235) · 收拢票 [#282](https://github.com/Jenlybein/NewblogX/issues/282)

## 后续修订登记（历史事实，结论以上述正文为准）

- **#247**：「分享」不走互动登录前置（纯前端剪贴板、无接口无计数）；WS 被动断线退避重连后必须重取权威未读汇总，不靠推送增量累积（与决策 4「权威值只来自服务端」同源，细节在 [ADR-0022](./0022-chat-dual-channel-http-write-ws-notify.md)）。相关代码：`readerLoginGate.ts`、`useChatSocket.ts`、`useMessageRealtime.ts`。
- **#248 评审修复**：取消收藏只删目标收藏夹；批量移除按实际语义返回（全部失败返回非 0 业务码）；`RemoveFavoriteItems` 的 swagger 注解改为 `Envelope[ArticleFavoriteRemoveResponse]` 并重生成两端类型；页内登录后补取私有态（`useArticlePrivateSession`），顺带删除 `ArticlePrivateState.relation` 死字段。
- **#251 / #252**：私有态投影从 2 个布尔扩到 4 个字段（含权威计数），修掉「按钮已赞、数字没变」；计数持久化改按关系行 `COUNT` 重算覆盖整列，增量只做两次并账之间的读路径叠加，细节在 [ADR-0025](./0025-count-ledger-and-reconciliation.md)。
- **#282 点赞缓存收拢**：决策 4 / 约束 5 的「页面主动 `readerSwrCache.delete('article-<id>')`」整体改为「阅读页取数模块统一管写响应回填与主动失效」；新增约束 6（只失效客户端 30 秒 SWR 缓存）与约束 8（不再断言源码删除动作、改行为层用例）。匿名服务端渲染缓存、私有态独立源、首页与评论分页缓存、渲染边界、开放接口契约、零 DDL 一概未动。

# ADR-0024：阅读页用户态与计数真相（私有态独立取数、幂等目标态、写响应回填）

- 状态：已交付
- 日期：2026-11-04
- 来源：读者反馈「点赞后刷新失效、30 秒后计数才 +1、按钮不显示已赞」· spec [#234](https://github.com/Jenlybein/NewblogX/issues/234) · 落地票 [#235](https://github.com/Jenlybein/NewblogX/issues/235) · 收尾票 [#247](https://github.com/Jenlybein/NewblogX/issues/247)（见文末「后续修订」）· [决策清单](../notes/2026-11-04-reading-interactions-and-message-center-decisions.md)（第 0、1 节）· 收窄 [ADR-0019](./0019-reader-rendering-and-cache-strategy.md) 的决策 4 / 约束 4 口径

## 背景与问题

ADR-0019 定下了「SSR 只输出匿名内容 + 客户端补齐用户态」与「客户端 30s SWR」两条规则，但落地后两者撞在一起，形成了三个可复现的缺陷：

1. **硬刷新后按钮永远显示「未赞」**。hydration 把**匿名** payload 预热进客户端 30s SWR（`useReaderSwrAsyncData.ts` 的 payload 分支），而同一 composable 对**任何** `cause` 的 fresh 命中也短路 `refresh()`（旧 `112-116` 行）。于是页面 `onMounted` 里那句「登录后 `refreshArticle()` 补齐用户态」实际一个请求都不发：30 秒内它拿到的永远是 hydration 时那份匿名数据。ADR-0019 承诺的「客户端补齐」在刷新/重进路径上根本不成立。
2. **陈旧状态会把自己真实的点赞取消掉**。旧接口 `PUT /articles/{id}/digg` 是 **toggle**（无请求体）。按钮因为上一条显示「未赞」时，用户再点一下 → 前端发 toggle → 后端发现关系已存在 → `Remove`。这不是显示问题，是**数据损坏**。
3. **计数要等 30 秒**。写路径只把增量写进 Redis，响应不返回最新计数；前端只能等下一次 30s 刷新（Nitro 匿名 HTML 缓存 + 客户端 SWR 两层 30s）才看到 +1。

附带一个既有缺陷：`FavoriteFolderModal.vue` 的「已收藏 → 移除」分支仍发 `POST /articles/favorite`，后端 `Favorite` 发现关系已存在后返回 `ErrItemExists`（"已收藏该文章"）。用户点「移出」得到一条失败提示，关系却没动。

## 决策

1. **SWR 语义收窄：只有首屏 `cause === 'initial'` 允许 fresh 命中**。手工 `refresh()` / 钩子刷新一律返回 `undefined`，让 handler 真发请求（stale 仍先渲染旧值、后台静默重取）。「30 秒内重进请求数为 0」的既有验收改由**内部第二波初始化**（作者资料）走 `refreshIfNeeded`（fresh 直接落数据、不发请求），不靠把 `refresh()` 也钉成缓存命中。
2. **用户私有态独立取数**：新增 `article-private-<id>` 源，`server: false` + `getCachedData: () => undefined` + `immediate: false`；页面在 `initializeSession()` 之后显式触发一次，并且**登录态 false→true（页内登录成功）时补一次**。它复用**带鉴权**的详情接口但只投影 `is_digg` / `is_favor`（详情接口不返回 `relation`，作者关系由 `authorProfile` 源负责；曾经的 `relation` 是永为 null 的死字段，已删除）。永不进 payload、永不进 30s SWR。ADR-0019 约束 2（SSR 只出匿名内容）**保持不变**。
3. **点赞改幂等目标态**：`PUT /articles/{id}/digg` 带 `{liked: boolean}`，服务端按目标收敛；重复同态即无操作（不改关系行、不改计数、不发通知），响应仍返回当前权威 `is_digg`。前端调用点传「目标态 = 当前状态取反」，不再传 toggle。
4. **写响应回填权威计数 + 主动失效**：`PUT /digg` 返回 `{is_digg, digg_count}`，`POST /articles/favorite` 返回 `{is_favor, favor_count}`；`DELETE /articles/favorite/contents` 返回 `{is_favor, favor_count, removed_count, failed_count}`（批量接口要能让调用方分辨部分失败，全部失败返回非 0 业务码）。计数是「库内快照 + 当前缓存增量」的绝对值，不是这次 ±1。前端收到后直接 patch 正文并 `readerSwrCache.delete('article-<id>')`，不等 TTL。
5. **不加大范围 purge 共享 SSR HTML 缓存**：接受「别人最多滞后 30 秒」。多实例下 Nitro 缓存是进程内 Map，跨进程失效无法做到；宁可不做，也不做一个只在单实例成立的假能力。
6. **收藏附带修复**：「已收藏 → 移除」走 `DELETE /articles/favorite/contents`，不再发 POST；收藏计数同样走写响应回填。
7. **门禁**：新增「hydration 后必须发出一次带鉴权的用户态请求」的**源码 + 行为**两层断言。源码层锁 `server:false` / 恒 undefined 的 `getCachedData` / 带鉴权取数 / 页面触发点；行为层真跑 composable，断言 payload 里的私有键不被采用、私有键不进 30s 缓存。把用户态塞回 payload、或把 refresh 重新短路，门禁都会变红。

## 理由与已否决方案

- **只修按钮显示（不换接口语义）**：看起来改一行 UI 就够了，但「显示未赞」与「服务端其实已赞」之间永远存在窗口；只要窗口在，toggle 就还会取消真实点赞。数据损坏必须从接口语义上消除。
- **把点赞态放回 SSR payload**：直接违反 ADR-0019 约束 2，匿名共享缓存会变成串号缓存；登录用户与匿名用户也不能再共用一份 HTML。
- **按用户区分服务端/客户端缓存键**：退化成 per-user 缓存，命中率与「登录共享」的收益一起消失（ADR-0019 约束 3 已经否决过一次）。
- **点赞用「先 GET 详情再决定发 POST 还是 DELETE」**：多一次往返，且两个请求之间状态仍可能变；目标态接口是原子的「按目标收敛」，天然幂等，重试也安全。
- **前端本地 ±1 回填计数**：并发点赞、重复点击、他人同时点赞都会让本地值与库内值漂移；写响应回填是唯一权威来源。
- **收藏移除时给 `Favorite` 加「已存在就转 removes」的兜底**：把「误发 POST」的缺陷藏进后端语义，会让 POST 的幂等性变得不可推理（收藏与取消收藏是两个不同的写）。正确做法是前端发对的动词。
- **全局 purge Nitro 匿名缓存**：多实例进程内 Map 无法跨进程失效；只做单实例可用的假能力比不做更糟。
- **把 30s TTL 调小甚至去掉**：会同时推翻 ADR-0019 的性能取舍（切页/返回的体感就是靠它）；计数滞后用写响应回填解决，不需要动 TTL。

## 代价 / 不做什么

- **登录用户每次进文章多一次带鉴权请求**。这是「SSR 只出匿名内容」的直接代价；只在登录后、且 `immediate:false` 由页面触发，匿名用户零成本。
- **登录态仍然「先默认、后跳变」**：匿名 HTML 先渲染未赞，客户端补齐后按钮跳变。ADR-0019 已接受这个观感，本 ADR 不改变它。
- **别人（跨用户）看到的计数仍可能滞后 30 秒**：不 purge 共享 HTML 缓存是明确取舍；作者自己的写操作是立即正确的。
- **契约是破坏性小改**：`PUT /digg` 的请求体从「可省略」变成**必填 `liked`**。仓库内唯一的调用点（读者站）同批更新；旧式空请求体会收到 400（`binding:"required"`），不会静默按 `false` 取消点赞。响应新增 `digg_count` / `favor_count` 字段，对旧前端是纯增量。
- **批量移除收藏的计数语义按「最后一条成功移除的文章」返回**：`is_favor` / `favor_count` 只有单篇消费方（收藏夹弹窗）该依赖；批量场景另给 `removed_count` / `failed_count`，全部失败返回非 0 业务码，不再有「成功 + 空 data」。
- **不做什么**：不做点赞/收藏的实时推送或 WebSocket 广播；不做点赞状态的服务端缓存；不改 ADR-0019 的两层 30s 结构；不把收藏改成 toggle。

## 当前生效约束

1. `useReaderSwrAsyncData` 只允许 `cause === "initial"` 复用 fresh 缓存；`refresh:manual` / `refresh:hook` / `watch` 必须落到 handler（真发请求）。内部第二波初始化若需要「fresh 不发请求」，用 `refreshIfNeeded`，不得回头把 `refresh()` 钉成缓存命中。
2. 用户私有态走独立键 `article-private-<id>`，必须同时满足：`server: false`、`getCachedData: () => undefined`、`immediate: false`、带鉴权取数、只投影 `is_digg` / `is_favor`。私有态不得进 SSR payload，也不得进 30s SWR / 共享缓存。
3. 登录初始化完成后，文章页必须发出**一次**私有态请求；**登录态从 false→true（页内登录成功）同样补一次**（初始化与 watch 共用单飞闸门，一次登录只发一次）。用户已手动操作过点赞（`articleDiggTouched`）时不覆盖乐观结果。
4. `PUT /articles/{id}/digg` 的请求体必须显式给目标态 `{liked: boolean}`；服务端按目标收敛，重复同态是无操作（关系行、计数、通知都不动），响应返回权威 `is_digg` / `digg_count`。
5. 写响应必须回填权威计数：`digg` → `digg_count`；收藏添加 → `is_favor` / `favor_count`；批量移除 → `is_favor` / `favor_count` + `removed_count` / `failed_count`（全部失败返回非 0 业务码）。前端据此直接 patch，并主动 `readerSwrCache.delete('article-<id>')`；不得用本地 ±1 冒充权威值。
6. 收藏的「已收藏 → 移除」分支必须走 `DELETE /articles/favorite/contents`；`POST /articles/favorite` 只用于添加。
7. 门禁必须保留源码 + 行为两层：源码层锁私有态源的选项、鉴权口径与页面触发点；行为层断言 hydration 后请求确实发出、payload 私有键不被采用、私有键不进缓存。canary 必须在违反写法下变红。
8. ADR-0019 的 SSR 白名单、匿名渲染边界、两层 30s 结构继续生效；本 ADR 只收窄「refresh 也遵守 SWR」这一条口径，不改缓存分层。

## 相关代码与文档

- SWR 语义：[useReaderSwrAsyncData.ts](../../frontend/app/composables/useReaderSwrAsyncData.ts) · [useReaderSwrAsyncData.test.ts](../../frontend/test/composables/useReaderSwrAsyncData.test.ts)
- 私有态取数：[useArticlePageSources.ts](../../frontend/app/composables/useArticlePageSources.ts) · [readerCacheKeys.ts](../../frontend/app/utils/readerCacheKeys.ts) · [readerPrivateFields.ts](../../frontend/app/utils/readerPrivateFields.ts) · [article.ts（services）](../../frontend/app/services/article.ts) · [article/[id].vue](../../frontend/app/pages/article/[id].vue)
- 点赞目标态：[useArticleDigg.ts](../../frontend/app/composables/useArticleDigg.ts) · [interaction.go](../../backend/internal/article/app/interaction.go) · [handler.go](../../backend/internal/article/adapter/http/handler.go) · [dto.go](../../backend/internal/article/adapter/http/dto.go)
- 收藏移除与计数回填：[FavoriteFolderModal.vue](../../frontend/app/components/favorite/FavoriteFolderModal.vue) · [favorite.ts（services）](../../frontend/app/services/favorite.ts) · [handler_sub.go](../../backend/internal/article/adapter/http/handler_sub.go)
- 门禁：[reader-private-state.test.ts](../../frontend/test/gates/reader-private-state.test.ts) · [ssr-anonymity.test.ts](../../frontend/test/gates/ssr-anonymity.test.ts) · [reader-page-sources.test.ts](../../frontend/test/design/reader-page-sources.test.ts)
- 上游：[ADR-0019](./0019-reader-rendering-and-cache-strategy.md)（渲染边界与两层缓存；本 ADR 收窄其决策 4 / 约束 4 的 refresh 口径）· [ADR-0003](./0003-generated-contract-single-source.md)（契约以代码生成为单一来源）
- 决策来源：[2026-11-04 阅读页互动、消息中心与历史记录：决策清单](../notes/2026-11-04-reading-interactions-and-message-center-decisions.md) · spec [#234](https://github.com/Jenlybein/NewblogX/issues/234) · 落地票 [#235](https://github.com/Jenlybein/NewblogX/issues/235)

## 后续修订（2026-11-04，#247）

决策与约束正文一字未改；#247 在本批收尾时补了两处**边界说明**，两处都不改变已生效的结论，只是把「哪些动作在这一条治理面内」写清楚。

1. **「分享」不走互动登录前置**。互动统一登录前置（#238 范围 7）只管**会产生用户态写请求**的动作：点赞 / 收藏 / 关注 / 评论提交，未登录先弹登录框、一个接口都不打。分享是纯前端剪贴板——没有接口、没有计数、也不产生用户态，所以游客也应该能复制文章链接；它在 ADR-0024 的治理面之外（既不需要私有态取数，也没有可回填的计数）。判据在 `readerLoginGate` 的注释与 `test/pages/articleCommentSection.test.ts`（断言分享按钮**不**调用 `requireLogin()`）。
2. **断线重连后必须重新取权威用户态，不靠推送增量累积**。WS 被动断线（含取票失败）按 1s → 2s → … → 上限 30s、最多 20% 抖动退避重连；主动 `close()`（登出 / 卸载）不重连。重连成功后立刻重取一次消息未读汇总，因为断线窗口里到达不了的通知**不会补发**，本地数字不会自己 +1。这与决策 4「权威值只能来自服务端、不靠本地推算」是同一条原则在未读数上的延伸——文章计数靠写响应回填，未读数靠重连后的整份汇总校准，两者都不允许拿"推送增量"当账本。通道与重连细节记在 [ADR-0022](./0022-chat-dual-channel-http-write-ws-notify.md) 的「后续修订（2026-11-04）」。

相关代码：`frontend/app/utils/readerLoginGate.ts`、`frontend/app/composables/useChatSocket.ts`（重连与 `onReconnected`）、`frontend/app/composables/useMessageRealtime.ts`（重连后 `refreshSummary`）、`frontend/test/utils/readerLoginGate.test.ts`、`frontend/test/composables/useChatSocket.test.ts`。

## 评审修复（2026-11-04，#248）

代码评审（[#248](https://github.com/Jenlybein/NewblogX/issues/248)）发现本 ADR 落地后的四处接缝问题，本节记录修复口径；正文的决策 / 约束 / 代价段落已同步改写，没有新增设计取舍。

1. **取消收藏只删目标收藏夹**：`Unfavorite` 过去用 `FindItemAnyFolder` 取任意一条、再按它的 folder 删，完全忽略调用方传入的 folder。同一篇文章收在 A/B 两夹时，点「移出 A」可能删掉 B 的那条（A 的勾还在、toast 却说「已从 A 移除」）。现在的口径：只认目标夹（`FolderID` 为零时与 `Favorite` 同口径落默认夹），找不到就报 `ErrItemNotFound`，不猜不兜底。`is_favor` 的语义统一为「**是否还在任一收藏夹**」（详情接口与 `Unfavorite` 的返回值同一口径）。
2. **批量移除按实际语义返回**：`DELETE /articles/favorite/contents` 旧实现把单条失败 `continue` 掉，一条都没成功也回 `OkWithMsg`（成功 + 空 data），前端读 `payload.is_favor` 直接 TypeError。现在全部成功 / 部分成功返回 `{is_favor, favor_count, removed_count, failed_count}`，全部失败返回非 0 业务码 + 明确文案；前端对缺失载荷做空值防御。
3. **契约与 handler 实际 helper 一致**：`RemoveFavoriteItems` 的 swagger 注解过去声明 `Envelope[ArticleFavoriteResponse]`，失败分支却走 `OkWithMsg`（`data: {}`）。现在成功分支恒走 `OkWithData`，注解改为 `Envelope[ArticleFavoriteRemoveResponse]`，并重生成两端类型。
4. **页内登录后补取私有态**：页面只在 `onMounted` 判一次 `initializeSession()`，用户在文章页被弹登录框、登录成功后才成为已登录的那条路径拿不到私有态（按钮继续显示未赞）。新增 `useArticlePrivateSession`：`initialize()`（首屏）+ `watch(isLoggedIn)`（false→true）共用单飞闸门，只在客户端跑，一次登录只补一次。顺带删除 `ArticlePrivateState.relation` 这个永为 null 的死字段（只投影 `is_digg` / `is_favor`）。

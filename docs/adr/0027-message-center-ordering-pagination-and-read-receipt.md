# ADR-0027：消息中心的排序、游标分页与整会话已读

- 状态：已接受
- 日期：2026-11-05
- 来源：2026-11-05 消息中心重构 grill-with-docs 会话（[决定清单 Q1–Q41](../notes/2026-11-05-message-center-redesign-decisions.md)）· 修订 [ADR-0022](./0022-chat-dual-channel-http-write-ws-notify.md) 的清除水位口径（文末「后续修订（2026-11-05，消息中心重构）」）· 与 [ADR-0025](./0025-count-ledger-and-reconciliation.md) / [ADR-0026](./0026-chat-send-side-risk-control.md) 的分工见「理由与已否决方案」末段

## 背景与问题

消息中心上线后，私信子项暴露了三处互不相干、但都必须改**契约或读写口径**的缺陷。它们不是文案或样式问题，改错要前后端一起回滚，因此单独立一条 ADR：

1. **顺序倒置**：`backend/internal/chat/adapter/mysql/chat.go:364` 以 `ORDER BY send_time desc` 返回（最新在前），而 `frontend/app/composables/useInboxCenter.ts:185` 把 `payload.list` **原样赋值**、既不 reverse 也不 sort，于是后发的消息显示在前一条上方。叠加两个放大因素：`send_time datetime(3)` 存在同毫秒并列、排序不确定；客户端 client-key 消息按 ADR-0022 决策 4 用 FNV-1a 派生主键、落在 **`[2^62, 2^63)`**，这个 ID 不是时序——所以「拿 `id` 补时间」看似顺手，实为错误。
2. **已读请求类型不符**：`frontend/app/services/inbox.ts:113` 按 `ctype.ID`（JSON 字符串）提交 `msg_id_list`，`backend/internal/chat/adapter/http/handler.go:93` 的 `IDListRequest` 却绑定 `[]uint64` → Gin 400/1001，被 `useInboxCenter.ts:188` 的 `.catch(() => undefined)` 吞掉 → `is_read` 与会话 / 消息中心两处 badge 永远陈旧。
3. **分页无游标**：`backend/internal/chat/domain/chat.go:177` 的 `Page.Normalize()` 默认每页 10，`backend/internal/chat/adapter/mysql/chat.go:341` 的 `ListMessages` 是纯 `LIMIT/OFFSET`、只返回 `count` 而没有 `has_more` 与游标。用户上翻读历史时只要落一条新消息，offset 之后的每一行整体错位；前端也无法可靠判断「是否还有更早的消息」。

此外还有一条与上述同源的隐患：`clear_before_msg_id` 目前用 `id > ?` 过滤（`backend/internal/chat/adapter/mysql/chat.go:349`）。在非单调 hash ID 下它既会漏掉、也会错放消息，并可能破坏 #266「清空会话 → 未读不复活」的并账口径；本批一并修掉，不留后续票（决定清单 Q21）。

## 决策

本 ADR **修订 ADR-0022 的清除水位假设**（`id > ?` 隐含「主键单调递增」），并新增三条难以逆向的契约口径；ADR-0022 决策 1「HTTP 唯一发送通道」、零 DDL 与发件箱三条不变。

1. **确定性并列 tiebreaker + 前端唯一升序数组**：后端在 `ORDER BY send_time DESC` 上追加 `id` 作**确定性并列裁决**（仅用于同一毫秒内稳定分页与顺序，**不是时序**），DESC 契约保持原样；前端维护**唯一一个升序数组**（最新在底部）：首页反转、更早页反转后前插。禁止按 `id` 单独排序。日期分组为自然日居中分隔（今天 / 昨天 / 日期），同发送人、同一天、2 分钟内的连续消息合并成一组（组头才出头像与时间），悬停气泡显示完整时间戳；其余四个 tab 保持最新在前。
2. **复合游标取代 offset**：消息列表改 `(before_send_time, before_id)` 复合游标，条件为 `WHERE (send_time < ? OR (send_time = ? AND id < ?))`，每页取 **7+1** 条以计算 `has_more`，响应返回 `has_more` 与 `next_cursor`（契约重新生成 swagger 与两端 `openapi.d.ts`）。进入会话加载最新 7 条并滚到底；上滚到顶再取 7 条更旧的，用 `scrollHeight` 差值锚点保位；全部加载完只提示一次「没有更早的消息」。WS 通知**增量 append** 到同一升序数组（`requestId` 防陈旧响应覆盖），不做整表替换；新消息仅在用户本来就在底部时自动跟随。
3. **整会话已读 + 字符串 ID + 两处 badge 同刷**：后端已读 DTO 从 `[]uint64` 改为 `[]ctype.ID`（**绝不转成 JS number**），不再吞错误；接口扩展 `session_id` 模式，一次把「接收方是我、`msg_status < read`、清除水位之上」的全部行标已读，`msg_id_list` 保留兼容。成功后同时 `refreshSummary()` 与 `loadChatSessions()`，让会话 badge 与消息中心 badge 立即清零；失败静默重试一次、不弹提示。自己发出的消息只在最后一条保留极小的「已送达 / 已读」。
4. **清除水位换成时间口径**：`clear_before_msg_id` 的 `id > ?` 改为基于 `send_time` 的水位，**消息过滤与未读对账公式同步改**，并补测试守住「清空会话 → 未读不复活」。
5. **四个通知 tab 进入即已读，但不做列表实时化**：首页加载成功后，按当前 tab 的分组批量标记，复用既有 `POST /api/sitemsg {t}` 与 `POST /api/global_notif/read`，**不新增端点**；清行未读与 badge，保留「全部已读」按钮作手动兜底，幂等、失败静默重试一次、无提示。四个 tab 仍**不订阅 WS、不轮询**，列表实时性记为已知边界（决定清单 Q24）。

## 理由与已否决方案

- **拿 `id` 当时钟 / 按 `id` 单独排序**：FNV 派生的 client-key 主键落在 `[2^62, 2^63)`（ADR-0022 决策 4），既不代表先后、也不与雪花号段连续；一排序就会把幂等消息甩到列表两端。**否决**，`id` 只能做并列裁决。
- **继续用 `LIMIT/OFFSET` 上翻**：用户滚到顶部读历史期间，任何一条新消息落库都会让后续 offset 整体平移，出现重复或跳条；`count` 也回答不了「还有没有更早的」。**否决**，改复合游标 + 7+1。
- **把字符串 ID 转成 JS number 迎合 `[]uint64`**：ID 超过 `2^53`，转换即静默丢精度，且 `ctype.ID` 底层为 string 是全仓硬约束（[ADR-0007](./0007-id-string-type-as-truth.md)）。**否决**，改后端 DTO。
- **只标已加载的 7 条**：更早的未读仍在，会话 badge 与消息中心 badge 都清不掉，「进入即已读」等于没做。**否决**，一次调用标整会话。
- **给四个通知 tab 加 WS 列表订阅 / 轮询**：会把本批扩成实时改造，且它们已有的 20 / 页向下加载与本决定无关。**本轮延后**（已知边界 Q24），只统一已读语义与视觉骨架。
- **与 ADR-0025 的分工**：ADR-0025 把**文章计数**降为按关系行重算的派生值；本 ADR 动的是私信**未读对账公式**，两者不是同一本账——但「公式不能依赖非单调主键」是同一条判据，所以决策 4 必须同时改过滤与对账两处，不能只改其中一处造成并账口径分裂。ADR-0025 的 30 分钟并账、读路径 `列 + 增量` 一概不动。
- **与 ADR-0026 的分工**：ADR-0026 是发送侧风控（按对限流 + 屏蔽 / 拉黑）并**首次为私信模块新增表** `chat_block_models`；本批**不新增表、不加列、不做 ALTER**，`chat_block_models` 仍是私信模块唯一被批准新增的表，整会话已读也不触碰它。ADR-0026 的限流预占 / 回滚、失败关闭与 HTTP 唯一发送通道一并继续生效。

## 代价 / 不做什么

- **契约必须同批重生成**：`has_more` / `next_cursor` / `session_id` / `[]ctype.ID` 都落在 swagger 与两端 `openapi.d.ts` 里；契约与两端前端拆开交付会让中间态 CI 红，因此整批一个 PR。
- **前端多一份状态责任**：唯一升序数组、反转 / 前插、`requestId` 防陈旧、`scrollHeight` 锚点都收在私信侧，不能顺手抽一个「共享分页 composable」给四个通知 tab——它们方向相反，共用只会耦合。
- **四个通知 tab 保持 20 / 页 + 向下滚动加载更多**：只统一视觉与骨架，不动分页代码；列表不订阅 WS、不轮询。
- **不做什么**：不新增表、不给既有表加列、不做 ALTER（零 DDL，部署无需 `-db`）；不做 WS ack/retry、消息编辑 / 撤回、举报队列（沿用 ADR-0022 / ADR-0026 非目标）；不做浏览器 E2E；不为四个 tab 新开批量已读端点；不改 ADR-0025 的并账周期与读路径。

## 当前生效约束

1. **HTTP 仍是唯一发送通道**（ADR-0022 决策 1 不变）；WS 只做下行通知，不承担落库或重发，写副作用仍走发件箱。
2. **列表 API 的 DESC 契约不变**：后端返回最新在前，`id` 只作确定性并列裁决、不是时序；**升序展示是前端职责**，前端只维护一个升序数组。
3. **消息分页只能用复合游标** `(before_send_time, before_id)`：禁止重新引入 offset，禁止把 `id` 单列当排序键；响应必须带 `has_more` 与 `next_cursor`，每页 7 条、7+1 探测。
4. **已读是整会话语义**：`session_id` 一次标全（保留 `msg_id_list` 兼容），ID 一律 `ctype.ID` 字符串、绝不转 JS number；成功后两处 badge 同刷。
5. **清除水位是时间口径**：消息过滤与未读对账公式都以 `send_time` 为准，不得再出现 `id > ?`；#266「清空会话 → 未读不复活」由测试守住。
6. **其余四个 tab**：进入即按组批量已读、复用既有端点；列表保持最新在前、20 / 页向下加载，不订阅 WS、不轮询；五个子项路由 / 语义、`ssr:false` 渲染边界、token 系统与暗色、全部设计门禁 canary 不变。

## 相关代码与文档

- 决定清单：[2026-11-05 消息中心重构决策日志](../notes/2026-11-05-message-center-redesign-decisions.md)（Q1–Q41，含三条根因与验收清单）
- 根因代码：`backend/internal/chat/adapter/mysql/chat.go`（`ListMessages` 的 ORDER BY / LIMIT-OFFSET / 清除水位）· `backend/internal/chat/domain/chat.go`（`Page.Normalize`）· `backend/internal/chat/adapter/http/handler.go`（`IDListRequest`）· `frontend/app/composables/useInboxCenter.ts`（原样赋值与吞错误）· `frontend/app/services/inbox.ts`（字符串 ID）
- 契约与类型：[架构 · 契约](../architecture/contracts.md) · [ADR-0003](./0003-generated-contract-single-source.md)（代码生成为单一来源）· [ADR-0007](./0007-id-string-type-as-truth.md)（`ctype.ID` 底层 string）· `backend/docs/swagger.json` · 两端 `app/types/openapi.d.ts`
- 上游决策：[ADR-0022](./0022-chat-dual-channel-http-write-ws-notify.md)（双通道、派生主键保留区间、发件箱；文末「后续修订（2026-11-05，消息中心重构）」）· [ADR-0025](./0025-count-ledger-and-reconciliation.md)（并账重算，判据同源、账本不同）· [ADR-0026](./0026-chat-send-side-risk-control.md)（发送侧风控与唯一获准新增表）；硬约束 A1 见 [架构总纲 §六](../architecture/overview.md)
- 概念：[GLOSSARY · 十一、私信与会话](../../GLOSSARY.md)（会话 / 私信）· [GLOSSARY · 零 DDL](../../GLOSSARY.md) · [GLOSSARY · 渲染边界](../../GLOSSARY.md)
- 门禁与渲染边界：`frontend/test/gates/`（设计门禁与 canary）· `frontend/shared/readerRenderingBoundary.ts` · [ADR-0019](./0019-reader-rendering-and-cache-strategy.md)（`/messages/**` 为 `ssr: false`）

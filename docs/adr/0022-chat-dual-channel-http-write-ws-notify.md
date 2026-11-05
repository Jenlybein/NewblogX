# ADR-0022：私信双通道——HTTP 落库为准、WebSocket 只做通知、写副作用走发件箱

- 状态：已交付
- 日期：2026-11-03
- 来源：2026-11-03 私信重构（[#189](https://github.com/Jenlybein/NewblogX/issues/189) 合批的[「私信」段](../notes/2026-11-02-grill-session.md)）· 落地分票 [#190](https://github.com/Jenlybein/NewblogX/issues/190)（模块分层与表名修正）· [#201](https://github.com/Jenlybein/NewblogX/issues/201)（后端双通道与发件箱）· [#202](https://github.com/Jenlybein/NewblogX/issues/202)（陌生人开关与前端契约）

## 背景与问题

私信此前只有一个通道：WebSocket。发送依赖一条活着的 WS 连接——网络一断、页面一刷新、对方离线，消息就发不出去；连接重连期间也无法补发。WS 处理器自己承担鉴权、票据、收件人校验、限流、落库与在线路由，任何一段失败都表现为"发不出消息"，是整批问题里最影响可用性的一条。

同时，项目有两条既有的硬约束：

1. **零 DDL**：不能给私信表加列，也不能为了幂等或投递状态另开一张表（新建表需要 ADR 立项）。
2. **写副作用走发件箱**：跨模块的写入副作用（通知、索引、缓存）必须在业务事务内写成 outbox 事件，由独立中继重试与进死信，不能在事务里直接操作连接池或索引。

于是需要一次决策，把"可靠发送"与"即时到达"拆成两个职责。

## 决策

1. **HTTP 落库是唯一发送通道**：`POST /chat/messages` 在数据库事务内完成消息写入，成功即视为发送成功。WS 断线不影响发送，重连后打开会话即可拉到消息。
2. **WebSocket 降级为通知通道**：`GET /chat/ws` 只负责鉴权、连接生命周期与下行通知帧（"有新消息 / 对方已读"）；接收方收到通知后自己走 HTTP 刷新会话、消息与未读数。WS 不再承担落库，也不承担重发。
3. **写副作用走既有发件箱**：发送与已读的事务里写 `chat.message.sent` / `chat.message.read` 事件；中继投递成 WS 通知。不在业务事务里直接写连接池。通知天然幂等：重复投递只是多一次刷新信号；对方不在线（推送 0 条）不算失败，不重试、不进死信。
4. **幂等用"派生主键"实现，不新增表**：客户端可带 `client_msg_id`；`(发送人, client_msg_id)` 经 FNV-1a 映射到 **[2^62, 2^63)** 的保留区间，作为这条消息的主键。重复提交撞主键，数据库保证不产生第二条消息；服务端返回首次落库的那条并标记 `duplicated=true`（调用方据此回滚发送额度预占）。
5. **保留区间不得被其他号段占用**：雪花 ID 在可预见的年份远小于 2^62（纪元 2024、41 位毫秒到 2058 年才逼近上界），因此与正常消息主键不相交；若 ID 生成器的纪元或位宽将来变化，必须先复核该前提，否则正常消息可能撞上幂等键。
6. **river 等无连接池的进程跳过私信事件**：只有真正持有 WS 连接池的进程（`-role=api` / `-role=all`）才能领取私信事件；其他角色通过 `outboxrelay.Options.SkipEventNames` 跳过 `chat.message.sent` / `chat.message.read`。**判据由装配处显式传入**（`app.OutboxWiringOptions{DeliverChatPush}`，main 按 role 映射），不再从"`WireChat` 有没有跑过"反推。否则事件会被"没人订阅也当成功"地标成 done，通知永久丢失。

## 理由与已否决方案

核心判断是：**"消息有没有发出去"和"对方此刻能不能看到"是两件事，可靠性只能落在数据库上**。WS 是易失的连接，把落库挂在它上面，等于把可用性交给网络；把落库收进 HTTP 事务，再把"即时到达"降级成可丢的通知，就能同时满足断线不丢消息与在线即时到达。

被否决的方案：

- **继续只用 WS 发送**（现状）：断线即不能发、重连无补发；处理器还要同时做鉴权、限流与落库，失败面不可收敛。**否决**。
- **HTTP 落库后在事务里直接推 WS**：事务没提交就推送，回滚时会推出不存在的消息；推送失败也无法重试。**否决**，改用发件箱。
- **新增"幂等键查重表"**：零 DDL 下要单开一张表并再立一条 ADR，只为存一个短期键；而"派生主键 + 撞主键"由数据库保证并发与崩溃场景，成本更低。**否决**。
- **给消息表加 `client_msg_id` 列**：直接违反既有表零列变更。**否决**。
- **为私信引入消息队列**：与项目既有的"发件箱 + 中继"重复，运维面反而变大。**否决**。

## 代价 / 不做什么

- **即时性有秒级延迟**：通知要等中继轮询领取事件，不是同一次请求内到达。消息已经落库，延迟只影响"对方什么时候看到红点"。
- **消息主键占用保留区间**：`[2^62, 2^63)` 从此不能给任何其他号段使用；ID 生成器变更时要同步复核（见决策 5）。
- **跳过开关是显式装配参数，但仍依赖调用方映射正确**：`WireOutboxRelay` 接收 `OutboxWiringOptions{DeliverChatPush}`，main 的 `outboxWiringOptionsForRole` 按 role 给出值；`relayOptions` 只在 `DeliverChatPush=false` 时设置 `SkipEventNames`。判据不再看包级连接池或调用顺序，但"角色映射写错"仍可能发生，因此两侧都有守卫测试：`app/outbox_role_test.go` 断言 relayOptions 对两种角色的跳过清单，`cmd/server/outbox_role_test.go` 断言 role → options 映射。
- **仍有一个死配置**：按人的 `stranger_chat_enabled` 字段与用户设置接口字段还在，但已不参与判定；清理需要另开票（零 DDL 不动既有表）。
- **不做什么**：不新增私信表或列；不引入消息队列；不做 WS 的 ack/retry 语义；不做消息编辑 / 撤回；不做端到端浏览器测试；发送侧的审核 / 风控留在 `docs/TODO.md` 待设计。

## 当前生效约束

1. **发送唯一通道是 HTTP**（`POST /chat/messages`），落库为准；WS 只做下行通知，不得承担落库或重发。
2. **每个写副作用在业务事务内写发件箱**（`chat.message.sent` / `chat.message.read`）；事务内不得直接写 WS 连接池、索引或缓存。
3. **幂等键派生主键**：`(发送人, client_msg_id)` 落在 `[2^62, 2^63)`；重复提交撞主键返回首次落库的消息，不产生第二条。该区间不得被雪花或其他号段占用，ID 生成器变更须复核。
4. **私信事件只由持有连接池的进程领取**：`api` / `all` 角色领取；river 等角色必须通过 `SkipEventNames` 跳过两条私信事件。
5. **"跳过"由 `OutboxWiringOptions{DeliverChatPush}` 显式决定**：api / all 为 true，river 为 false；不得再引入从装配结果（包级连接池、WireChat 是否调用）反推的判据。修改角色映射时必须同步 `outboxWiringOptionsForRole` 与两侧守卫测试。
6. **通知失败不算失败**：对方离线时推送 0 条属正常，不重试、不进死信；消息已落库，下次打开会话可拉到。
7. **陌生人开关是站点级运行时配置，缺省为"关"**：默认值必须明示，不靠"配置缺行"隐式关闭；按人开关仍是死配置，另行清理。

## 相关代码与文档

- 领域与用例：[chat/domain](../../backend/internal/chat/domain/chat.go)（事件名、`ClientKeyIDBase`、`MessageIDFromClientKey`、事务内写发件箱的命令端口）· [chat/app](../../backend/internal/chat/app/service.go)（发送 / 已读 / 删除用例与额度）
- 装配与中继：[app/chat_wiring.go](../../backend/app/chat_wiring.go)（`WireChat`、`Infra.ChatHub`、`chatSubscriber`）· [app/outbox_wiring.go](../../backend/app/outbox_wiring.go)（`WireOutboxRelay` 的 `OutboxWiringOptions` 与 `relayOptions`）
- HTTP / WS 适配：[chat/adapter/http](../../backend/internal/chat/adapter/http/handler.go)（`POST /chat/messages`、`GET /chat/ws`）· [chat_router.go](../../backend/internal/platform/http/router/chat_router.go)
- 中继机制：[ADR-0008](./0008-dead-letter-replay.md)（重投与死信）· [outboxrelay](../../backend/internal/platform/cdc/outboxrelay/relay.go)
- 概念：[GLOSSARY · 私信 / 会话](../../GLOSSARY.md) · [GLOSSARY · 零 DDL](../../GLOSSARY.md)
- 运行与约束：[README §十三 代码约定](../../README.md#十三代码约定改代码前请读) · [docs/TODO.md](../TODO.md)（发送侧审核 / 风控待设计）

## 后续修订（2026-11-04，#243 / #247）

决策与约束正文一字未改；以下四处口径已被后续票取代或补充，**约束 7 与「代价 / 不做什么」第 4 条按本节为准**。

| 本 ADR 位置 | 原文口径 | 修订后 |
| --- | --- | --- |
| 约束 7 末句 | 按人开关仍是死配置，另行清理 | **按人开关已启用**：`user_conf_models.stranger_chat_enabled`（账号设置隐私区，默认开），`PUT /users/info` 新增可选字段 `stranger_msg_enabled` 读写它；有效判决 = 站点总闸 **AND** 接收方个人开关，任一级关闭即拒绝（#243） |
| 代价段第 4 条 | 仍有一个死配置：按人字段不参与判定 | 同上，不再是死配置；站点级默认值仍是**显式关闭**（`site_default_settings.yaml` 写 `stranger_msg_enabled: false`），不靠缺键隐式关闭 |
| 拒绝表达 | 只有站点级文案（把人引向管理员） | 判定结论带「哪一级拒绝」：站点级 `chat.stranger_disabled`（正确动作是联系管理员开启站点开关）、接收方级 `chat.recipient_stranger_disabled`（正确动作是关注对方 / 等对方开启，找管理员查不出问题）（#247） |
| WS 生命周期 | 只说「下次打开会话可拉到」 | **被动断线自动退避重连**：1s → 2s → … → 上限 30s、每次最多 20% 抖动；取票失败同样排重试；主动 `close()`（登出 / 卸载）不重连。重连成功后立刻重取一次未读汇总（`onReconnected` → `refreshSummary`），因为断线窗口里到达的通知根本不会补发（#247） |

HTTP 仍是唯一发送通道（决策 1 不变）：重连只影响"通知能不能到达"和"未读准不准"，不承担落库或补发。

相关代码：[chat/domain](../../backend/internal/chat/domain/chat.go)（`StrangerVerdict` / `CheckStranger`）· [app/chat_wiring.go](../../backend/app/chat_wiring.go)（两级取值与失败关闭）· [user/adapter/http/compat_dto.go](../../backend/internal/user/adapter/http/compat_dto.go)（`stranger_msg_enabled`）· [apperr](../../backend/internal/apperr/apperr.go)（两个陌生人错误码）· [useChatSocket.ts](../../frontend/app/composables/useChatSocket.ts) · [useMessageRealtime.ts](../../frontend/app/composables/useMessageRealtime.ts)

## 后续修订（2026-11-04，#268）

决策与约束正文一字未改；以下两处口径已被 [ADR-0026](./0026-chat-send-side-risk-control.md) 取代或补充，**「代价 / 不做什么」末条按本节为准**。

| 本 ADR 位置 | 原文口径 | 修订后 |
| --- | --- | --- |
| 「代价 / 不做什么」末条 | 发送侧的审核 / 风控留在 `docs/TODO.md` 待设计 | **发送侧风控已交付**：限流改为每对 `(sender, receiver)` 1 条 / 10 秒 + 99 条 / 天（替换旧的 60/60s、30/60s 与周配额 1 / 3 条），并新增「屏蔽 / 拉黑」分级；见 [ADR-0026](./0026-chat-send-side-risk-control.md)（#269 规格 / #268 落地票） |
| 决策 1 的零 DDL 立项要求 | 不新增私信表或列；新建表需要 ADR 立项（一直未行使） | **首次行使**：新增私信模块第一张新表 `chat_block_models`（屏蔽 / 拉黑记录），由 ADR-0026 立项；既有三张私信表仍零列变更，表按 `schema.sql` 的 `CREATE TABLE IF NOT EXISTS` + 手动 `-db` 落地（README §11.4.2） |

HTTP 唯一发送通道（决策 1）、零列变更、写副作用走发件箱**三条不变**：风控判定收在用例层且失败关闭，不改 WS 只做通知、不改发件箱口径。WS ack/retry、消息编辑 / 撤回、举报队列、浏览器 E2E 仍是本 ADR 的非目标。

相关代码与文档：[ADR-0026](./0026-chat-send-side-risk-control.md) · [GLOSSARY · 私信与会话](../../GLOSSARY.md)（屏蔽 / 拉黑）· [docs/TODO.md](../TODO.md) · `backend/internal/chat/` · `backend/app/chat_wiring.go` · `backend/internal/platform/mysql/ddl/schema.sql`

## 后续修订（2026-11-05，消息中心重构）

决策与约束正文一字未改；以下三处口径由 [ADR-0027](./0027-message-center-ordering-pagination-and-read-receipt.md) 取代或补充，**「代价 / 不做什么」的 WS / 前端列表刷新边界、「消息列表读取」与「清除水位」按本节为准**。

| 本 ADR 位置 | 原文口径 | 修订后 |
| --- | --- | --- |
| 「代价 / 不做什么」的 WS / 前端列表刷新边界 | 接收方收到通知后自己走 HTTP 刷新会话、消息与未读数；四个通知 tab 的列表不订阅 WS 列表 | **四个 tab 仍不订阅 WS 列表、不轮询（不变，Q24 已知边界）**；私信改为在同一升序数组上**增量 append**，不再整表替换已积累列表（保留 `requestId` 防陈旧响应） |
| 「消息列表读取」 | 通知触发后整体刷新消息列表；`POST /api/chat/read` 按已加载消息逐条标已读，DTO 绑定 `[]uint64` | **复合游标** `(before_send_time, before_id)` 每页 7 条 + `has_more` / `next_cursor` 取代 offset；后端 DESC 契约不变、前端维护**唯一升序数组**；**整会话已读**（`session_id` 模式、`msg_id_list` 保留兼容），DTO 改 `[]ctype.ID`，成功后同刷会话与消息中心两处 badge |
| 「清除水位」 | `clear_before_msg_id` 按 `id > ?` 过滤消息，并账沿用主键大小 | **改为基于 `send_time` 的时间水位**：消息过滤与未读对账公式同步改并补测试，守住「清空会话 → 未读不复活」（#266）；派生主键落在 `[2^62, 2^63)` 非单调，不能再当水位 |

HTTP 唯一发送通道、零 DDL、发件箱三条不变。

## 后续修订（2026-11-06，#280）

决策与约束正文一字未改；**零 DDL、HTTP 唯一发送通道、发件箱同事务三项继续一字不动**。以下口径由 [ADR-0026](./0026-chat-send-side-risk-control.md) / [ADR-0027](./0027-message-center-ordering-pagination-and-read-receipt.md) 的本次重写补充：

| 本 ADR 位置 | 原文口径 | 修订后 |
| --- | --- | --- |
| 「代价 / 不做什么」与约束 7 的发送侧准入 | 陌生人两级开关 + 各处判定 | **私信发送资格一次判完、一律失败关闭**：站点总闸、接收方个人开关、屏蔽 / 拉黑分级、按对限流、图片来源在 `sendGuard.check` 一个点判定；依赖缺失或异常都拒绝，不再有 nil 放行分支（[ADR-0026](./0026-chat-send-side-risk-control.md)） |
| 私信装配位置 | 处理器由 `app.WireChat` 写入路由包包级变量 | **装配收进 `internal/chat.New`**；合成根只注入基础设施与跨模块窄能力，处理器经 `router.Handlers{Chat: ...}` 显式传入；只清私信这一处包级状态，不宣称全仓已清 |
| 会话列表的拉黑过滤 | 先查库分页、再在内存里按全部拉黑目标数扣总数 | **过滤与计数都下沉 SQL**（`SessionQuery.HideBlocked`），`total` 为过滤后真值；管理员视图不过滤拉黑是写明了的**有意例外** |
| 会话可见集 / 未读数 | 三处查询判定式 | **同一份实现**（`visibility.go`）；消息列表、整会话已读、消息中心角标与并账都消费它（[ADR-0027](./0027-message-center-ordering-pagination-and-read-receipt.md)） |

HTTP 唯一发送通道、零 DDL（`chat_block_models` 仍是唯一经 ADR 指名新增的私信表、既有表零列变更）、发件箱同事务三条不变。WS ack/retry、消息编辑 / 撤回、举报队列、浏览器 E2E 仍是本 ADR 的非目标。

相关代码：[internal/chat/module.go](../../backend/internal/chat/module.go) · [internal/chat/app/eligibility.go](../../backend/internal/chat/app/eligibility.go) · [internal/chat/adapter/mysql/chat.go](../../backend/internal/chat/adapter/mysql/chat.go) · [router/chat_router.go](../../backend/internal/platform/http/router/chat_router.go) · [GLOSSARY · 私信与会话](../../GLOSSARY.md)

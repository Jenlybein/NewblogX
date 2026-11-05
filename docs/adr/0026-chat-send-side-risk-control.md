# ADR-0026：私信发送资格——一次判完、一律失败关闭

- 状态：已交付
- 日期：2026-11-06（初版 2026-11-04；按 #272 / #280 正文整体重写）
- 来源：规格 [#272](https://github.com/Jenlybein/NewblogX/issues/272) · 落地票 [#280](https://github.com/Jenlybein/NewblogX/issues/280) · 初版来自发送侧风控评审 [#269](https://github.com/Jenlybein/NewblogX/issues/269) / [#268](https://github.com/Jenlybein/NewblogX/issues/268) · 修订 [ADR-0022](./0022-chat-dual-channel-http-write-ws-notify.md)

## 背景与问题

[ADR-0022](./0022-chat-dual-channel-http-write-ws-notify.md) 把发送收成 HTTP 唯一通道，但发送前的准入仍散在用例层：站点总闸与接收方个人开关合成在装配层的 `StrangerPolicy` 里，屏蔽 / 拉黑、关注关系、按对限流、图片来源分别在各处判断。每个依赖都有自己的"为空就跳过"分支——`blocks` / `relations` / `limiter` 没注入时直接放行，`stranger` 没注入时则会在判定点 panic。结果是同一个问题（这个人现在能不能给那个人发消息）没有一处完整答案：

- 装配遗漏表现为静默失效：风控没上场，发送照样成功；
- 判定顺序靠读代码才能还原，新增一项资格很容易漏掉某个分支；
- 失败关闭只在部分依赖上成立（图片来源），其余依赖是"出错再说"。

#280 要求把发送资格收成**一次判完、一律失败关闭**：站点总闸、接收方个人开关、屏蔽 / 拉黑分级、按对限流、图片来源五项在一次判定里给出结论；任何一项依赖不可用或查询异常，都不得放行发送。

同一批还收口了两件同源的事：会话列表的拉黑过滤原先"先查库分页、再在内存里按全部拉黑目标数扣总数"，总数不是过滤后的真值；私信装配原先摊在 `backend/app/chat_wiring.go`，并把处理器写进路由包的包级变量。

## 决策

1. **发送资格只有一个判定点**：`backend/internal/chat/app` 的 `sendGuard.check` 一次完成五项判定——图片来源（仅图片消息）、屏蔽 / 拉黑（双向 `Level`）、用户展示与关注关系、陌生人两级策略、按对限流预占。通过时返回完整的发送计划（双方展示快照、关系级别、已预占额度、归一化正文），否则返回拒绝；调用方不再逐项判断。
2. **一律失败关闭**：五项资格依赖（`blocks` / `relations` / `stranger` / `limiter` / `imageCheck`）任一为零值，或任一查询返回错误，发送都被拒绝（缺依赖时用 `chat.send_failed`，不冒充成"被屏蔽"或"陌生人被拒"）。构造 `Service` 只对结构必需的 `read` / `write` / `users` panic；资格依赖缺失留到发送时变成可观测的拒绝，而不是启动期静默跳过。
3. **管理员视图仍看全量，且写明是有意例外**：读者视图的会话列表在查库层过滤拉黑（`SessionQuery.HideBlocked=true`）；管理员视图传 `HideBlocked=false` + `Unscoped=true`，连软删会话一起看。这不是漏判，是运维排查的必要例外，代码与测试都显式标注。
4. **拉黑过滤下沉到查库层**：`ListSessions` 的 SQL 用 `NOT EXISTS (SELECT 1 FROM chat_block_models ... level = 2 ...)` 过滤，计数与取页共用同一条件，`total` 是过滤后的真值；用例层不再拿"全部拉黑目标数"扣总数。`BlockPolicy` 端口随之只保留 `Level`。
5. **装配收进模块、包级状态只清私信一处**：九个协作者由 `internal/chat.New` 在模块内装配成服务与处理器，合成根（`backend/cmd/server/main.go` 经由 `backend/app`）只注入基础设施与跨模块窄能力（关注关系、用户偏好、认证）。路由包不再持有 `chatHandler` / `SetChatHandler`，处理器经 `router.Handlers{Chat: ...}` 显式传入。其他模块的包级 `xxxHandler` / `SetXxxHandler` 是存量，本次不清，也不宣称全仓已无包级可变状态。
6. **零 DDL 边界不变**：本批不新增表、不加列、不改列、不做 ALTER；`chat_block_models` 仍是私信模块唯一经 ADR 指名新增的表（由本 ADR 初版立项），既有三张私信表列一律未动。新增表只走 `schema.sql` 的 `CREATE TABLE IF NOT EXISTS` + 手动 `-db`，无运行时 AutoMigrate。

## 理由与已否决方案

**为什么把五项收进一个判定点**：发送资格是一件事，不是五件相邻的事。分散判断时，任何一项的"依赖没注入"都可能变成放行；聚成一处后，失败关闭是唯一出口，新增一项资格也只有一个地方要改。五个依赖仍然以窄接口注入（端口由使用方定义），没有把 `*gorm.DB` 或别的模块的 service 塞进来。

被否决的方案：

- **保留每个依赖的 nil 放行分支**：装配遗漏等于风控静默关闭，且 `stranger` 的 nil 还会 panic——这正是本批要修的病灶。**否决**。
- **把资格依赖缺失改成构造期 panic**：能让服务起不来，但掩盖了"哪一项没上场"；而且单测 / 记录回放类装配会被迫构造全套依赖。**否决**，改为发送时失败关闭。
- **把五项合成一个跨模块大接口**：会让私信模块反过来定义别的模块该实现什么，破坏"端口由使用方定义、模块间零引入"。**否决**，保持五个窄端口，判定点收在模块内部。
- **管理员视图也过滤拉黑**：运维排查看不到被拉黑用户的会话，把"有意例外"当"漏判"改掉。**否决**。
- **继续在内存里扣总数**：扣的是全部拉黑目标数，不是本页被扣掉的行数；分页与总数会一起失真。**否决**，过滤与计数都交给 SQL。
- **把处理器继续挂在路由包包级变量上**：包级可变状态让路由表依赖装配顺序，测试之间互相污染（`NewEngine` 在没注入时 panic）。**否决**，改为显式参数；但不顺手清其他模块（范围外）。
- **为发送资格新增表 / 列**：零 DDL 下不需要也不允许；按对限流仍在 Redis，屏蔽 / 拉黑只读既有 `chat_block_models`。**否决**。

## 代价 / 不做什么

- **校验依赖必须全部装配**：生产装配漏掉任一项，发送会全部被拒（`chat.send_failed`）。这是有意的：宁可发不出去，也不静默失效；启动守卫与门禁负责尽早发现。
- **多一次查询**：屏蔽 / 拉黑每次发送查两行、关注关系查一次、限流仍在 Redis；这些都在发送路径上，接受。
- **会话列表的 SQL 直接引用 `chat_block_models`**：私信读端口与拉黑表同库同模块，语义仍是"过滤我拉黑的人"；管理员例外由调用方显式传参。
- **只清私信一处包级状态**：其他模块仍保留 `SetXxxHandler`，全仓清理不在本批。
- **不做什么**：不做内容审核队列与识别；不做 WS ack/retry、消息编辑 / 撤回、举报队列、浏览器 E2E；不新增表 / 列；不引入运行时 AutoMigrate；不提前改消息列表方向（#281 的范围）。

## 当前生效约束

1. **一次判定、一律失败关闭**：发送资格的五项在 `sendGuard.check` 一次判完；依赖缺失或异常一律拒绝，任何 nil 放行分支都不得重新出现。
2. **HTTP 仍是唯一发送通道**（ADR-0022 决策 1 不变）：`POST /chat/messages` 事务内落库为准，WS 只做下行通知；写副作用仍走发件箱。
3. **管理员视图不过滤拉黑是有意例外**：读者视图 `HideBlocked=true`；管理员视图 `HideBlocked=false` + `Unscoped=true`，任何"为整齐统一"的改动都视为回归。
4. **会话列表的拉黑过滤在查库层完成**：`total` 与分页都基于过滤后的行集，禁止恢复"先分页、再按全部拉黑目标数扣总数"。
5. **装配收进模块**：`internal/chat.New` 是私信九个协作者的唯一装配点；合成根只注入基础设施与跨模块窄能力；私信处理器经 `router.Handlers` 显式传入，不得回到包级变量。
6. **零 DDL**：`chat_block_models` 是唯一经 ADR 指名新增的私信表；既有私信表零列变更；不再新增表 / 列，不做 ALTER。

## 相关代码与文档

- 判定点：[internal/chat/app/eligibility.go](../../backend/internal/chat/app/eligibility.go)（`sendGuard` / `sendPlan`）· [internal/chat/app/service.go](../../backend/internal/chat/app/service.go)（`Send` / `Sessions`）
- 查库层过滤：[internal/chat/adapter/mysql/chat.go](../../backend/internal/chat/adapter/mysql/chat.go)（`ListSessions` / `SessionQuery.HideBlocked`）
- 装配：[internal/chat/module.go](../../backend/internal/chat/module.go)（`chat.New`）· [backend/app/chat_wiring.go](../../backend/app/chat_wiring.go)（跨模块适配器）· [router/chat_router.go](../../backend/internal/platform/http/router/chat_router.go)
- 概念：[GLOSSARY · 十一、私信与会话](../../GLOSSARY.md)（私信发送资格 / 会话可见集 / 会话未读数 / 屏蔽 / 拉黑）
- 上游决策：[ADR-0022](./0022-chat-dual-channel-http-write-ws-notify.md)（双通道、零 DDL、发件箱；文末「后续修订（2026-11-06，#280）」）· [ADR-0027](./0027-message-center-ordering-pagination-and-read-receipt.md)（会话可见集与未读数）· [ADR-0001](./0001-capability-interfaces-over-deps.md) / [ADR-0002](./0002-vertical-modules-and-composition-root.md)（能力接口与合成根的适用范围）
- 建表：[backend/internal/platform/mysql/ddl/schema.sql](../../backend/internal/platform/mysql/ddl/schema.sql) · [README §11.4.2](../../README.md#1142-升级到带新表的版本手动建一次表)（升级手动 `-db`）

# ADR-0018：私信双通道——HTTP 落库为准、WebSocket 只做通知、写副作用走发件箱

- 状态：已交付
- 日期：2026-11-03
- 来源：2026-11-03 私信重构（[#189](https://github.com/Jenlybein/NewblogX/issues/189) 合批的「私信」段，2026-11-02 grill 会话记录以纯文本引用）· 落地分票 [#190](https://github.com/Jenlybein/NewblogX/issues/190)（模块分层与表名修正）· [#201](https://github.com/Jenlybein/NewblogX/issues/201)（后端双通道与发件箱）· [#202](https://github.com/Jenlybein/NewblogX/issues/202)（陌生人开关与前端契约）

## 背景与问题

私信要同时解决三组问题，任何一组失守都会直接表现为「消息发不出去、看不到，或红点不对」：

1. **发送可靠性**：把落库挂在 WebSocket 连接上，网络断开、页面刷新、对方离线时消息就发不出去；连接重连期间也无法补发。WS 处理器若同时承担鉴权、票据、收件人校验、限流、落库与在线路由，任何一段失败都表现为「发不出消息」，失败面不可收敛。
2. **发送准入**：站点总闸、接收方个人开关、屏蔽 / 拉黑分级、按对限流、图片来源若散在各处判定，装配遗漏会变成静默放行；「这个人现在能不能给那个人发消息」没有一处完整答案。
3. **消息中心的读取口径**：`send_time` 有同毫秒并列、派生主键不是时序，`id > ?` 与 `LIMIT/OFFSET` 都会让列表错序、跳条；已读若只标已加载的几条，会话与消息中心两处 badge 永远清不掉。

两条硬约束贯穿全条：

1. **既有表零列变更**：私信既有三张表不加列；唯一新增的私信表是 `chat_block_models`（屏蔽 / 拉黑记录），只走 `schema.sql` 的 `CREATE TABLE IF NOT EXISTS` + 手动 `-db`，没有运行时 AutoMigrate。幂等与投递状态都不另开表。
2. **写副作用走发件箱**：跨模块的写入副作用（通知、索引、缓存）必须在业务事务内写成 outbox 事件，由独立中继重试与进死信，不能在事务里直接操作连接池或索引。

于是把「可靠发送」与「即时到达」拆成两个职责，并把准入与读取口径各自收成一个判定点、一份实现。

## 决策

1. **HTTP 落库是唯一发送通道**：`POST /chat/messages` 在数据库事务内完成消息写入，成功即视为发送成功。WS 断线不影响发送，重连后打开会话即可拉到消息。
2. **WebSocket 降级为通知通道**：`GET /chat/ws` 只负责鉴权、连接生命周期与下行通知帧（「有新消息 / 对方已读」）；接收方收到通知后自己走 HTTP 刷新会话、消息与未读数。WS 不承担落库，也不承担重发。被动断线按 1s → 2s → … → 上限 30s（每次最多 20% 抖动）自动退避重连，取票失败同样排重试；主动 `close()`（登出 / 卸载）不重连；重连成功后立刻重取一次权威未读汇总（`onReconnected` → `refreshSummary`），因为断线窗口里到达的通知不会补发。
3. **写副作用走既有发件箱**：发送与已读的事务里写 `chat.message.sent` / `chat.message.read` 事件；中继投递成 WS 通知。不在业务事务里直接写连接池。通知天然幂等：重复投递只是多一次刷新信号；对方不在线（推送 0 条）不算失败，不重试、不进死信。
4. **幂等用「派生主键」实现，不新增表**：客户端可带 `client_msg_id`；`(发送人, client_msg_id)` 经 FNV-1a 映射到 **[2^62, 2^63)** 的保留区间，作为这条消息的主键。重复提交撞主键，数据库保证不产生第二条消息；服务端返回首次落库的那条并标记 `duplicated=true`（调用方据此回滚发送额度预占）。
5. **保留区间不得被其他号段占用**：雪花 ID 在可预见的年份远小于 2^62（纪元 2024、41 位毫秒到 2058 年才逼近上界），因此与正常消息主键不相交；若 ID 生成器的纪元或位宽将来变化，必须先复核该前提，否则正常消息可能撞上幂等键。
6. **river 等无连接池的进程跳过私信事件**：只有真正持有 WS 连接池的进程（`-role=api` / `-role=all`）才能领取私信事件；其他角色通过 `outboxrelay.Options.SkipEventNames` 跳过 `chat.message.sent` / `chat.message.read`。**判据由装配处显式传入**（`app.OutboxWiringOptions{DeliverChatPush}`，main 按 role 映射），不得从「`WireChat` 有没有跑过」反推，否则事件会被「没人订阅也当成功」地标成 done，通知永久丢失。
7. **发送资格一次判完、一律失败关闭**：`backend/internal/chat/app` 的 `sendGuard.check` 一次完成五项判定——站点总闸、接收方个人开关、屏蔽 / 拉黑（双向 `Level`）、按对限流、图片来源（仅图片消息），并给出用户展示与关注关系、已预占额度、归一化正文的完整发送计划；调用方不再逐项判断。任一资格依赖缺失或查询异常都拒绝（缺依赖用 `chat.send_failed`，不冒充「被屏蔽」或「陌生人被拒」）；构造 `Service` 只对结构必需的 `read` / `write` / `users` panic。
8. **陌生人开关是用户级两级判决**：有效判决 = 站点总闸 **AND** 接收方个人开关（`user_conf_models.stranger_chat_enabled`，账号设置隐私区默认开，由 `PUT /users/info` 的可选字段 `stranger_msg_enabled` 读写）；站点级默认值必须**显式 `false`**（`site_default_settings.yaml` 写 `stranger_msg_enabled: false`），不靠缺键隐式关闭。任一级关闭即拒绝，且结论带「哪一级拒绝」：站点级 `chat.stranger_disabled`（正确动作是联系管理员开启站点开关）、接收方级 `chat.recipient_stranger_disabled`（正确动作是关注对方 / 等对方开启），两个错误码各指向正确动作。
9. **会话可见集与未读数一份实现**：`visibility.go` 一处回答「某用户在某会话里能看到的全部消息（时间水位之上、非本人删除、含自己发的）」与「可见集里接收者是我且未读的数」；消息列表、整会话已读、消息中心角标与并账都消费它，不再各写一份判定式。
10. **私信列表按 `send_time` 升序、复合游标分页**：后端按 `send_time` **升序**（最旧在前）返回，`id` 只作**确定性并列裁决**（同一毫秒内稳定分页与顺序，**不是时序**）；第一页不带游标时是最近一窗，`next_cursor` 指向本页最旧一条，继续往前翻更早的页。分页用 `(before_send_time, before_id)` 复合游标（`WHERE (send_time < ? OR (send_time = ? AND id < ?))`），每页取 **7+1** 条算 `has_more`，响应带 `has_more` 与 `next_cursor`。前端只维护**一个升序数组**（最新在底部）：第一页原样收下、更早页前插、WS 通知**增量 append**（`requestId` 防陈旧响应覆盖），不做任何反转、不做整表替换。日期分组为自然日居中分隔（今天 / 昨天 / 日期），同发送人、同一天、2 分钟内的连续消息合并成一组，悬停气泡显示完整时间戳；其余四个 tab 保持最新在前。
11. **已读是整会话语义**：`POST /api/chat/read` 支持 `session_id` 模式，一次把「接收方是我、`msg_status < read`、清除水位之上」的全部行标已读，`msg_id_list` 保留兼容；DTO 一律 `[]ctype.ID` 字符串（**绝不转成 JS number**），不再吞错误；成功后同时 `refreshSummary()` 与 `loadChatSessions()`，会话 badge 与消息中心 badge 立即清零；失败静默重试一次、不弹提示。自己发出的消息只在最后一条保留极小的「已送达 / 已读」。
12. **清除水位是时间口径**：`clear_before_msg_id` 以 `send_time` 水位过滤消息，未读对账公式同步以 `send_time` 为准，并补测试守住「清空会话 → 未读不复活」；派生主键落在 `[2^62, 2^63)` 非单调，不得再当水位或排序键。
13. **四个通知 tab 进入即已读，但不做列表实时化**：首页加载成功后按当前 tab 的分组批量标记，复用既有 `POST /api/sitemsg {t}` 与 `POST /api/global_notif/read`，不新增端点；清行未读与 badge，保留「全部已读」按钮作手动兜底，幂等、失败静默重试一次、无提示。四个 tab 仍不订阅 WS、不轮询，保持 20 / 页向下加载，列表实时性记为已知边界。
14. **装配与查库层收口**：九个协作者由 `internal/chat.New` 在模块内装配成服务与处理器，合成根（`backend/cmd/server/main.go` 经由 `backend/app`）只注入基础设施与跨模块窄能力（关注关系、用户偏好、认证）；处理器经 `router.Handlers{Chat: ...}` 显式传入，路由包不再持有 `chatHandler` / `SetChatHandler`，只清私信这一处包级状态，不宣称全仓已清。会话列表的拉黑过滤与计数都下沉 SQL（`SessionQuery.HideBlocked` 的 `NOT EXISTS (... level = 2 ...)`），`total` 是过滤后真值；管理员视图传 `HideBlocked=false` + `Unscoped=true`，连软删会话一起看，是写明的有意例外。
15. **零 DDL 边界**：`chat_block_models` 是唯一经 ADR 指名新增的私信表；既有三张私信表零列变更，不新增表 / 列、不做 ALTER、无运行时 AutoMigrate；消息中心这批不新增表 / 列、不做 ALTER，部署无需 `-db`。契约字段（`has_more` / `next_cursor` / `session_id` / `[]ctype.ID`）与两端 `openapi.d.ts` 必须同批重生成。

## 理由与已否决方案

核心判断是：**「消息有没有发出去」和「对方此刻能不能看到」是两件事，可靠性只能落在数据库上**。WS 是易失的连接，把落库挂在它上面，等于把可用性交给网络；把落库收进 HTTP 事务，再把「即时到达」降级成可丢的通知，就能同时满足断线不丢消息与在线即时到达。同理，发送资格是一件事，不是五件相邻的事：分散判断时任何一项「依赖没注入」都可能变成放行，聚成一处后失败关闭是唯一出口；五个依赖仍以窄接口注入（端口由使用方定义），不把 `*gorm.DB` 或别的模块的 service 塞进来。列表方向、水位、可见集也必须各自只有一个真相，否则「清空会话 → 未读不复活」与两处 badge 会各算各的。

被否决的方案：

- **继续只用 WS 发送**：断线即不能发、重连无补发；处理器还要同时做鉴权、限流与落库，失败面不可收敛。**否决**。
- **HTTP 落库后在事务里直接推 WS**：事务没提交就推送，回滚时会推出不存在的消息；推送失败也无法重试。**否决**，改用发件箱。
- **新增「幂等键查重表」**：零 DDL 下要单开一张表并再立一条 ADR，只为存一个短期键；而「派生主键 + 撞主键」由数据库保证并发与崩溃场景，成本更低。**否决**。
- **给消息表加 `client_msg_id` 列**：直接违反既有表零列变更。**否决**。
- **为私信引入消息队列**：与项目既有的「发件箱 + 中继」重复，运维面反而变大。**否决**。
- **保留每个资格依赖的 nil 放行分支**：装配遗漏等于风控静默关闭，且 `stranger` 的 nil 还会 panic。**否决**，一律失败关闭。
- **把资格依赖缺失改成构造期 panic**：能让服务起不来，但掩盖了「哪一项没上场」，单测 / 记录回放类装配也被迫构造全套依赖。**否决**，改为发送时失败关闭。
- **把五项合成一个跨模块大接口**：会让私信模块反过来定义别的模块该实现什么，破坏「端口由使用方定义、模块间零引入」。**否决**，保持五个窄端口。
- **管理员视图也过滤拉黑**：运维排查看不到被拉黑用户的会话，把「有意例外」当「漏判」改掉。**否决**。
- **继续在内存里扣会话总数**：扣的是全部拉黑目标数，不是本页被扣掉的行数；分页与总数会一起失真。**否决**，过滤与计数都交给 SQL。
- **把处理器继续挂在路由包包级变量上**：包级可变状态让路由表依赖装配顺序，测试之间互相污染。**否决**，改为显式参数；不顺手清其他模块。
- **为发送资格新增表 / 列**：零 DDL 下不需要也不允许；按对限流仍在 Redis，屏蔽 / 拉黑只读既有 `chat_block_models`。**否决**。
- **拿 `id` 当时钟 / 按 `id` 单独排序**：FNV 派生的 client-key 主键落在 `[2^62, 2^63)`，既不代表先后、也不与雪花号段连续；一排序就会把幂等消息甩到列表两端。**否决**，`id` 只能做并列裁决。
- **继续用 `LIMIT/OFFSET` 上翻**：用户滚到顶部读历史期间，任何一条新消息落库都会让后续 offset 整体平移，出现重复或跳条；`count` 也回答不了「还有没有更早的」。**否决**，改复合游标 + 7+1。
- **把字符串 ID 转成 JS number 迎合 `[]uint64`**：ID 超过 `2^53`，转换即静默丢精度，且 `ctype.ID` 底层为 string 是全仓硬约束。**否决**，改后端 DTO。
- **只标已加载的 7 条**：更早的未读仍在，会话 badge 与消息中心 badge 都清不掉，「进入即已读」等于没做。**否决**，一次调用标整会话。
- **为兼容旧客户端保留 DESC + 前端反转**：系统对外每刻只有一个形状，长期兼容层会让方向再分裂。**否决**，后端直接升序、前端唯一升序数组。
- **给四个通知 tab 加 WS 列表订阅 / 轮询**：会把消息中心扩成实时改造，且与它们已有的 20 / 页向下加载无关。**已知边界**，只统一已读语义与视觉骨架。
- **清除水位继续用 `id > ?`**：非单调 hash ID 下既会漏掉、也会错放消息，并可能破坏并账口径。**否决**，过滤与对账两处同步改成时间水位。

## 代价 / 不做什么

- **即时性有秒级延迟**：通知要等中继轮询领取事件，不是同一次请求内到达；消息已经落库，延迟只影响「对方什么时候看到红点」。
- **消息主键占用保留区间**：`[2^62, 2^63)` 从此不能给任何其他号段使用；ID 生成器变更时要同步复核。`id` 也不是时序，排序与水位只能用 `send_time`。
- **跳过开关依赖调用方映射正确**：`WireOutboxRelay` 接收 `OutboxWiringOptions{DeliverChatPush}`，main 的 `outboxWiringOptionsForRole` 按 role 给出值；判据不再看包级连接池或调用顺序，但「角色映射写错」仍可能发生，因此两侧都有守卫测试：`app/outbox_role_test.go` 断言 relayOptions 对两种角色的跳过清单，`cmd/server/outbox_role_test.go` 断言 role → options 映射。
- **发送资格依赖必须全部装配**：生产装配漏掉任一项，发送会全部被 `chat.send_failed` 拒绝；这是有意的失败关闭，宁可发不出去也不静默失效。发送路径多一次屏蔽 / 拉黑、关注关系查询，按对限流仍在 Redis。
- **会话列表 SQL 直接引用 `chat_block_models`**：私信读端口与拉黑表同库同模块，管理员例外由调用方显式传参，不为「整齐统一」而去掉。
- **前端多一份状态责任**：唯一升序数组、前插、`requestId` 防陈旧、`scrollHeight` 差值锚点保位、日期分组与 2 分钟合并都收在私信侧；新消息仅在用户本来就在底部时自动跟随，全部加载完只提示一次「没有更早的消息」。不能顺手抽一个「共享分页 composable」给四个通知 tab——它们方向相反，共用只会耦合。
- **四个通知 tab 的列表不订阅 WS、不轮询**：只统一已读语义与视觉骨架，20 / 页向下加载不动。
- **不做什么**：不引入消息队列；不做 WS 的 ack/retry 语义；不做消息编辑 / 撤回、举报队列、内容审核队列与识别；不做浏览器 E2E；除 `chat_block_models` 外不新增私信表或列、不做运行时 AutoMigrate；不给列表方向留新旧兼容层；不为四个 tab 新开批量已读端点。

## 当前生效约束

1. **发送唯一通道是 HTTP**（`POST /chat/messages`），落库为准；WS 只做下行通知，不得承担落库或重发。
2. **每个写副作用在业务事务内写发件箱**（`chat.message.sent` / `chat.message.read`）；事务内不得直接写 WS 连接池、索引或缓存。
3. **幂等键派生主键**：`(发送人, client_msg_id)` 落在 `[2^62, 2^63)`；重复提交撞主键返回首次落库的消息并标记 `duplicated=true`，不产生第二条。该区间不得被雪花或其他号段占用，ID 生成器变更须复核；`id` 不是时序，不得单独当排序键或水位。
4. **私信事件只由持有连接池的进程领取**：`api` / `all` 角色领取；river 等角色必须通过 `SkipEventNames` 跳过两条私信事件。
5. **「跳过」由 `OutboxWiringOptions{DeliverChatPush}` 显式决定**：api / all 为 true，river 为 false；不得引入从装配结果（包级连接池、`WireChat` 是否调用）反推的判据。修改角色映射时必须同步 `outboxWiringOptionsForRole` 与两侧守卫测试。
6. **通知失败不算失败**：对方离线时推送 0 条属正常，不重试、不进死信；消息已落库，下次打开会话可拉到。
7. **陌生人开关是用户级两级判决**：有效判决 = 站点总闸 **AND** 接收方个人开关（`user_conf_models.stranger_chat_enabled`，由 `PUT /users/info` 的可选字段 `stranger_msg_enabled` 读写）；站点级默认**显式 `false`**，不靠缺键隐式关闭；任一级关闭即拒绝，站点级 `chat.stranger_disabled` 与接收方级 `chat.recipient_stranger_disabled` 两个错误码不得合并。
8. **WS 被动断线必须自动退避重连**（1s → 2s → … → 上限 30s、每次最多 20% 抖动），取票失败同样排重试；主动 `close()`（登出 / 卸载）不重连；**重连成功后立刻重取一次权威未读汇总**（`onReconnected` → `refreshSummary`）。
9. **发送资格一次判完、一律失败关闭**：五项（站点总闸、接收方个人开关、屏蔽 / 拉黑分级、按对限流、图片来源）在 `sendGuard.check` 一个点判定；依赖缺失或异常一律拒绝，任何 nil 放行分支都不得重新出现；`chat.send_failed` 只用于缺依赖 / 异常。
10. **管理员视图不过滤拉黑是有意例外**：读者视图 `HideBlocked=true`；管理员视图 `HideBlocked=false` + `Unscoped=true`；任何「为整齐统一」的改动都视为回归。
11. **会话列表的拉黑过滤与计数都下沉 SQL**（`SessionQuery.HideBlocked`），`total` 是过滤后真值；禁止恢复「先分页、再按全部拉黑目标数扣总数」。
12. **装配收进 `internal/chat.New`**（私信九个协作者的唯一装配点）；合成根只注入基础设施与跨模块窄能力；私信处理器经 `router.Handlers` 显式传入，不得回到包级变量；只清私信这一处包级状态。
13. **`chat_block_models` 是唯一经 ADR 指名新增的私信表**；既有三张私信表零列变更；不再新增表 / 列、不做 ALTER、无运行时 AutoMigrate；新表只走 `schema.sql` 的 `CREATE TABLE IF NOT EXISTS` + 手动 `-db`。
14. **列表 API 返回升序**：按 `send_time` 升序（最旧在前），`id` 只作确定性并列裁决；前端只维护一个升序数组，不做任何反转；WS 通知增量 append 到同一数组（`requestId` 防陈旧），不整表替换。
15. **消息分页只能用复合游标 `(before_send_time, before_id)`**：禁止重新引入 `LIMIT/OFFSET`、禁止把 `id` 单列当排序键；每页 7 条、7+1 探测，响应必须带 `has_more` 与 `next_cursor`。
16. **已读是整会话语义**：`session_id` 一次标全（`msg_id_list` 保留兼容），DTO 一律 `[]ctype.ID` 字符串、绝不转 JS number；成功后会话 badge 与消息中心 badge 两处同刷。
17. **清除水位是时间口径**：`clear_before_msg_id` 以 `send_time` 为准，消息过滤与未读对账公式同步改，不得再出现 `id > ?`；「清空会话 → 未读不复活」由测试守住。
18. **会话可见集与未读数只有一份实现**（`visibility.go`）：消息列表、整会话已读、消息中心角标与并账都消费它；`total` / 未读数 / 已读命中数不得各算各的。
19. **四个通知 tab 首页加载成功后按组批量已读**（复用既有端点，不新增）；列表保持最新在前、20 / 页向下加载，不订阅 WS、不轮询。
20. **私信列表的契约字段**（`has_more` / `next_cursor` / `session_id` / `[]ctype.ID`）与两端 `openapi.d.ts` 必须同批由生成器产出；不新增表 / 列、不做 ALTER。
21. **不做什么**：不引入消息队列、不做 WS ack/retry、不做消息编辑 / 撤回、不做举报队列、不做内容审核队列与识别、不做浏览器 E2E、不给列表方向留兼容层。

## 相关代码与文档

- 领域与用例：[chat/domain](../../backend/internal/chat/domain/chat.go)（事件名、`ClientKeyIDBase`、`MessageIDFromClientKey`、`StrangerVerdict` / `CheckStranger`、`MessageCursor` / `Page.Normalize`、事务内写发件箱的命令端口）· [chat/app](../../backend/internal/chat/app/service.go)（发送 / 已读 / 删除用例与额度）· [eligibility.go](../../backend/internal/chat/app/eligibility.go)（`sendGuard` / `sendPlan`）· [internal/chat/module.go](../../backend/internal/chat/module.go)（`chat.New`）
- 查库与可见集：[chat/adapter/mysql/chat.go](../../backend/internal/chat/adapter/mysql/chat.go)（`ListMessages` 的升序 / 复合游标 / 清除水位、`ListSessions` 与 `SessionQuery.HideBlocked`）· [visibility.go](../../backend/internal/chat/adapter/mysql/visibility.go)（会话可见集与未读数唯一实现）
- 装配与中继：[app/chat_wiring.go](../../backend/app/chat_wiring.go)（`WireChat`、`Infra.ChatHub`、`chatSubscriber`、两级取值与失败关闭）· [app/outbox_wiring.go](../../backend/app/outbox_wiring.go)（`WireOutboxRelay` 的 `OutboxWiringOptions` 与 `relayOptions`）· [ADR-0008](./0008-dead-letter-replay.md)（重投与死信）· [outboxrelay](../../backend/internal/platform/cdc/outboxrelay/relay.go)
- HTTP / WS 适配：[chat/adapter/http](../../backend/internal/chat/adapter/http/handler.go)（`POST /chat/messages`、`GET /chat/ws`、`IDListRequest`）· [chat_router.go](../../backend/internal/platform/http/router/chat_router.go)
- 前端：[useChatSocket.ts](../../frontend/app/composables/useChatSocket.ts)（退避重连）· [useMessageRealtime.ts](../../frontend/app/composables/useMessageRealtime.ts) · [useInboxCenter.ts](../../frontend/app/composables/useInboxCenter.ts)（唯一升序数组与 `requestId`）· [chatTimeline.ts](../../frontend/app/utils/chatTimeline.ts)（合并与日期分组）· [inbox.ts](../../frontend/app/services/inbox.ts)
- 用户设置与错误码：[compat_dto.go](../../backend/internal/user/adapter/http/compat_dto.go)（`stranger_msg_enabled`）· [apperr](../../backend/internal/apperr/apperr.go)（两个陌生人错误码）
- 概念与约束：[GLOSSARY · 私信 / 会话](../../GLOSSARY.md)（发送资格 / 会话可见集 / 会话未读数 / 屏蔽 / 拉黑 / 零 DDL）· [ADR-0007](./0007-id-string-type-as-truth.md)（`ctype.ID` 底层 string）· [ADR-0003](./0003-generated-contract-single-source.md) · [ADR-0001](./0001-capability-interfaces-over-deps.md) / [ADR-0002](./0002-vertical-modules-and-composition-root.md)（能力接口与合成根）· [ADR-0016](./0016-reader-rendering-and-cache-strategy.md)（`/messages/**` 为 `ssr: false`）· [架构总纲 §六 A1](../architecture/overview.md)（零 DDL）
- 运行与约束：[README §十三 代码约定](../../README.md#十三代码约定改代码前请读) · [README §11.4.2](../../README.md#1142-升级到带新表的版本手动建一次表) · [docs/TODO.md](../TODO.md) · [schema.sql](../../backend/internal/platform/mysql/ddl/schema.sql)

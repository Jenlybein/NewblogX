# ADR-0025：计数账本、配置化并账与单一权威读路径

- 状态：已交付
- 日期：2026-11-04（初版）· 2026-11-06（正文整体重写）
- 部署：2026-11-04 随 PR [#259](https://github.com/Jenlybein/NewblogX/pull/259) 上线（`42f0938`），记录见[部署记录](../notes/2026-11-04-reading-ui-and-admin-batch-deploy-record.md)；2026-11-06 的整体重写随 [#279](https://github.com/Jenlybein/NewblogX/issues/279) 落地（父规格 [#272](https://github.com/Jenlybein/NewblogX/issues/272)）
- 来源：读者反馈「点赞后刷新，按钮已赞但计数没变」· [决定清单](../notes/2026-11-04-reading-ui-and-admin-batch-decisions.md)（§1–2）· 规格 #272 要求「并账周期配置化 + 读路径只认一个权威值入口」
- 变更说明：按 #272 / #279 的要求**正文整体重写**，显式覆盖本文 README 规则 3「已交付的 ADR 不改结论」——初版的判断（计数是派生值、并账按关系行重算覆盖、CDC 实时并账为非目标）**全部保留**，本版改的是「周期从哪来」与「值从哪个入口读」，并把被取代的旧口径写死。
- 关联：[ADR-0019](./0019-reader-rendering-and-cache-strategy.md)（两层缓存）· [ADR-0024](./0024-reader-user-state-and-count-truth.md)（写响应回填权威计数）· [ADR-0027](./0027-message-center-ordering-pagination-and-read-receipt.md)（消息的排序与水位属于消息时间口径）· [GLOSSARY · 计数账本](../../GLOSSARY.md)

## 背景与问题

文章计数（`digg_count` / `favor_count` / `comment_count` / `view_count`）不是 `COUNT(关系行)` 算出来的，而是一本**独立维护的账**：

- **持久值**：`article_models` 的计数列（并账已入账的主值）；
- **实时差**：Redis 哈希里的增量（`HINCRBY` 累加，尚未入账的部分）；
- **并账**：一条 cron 把增量并进库：`digg` / `favor` 用 `COUNT(关系行)` **覆盖整列**，`view` / `comment` 仍 `列 += 增量`（`backend/internal/article/adapter/mysql/counter_flush.go`）。

初版（2026-11-04）解决的是「并账是加减，不是重算」带来的永久漂移。落地之后暴露出两个结构性问题：

1. **并账周期是编译期常量**。`backend/app/cron_wiring.go` 里写着 `counterSyncInterval = 30 * time.Minute`：想调周期要改代码、重新发版；而「文章计数并账」与「私信未读重算」共用同一个 tick，周期却没有一个可配置的单一来源。运维看到的只是"数字不对就重启试试"。
2. **读路径有两套口径并存**。一套是「列 + 增量」：详情、列表、收藏内容、点赞写响应各自把 Redis 增量加到聚合上；另一套是「只读列」：首页「热门置顶」与作者置顶卡片直接读库里的列。结果同一篇文章在**同一时刻**可以有两个数（首页 5、点进去 7），而且差异恰好出现在"有增量还没并账"的那段时间——用户最容易看到的时间段。

## 决策

1. **并账周期收成运行期配置**：唯一来源是 `site.reconcile.interval_minutes`（单位分钟，默认 30，缺省 / 负数回落 30，上限一天），换算只有一处：`site.Reconcile.Interval()`。代码里不再有第二份默认周期常量。它是**调度注册参数**：与「日志清理每天几点」同一口径——改完要重启才生效（换周期必须重新注册任务），不假装热更新。
2. **仍只有一个并账调度任务**：文章计数刷盘（增量驱动，四个指标共享一个任务体与一把锁）与私信未读重算（全量重算，私信没有 Redis 增量源）**跑在同一个 tick 的同一个任务体里**；一个失败只记日志并继续另一个，最后把错误交给调度器记一条失败——失败彼此隔离，不新增调度器，也不拆成"两个 tick"。
3. **写库口径不变**：`digg` / `favor` 必须按关系行 `COUNT` 重算覆盖整列；`view` / `comment` 没有等价关系行，仍是 `列 += 增量`。并账瞬间同一次事件可能被「COUNT」与「增量」各算一次，**不累积**（下一轮覆盖即自愈）。
4. **读路径只有唯一权威入口 `app.CountLedger`**：权威值 = 持久列 + 待并账增量。增量只从 `CounterPort` 读（`CountLedger.Pending`），合成只发生在账本（`CountLedger.Authoritative`），所有对外可见的计数都经它写回：详情、列表、点赞 / 收藏写响应、收藏内容列表、首页热门置顶卡片、作者置顶卡片。
5. **被取代口径**：**「列 + 增量」作为读路径的写法**不再生效——即"每个读点各自把 Redis 增量加到聚合上"、以及"卡片只读列不叠增量"这两种做法一并退场。它们被决策 4 的单一入口取代；「列 + 增量」这个词今后只允许用来描述账本的**主值构成**，不许再当成一处读路径的实现口径。
6. **号段与派生主键是两件事，规则收进具名模块**：私信「幂等键 → 消息主键」的派生规则收进 `backend/internal/chat/domain/msgid`，号段与哈希算法都是该模块的**实现细节**（不再导出常量）。由此明确一条硬约束：**排序、水位、分页一律不读 ID 大小**——派生主键的大小与消息先后无关，ID 只作同一时刻的确定性并列裁决与扫描游标；消息的先后只由 `send_time` 决定（水位与并列裁决细则见 ADR-0027）。

## 理由与已否决方案

- **保持 ±1 并账、另加每日全量对账**：账本仍可能错 24 小时，且多一条任务 + 多一次全表 `COUNT`。**否决**。
- **只给 CLI / 后台按钮手动对账**：零自动成本，但没人会去点，等于把「永久错」变成「有人发现才修」。**否决**。
- **本批直接上 CDC 投影**：链路准，但票数与周期翻倍，与本批「先修可见缺陷」的目标不符。**本轮非目标**（见「不做什么」）。
- **读路径改成每次 `COUNT`**：列表页 20 篇文章就是 20 次 `COUNT`，且「按点赞数排序」失去廉价路径。**否决**。
- **把并账周期做成秒级 / 新增第二个调度器**：并账是扫表的运维动作，秒级周期只放大写放大；拆成两个调度器会让"文章计数失败挡住私信未读"重新变成可能，且多一份锁与日志。**否决**。
- **周期放进新配置段**：运行期配置的段清单是九段（见 [配置分层](../architecture/configuration.md)），零 DDL 也要求"加配置不改表"；`site` 段本来就是自由键值块（深度合并会把老库缺的键按默认值补齐），因此不新开段、不加表、不做数据迁移。**否决新开段**。
- **让每个读面继续各自合并增量，只靠 code review 对齐**：这正是本次要修的病；卡片那条路径甚至不在同一个合并函数里。**否决**。
- **改 30 分钟并账频率来"让数字更快准"**：计数本来就不等并账（见约束 7），调频率解决不了任何可见问题，只增写放大。**否决**（周期可配是为了运维按量调整，不是让数字变快）。

## 代价 / 不做什么

- **每次并账多做一次 `COUNT`**：只对「这一批有增量的文章 id」做，`article_id` 有索引，批内通常几十条，可忽略。
- **置顶卡片每页多一次 Redis 批量读**：换来的是"首页与详情同一时刻同一个数"。卡片数量是运营位数量级（个位数到十几条），一次 `HMGET` 管道即可。
- **改并账周期要重启**：它是调度注册参数，不做热重载（热重载需要"监听配置 + 重建任务"的第二套机制，收益不抵复杂度）。
- **计数不能再有"非关系行来源"**：一旦将来引入迁移来的历史点赞数（不落关系行），`COUNT` 覆盖就会把它们抹掉。要引入时必须先改本 ADR 决策 3。
- **不做**：不做实时推送计数、不做跨进程缓存失效、不改 ADR-0019 的两层 30s 结构、不改 `comment_count` 的既有语义、不把并账拆成两个调度器、不把周期做成秒级、不给任何既有表加列（零 DDL 不变，本批无新表、无 ALTER）。

## 当前生效约束

1. 并账周期**只能**来自运行期配置 `site.reconcile.interval_minutes`；代码里不得再出现第二份默认周期常量。
2. 文章计数刷盘与私信未读重算**必须**挂在同一个调度任务上；任一失败不得跳过另一个；不得新增调度器，也不得拆成两个 tick。
3. 并账写库：`digg` / `favor` **必须**按关系行重算覆盖，禁止 `列 += 增量`；`view` / `comment` 保持 `列 += 增量`。
4. 读路径**只认** `app.CountLedger`：任何新增读面都必须经它取权威计数，禁止自己拼「列 + 增量」。
5. 「列 + 增量」作为**读路径口径已被取代**；文档、注释与评审意见一律按「单一权威读入口」表述。
6. 私信「幂等键 → 主键」派生**只允许**在 `msgid` 模块里实现，号段是该模块的实现细节；**排序、水位、分页不读 ID 大小**，消息先后只由 `send_time` 决定。
7. 前端可见的计数**永不等并账**；刷新后的权威值来自写响应回填或私有态取数（ADR-0024）。
8. 私信未读的「真值口径」不在本 ADR 复述：可见集 / 未读数 / 清空界线由 [ADR-0027](./0027-message-center-ordering-pagination-and-read-receipt.md) 与 chat 模块的唯一实现回答，本 ADR 只规定"它必须与文章计数并账共用同一个 tick"。

## 相关代码与文档

- 配置：`backend/conf/site/enter.go`（`Reconcile.Interval()`，默认值与边界在此一处）· `backend/config/site_default_settings.yaml`（`site.reconcile.interval_minutes`）· `backend/internal/platform/config/runtime.go`（段读取 / 深度合并）
- 并账调度：`backend/app/cron_wiring.go`（`reconcileInterval` / `WireCron` 的单一任务体；调度器由 `taskScheduler` 窄接口注入，用例在注册接缝上观察行为）
- 并账写库：`backend/internal/article/adapter/mysql/counter_flush.go`（`ReconcileArticleCount`）· `backend/internal/chat/adapter/mysql/unread_reconcile.go`
- 读路径唯一入口：`backend/internal/article/app/ledger.go`（`CountLedger`）· 调用点 `query.go`（详情 / 列表）· `subdomain.go`（收藏内容）· `interaction.go`（写响应 / 作者置顶卡片）· `hottop.go`（热门置顶卡片）
- 派生主键：`backend/internal/chat/domain/msgid/msgid.go`
- 增量来源：`backend/internal/platform/redis/redis_article/enter.go` · `backend/internal/article/adapter/mysql/counters.go`
- 前端可见值：[ADR-0024](./0024-reader-user-state-and-count-truth.md) · `frontend/app/utils/readerPrivateFields.ts`
- 上游：[ADR-0019](./0019-reader-rendering-and-cache-strategy.md) · [ADR-0024](./0024-reader-user-state-and-count-truth.md) · [ADR-0027](./0027-message-center-ordering-pagination-and-read-receipt.md)

## 实施记录

- **2026-11-04（#252）**：落地决策 1–5，真库验证通过（`go test -tags integration ./internal/article/adapter/mysql/`）。首次真跑抓到夹具外键缺陷（`user_id=300` 未建用户），只做类型校验时看不见——SQLite 冒充 MySQL 的风险是真实的。2026-11-04 随 PR #259 上线，30 分钟 cron 做第一次覆盖式并账，把历史漂移一次抹平；无 DDL、无索引回填。
- **2026-11-06（#279）**：正文整体重写。并账周期从编译期常量改为运行期配置（`site.reconcile.interval_minutes`，默认仍是 30 分钟）；读路径收成 `CountLedger` 一个入口，热门置顶 / 作者置顶卡片不再只读列；幂等键 → 主键派生收进 `msgid` 具名模块。行为用例：`backend/app/cron_reconcile_tick_test.go`（周期来自配置 / 一次任务体同时做两个并账 / 失败彼此隔离）、`backend/internal/article/adapter/http/counter_authority_e2e_integration_test.go`（真 MySQL：同一条文章在详情与两处卡片上是同一个权威值）、`backend/internal/chat/adapter/mysql/chat_messages_cursor_test.go`（派生大 ID 的最早消息仍排在最后、分页按时间翻）。零 DDL、无新表、无 ALTER。

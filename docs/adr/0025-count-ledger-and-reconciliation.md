# ADR-0025：计数账本与并账（计数降为派生值）

- 状态：已交付
- 日期：2026-11-04
- 来源：读者反馈「点赞后刷新，按钮已赞但计数没变」· [决定清单](../notes/2026-11-04-reading-ui-and-admin-batch-decisions.md)（§1–2）· 收窄 [ADR-0024](./0024-reader-user-state-and-count-truth.md) 决策 5 的适用范围
- 关联：[ADR-0019](./0019-reader-rendering-and-cache-strategy.md)（两层缓存）· [GLOSSARY · 计数账本](../../GLOSSARY.md)

## 背景与问题

文章计数（`digg_count` / `favor_count` / `comment_count` / `view_count`）不是 `COUNT(关系行)` 算出来的，而是一本**独立维护的账**：

- **持久值**：`article_models` 的计数列；
- **实时差**：Redis 哈希 `article_digg` / `article_favorite` 的 `HINCRBY` 增量；
- **读**：`列 + 增量`（`internal/article/app/query.go` 的 `enrichCounters`）；
- **并账**：每 30 分钟一条 cron，把增量 **`列 += 增量`** 进库再清掉（`cron_wiring.go` / `counter_flush.go`）。

问题出在「并账是加减，不是重算」：只要某次增量没进库（Redis 丢、flush 的 `RENAME → :syncing` 窗口、事务回滚后 counter 已加），这本账就**永久偏**，而且**没有任何路径把它拉回来**。表现是 `is_digg = true` 但数字永远不加。

读者这次反馈的现象**主要是前端缓存**造成的（见 ADR-0024 的「后续修订（2026-11-04）」），但同一份读者反馈暴露出上面这个结构性缺口：两者都必须修，且**只有后者需要新决策**。

## 决策

1. **计数降为派生值**：并账时不再累加，而是按关系行重算覆盖 —— `digg_count = COUNT(article_digg_models WHERE article_id = ?)`、`favor_count = COUNT(收藏关系行)`。任何历史漂移在**下一个并账周期（≤30 分钟）自愈**，不新增定时任务。
2. **读路径不动**：仍然是 `列 + 增量`。30 分钟是**并账周期**，不是「计数更新周期」——用户看到的数字在点赞写响应里立即回填，任何一次读都是当时最新的 `列 + 增量`。
3. **`GET /articles/favorite/contents` 补上增量合并**：该列表过去直接 join `article_models` 读列、不并 Redis 增量，于是同一篇文章会出现「详情页 6、收藏列表 5」。现在与详情 / 列表同一口径。
4. **理想形态（CDC 投影）列为显式非目标**：把「关系行 → 计数」做成 binlog 驱动的第三条 CDC 链路（与 `river` 的 ES 投影同构）是更干净的终局，但属于新建一条消费链路 + 位点 + 死信 + 回补的 feature 量级，本批不做、也不开票。
5. **并账瞬间的临时 ±1 记为已知瑕疵**：同一次事件可能被「COUNT」与「增量」各算一次，**不累积**（下一轮覆盖即自愈）。

## 理由与已否决方案

- **保持 ±1 并账，另加每日全量对账**：账本仍可能错 24 小时，且多一条任务 + 多一次全表 `COUNT`。**否决**。
- **只给 CLI / 后台按钮手动对账**：零自动成本，但没人会去点，等于把「永久错」变成「有人发现才修」。**否决**。
- **本批直接上 CDC 投影**：链路准，但票数与周期翻倍，与本批「先修可见缺陷」的目标不符。**本轮非目标**（见决策 4）。
- **读路径改成每次 `COUNT`**：列表页 20 篇文章就是 20 次 `COUNT`，且「按点赞数排序」失去廉价路径。**否决**。
- **改 30 分钟并账频率来"让数字更快准"**：计数本来就不等并账（决策 2），调频率解决不了任何可见问题，只增写放大。**否决**。

## 代价 / 不做什么

- **每次并账多做一次 `COUNT`**：只对「这一批有增量的文章 id」做，`article_id` 有索引，批内通常几十条，可忽略。
- **计数不能再有"非关系行来源"**：一旦将来引入迁移来的历史点赞数（不落关系行），`COUNT` 覆盖就会把它们抹掉。要引入时必须先改本 ADR 的决策 1。
- **不做**：不做实时推送计数、不做跨进程缓存失效、不改 ADR-0019 的两层 30s 结构、不改 `comment_count` 的既有语义。

## 当前生效约束

1. 并账写库**必须**按关系行重算覆盖，禁止再写 `列 += 增量`。
2. 读路径**只能**是「列 + 增量」；不得在列表/详情之外再造第三种算法。
3. `GET /articles/favorite/contents` 与详情 / 列表共用同一套增量合并（`enrichCounters`）。
4. 「计数由关系行派生」是本账本的前提：任何非关系行来源的计数都必须先改本 ADR。
5. 前端可见的计数**永不等并账**；刷新后的权威值来自写响应回填或私有态取数（ADR-0024 后续修订）。

## 相关代码与文档

- 并账：`backend/internal/article/adapter/mysql/counter_flush.go` · `backend/app/cron_wiring.go`
- 增量：`backend/internal/platform/redis/redis_article/enter.go` · `backend/internal/article/adapter/mysql/counters.go`
- 读路径合并：`backend/internal/article/app/query.go`（`enrichCounters`）· `backend/internal/article/adapter/mysql/social_repo.go`（收藏内容列表补齐点）
- 前端可见值：[ADR-0024](./0024-reader-user-state-and-count-truth.md) 的「后续修订（2026-11-04）」· `frontend/app/utils/readerPrivateFields.ts`
- 上游：[ADR-0019](./0019-reader-rendering-and-cache-strategy.md) · [ADR-0024](./0024-reader-user-state-and-count-truth.md)

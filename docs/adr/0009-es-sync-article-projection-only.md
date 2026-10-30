# ADR-0009：删除"通用规则"同步路径，ES 同步只留文章搜索投影

- 状态：已交付
- 日期：2026-10（v1 归档期）
- 来源：从 [04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md) 拆分（原文完整保留，v1 未在其他文档展开）

## 背景与问题

`internal/platform/cdc/river` 里长期并存两条 "MySQL → ES" 路径：

| | 通用规则路径 | 文章搜索投影 |
| --- | --- | --- |
| 配置 | `river.rule`：表 → 索引 + 字段白名单 + 类型转换 | 代码里固定 6 张表 |
| 数据来源 | 只用 binlog 那一行 | 回库按主键重读并 join 用户/分类/标签/置顶 |
| 文档形状 | 这张表自己的列 | 搜索卡片需要的一整篇 |
| 失败证据 | 完整 bulk body（可原样重发） | 曾是"表名+动作+行数"（不可重发） |
| 重试 | `doBulkWithRetry`（2 次 + 200ms） | 无 |

## 决策

删除整条通用规则路径。核查确认了三件事，是本次删除的依据：

1. **通用路径对本项目的每一张表都不可达**——`OnRow` 先把 outbox、再把 6 张源表交给 `handleArticleSearchProjectionEvent` 并提前 `return`；配置里 `source` 的 6 张表全在投影清单里，唯一的 `rule` 指的也是 `article_models`；
2. **它拼不出搜索文档**——白名单只有 8 个列，缺作者名、分类名、标签、各项计数，即使可达，产出的文档也不是搜索结果要的那份；
3. **它独有的两项能力都不再被需要**——"删除类死信重投"已由投影侧的 `ProjectionReplayer`（带行主键、按主键重算）覆盖；`doBulkWithRetry` 从未作用于投影路径。

删除清单：

- **代码**：`river/rule/`（整包）、`sync_request.go`、`sync_bulk.go`（只留 `saveProjectionErrorToDLQ` 与 `buildCDCJobID`，已并入 `sync.go`）、`replay.go`（`ESReplayer`），以及 `river.go` 里的 `rules` / `prepareRule` / `parseSource` / `updateRule` / `ruleKey`；
- **同步循环**：`sync.go` 的批量合并与重试循环一并删除，`syncCh` 从 `chan interface{}` 收窄成 `chan posSaver`，只剩"按 3 秒节流落位点"；
- **配置**：`conf.River.Rules`、`flush_bulk_time`、`river.retry`、两份 `settings.yaml` 的 `rule:` 段，以及只为它存在的 `BLOGX_RIVER_INDEX`（搜索索引一直取 `es.index`）；
- **装配**：`analytics_wiring` 不再注册 `es_river` 执行器；顺带删除因此变成死代码的 `es_service.Bulk`。

**保留**：`master`（位点）与 canal 的 dump 配置——全量转储会把行按 INSERT 重放一遍，投影分支照常把它们变成文档，**首次部署的数据初始化仍然可用**。

## 理由与已否决方案

死信侧语义已随 [ADR-0008](./0008-dead-letter-replay.md) 收敛（投影失败不再停掉整个 canal，而是"记死信 + 继续跑"，且死信带 `ids` 可重投），**旧路径存在的最后一个理由（"只有它能把 bulk body 原样重发"）随之消失**。

已否决的方案：

- **保留但标注"当前不产生数据"**：成本是一个永不出数据的执行器 + 一份会误导人的 `rule` 配置；上一轮就是这么选的，结论是"标注"挡不住下一个人照着它改配置；
- **把通用路径改成"配置 + 投影函数"**：本质是给一个只有一个实现的场景加一层间接，且仍要为每张表写代码。

## 代价 / 不做什么（必须接受的）

- **加一张"不需要 join 的表"进 ES，现在要写代码，不能只改配置。** 步骤固定三步：① 在 `article_search_projection.go` 的分派里加一个 case；② 在 `projection_replay.go` 的 `projectionReplayEvents` 里登记 `(表, 动作) → 事件类型`；③ 补一条映射用例。约半小时，比"配置 + 猜字段"更可控；
- **ES 写入没有重试了。** 投影路径本来就是一次 `IndexBulk` 失败即落死信（以前也一样），所以这不是本次回归；但"想加重试"的人应该加在 `articlesync` 的批量写里，**而不是恢复一条独立的同步路径**；
- **历史 `es_river` 死信不再可重投。** 页面上仍显示流名（保留词表是为了历史记录可读），但点重投会得到 `unsupported`；需要补历史数据时用全量重建；
- 删掉的那批 `es_service` 单文档 API（`CreateDocument` / `Search` / `GetMapping` 等）在本次之前就已无生产调用方，属既有历史包袱。

**变好的**：少约 1,000 行、10 个文件；表清单从两份（`source` + `rule`）收敛成一份；消灭一类"配置看起来在生效"的陷阱——`rule` 段解析时是**非严格**的，删了字段却留着 yaml 只会静默失效；死信只剩 3 条流，死信页上不会再出现一条永远不会有数据的 `es_river`。

## 当前生效约束

1. ES 同步**只有文章搜索投影一条路径**，禁止再引入配置驱动的通用同步（新增表走三步代码登记）；
2. 表清单单一来源：只认 `article_search_projection.go` 里的分派，不认 `settings.yaml` 的 `rule:` 段；
3. 重试若需加强，加在 `articlesync` 的批量写里，不得恢复独立同步路径；
4. 补历史 `es_river` 数据用全量重建命令 `./server -es -s article-sync`，不要用页面重投（必然返回 `unsupported`）；
5. `river` 保留 `master` 位点与 canal dump 配置，用于首次部署的全量初始化。

## 相关代码与文档

- 代码：[cdc/river/article_search_projection.go](../../backend/internal/platform/cdc/river/article_search_projection.go)（表清单的唯一来源）
- 代码：[cdc/river/projection_replay.go](../../backend/internal/platform/cdc/river/projection_replay.go)（`(表, 动作) → 事件类型` 登记）
- 代码：[cdc/river/river.go](../../backend/internal/platform/cdc/river/river.go)、[cdc/river/sync.go](../../backend/internal/platform/cdc/river/sync.go)（位点与 3 秒节流落位点）
- 文档：[ADR-0008：死信重投](./0008-dead-letter-replay.md)（投影失败改为"记死信 + 继续跑"）
- 原始记录：[04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md)

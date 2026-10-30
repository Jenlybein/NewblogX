# ADR-0008：死信重投（DLQ Replay）——让"同步失败"有闭环

- 状态：已交付
- 日期：2026-10（v1 归档期）
- 来源：从 [04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md) 拆分（原文完整保留，v1 未在其他文档展开）

## 背景与问题

三个 CDC 子链路（`es_projection` / `outbox_relay` / `image_ref_river`）在重试耗尽后都会调 `deadletter.SaveBatch` 写 `cdc_dead_letter`，状态一律 `pending`；但**没有任何代码把状态改成别的值、也没有任何代码去读它重做一次**——死信表是只进不出的黑匣子，出问题只能人工进 MySQL 翻、手动补数据。

展示端更误导：`replay_event_logs` 的读端（analytics `/logs/replay`）与写端 `writer.EmitReplayEvent` 都写好了，但**没有任何地方调用它**，表永远空着。

一句话：**"失败留证据"做了，"照证据补救"没做。**

## 决策

建一套最小但完整的死信重投闭环，分四层：

```
管理端页面（列表 + 勾选重投 + 回放日志）
      ↓
POST /logs/dead-letter/replay        GET /logs/dead-letter
      ↓ 端口 domain.DeadLetterReplayer
platform/cdc/dlq.Service  ── 编排：取记录 → 分派执行器 → 更新状态 → 写回放日志
      ├── es_projection   → 按死信里的行主键重算投影文档（幂等）
      ├── image_ref_river → 删除类：重新清理该对象的图片引用
      └── outbox_relay    → 把 outbox 行从 dead 重置为 pending（交给中继自己重投）
```

### 1. 状态机只用已有列，零 DDL

```
pending ──重投成功──> resolved（终态，不再参与重投）
pending ──重投失败──> pending（retry_count+1、error_code/error_msg 覆盖为本次失败）
```

**不引入 `replaying` 中间态**：重投是人工、低频、单实例的管理操作，中间态会带来"进程崩溃留下僵尸 replaying"的恢复逻辑，而它防的问题（两人同时点同一个按钮）最坏结果是重投两次——两个执行器都是幂等的（ES 按文档 id index/delete；outbox 重置是纯状态写），重复执行无害。**用"幂等执行器"换掉"分布式互斥"，代价低得多。**

### 2. 支持范围按 payload 自洽性划定

| stream | 死信 payload 里有什么 | 重投怎么做 | 支持 |
| --- | --- | --- | --- |
| `outbox_relay` | `{payload}` + `CdcJobID=outbox:<id>` | 把该 outbox 行 `dead → pending`、`attempts=0`，由中继按自己的退避重投 | ✅ |
| `es_projection` | `{schema,table,action,rowCount,ids}` | 按 `ids` 重算投影（`SyncArticleSearchProjection`），只动受影响的文档 | ✅ |
| `image_ref_river` · 删除 | `{owner_id, ref_type}` | 直接重新清理该对象的图片引用（payload 自洽） | ✅ |
| `image_ref_river` · 新增/更新 | `{table,row}` / `{table,before,after}` | `row` 是 `[]any`，JSON 往返后数字变 float64、时间变字符串，重建行快照会**静默算错**候选图 | ❌ 返回 `unsupported` + "重新保存该业务对象" |

**`es_projection` 的 payload 必须带 `ids`。** 没有主键就只能全量重建索引（几分钟、期间新旧文档混杂）；带上主键后，重投是"重算这几篇"而不是"重建全库"。（历史 stream `es_river` 的 payload 是完整 ES bulk body，可原样重发，但该路径已随 [ADR-0009](./0009-es-sync-article-projection-only.md) 删除；死信页保留该 stream 名仅为历史记录可读。）

### 3. "不支持"与"失败"必须是两种结果

`deadletter.ErrUnsupported` 这个哨兵就是为这条边界存在的：

| 执行器返回 | 结果 | 状态 / retry_count | 界面 |
| --- | --- | --- | --- |
| `nil` | `success` | `resolved` | 绿色"成功" |
| `ErrSkipped` | `skipped` | 不动 | 灰色"跳过" |
| `ErrUnsupported` | `unsupported` | **不动** | 橙色"不支持重投" |
| 其它 error | `failed` | 仍 `pending`，`retry_count+1` | 红色"失败" |

不支持的流不是"以后再说"，而是**当场告诉操作员该改用什么办法**：`POST /replay` 返回 `unsupported` + 一句人话，**不改数据库、不涨 `retry_count`**——"不支持"和"这次失败"是两件事，前者再点一百次也是同一个结果，记成失败只会诱导人反复点。

### 4. 回放日志：把闲置的写端接上

每次重投尝试（成功/失败/跳过/不支持）都 `writer.EmitReplayEvent` 写一条到 ClickHouse `replay_event_logs`，字段取自死信记录：`cdc_job_id` / `stream` / `source_table` / `action` / `target_key` / `retry_count` / `result`。

收益：管理端「回放日志」页第一次真正有数据；同一条死信的"失败→重投→再失败→再重投→成功"有完整时间线（`cdc_job_id` 可串）；"点了没反应"也能查（跳过与不支持同样留痕）。

### 5. 幂等与安全边界

- 已 `resolved` 的记录不再重投，返回 `skipped`（避免重复打 ES）；
- 更新状态用条件更新（`WHERE id=? AND status<>'resolved'`），不引入锁；
- **不自动重投**——死信可能是"代码 bug 导致必然失败"，自动重投只会把 ES 打热；
- 重投不修改死信里的 payload，失败证据保持原样便于事后比对；
- 删除类（`image_ref_river`）重投是"清掉该对象**当前**的所有引用"，不校验对象是否还在。**先重新保存过对象、再点旧死信，会把刚重建的引用一起删掉**——幂等 ≠ 无害，这条只通过对操作口径的约束（重建过就别点旧死信）来规避。

## 理由与已否决方案

**变好的**：同步失败处置从"进 MySQL 手改"变成"页面上勾选重投，结果立刻可见"；死信有了终态，运维能区分"还没处理"与"已补上"；`replay_event_logs` 从死代码变成有数据的时间线；**投影失败不再拖垮整个同步**——因为死信带主键、补得回来，`OnRow` 从"失败即 `cancel()`"改成"记死信 + 继续跑"，此前一篇文章投影失败会让 MySQL→ES 同步永久停摆，而 API 照常服务、没人会知道。

已否决的方案：

- **只给页面、重投仍靠人工 SQL**：看得见补不上，等于把问题挪个地方；
- **自动重投（定时扫描 pending）**：死信多半是"改之前必然失败"的（字段不兼容、索引不存在），自动重投会在故障期间把 ES 打出一片 400 并掩盖真实原因，而人工触发天然带"我先看看为什么失败"这一步；
- **重投走消息队列**：引入新基础设施，当前人工处理的死信量级完全用不上。

## 代价 / 不做什么

- 多 2 个契约接口与 2 个管理端页面需维护；
- `outbox_relay` 的重投是"重置状态"而非"立即执行"，最长要等一个中继轮询周期（默认 2s 退避起步）——这是有意的，复用中继自己的重试与背压；
- **重投的力度是"重算"不是"重放一行"**（改 1 篇文章只影响 1 篇，改一个用户昵称会影响他名下几十到上百篇），这是"展示字段冗余进 ES 文档"换来的代价；
- **`es_projection` 可重投依赖"投影与业务表在同一个库"这一前提**（文档组装要跨 `article / user / category / tag / article_tag / user_top_article` 六张表做应用层批量查询，不是 SQL JOIN），**一旦拆库或拆服务，这条路径必须改成"订阅各服务事件 + 本地读模型"，不能靠改连接串对付过去**；
- 旧版本无 `ids` 的 `es_projection` 死信仍只能全量重建，接口会明确告知。

## 当前生效约束

1. 零 DDL：只用 `cdc_dead_letter` 已有列（`status` / `retry_count` / `error_*` / `updated_at`）；
2. 新增契约接口的响应必须用 `kernel.Envelope[ListData[T]]`（后端注解引用真实 DTO）；
3. `TestContractMatchesRoutes` 会自动要求这 3 个新接口在路由表里存在；
4. 新增 stream 时，执行器必须显式返回 `ErrUnsupported` 而不是默认"支持"；
5. 重投执行器必须幂等（这是不引入 `replaying` 中间态的前提，破坏幂等必须先补中间态）。

## 相关代码与文档

- 代码：[cdc/dlq/service.go](../../backend/internal/platform/cdc/dlq/service.go)（编排：取记录 → 分派执行器 → 更新状态 → 写回放日志）
- 代码：[cdc/deadletter](../../backend/internal/platform/cdc/deadletter)（`cdc_dead_letter` 读写与 `ErrUnsupported` 哨兵）
- 代码：[cdc/river/projection_replay.go](../../backend/internal/platform/cdc/river/projection_replay.go)（`es_projection` 按 `ids` 重算）
- 代码：[cdc/outboxrelay/replay.go](../../backend/internal/platform/cdc/outboxrelay/replay.go)、[cdc/imageref/replay.go](../../backend/internal/platform/cdc/imageref/replay.go)（另两条流的执行器）
- 代码：[kernel/envelope.go](../../backend/internal/kernel/envelope.go)、[platform/http/router/contract_routes_test.go](../../backend/internal/platform/http/router/contract_routes_test.go)（`Envelope` 与路由契约校验）
- 文档：[契约：单一来源、生成流水线与类型真相](../architecture/contracts.md)
- 原始记录：[04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md)

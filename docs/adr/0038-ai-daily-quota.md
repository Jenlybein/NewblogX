# ADR-0038：AI 每日额度按场景、按人、按自然日计数；额度耗尽不阻塞发布而是次日自动重审

- 状态：已交付
- 日期：2026-11-10
- 来源：[#336](https://github.com/Jenlybein/NewblogX/issues/336)（AI 每日额度 + 后台菜单拆分）· 决定过程见上一轮 grill 记录（未单独归档）

## 背景与问题

后台「AI 助手」页从上线起就摆着「每日配额（0 不限）」输入框，而**后端没有任何一处读取它**——这个开关从来没有生效过。与此同时全站有 9 个功能级模型调用点、7 个场景，每一个都能被无限次触发：一个账号可以反复调改写、反复重评、反复让合规判定跑模型；站长既看不到「今天谁用掉了多少」，也没有任何手段把成本关在门外。

三件事让「加一个总次数」不够用：

1. **一次动作会吃掉多次调用**。投稿评分对长文按段落分块，一次投稿内部顺序调 2..N 次模型；AI 搜索一次请求先改写再作答。若只按「每次底层调用」计数，长文作者会在远早于投稿配额的地方撞墙，而且撞到的是另一个错误码。
2. **异步链路没有归属**。合规判定与补评分都由发件箱异步触发，调用元数据里的用户是空的：日志看不出是谁消耗的，也无法按作者计量。
3. **额度耗尽没有落点**。合规判定拿不到结论时，现有实现会把文章改回审核中并给管理员建一条待处理任务；如果原因只是「作者今天额度用完了」，这条任务纯属噪音，作者则完全不知道自己被挡了。

## 决策

1. **七个场景各一个「每人每自然日」上限**，具名结构 `ai.daily_limits`（`scoring` / `metainfo` / `overwrite` / `diagnose` / `search_rewrite` / `search_answer` / `moderation`）。`0` 合法，表示不限。默认：评分 20、合规判定 20、诊断 20，其余四个 50。
2. **收费点是唯一出口**：`ai.Chat` / `ai.ChatStream` 两个函数。业务侧看不到计数，新增调用点自动被覆盖；归属信息来自 ctx 上的 `CallMeta`（场景 + 用户）。
3. **计量单位默认是「一次出站调用」**，唯一例外是评分：**一次评分动作只算一次**（闸门与作者主动重评的入口先预占，并给 ctx 打「已预扣」标记，内部分块子调用不再重复扣）。
4. **三条跳过规则**：管理员（`CallMeta.IsAdmin`）、无用户归属（`UserID` 为零）、已预扣 / 只记日志（`Billing`）。管理员豁免是固定规则，不随站点开关变化。
5. **归属补齐**：发布闸门的同步评分写入作者并带上 `AuthorIsAdmin`；异步合规判定回查文章作者与角色；异步补评分只补日志归属、**不计费**（它与闸门评分存在竞态，重复计量没有意义）。
6. **计数落在 Redis 日桶**：键 `ai:daily:<scene>:<yyyymmdd>:<userID>`，26 小时 TTL，一个 Lua 内完成「读—判—自增 + 设 TTL」；释放是独立幂等脚本。跨零点靠键自然过期，不依赖定时任务。`limit <= 0` 直接放行且不写键。
7. **失败语义**：预占之后因系统性失败（模型失败 / 断流）归还，**失败不占额度**；Redis 不可用或脚本报错时**失败关闭**（与投稿配额、私信限流同一条口径）。
8. **错误码分开**：额度用尽是 `ai.daily_limit_exceeded`（明天再来），额度服务故障是 `ai.quota_unavailable`（稍后再试）；与「整站 AI 不可用」「没有启用的模型配置」「评分不可用」各自独立。
9. **被额度拒绝的调用也写一条 `ai_call_logs`**（失败原因即错误码）：否则后台只能听用户说"我用不了"，看不出今天有多少人被挡、挡在哪个场景。
10. **额度耗尽的合规判定**：不内联重试；文章从「已发布 + 待可见」改回 **「审核中」且不建人工任务**；同一条事件按发件箱的 `available_at` 改到次日零点后（带 0–10 分钟抖动）重投；给作者发一条系统通知说明"明日自动重审、期间读者看不到"。
11. **兜底扫描**：每日扫「审核中 + 无待处理任务 + 超过 24 小时」的文章转人工，防止延迟事件丢失导致永久悬挂。
12. **保存期校验**：负数与 `> 9999` 一律 400 拒绝（`site.ai_daily_limit_invalid`）；`0` 不拒绝。读取侧另有一道归一化防线，把库里被手改坏的旧值回落到默认值。
13. **零 DDL**：不加表、不加列。延迟重投复用发件箱已有的 `available_at` 列，通过新增的 `RescheduleError` 在中继记账阶段完成（订阅者直接改行会被 `markDone` 覆盖）。

## 理由与已否决方案

- **否决「只留一个站点级总次数」**：一次 30 秒的评分与一次两行的改写成本差两个数量级，共用一个预算只会让运营在对成本无感的地方先撞墙；而且"所有调用点都要有限制"在单一数字下只能靠"总数用完全停"间接满足。
- **否决「按用户动作计量（全部场景）」**：收费点若落在 7 个用例入口，新增场景忘了打点就静默漏掉，也没有门禁能拦；出口收费才是"漏不了"的那个位置。评分是唯一有"一次动作多次调用"事实的场景，因此只对它做动作级去重。
- **否决「新建账本表」**：语义最干净，但零 DDL 下要多一条 ADR 指名与 `schema.sql` / testkit 两处登记，而它解决的问题和 Redis 日桶完全重合（ADR-0031 已经为投稿配额定过这个调子）。
- **否决「从 ClickHouse `ai_call_logs` 事后统计」**：日志可丢、异步落地，不能当准入闸门（它只适合回答"昨天花了多少"）。
- **否决「额度耗尽就跳过机审直接 visible」**：等于"把额度用完即可跳过审核"，当场把限制变成绕过口子。
- **否决「额度耗尽落死信、次日重投」**：死信语义是"改之前必然失败"，而这里只是"今天没额度"，重投是必然成功的；复用死信会污染它的判据。
- **否决「额度耗尽仍建人工任务」**：管理员队列会多出一批"其实等到明天就自动过"的行，把真正需要人判的文章淹掉。
- **否决「匿名调用按 IP 计数」**：匿名 LLM 调用实际上不存在（`scoring/article` 的 type=3 在 service 层要求登录），为一条不存在的路径新增 IP 桶不划算。

## 代价 / 不做什么

- **Redis 计数不是绝对 durable**：`flushdb`、换实例、误删目录会清空当日计数；与投稿配额同一个前提，不额外引入分布式协调。
- **`scoring` 的口径与全局不同**：它的"一次"是评分动作而不是模型调用。这是一处刻意的不一致，读代码时必须知道（本 ADR 与词条都写了）。
- **一次 AI 搜索消耗两个场景各一次**：改写与作答各扣一次，"50 次问答"的前提是两个数字相同。
- **不做**：按 token / 费用计预算、分钟级窗口、按信任等级分档、剩余次数暴露给前端、额度告警。
- **延迟重投的抖动是 0–10 分钟**，不做精确排期；同一批次涌入由抖动摊开。
- **兜底扫描只在装配了 AI 的进程里生效**（`api` / `all`）；`river` / `worker` / `cron` 角色不跑。

## 当前生效约束

1. `ai.daily_limits` 是唯一定义处：`site_default_settings.yaml` 写字面量，`conf.AI` 承载类型，`platform/config` 的生效值模块负责归一化；默认值只在 `effective.go` 声明一次。
2. 任何新增会真的调用模型的路径，必须经过 `ai.Chat` / `ai.ChatStream`（否则它不受额度约束）；为此新增场景要同步在 `CallMeta` 的 `Scene*` 常量与 `SceneDailyLimit` 映射里登记。
3. 计费档位只在动作入口设置（`BillingPrecharged` / `BillingUnmetered`）；出站口不得改写档位。
4. 额度拒绝必须是 `ai.daily_limit_exceeded`，不得改写成通用 `ai.request_failed`。
5. 预占后的系统性失败必须释放；释放必须幂等。
6. 键格式 `ai:daily:<scene>:<yyyymmdd>:<userID>` + 26 小时 TTL 是口径的一部分：改键名等于让当日计数清零，改 TTL 必须仍然跨过零点。
7. 合规判定因额度被拒时，**不建人工任务**；兜底扫描是唯一的人工兜底入口。
8. 零 DDL 口径不变：不加表、不加列；延迟重投只能借 `available_at`。

## 相关代码与文档

- 概念：[GLOSSARY · AI 每日额度](../../GLOSSARY.md) · [GLOSSARY · 额度延迟重审](../../GLOSSARY.md)
- 收费点：`backend/internal/ai/{quota.go,llm.go,quota_test.go}`
- 调用元数据：`backend/internal/ai/callmeta.go`（`IsAdmin` / `Billing`）
- 计数：`backend/internal/platform/redis/redis_ai_quota/` · 适配器 `backend/internal/ai/adapter/redis/`
- 用例层预扣：`backend/internal/ai/app/{service.go,quota.go}`
- 归属与延迟重投：`backend/app/{outbox_wiring.go,cron_wiring.go,rows.go}`
- 转态：`backend/internal/article/app/{compliance_transition.go,compliance_sweep.go}`
- 中继：`backend/internal/platform/cdc/outboxrelay/reschedule.go`
- 配置：`backend/conf/conf_ai.go` · `backend/config/site_default_settings.yaml` · `backend/internal/platform/config/effective.go` · `backend/internal/site/app/service.go`
- 错误码：`backend/internal/apperr/apperr.go`
- 文档：[AI 接入](../architecture/ai-integration.md) §五

# ADR-0025：按风险分流的发布口径（标红直接拒、干净文章自动发）

- 状态：已交付
- 日期：2026-11-09
- 来源：2026-11-09 的发布治理设计评审 · 关联 [ADR-0022](./0022-submit-quota.md)（配额）· [ADR-0023](./0023-quality-gate.md)（同步端口）· [ADR-0024](./0024-risk-models.md)（第③层）

## 背景与问题

提交那一刻只能拿到前两层机审（词表 + 评分派生标签）的结论，第③层模型判定是异步的，因此"干净文章自动发"与"模型审核通过之后才可见"之间存在时间差：文章必须先在作者侧显示为已发布，又不能立刻对读者公开。

由此要一次定清楚五件事：

1. **同步扫描的规则从哪来？** 提交时的拦截与审核列表的标红必须是同一份规则，否则两处口径漂移。
2. **等待模型结论期间文章落在哪？** 需要一种"已发布但暂不可见"的表达，且不能动表结构。
3. **异步结论如何分流？** `clean` / `suspect` / `violation` / 判定失败各要有一个终态出口，不允许文章永远停在"作者以为发了、读者看不到"。
4. **系统拒与人拒怎么区分？** 自动拒的误判不能被算成审核员判断失误，也不能把判定依据泄露给作者。
5. **处置动作的边界在哪？** 哪些状态可被自动拒、配额会不会被扣两次、作者响应里能看到什么。

## 决策

1. **同步风险扫描**：提交时（事务外）把第①层词表与第②层评分派生标签合并跑一次 `moderation.Evaluate`——与审核列表标红用**同一份**规则。新建文章通常还没有评分记录，此时只跑词表层、不报错。
2. **命中 `medium` 及以上（含 `high`）→ 直接 `REJECTED`**，不给改；留一条 `source=risk_check` 待处理任务供管理员复盘。作者只拿到固定文案（`article.risk_rejected` / 枚举 `risk_check`），命中的词、维度、等级一律不外泄。词表命中即拒。
3. **`none` / `low` → 立刻以 `StatusPublished + VisibilityPendingReview` 落库**。作者立刻拿到"已发布"回执，但匿名访客读不到：`IsPubliclyVisible()` 与公开读面 SQL 都只认 `published + visible`。
4. **新可见性取值 `pending_review`**：是 `visibility_status`（varchar 32）的新取值，**不是第五个状态、不是新列**，满足零 DDL 约束。`pending_review` 只能由系统写入，且必须由异步判定或审核通过转成终态；作者与管理员都不能用"改可见性"接口提前放行（`CanSetVisibility` 双向拦截）。
5. **模型结论分流**（异步，仍走 outbox）：
   - `clean` → `PublishForCompliance`：转 `visible`，真正对读者可见；
   - `suspect` / `violation` 但 `risk_level < 2` → `BackToReviewForCompliance`：改回 `EXAMINING` 并建 `model_suspect` 任务，一律交人工；
   - `violation` 且 `risk_level ≥ 2` → `AutoRejectForCompliance`：`REJECTED` + `source=compliance` 复盘任务；
   - 判定失败 → 用 outbox 的 `Event.Attempts` 重试**一次**，第二次仍失败则同样 `BackToReviewForCompliance`（`model_failure`），绝不留在 `pending_review` 变成隐形拒稿。
6. **模型自动拒的高置信门槛固定为 `violation` + `risk_level ≥ 2`**；`suspect` 一律交人，误判会直接落到作者身上而作者没有申诉渠道。**已拒绝不重复处理**（同一篇文章创建与发布各判一次，第二次是空操作）；**草稿不被自动拒**（还没出去过，作者自己改就行，拒了反而更难恢复）。只有「审核中」与「已发布」会被自动拒。
7. **审核任务来源 `source=compliance` 必须保留**，以区分「人拒的」与「系统拒的」；不得为了省一个枚举值复用到 `create`。同步 `risk_check` 与异步 `compliance` 是两条独立的复盘通道。
8. **面向作者的响应不得包含模型类别、依据、命中词、风险等级与维度**：同步风险命中时唯一可见的是固定文案 `article.risk_rejected`，模型自动拒时唯一可见的是枚举文案「内容不合规」。
9. **通知缺少驳回原因时兜底到合规文案**：自动拒的任务是待处理、没有驳回原因可读，通知不能出现"理由："后面空白。兜底读的是风险记录里的档位，只用来选固定文案，不携带细节。
10. **自动拒不二次预占配额**：提交时预占过一次，自动拒是异步副作用；风险命中与模型自动拒都不再预占。一次提交只预占一次。
11. **同步路径不再建人工任务**：人工队列只由异步副作用产生（存疑 / 失败 / 自动拒复盘）。
12. **`skip_examining` 开启时全部直接真发布**（`visible`），同步扫描也不拦；第③层只作事后兜底。这是运营的紧急放行手段，优先级最高。
13. **质量闸门不变**：不达标仍落草稿（可改可重投），且排在风险扫描之后判定——风险命中的落点是 `REJECTED` 不给改，不能被闸门的草稿分支盖掉。
14. **三种落点**：质量不达标 → 草稿；同步风险命中 → 已拒绝 + `risk_check` 任务；模型自动拒 → 已拒绝 + `compliance` 任务。
15. **不做申诉入口、不做批量撤销/恢复**：没有"把被自动拒的文章批量改回草稿"的工具，作者只能改稿重投。
16. **零 DDL**：新可见性取值复用既有 varchar 列，不加列、不加表。

## 理由与已否决方案

- **否决"提交时同步等模型判定"**：会让每次提交多等 5–30 秒，且评分/判定都得挤在事务外，也违背合规判定必须异步的约定。改为「先发后判、判完才可见」，用可见性维度承接这个时间差。
- **否决"干净文章直接真发布、模型只事后自动拒"**：模型审核通过之后才改成可见状态，直接可见会留下违规内容的公开窗口。
- **否决"判定失败就自动放行"**：AI 挂了把未知当作干净，等于风控形同虚设。
- **否决"判定失败无限重试到死信"**：会让文章永远停在"作者以为发了、读者看不到"。重试一次后转人工，是失败关闭与不卡死之间的折中。
- **否决"把 `pending_review` 做成第五个文章状态"**：状态枚举要动库里 tinyint 的语义、涉及所有读面；复用可见性取值只需扩展一个 varchar 枚举，零 DDL。
- **否决"放宽自动拒门槛到 suspect / 只按 violation 不看级别"**：档位已经三档，不再区分级别会让一条措辞可疑的稿子直接进"已拒绝"，恢复成本远高于先看看到底怎么回事。
- **否决"合规命中也落草稿"**：草稿列表是作者的工作台，一篇明确违规的稿子混在里面既不会有人看，也不该被作者继续"小修小补"再投一次。
- **否决"自动拒时再扣一次配额"**：一次提交扣两次额度，作者会莫名其妙地少掉一半机会。
- **否决"给作者看模型依据"**：等于公开判定规则，等于教人绕过。
- **否决"已发布文章不撤"**：那自动拒在最常见的场景（站点默认跳过审核）里永远不生效——投稿即发布，判定到达时文章已经在公网上了。

## 代价 / 不做什么

- **词表误报现在直接压到作者身上**：字符串匹配没有语境（"六四"命中"六十四位"），且 `REJECTED` 不给改、不可申诉。因此固定文案与 `risk_check` 复盘任务必须保留，管理员要能看见误判。
- **存在"已发布但暂不可见"窗口**：期间作者本人与管理员可见，其他读者不可见。任何把 `pending_review` 转成终态的路径都必须存在（clean → visible、suspect/失败 → EXAMINING、violation → REJECTED），不允许新增一条"留在 pending_review 不动"的成功出口。
- **人工队列性质**：里面不再是全部投稿，而是模型存疑、判定失败与系统直接拒的复盘任务；批量审核 UI 仍可用，但队列会小很多。自动拒的复盘任务是待处理状态、对应文章状态是已拒绝，管理员点"通过"会撞上状态机（只有审核中可审），实际处置是"看一眼、删掉/关闭"，不是真正的审核队列。
- **误判不可申诉、通知是固定文案**：作者只能知道"不合规"，不知道为什么。
- **不做**：申诉入口、批量撤销/恢复、把自动拒的任务标成已拒绝（那样管理员在待处理里看不到它，等于系统单方面做了决定且不告知）。
- **不改配额语义**：一次提交仍只预占一次；风险命中与模型自动拒都不二次扣。
- **零 DDL**：不加表、不加列。

## 当前生效约束

1. 同步扫描与审核列表标红必须共用 `moderation.Evaluate`，禁止出现第二份规则。
2. 同步门槛是 `medium` 及以上；模型自动拒门槛是 `violation` + `risk_level ≥ 2`，`suspect` 一律交人。**两者不得混用**，放宽模型门槛到 suspect 必须重新评估申诉成本。
3. `pending_review` 只能由系统写入，且必须由异步判定或审核通过转成终态；作者/管理员改可见性接口不得绕过。不允许存在"留在 pending_review 不动"的成功出口。
4. 面向作者的响应不得包含命中词、风险等级、模型类别与依据；同步命中只见 `article.risk_rejected`，模型自动拒唯一可见的是枚举文案「内容不合规」。
5. `skip_examining` 开启时的一致性放行（含标红）是显式例外，不得悄悄收紧。
6. 草稿不被自动拒；已拒绝不重复处理。只有「审核中」与「已发布」会被自动拒。
7. 审核任务来源 `source=compliance` 必须保留，以区分「人拒的」与「系统拒的」；不得复用到 `create`。同步复盘任务来源固定为 `source=risk_check`。
8. 通知缺少驳回原因时兜底到合规文案；兜底只读风险记录档位选固定文案，不携带细节。
9. 自动拒不二次预占配额；一次提交只预占一次，风险命中与模型自动拒都不再预占。
10. 同步路径不建人工任务；人工队列只由异步副作用产生。
11. 三种落点不得合并：质量不达标 → 草稿；同步风险命中 → 已拒绝 + `risk_check` 任务；模型自动拒 → 已拒绝 + `compliance` 任务。质量闸门判定排在风险扫描之后。
12. 不做申诉入口、不做批量撤销/恢复。
13. 零 DDL 口径不变：`pending_review` 是 `visibility_status` 的新取值，不是新状态、不是新列。

## 相关代码与文档

- 落点决策：`backend/internal/article/app/submit_outcome.go` · 同步扫描：`submit_risk.go`
- 状态转换：`backend/internal/article/domain/article.go`（`SubmitWithOutcome` / `PublishForCompliance` / `BackToReview` / `RevokeForCompliance` / `Approve`）· `status.go`（`VisibilityPendingReview`）· `rule.go`（`CanSetVisibility`）
- 自动拒用例：`backend/internal/article/app/auto_reject.go` · `compliance_transition.go` · `create.go` · `update.go`
- 异步分流：`backend/app/outbox_wiring.go`（`complianceSubscriber` / `rejectionReason` 兜底）
- 枚举：`backend/internal/kernel/enum/article_visibility_status.go` · `article_review_enum/article_review.go`（`SourceCompliance`）· `backend/internal/article/adapter/http/enums.go`
- 测试：`backend/internal/article/app/submit_risk_test.go` · `service_test.go`
- 前端：发布弹窗的待可见提示 `frontend/app/components/studio/StudioWritePublishDialog.vue` · 后台来源文案 `frontend_admin/app/pages/review/index.vue` · `frontend_admin/app/utils/reviewRisk.ts`
- 术语：[GLOSSARY · 三层机审 / 待可见 / 阻断级·观察级词表](../../GLOSSARY.md)
- 相邻决策：[ADR-0022](./0022-submit-quota.md) · [ADR-0023](./0023-quality-gate.md) · [ADR-0024](./0024-risk-models.md)

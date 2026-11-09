# ADR-0035：按风险分流的发布口径（标红直接拒、干净文章自动发）

- 状态：已交付
- 日期：2026-11-09
- 来源：用户 2026-11-09 提出的口径变更（"标红直接拒绝，只有有风险才转人工、干净文章自动发"）· 取代 [ADR-0034](./0034-auto-reject-and-landing.md) 中与之冲突的条款 · 关联 [ADR-0032](./0032-quality-gate.md)（同步端口）· [ADR-0033](./0033-risk-models.md)（第③层）· [ADR-0031](./0031-submit-quota.md)（配额）

## 背景与问题

旧口径（#302 一期）下，**每一篇投稿都进人工审核队列**，与有没有风险无关：站点 `skip_examining` 默认为关，`ResolveSubmitStatus(false)` 一律返回 `StatusExamining` 并建 `StageManual` 任务。机审前两层（词表 + 评分派生标签）只给队列加标红和排序，不改变文章状态；第③层模型判定异步产出结论，只有高置信违规才自动拒。

用户认为这不对：标红就该直接拒，人工只看真正有风险的，干净文章应该自动发。但"自动发"与"模型判定是异步的"存在张力——提交那一刻还拿不到模型结论，无法知道文章是否"干净"。

## 决策

1. **同步风险扫描**：提交时（事务外）把第①层词表与第②层评分派生标签合并跑一次 `moderation.Evaluate`——与审核列表标红用**同一份**规则，避免两处口径漂移。新建文章通常还没有评分记录，此时只跑词表层、不报错。
2. **命中 `medium` 及以上（含 `high`）→ 直接 `REJECTED`**，不给改；留一条 `source=risk_check` 待处理任务供管理员复盘。作者只拿到固定文案（`article.risk_rejected` / 枚举 `risk_check`），命中的词、维度、等级一律不外泄。这**推翻** ADR-0034「不做词表自动拒」与 #311「只标红、观察级交人」。
3. **`none` / `low` → 立刻以 `StatusPublished + VisibilityPendingReview` 落库**。作者立刻拿到"已发布"回执，但匿名访客读不到：`IsPubliclyVisible()` 与公开读面 SQL 都只认 `published + visible`。
4. **新可见性取值 `pending_review`**：是 `visibility_status`（varchar 32）的新取值，**不是第五个状态、不是新列**，满足零 DDL 约束。作者与管理员都不能用"改可见性"接口提前放行（`CanSetVisibility` 双向拦截），要放行只能走审核通过。
5. **模型结论分流**（异步，仍走 outbox）：
   - `clean` → `PublishForCompliance`：转 `visible`，真正对读者可见；
   - `suspect` / `violation` 但 `risk_level < 2` → `BackToReviewForCompliance`：改回 `EXAMINING` 并建 `model_suspect` 任务；
   - `violation` 且 `risk_level ≥ 2` → 沿用 ADR-0034 的 `AutoRejectForCompliance`（`REJECTED` + `compliance` 复盘任务）；
   - 判定失败 → 用 outbox 的 `Event.Attempts` 重试**一次**，第二次仍失败则同样 `BackToReviewForCompliance`（`model_failure`），绝不留在 `pending_review` 变成隐形拒稿。
6. **同步路径不再建人工任务**：人工队列只由异步副作用产生（存疑 / 失败 / 自动拒复盘）。
7. **`skip_examining` 开启时全部直接真发布**（`visible`），同步扫描也不拦；第③层只作事后兜底。这是运营的紧急放行手段，优先级最高。
8. **质量闸门不变**：不达标仍落草稿（可改可重投），且排在风险扫描之后判定——风险命中的落点是 `REJECTED` 不给改，不能被闸门的草稿分支盖掉。
9. **三种落点**：质量不达标 → 草稿；同步风险命中 → 已拒绝；模型自动拒 → 已拒绝 + `compliance` 任务。ADR-0034 的"两种落点"由此扩展。

## 理由与已否决方案

- **否决"提交时同步等模型判定"**：那会推翻 ADR-0033 的异步约定，让每次提交多等 5–30 秒，且评分/判定都得挤在事务外。改为「先发后判、判完才可见」，用可见性维度承接这个时间差。
- **否决"干净文章直接真发布、模型只事后自动拒"**：用户明确要"模型审核通过之后才改成可见状态"。直接可见会留下违规内容的公开窗口。
- **否决"判定失败就自动放行"**：AI 挂了把未知当作干净，等于风控形同虚设。
- **否决"判定失败无限重试到死信"**：会让文章永远停在"作者以为发了、读者看不到"。重试一次后转人工，是失败关闭与不卡死之间的折中。
- **否决"把 `pending_review` 做成第五个文章状态"**：状态枚举要动库里 tinyint 的语义、涉及所有读面；复用可见性取值只需扩展一个 varchar 枚举，零 DDL。

## 代价 / 不做什么

- **词表误报现在直接压到作者身上**：字符串匹配没有语境（"六四"命中"六十四位"），且 `REJECTED` 不给改、不可申诉。因此固定文案与 `risk_check` 复盘任务必须保留，管理员要能看见误判。
- **存在"已发布但暂不可见"窗口**：期间作者本人与管理员可见，其他读者不可见。任何把 `pending_review` 转成终态的路径都必须存在（clean → visible、suspect/失败 → EXAMINING、violation → REJECTED），不允许新增一条"留在 pending_review 不动"的成功出口。
- **人工队列性质变了**：里面不再是全部投稿，而是模型存疑、判定失败与系统直接拒的复盘任务；批量审核 UI 仍可用，但队列会小很多。
- **不做申诉入口、不做批量撤销/恢复**（沿用 ADR-0034 代价段）。
- **不改配额语义**：一次提交仍只预占一次；风险命中与模型自动拒都不二次扣。

## 当前生效约束

1. 同步扫描与审核列表标红必须共用 `moderation.Evaluate`，禁止出现第二份规则。
2. 同步门槛是 `medium` 及以上；模型自动拒门槛仍是 `violation` + `risk_level ≥ 2`。两者不得混用。
3. `pending_review` 只能由系统写入，且必须由异步判定或审核通过转成终态；作者/管理员改可见性接口不得绕过。
4. 面向作者的响应不得包含命中词、风险等级、模型类别与依据。
5. `skip_examining` 开启时的一致性放行（含标红）是显式例外，不得悄悄收紧。
6. 草稿不被自动拒（沿用 ADR-0034 约束 6）。

## 相关代码与文档

- 落点决策：`backend/internal/article/app/submit_outcome.go` · 同步扫描：`submit_risk.go`
- 状态转换：`backend/internal/article/domain/article.go`（`SubmitWithOutcome` / `PublishForCompliance` / `BackToReview` / `Approve`）· `status.go`（`VisibilityPendingReview`）· `rule.go`（`CanSetVisibility`）
- 异步分流：`backend/app/outbox_wiring.go`（`complianceSubscriber` / `rejectionReason` 兜底）
- 用例与装配：`backend/internal/article/app/compliance_transition.go` · `create.go` · `update.go`
- 枚举：`backend/internal/kernel/enum/article_visibility_status.go` · `article_review_enum/article_review.go`
- 测试：`backend/internal/article/app/submit_risk_test.go` · `service_test.go`
- 前端：发布弹窗的待可见提示 `frontend/app/components/studio/StudioWritePublishDialog.vue` · 后台来源文案 `frontend_admin/app/pages/review/index.vue`
- 术语：[GLOSSARY · 三层机审 / 待可见 / 阻断级·观察级词表](../../GLOSSARY.md)

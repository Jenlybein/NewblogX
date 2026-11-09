# ADR-0033：第③层模型合规判定与 `article_risk_models`

- 状态：已交付
- 日期：2026-11-08
- 来源：parent spec [#302](https://github.com/Jenlybein/NewblogX/issues/302)（发布治理一期）· 落地票 [#312](https://github.com/Jenlybein/NewblogX/issues/312)（合规判定）· 决定过程见 [grill 记录 §4 / §五](../notes/2026-11-07-publish-gate-and-moderation-grill.md)

## 背景与问题

机审前两层（词表、评分派生标签）已经在审核列表上标红，但都是**便宜的启发式**：词表不知道语境（"我在讨论诈骗手法"会被当成诈骗），评分只关心写得好不好（一篇文采很好的诈骗教程能拿 82 分）。第③层要把文章的**内容语义**交给模型判断一次，并把结论留给审核员。

三个约束逼住设计：

1. **零 DDL**：不能给 `article_models` 或审核任务表加列。
2. **AI 是异步且会失败的**：与提交请求同步做合规判定，会让一次投稿的延迟从毫秒级变成十秒级，而且 AI 一挂所有投稿都挂。
3. **结论必须能区分"没判定"与"判定为干净"**：这两个状态如果合并展示，审核员会把没检查过的文章当成已检查过的放过去。

## 决策

1. **合规判定是新表 `article_risk_models` 的唯一用途**。这是零 DDL 授权通道的又一次使用：由本 ADR 指名、在 `schema.sql` 与 testkit 两处登记（`ArticleRisk` 组 + 平铺清单）。一篇文章一行、唯一键在 `article_id`、随判定覆盖、**不存历史**。
2. **判定走异步（事务型发件箱）**，挂在 `article.created` 与 `article.published` 两个事件上，与评分订阅者并列。拿不到结论时返回 error 交给重试与死信——后台看到的是"未判定"，不是"干净"。
3. **结论三档** `clean / suspect / violation`，不是布尔。认不出的档位一律收敛到 `suspect`：归到 clean 等于静默放行，归到 violation 等于静默拒绝，两者都比"交人看"更糟。
4. **风险级别 0–3 独立于档位**，用于审核列表排序；越界值收敛到区间内。
5. **判定规则来自运行时策略文本**：第 7 个 AI 场景 `moderation` 绑定 `skills/moderation/SKILL.md`（内容哈希作版本）。策略文本解释"这个站点在意什么"，提示词里内置的那段只规定**输出契约**（JSON 结构），两者分开，改文案不会改解析。
6. **驳回话术不进 skill**：驳回理由用固定枚举（#308）加一句补充，审核员不需要模型教他怎么说话。
7. **结论只给管理员看**：审核列表接口本身是管理员专属；`reasons`（模型给的依据）也不出现在任何面向作者的响应里——把判定依据给作者等于把规则公开，等于教人绕过。
8. **AI 模块不写别人的表**：判定由 AI 模块产出，落库由文章模块的用例层执行（`SaveComplianceResult`）。AI 模块只返回结论结构。

## 理由与已否决方案

- **否决"同步判定"**：把 5–30 秒的模型调用塞进提交请求，与 ADR-0032 拒绝把评分放进事务是同一条理由的延伸——延迟与失败面都不该压到作者身上。
- **否决"给文章表加一列"**：零 DDL 是硬约束；加列要走例外授权，而这张表要存类别数组、依据数组、模型名、策略版本、判定时间，一行九列不是"加一列"。
- **否决"布尔结论"**：丢掉了"可疑但不定性"这一档，而人工审核的价值恰恰在中间地带。
- **否决"结论合并进评分记录"**：评分是写得好不好、合规是能不能发，两者会矛盾（82 分的诈骗教程），混在一行会让人无法分辨该信哪个；失败与重试口径也不同。
- **否决"判定规则写死在提示词里"**：站点之间在意的东西不同，而代码内提示词发版才能改；skill 让运营热改且带版本号。
- **否决"把结论也发给作者"**：违反"驳回话术由人决定"的前提，且等于公开判定规则。
- **否决"存判定历史"**：白送的成本；误判不可复盘与评分记录的取舍一致（#302 已定）。

## 代价 / 不做什么

- **判定可能幻觉**：模型会把科普文章判成违规。缓解是三档 + 只标红不自动处置（自动拒留给 #313，且只对明确违规）。
- **结论不可申诉**：不存历史，作者看不到依据，管理员是唯一读者。
- **一篇文章一行**：重复提交会覆盖，无法比较不同版本。
- **成本**：每次创建与发布各一次模型调用；同一篇文章如果"创建即发布"会跑两次（两次事件）。刻意不做去重——两次事件的语义不同（发布时内容可能已改）。
- **不是自动拦截**：本票只产出结论与标红，处置权在人（自动拒在 #313 单独立项）。（**2026-11-09 修订**：新口径下结论会决定文章是否对读者可见、以及是否改回人工审核；但仍不直接决定"提交请求"的成败，见 [ADR-0035](./0035-risk-gated-publish.md)。）

## 当前生效约束

1. **新表只允许经 ADR 指名**；`article_risk_models` 已登记，改表结构要走新的 ADR 或修订本节。
2. **结论三档、未知档位收敛到 suspect**，不得改成布尔或改收敛方向。
3. **合规判定必须异步**，不得挪进提交请求链路。
4. **"未判定"必须与"判定为干净"在响应里可区分**（`compliance` 为 null vs `verdict=clean`）。
5. **第 7 个场景名固定为 `moderation`**；新增场景要同时更新后台筛选、图例与文档。
6. **AI 模块不得直接写 `article_risk_models`**：跨模块写只能由拥有表的模块执行。
7. **驳回理由不用 skill**，用固定枚举 + 补充文本。

## 相关代码与文档

- 概念：[GLOSSARY · 三层机审 / 风险记录](../../GLOSSARY.md) · [grill §4 §五](../notes/2026-11-07-publish-gate-and-moderation-grill.md) · [#312](https://github.com/Jenlybein/NewblogX/issues/312)
- 判定：`backend/internal/ai/moderation/moderation.go` · `backend/internal/ai/app/service.go`（`CheckArticleCompliance`）
- 场景与策略文本：`backend/internal/ai/callmeta.go`（`SceneModeration`）· `skills/moderation/SKILL.md`
- 记录与仓储：`backend/internal/article/domain/risk/risk.go` · `backend/internal/article/adapter/mysql/risk_repo.go` · `backend/internal/article/app/compliance.go`
- 表与夹具：`backend/internal/platform/mysql/ddl/schema.sql` · `backend/internal/testkit/schema/{rows.go,schema.go}`
- 异步触发：`backend/app/outbox_wiring.go`（`complianceSubscriber`）
- 暴露：`backend/internal/article/adapter/http/{dto,response}.go`
- 配置默认：`backend/config/site_default_settings.yaml`（`ai.scene_skills.moderation`）

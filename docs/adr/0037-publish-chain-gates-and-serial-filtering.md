# 0037 发布链路串行化与三道闸门可配置

- 状态：已生效（2026-11-28）
- 相关票：GitHub #497（父票），子票 #498–#505

## 背景与问题

发布判定原本是三条**并行**的闸门：关键词过滤、质量评分、AI 合规判定。三条各自
读配置、各自决定文章去哪，彼此不知道对方已经拒了。三个具体病：

1. **花了不该花的钱**。一篇满是"代开发票"的稿子，关键词那一关已经注定不过，但
   质量评分与 AI 合规判定照旧各调一次模型 —— 两份 token 换来一个早已确定的结论。
2. **闸门开关不完整**。「文章审核设置」上的质量闸门只有一个 `enabled`，没有「评
   分本身要不要产生」的开关：闸门关掉之后，后台仍会为每篇稿子调模型评分，读者站
   和写稿页仍显示分数。运营关的是"用分数拦人"，实际关不掉"花模型的钱"。
3. **AI 判定不可用时只有一条路**：转人工。运营想"先放行、等模型好了再补判"或
   "先退回草稿让作者重投"都做不到，只能干等人工。

再加上同类但更早的问题：命中词只有"打星"与"拒发"两档，中间缺一档"退回草稿让
作者改"——那才是命中推广话术时最该发生的处置。

## 决策

### 1. 串行短路（关键词 → 质量 → AI）

判定链改成串行，任一环给出非放行结论就短路：

- 关键词命中且处置非「放行」→ 后面的质量评分、AI 合规判定、额度扣减全部不发生；
- 质量闸门判不过 → 不调 AI 合规判定；
- 前一环放行才进入下一环。

这不只是省钱：短路让「文章为什么没过」只有一个原因，作者看到的理由不会因为三条
并行判定各说一套而互相矛盾。

### 2. 命中处置三档（拦截）/ 两档（观察）

`content_filter.auto_dispose` 的两个字段值域改为：

| 档位 | 可选处置 | 文章落点 | 评论落点 |
| --- | --- | --- | --- |
| 拦截词 `block_action` | `release` / `draft` / `reject` | 打星照发 / 退回草稿 / 落已拒绝 | 打星 / 拒收 / 拒收 |
| 观察词 `watch_action` | `release` / `draft` | 打星照发 / 退回草稿 | 打星 / 拒收 |

- 旧值 `mask` 与 `release` 同义：**读面就地映射**（`mask → release`），保存路径把它
  升级成 `release`。这样发版当天线上行为不变，一次后台保存即完成数据迁移。
- `draft` = 落草稿 + **提交配额不归还** + 通知作者（文案里不出现命中的词）+ 仍记
  一条关键词风险。配额不还的理由是每次提交都真的产生了一条审核任务。
- 评论只有两档语义：`draft` 与 `reject` **等价于拒收**。评论没有草稿态（值 1「审核
  中」已退场），也没有申诉通道，把它放进"待审队列"等于让作者以为发出去了却永远看
  不到。

### 3. 三个开关的分工

| 开关 | 段 | 关掉之后 |
| --- | --- | --- |
| `score_enabled`（开启质量评分） | `site.article` | 三条评分产生路径全停（提交同步评分、后台重评、发布判定补算）；读者站 / 写稿页 / 审核队列不再显示任何分数；**质量闸门被强制关**；评分接口统一返回 `article.score_disabled` |
| `ai_compliance_enabled`（开启 AI 合规判定加强） | `site.article` | 质量过线的文章**直接对读者可见**，不调第③层模型、不留合规结论 |
| `ai_failure_action`（AI 不可用落点） | `site.article` | 见下 |

`score_enabled` 是闸门的**父开关**：没有评分就没有判据，闸门开着只能是"所有投稿都
被拒"。因此保存时后端强写 `quality_gate.enabled=false` —— 矛盾值不可能进库，绕过
页面直接改配置也一样。

`ai_compliance_enabled=false` 与 `skip_examining` 走**同一个代码分支**（提交即可见）。
两个开关语义不同（前者是常态配置，后者是临时放行），但落点完全一致，共用一条分支
避免了两份几乎相同的判定散落在两处。

### 4. `ai_failure_action` 四档，只支配「调用失败」

| 取值 | 落点 | 代价 |
| --- | --- | --- |
| `manual`（默认） | 转人工 + 建待处理任务 | 作者要等，但误判不落在他头上 |
| `hold` | 保持审核中、无任务、次日重投 | 不需要人，但要等到明天 |
| `draft` | 退回草稿 + 通知「审核服务暂时不可用」 | 作者改不了什么（不是内容问题） |
| `release` | 直接放行 + 落「未判定」+ 警告日志 | 没有合规结论，事后自动撤下对它不生效 |

**边界是本条决策最容易搞错的地方**：

- 模型判「存疑」或「低风险违规」是**结论**，不是失败 → 仍转人工，不受本项支配；
- 当日额度用尽是**另一个原因** → 固定 `hold`（重试注定再被拒，还会白扣一次）；
- 连续三次拿不到分数 → 仍转人工（评分失败与合规失败是两件事）。

### 5. 两个子页、一个聚合端点

「文章审核设置」拆成两个子页，菜单从三级变四级：

```
站点配置 → 内容修改与发布 → 文章审核设置 → 审核规则（/site/article-review）
                                          → 关键词命中处理（/site/article-review/keywords）
```

拆分理由：审核规则写 `site.article`，关键词命中处理写 `content_filter.auto_dispose`；
ADR-0029 的原则是"一个页面只碰一个配置段"，否则保存按钮会同时覆盖两段。

两页共用一个聚合端点 `PUT /site/article-review`：

- 请求体两段都可选（`article` 整段 read-modify-write、`auto_dispose` 字段级补丁），
  两段都不传报 1001；
- **先校验后写**：两段都归一化，任何一段不合法就一个字节都不写。跨段事务做不到
  （运行时配置服务是两段各自写的），但"填错了值"这个最常见的失败模式不会留下半套
  配置。

同时 `PUT /site/content_filter` 从整段覆盖改为**字段级补丁**（按 raw JSON 判断哪些
键真的传了，现值兜底）：词表页与命中处理页写同一段，整段覆盖会让任一方保存时把对方
的值静默恢复默认。

### 6. 删掉 `quality_auto_release`

它表达的是"质量过线就别等 AI 合规判定，直接可见" —— 这正好就是
`ai_compliance_enabled=false`。留着它等于同一件事有两个开关，而且组合起来会出现
"自动放行但没有评分"这种解释不清的状态。存量取值的迁移规则：

- `quality_auto_release=false`（线上现状）→ `ai_compliance_enabled` 保持默认 `true`，
  即**行为变严**（这类站点本来就希望 AI 判一遍）；
- `quality_auto_release=true` → `ai_compliance_enabled=false`（自动放行 → 跳过 AI）。

## 理由与已否决方案

- **否决"三闸门并行 + 事后汇总"**：既费钱又给作者互相矛盾的理由。串行的代价是
  "第①环的结论会掩盖后两环的问题"，但那是可接受的 —— 命中词的处置本来就是更强的
  理由。
- **否决"命中只记事实、由各闸门各自判断"**：三处各写一份 `if masked && reject`，
  明天加一档就要改三处。现在过滤层只**报事实**（`BlockTierMasked` /
  `WatchTierMasked`），档位判定只有 `dispositionAfterFilter` 一处。
- **否决"评论也放进待审队列（复用 draft）"**：评论没有草稿态、没有申诉通道，进队列
  等于黑洞。评论面因此收敛成二档。
- **否决"`score_enabled=false` 时把分数一并从库里删掉"**：历史分数是有用的审计
  线索，而且重新打开开关时应当立刻恢复显示。开关决定的是"显示与产生"，不是"销毁"。
- **否决"`ai_failure_action=release` 也让额度用完走放行"**：额度用完是**可预期**的
  资源约束，不是故障；放行意味着"因为没额度所以不判"，这在安全上是反的。
- **否决"给两个子页各开一个端点"**：那样"保存成功"被拆成两次网络往返，中间失败会
  留下半套配置；一次请求、一次校验更简单。

## 代价 / 不做什么

- **不做**「命中词按类目分别配置处置」（例如"涉政一律拒、诈骗一律退草稿"）。当前
  只有两档一个口径，按类目配置会让设置页变成一张矩阵表，而运营实际只有一条线。
- **不做**「评分关闭时把闸门的数值也一起隐藏」：数值留着（灰掉），因为重新开评分
  时运营要看到原来的阈值，否则等于让他们重新猜一遍。
- **不做**「AI 失败落点按失败类型细分」（超时 / 5xx / 网络各一档）：失败原因对运营
  不可见也无从选择，四档已经是可解释的上限。
- **不做**「评论命中进申诉通道」：评论没有申诉入口，先做拒收，申诉是另一条产品线
  （ADR-0036 已记录评论处置的整体口径）。
- **接受**：`ai_failure_action=draft` 时作者会收到"服务不可用"通知但改不了内容 ——
  文案必须说清是服务侧问题，否则作者会反复改稿。

## 当前生效约束

1. 判定链**必须串行**：关键词 → 质量 → AI；任一环非放行即短路，后面的环节不得被
   调用（不得为了"顺便补个分数"绕过短路）。
2. 过滤层**只报事实**（`BlockTierMasked` / `WatchTierMasked`），档位判定只在用例层
   的 `dispositionAfterFilter` 一处；白名单覆盖的命中算"没命中"。
3. `score_enabled=false` ⇒ `quality_gate.enabled` 必须为 `false`，由后端在归一化时
   **强写**，不依赖前端置灰。
4. `score_enabled=false` 时评分接口（`POST /api/ai/scoring/article` 的三个 type）
   一律返回 `article.score_disabled`（`1001`）：只隐藏界面不改接口 = 开关没生效。
5. `ai_failure_action` 只支配「调用失败」；模型结论与额度用尽**不得**读它。
6. 评论面只有二档：`release` 打星、其余一律拒收；不得为评论引入第三态。
7. 聚合端点必须**先校验后写**；`article` 是全量覆盖、`auto_dispose` 是字段级补丁；
   `PUT /site/content_filter` 必须是字段级补丁（缺键保留现值）。
8. 旧值 `mask` 在**读面**必须映射为 `release`；保存路径负责把库里的值升级。
9. 新增字段必须在 `config/site_default_settings.yaml` 里显式写出（缺键会静默变零值，
   而这里三个新开关的零值恰好都是"关"，会把默认行为翻转）。

## 相关代码与文档

- 配置形状：[`backend/conf/site/enter.go`](../../backend/conf/site/enter.go)（`Article`、
  四档常量）、[`backend/conf/conf_content_filter.go`](../../backend/conf/conf_content_filter.go)、
  [`backend/conf/conf_ai.go`](../../backend/conf/conf_ai.go)（`AssistantEnabled`）
- 默认值：[`backend/config/site_default_settings.yaml`](../../backend/config/site_default_settings.yaml)
- 归一化与校验：[`backend/internal/site/app/service.go`](../../backend/internal/site/app/service.go)
  （`normalizeArticle` / `normalizeDisposeAction` / `UpdateArticleReview`）
- 短路与落点：[`backend/internal/article/app/filter.go`](../../backend/internal/article/app/filter.go)
  （`dispositionAfterFilter` / `maxDisposition`）、
  [`backend/internal/article/app/create.go`](../../backend/internal/article/app/create.go)、
  [`backend/internal/article/app/update.go`](../../backend/internal/article/app/update.go)、
  [`backend/internal/article/app/submit_outcome.go`](../../backend/internal/article/app/submit_outcome.go)
- 发布判定：[`backend/app/publish_judge.go`](../../backend/app/publish_judge.go)、
  [`backend/app/outbox_wiring.go`](../../backend/app/outbox_wiring.go)（`settleAIFailure`）
- 评论面：[`backend/internal/comment/app/service.go`](../../backend/internal/comment/app/service.go)、
  [`backend/app/comment_wiring.go`](../../backend/app/comment_wiring.go)
- 端点与 DTO：[`backend/internal/site/adapter/http/handler_runtime_sections.go`](../../backend/internal/site/adapter/http/handler_runtime_sections.go)、
  [`backend/internal/platform/http/router/site_router.go`](../../backend/internal/platform/http/router/site_router.go)
- 后台页面：[`frontend_admin/app/components/settings/SettingsArticleReview.vue`](../../frontend_admin/app/components/settings/SettingsArticleReview.vue)、
  [`frontend_admin/app/components/settings/SettingsHitDisposition.vue`](../../frontend_admin/app/components/settings/SettingsHitDisposition.vue)、
  [`frontend_admin/app/utils/menu.ts`](../../frontend_admin/app/utils/menu.ts)
- 读者面：[`frontend/app/pages/article/[id].vue`](../../frontend/app/pages/article/[id].vue)、
  [`frontend/app/pages/studio/write.vue`](../../frontend/app/pages/studio/write.vue)
- 其他 ADR：[0029](./0029-admin-nav-three-levels-and-site-split.md)（菜单与页面拆分）、
  [0032](./0032-content-filter.md)（内容过滤与命中记录）、
  [0036](./0036-comment-moderation.md)（评论处置与申诉）、
  [0023](./0023-quality-gate-and-bucket-judgement.md)（质量闸门木桶判定）
- 术语：[`GLOSSARY.md`](../../GLOSSARY.md)「发布链路」「命中处置」「质量评分开关」
  「AI 合规判定」「AI 不可用落点」
- 架构：[`docs/architecture/configuration.md`](../architecture/configuration.md)（配置分层与运行时段）

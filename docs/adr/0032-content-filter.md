# ADR-0032：全局内容过滤（命中替换为星号、风险记录交后台处置）

- 状态：已接受（2026-11-17 落地，实施记录见文末「实施情况」）
- 日期：2026-11-17
- 来源：2026-11 腾讯云内容违规通知后的自查 · 关联 [ADR-0020](./0020-config-section-registry-and-named-sections.md)（配置段）· [ADR-0024](./0024-risk-models.md)（第③层）· [ADR-0025](./0025-risk-gated-publish.md)（发布分流）

## 背景与问题

2026-11 域名被腾讯云判定为「发布传播违法违规信息」。自查结论：违规文本来自站长自己的测试账号——昵称被改成「测试售卖枪支冰毒」并在公开个人主页展示了三天；三条含毒品词的评论被评论机审扣在「审核中」，但昵称、简介这类公开渲染的字段**没有任何过滤**。

现有机制同时暴露了四个结构问题：

1. **词表有两份**（文章 `article/domain/moderation`、评论 `comment/app/moderation`），匹配口径不同（文章有 ASCII 词边界、评论没有）。
2. **词表匿名可读**：`GET /api/site/site` 整段返回 `article.moderation` / `comment.moderation`，任何人一次请求就能拿到完整词表。
3. **匹配太弱**：只做 `ToLower`，全角、零宽字符、插入空格与分隔符都能绕过。
4. **覆盖面窄**：只有文章投稿与评论两个入口；昵称、简介、私信、合集标题完全没有过滤。

由此要一次定清楚六件事：

1. 关键词命中之后是**拦**还是**放**？拦在哪里、依据什么？
2. 过滤发生在**落库前**还是**读时**？原文是否保留？
3. 词表放在哪、怎么保证不公开、怎么维护上千条词？
4. 评论现有的审核系统（状态机 + AI 判定 + 人工队列）还要不要？
5. 命中记录给谁看、怎么处置、保留多久？
6. 存量违规文本（含三条审核中的评论）怎么处理？

## 决策

1. **删除两处词表机审**：删除 `article/domain/moderation` 的词表层（`WordLists` / `MatchWords`）与 `comment/app/moderation.go` 的 `matchWords`，删除 `site.article.moderation` / `site.comment.moderation` 两处配置与后台两个 textarea。文章的**质量闸门、AI 风险判定、发布分流保留**；评分派生标签（低分 / 维度塌陷 / 篇幅异常）保留，审核列表不再有 `block_word` / `watch_word` 两个信号。
2. **评论审核系统整体退场**：评论不再有「审核中」状态，`status` 收敛为「已发布 / 已删除」；删除 AI 评论判定（`comment.ai`、`ComplianceChecker`）、`/comments/man`、设置页「评论审核」卡片与前端「审核中」UI。评论一律发布。
3. **全站唯一过滤层**：新增运行时配置段 `content_filter`（第 11 段），只给后台读写，公开端点不返回。接入六个文本面：文章标题与正文、评论、昵称、简介、私信、合集标题与描述；图片内容不在本层（另走图片审核）。
4. **命中的统一处置是替换不是拒绝**：命中词替换为 `*`（按 rune 等长），文本继续保存/发布；同时写一条 `content_filter_hit` 记录，进后台「审核 → 关键词风险」页。拦截词与观察词**都替换**，颜色只用于风险页标识（拦截红、观察黄）。白名单词命中不替换，但仍记录。
5. **过滤在落库前**：替换后的文本才是唯一落库、入 ES、发通知、推 WS 的版本，公开面因此不可能出现命中词。**不保留原文快照**；风险记录里存命中词、档位、命中位置前后 20 字上下文（高亮命中词）、文本面、内容 ID、用户 ID。
6. **顺序是先替换、再审核**：质量闸门与 AI 风险读的是带 `*` 的文本。这是显式接受的代价（见「代价」）。
7. **引擎**：`github.com/yihleego/trie`（MIT，AC 自动机，rune 级返回命中位置）。归一化不改变 rune 对齐：全角→半角 1:1、拉丁小写 1:1、零宽字符换哨兵；另生成「压掉空白与分隔符」的影子串并记录每个字符的原始下标，AC 命中区间经映射回原文再替换。不做整段 NFKC（会破坏对齐）。
8. **词表**：来源 `houbb/sensitive-word-data`（Apache-2.0）的类目子集，导入 3 万条，**双字起**（单字不入表），配白名单；按类目默认分档（毒品/枪支、涉政、暴恐、假证发票、诈骗 → 拦截；赌博/色情中的宽泛推广词 → 观察），词云可逐词改。**涉政、暴恐类词只进数据库**，不进仓库、不进 `site_default_settings.yaml`、不进 CI 日志。
9. **词表维护**：后台「站点 → 内容过滤」页做词云（字号 = 90 天命中次数、颜色 = 档位）+ 搜索筛选 + 点词编辑 + 批量导入（每行一词）+ 白名单 tab。启动时构建一次 AC，之后保存词表即重建并经 `atomic.Value` 原子切换，请求路径不做构建。
10. **风险页**：`content_filter_hit` 表 + 后台页（导航「审核」下），支持按文本面/状态/用户筛选、顶部命中用户 Top、单条与批量处置。动作四个：删除内容（文章/评论/合集软删；昵称重置为「问题昵称+uid 末 4 位」、简介清空）、忽略、封禁（账号禁用，沿用账号页既有口径 `status=3`、吊销会话）、禁言（带时长，单次处置参数 1–365 天、缺省 7；只拦发布类动作，到期惰性判定 + 每小时对账恢复）。命中状态四个：`pending` / `ignored` / `deleted` / `account`（账号已处置、内容保留）；记录保留 90 天（可配）。
11. **失败方向**：风险记录写失败不阻断主流程（记 error 日志）；过滤引擎构建失败**阻断启动**（无过滤上线等于裸奔）。
12. **迁移是显式的，不做自动合并**：旧的两份词表随词表层一起被删，**没有启动期合并迁移**——那种「段为空就自动搬旧词」的迁移会让「本来就想用一份空词表」与「还没导入」两种情况无法区分，而且它依赖旧结构存在，旧结构一删就只剩一段死代码。上线顺序固定为：`-filter-import`（默认 dry-run，`-filter-import-apply` 才写）→ `-filter-sweep`（默认 dry-run，`-filter-sweep-apply` 才写）清洗六个文本面并补写风险记录 → 重建 ES 索引（`-es -s init` + `-es -s article-sync`，或带 `-filter-sweep-reindex`）。词表为空时清洗命令**直接报错**而不是打一份「0 命中」的报告；存量 `status=1`（审核中）的评论**在进程启动时退役为已删除**（`RetireCommentExamining`：
条件更新只命中 `status=1`，幂等，跑第二遍 0 行），清洗命令也会顺带报一次条数。
这两件事要分清：**词表不做自动合并**（旧结构已删，无法安全猜测该不该合并），
而**状态值的退役必须自动做** —— 代码上线后已经没有任何入口能把评论置回「审核中」，
留在库里就是一批永远无法处理的行。
13. **契约**：新增 `content_filter` 段具名端点、风险页列表/处置端点；删除评论审核相关字段与端点、删除 `conf.Site.Moderation`；`contract-structures.json` 的条目数变化（`Moderation` schema 删除导致的下降）按仓库规则在 comment / 本 ADR / 提交说明三处写明理由。

## 理由与已否决方案

- **否决「命中即拒 / 进审核队列」**：关键词没有语境，拒绝会把误报直接压给作者，而人工队列会退化成「看谁被误伤」。替换 + 风险页让公开面保持干净，同时把人的注意力留给真正可疑的内容。
- **否决「原文落库、读时替换」**：读路径太多（详情、摘要、ES 搜索、AI 搜索上下文、站内信、私信 WS、sitemap），漏一处就是这次事故的复现。落库前替换是唯一能证明「公开面干净」的做法。代价是不可恢复，已显式接受。
- **否决「词表仍放 `site` 段」**：`GET /api/site/site` 匿名可读，且 ADR-0020 的「公开读与同段具名响应同形」会让每次加字段都要重新判断敏感性。单独开段一次解决。
- **否决「只存命中词、不留上下文」**：风险页无法判断误报，处置只能靠猜。
- **否决「自己写 trie」**：两万词过滤整篇博文需要 AC 的失败指针，自写要 250–400 行，还要自己处理重叠命中与 rune 下标；`yihleego/trie` 单文件约 400 行、MIT、正好返回 rune 区间。
- **否决 `importcjj/sensitive`**（master 是纯 trie，无失败指针，且 `FindAll` 不返回位置）、**`cloudflare/ahocorasick`**（返回词表下标而非文本位置，matcher 构建后不可变）、**`observerss/textfilter`**（未声明许可证，词表文件已被作者删除）。
- **否决「整段 NFKC 后再匹配」**：NFKC 会改变 rune 数量，命中位置无法映射回原文，替换必然错位。
- **否决「跳过代码块与 URL」**：用户明确选择无差别替换；后果见「代价」。
- **否决「禁言等于禁止登录」**：误伤一次就让读者无法访问，投诉成本高于收益；禁言只拦发布类动作。

## 代价 / 不做什么

- **先替换再审核**：质量闸门与 AI 风险读到带 `*` 的文本，技术文可能因「内容不完整」被降分、甚至被判不合格；AI 也可能因上下文缺失误判。已显式接受，风险页可人工放行。
- **全文无差别替换**：文章里的链接 `?q=破解` 会变成 `?q=**` 而 404，代码示例也会被打星。上线时需在发布须知里写明。
- **不留原文**：误报不可恢复，只能「删除内容 / 忽略」；风险页只能靠 ±20 字上下文判断，处置质量依赖管理员。
- **评论审核退场后没有人工队列兜底**：违规评论只能靠 AI 风险（文章侧）与事后处置；评论侧不再有任何自动拦截。
- **词表规模与内存**：6 万词建 AC 估算 30–50MB 常驻（未实测），因此只导入 3 万条子集；实际常驻要看部署后的进程 RSS。
- **依赖维护真空**：`yihleego/trie` 2022 年后无提交，属算法冻结型依赖；缓解方式是锁版本 + 自带中文/重叠/长文本回归用例。
- **不做**：原文快照与误报恢复、评论人工审核、申诉入口、按文本面的独立词表、拼音/谐音/形近匹配（由 AI 层兜底）。

## 当前生效约束

1. 公开面不得出现未替换的命中词；新增任何文本面必须同时接入过滤并补一条测试。
2. `content_filter` 段不得进入任何匿名可读端点；词表与命中词不得写入公开仓库、默认 yaml 或 CI 日志。
3. 过滤必须在落库前完成；替换后文本是唯一入库、入 ES、进通知、进 WS 的版本。
4. 风险记录写失败不阻断主流程；过滤引擎构建失败必须阻断启动。
5. 评论不再有「审核中」状态与人工审核队列；`status` 只有已发布 / 已删除。
6. 文章审核列表不再有 `block_word` / `watch_word` 信号；质量闸门、AI 风险与发布分流保持不变。
7. 风险记录保留 90 天（可配），处置动作固定四个（删除 / 忽略 / 封禁 / 禁言），不提供误报恢复。
8. 契约改动必须同批重新生成产物，并在 `contract-structures.json` 记录条目变化理由。

## 相关代码与文档

- 新增：`backend/internal/platform/contentfilter/`（引擎、归一化、替换、白名单）· `backend/internal/filter/`（命中表、处置用例、后台 handler）· `backend/conf/conf_content_filter.go` 与 `platform/config/registry.go`
- 删除：`internal/article/domain/moderation` 词表层 · `internal/comment/app/moderation.go` · 评论 status/AI 判定链路 · 后台词表 textarea
- 改动：`internal/article/app/submit_risk.go` · `internal/user/app/service.go` · `internal/chat/app/service.go` · collection 用例 · 六个前端入口的提示
- 文档：本 ADR · [GLOSSARY](../../GLOSSARY.md)（内容过滤 / 拦截词 / 观察词 / 关键词风险）· [ADR-0020](./0020-config-section-registry-and-named-sections.md)（十段→十一段）· [ADR-0025](./0025-risk-gated-publish.md)（词表层退场、先替换后审核）· [configuration.md](../architecture/configuration.md)（段表与公开读说明）

## 实施情况（2026-11-17）

| 票 | 内容 | 落地形状 |
| --- | --- | --- |
| [#394](https://github.com/Jenlybein/NewblogX/issues/394) | 过滤引擎 + 第 11 段配置 | `platform/contentfilter`（fold + 影子串 + AC）。引擎用 `yihleego/trie`（MIT，rune 级 `Emit{Begin,End}`），归一化与影子串映射都在本仓内实现，外加重叠命中 / 中文 / 长文本回归用例。 |
| [#395](https://github.com/Jenlybein/NewblogX/issues/395) | 初始词表 | `scripts/content-filter/` 从 houbb 子集生成 3 万词；生成期排除涉政 / 暴力类，这两类只进数据库。 |
| [#396](https://github.com/Jenlybein/NewblogX/issues/396) | 评论最小闭环 + 只读风险页 | 评论走 `filterContent` → 单次落库；风险页只读。 |
| [#397](https://github.com/Jenlybein/NewblogX/issues/397) | 其余五个面接入 | 六个文本面全部接入；面枚举钉在装配层（`app/*_wiring.go` 的 bridge），模块之间互不认识。 |
| [#398](https://github.com/Jenlybein/NewblogX/issues/398) | 风险页处置 | `POST /content-filter/hits/{id}/handle` + `handle-batch`（逐条结果）；`Disposer` 端口在 `filter/domain`，实现在 `app/filter_wiring.go`。**删除复用各模块自己的用例**，不直接写 SQL。 |
| [#399](https://github.com/Jenlybein/NewblogX/issues/399) | 封禁与禁言 | `user_models.mute_until`；禁言只拦发布类动作（`user.Service.EnsureCanPublish` + 三模块窄端口）；封禁 = `status=3`；`POST /users/admin/unmute` 提前解除；每小时对账恢复。 |
| [#400](https://github.com/Jenlybein/NewblogX/issues/400) | 评论审核退场 | `status` 收敛为已发布 / 已删除；AI 判定、`/comments/man`、设置页卡片、前端「审核中」UI 全部删除。 |
| [#401](https://github.com/Jenlybein/NewblogX/issues/401) | 文章词表机审退场 | `moderation.Evaluate` 只吃评分派生标签；`conf.Site.Moderation` 与契约里的 `Moderation` schema 删除（本轮条目数下降的唯一来源）。 |
| [#402](https://github.com/Jenlybein/NewblogX/issues/402) | 存量清洗 | `-filter-sweep`（dry-run 默认）+ `-filter-sweep-apply` + `-filter-sweep-reindex`；幂等判定是「这次替换是否真的改变了文本」。 |
| [#403](https://github.com/Jenlybein/NewblogX/issues/403) | 文档与契约收口 | 本 ADR 定稿 · `docs/adr/README.md` 登记 · GLOSSARY 词条改写 · ADR-0020 段数 · ADR-0025 改写 · `docs/architecture/configuration.md` · README 上线顺序。 |

与草案的三处偏差（均已在上文决策里改为最终口径）：

1. **词表规模从 1–3 万提到 3 万**（见决策 8）：生成脚本按类目筛出 30k 词；构建时间与常驻内存**未实测**，按开源词库的同类经验估在可接受范围，部署后看进程 RSS 确认。
2. **没有启动期旧词合并迁移**（见决策 12）：改成显式 `-filter-import`。理由是旧结构删除后那段迁移只剩死代码，且它让「空词表」与「还没导入」无法区分。
3. **命中状态多一个 `account`**（见决策 10）：封禁 / 禁言是账号级处置，内容并没有被删，用 `ignored` 或 `deleted` 都会误导读者。



## 补丁：类目覆盖自检（2026-11-22）

决策 8 把涉政 / 暴恐两类放在仓库之外，代价在 2026-11-22 兑现了一次：生产库里
`politics` 0 条、`violence` 0 条，而六个文本面对涉政内容完全不拦。**没有任何环节报错**
——仓库与 CI 按设计就排除这两类，后台页面显示三万条看起来也"很满"，部署侧导入那一步
从没被执行过。

补了三处红灯，判据同一份（`contentfilter.RequiredCategories`）：

1. **启动与保存词表时**打 `Warn`（`app/filter_wiring.go` 的 `warnMissingCategories`），
   并在正常的"引擎已构建"日志里附上七类分布；
2. **`-filter-coverage`**（只读）随时可查当前库里的类目分布，缺类目时输出 ⚠ 与下一步动作；
3. **后台「内容过滤」页**顶部红条，提示这两类只能由部署侧导入。

同时给 `-filter-sweep` 的报告加了"按类目的替换处数"一行：词表变大后，"命中 N 处"
这个总数无法判断是误伤还是真垃圾，必须能看出命中来自哪一类，才谈得上决定要不要 `apply`。

报告与日志一律只输出**类目名与数字**，不输出词条。
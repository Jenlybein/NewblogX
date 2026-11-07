# 发布闸门、投稿配额与三层机审（2026-11-07 grill 记录）

这份文档是什么：2026-11-07 一场围绕「SEO / 每日投稿上限 / 发布前质量评分 / 审核系统完善 / `skills/` 目录」的 grill 会话的决策记录。它回答"当时定了什么、否掉了什么、代价是什么"。
什么时候翻它：要动发布流程、审核队列、评分口径、投稿配额，或往 `skills/` 加文件之前。

**状态：全部为「提议」，尚无任何代码落地。** 词条级的正名见根 `GLOSSARY.md` 第十二节（同样标注未交付）。

---

## 一、起点：五个前提里四个与代码不符

| 会话中的前提 | 代码里的实情 |
| --- | --- |
| 评分是发布前的一道闸 | 评分**已存在**，但方向相反：它是审核通过之后的**异步副作用**（`internal/ai/app/service.go:331` 注释"scoring + user_id=0 表示异步补评分"），不是闸门 |
| 审核是空白的 | 审核**已完整**：`domain/review`（pending/approved/rejected/canceled）+ 审核队列 + `HandleReviewTask`；`site.article.skip_examining` 默认 `true`，提交即发布 |
| 评分很粗糙 | 六维度 + 20 种文体加权 + 超长文分块滚动状态 + `prompt_version` 落库，`internal/ai/scoring/` 约 800 行 |
| 没有任何配额 | 属实：文章零配额；只有私信 `rate_daily_max` 与 AI `daily_quota` |
| 没有 SEO | `useSeoMeta` 各页都有 title/description；**没有** sitemap、RSS、canonical、JSON-LD，文章 URL 是 `/article/{数字id}` |

另有三处硬事实决定了整场设计：

1. **`ai.enable` 默认 `false`**（`backend/config/site_default_settings.yaml:111`）⇒「AI 不可用」是默认状态，不是边界情况。
2. **`article_ai_score_record_models` 一篇文章一辈子只许一条**（`ai/adapter/mysql/repo.go:124-133` 的 `CreateIfAbsent` 在 `count>0` 时直接返回不写）⇒ 打回后改好重投，分数仍是旧的。
3. **SSR 白名单只有 `/` 与 `/article/**`**（`frontend/shared/readerRenderingBoundary.ts:29`，ADR-0019 决策 1）⇒ 合集/项目/搜索页对爬虫是空壳，给它们做 sitemap 是自相矛盾。

---

## 二、决策全集

### 2.1 流水线与闸门

| # | 决定 |
| --- | --- |
| 1 | 双开关：`quality_gate.enabled`（评分闸门）与 `skip_examining`（人工审核），互相独立，四种组合都合法 |
| 2 | 顺序：提交 → 配额 → 评分闸门 →（人工审核）→ 发布；**评分永远排在人工之前** |
| 3 | **评分必须在 DB 事务外**（LLM 5–30s，不能在 `WithinTx` 里等；与 `create.go:55`"判定放在事务之前"同一原则） |
| 4 | 闸门默认**关**；"开启闸门"这个动作本身要校验 `ai.enable` + 有启用模型，不满足**拒绝保存** |
| 5 | fail-closed：拿不到可用分数 → 拒绝投稿（`article.score_unavailable`） |
| 6 | 有分数就按分数判（哪怕 23 分），走 `article.quality_rejected`——"写得差"与"服务挂了"必须是两个错误码 |
| 7 | 故障性失败**不扣**配额；只有拿到分数才计一次 |
| 8 | 阈值与档界**都自由可配、不做校验**（接受"阈值 50 而 31-59 叫不合格文章"这种配置可保存） |
| 9 | 灰带 = 阈值 ±2：不放行，文案明说"差 N 分才达发布线，按 issues 改一版再投" |
| 10 | 作者侧**照旧显示档名**（接受可能出现"不合格文章 · 55 分 · 已通过"） |

### 2.2 评分

| # | 决定 |
| --- | --- |
| 11 | scoring 单独固定 `temperature=0`，不继承全站 `0.7` |
| 12 | **不做多次采样**；边界抖动靠灰带缓解，不靠加调用次数 |
| 13 | 评分记录改为**单行 Upsert 就地覆盖**（放开 `CreateIfAbsent` 的"存在即拒"） |
| 14 | **不保留分数历史**：只保证"当前分数正确"，放弃"曾 43 分被打回"的可追溯性 |

### 2.3 投稿配额

| # | 决定 |
| --- | --- |
| 15 | 每次**提交动作**计 1 次；被打回也计；评分故障的失败不计 |
| 16 | 默认 15 篇/自然日，单值后台可调，管理员豁免 |
| 17 | Redis 日桶 `article:submit:daily:<yyyymmdd>:<uid>`，TTL 26h（照抄私信 `chat:rate:daily:*`） |
| 18 | 持久化无需改动：`deploy/state/redis/data` 已有卷 + `appendonly yes` 已开 |
| 19 | 加一条告警：当日计数键缺失或计数骤降 |

### 2.4 审核与三层机审

| # | 决定 |
| --- | --- |
| 20 | 三层机审：① 纯代码高危词 ② 评分派生优先级 ③ AI 合规判定 |
| 21 | ①② **不落库**（纯函数，实时算）；③ 落新表 `article_risk_models` |
| 22 | 词表分**阻断级 / 观察级**：只有阻断级能自动拒，观察级只标红 |
| 23 | 三层都能拒：低分（评分）、阻断级词表、AI 高置信违规 |
| 24 | 拒后状态：质量/词表 → **草稿(1)**；AI 合规违规 → **已拒绝(4)** |
| 25 | 驳回原因 = **代码枚举 + 自由文本补充**（不进 `skills/`） |
| 26 | AI 合规判定细节**仅管理员可见**；不给作者看"你命中了哪个词" |
| 27 | 高危词表放 `runtime_config`（运营热改），**规则结构**才进 `skills/` 或代码 |
| 28 | 机审默认必须**关**：`ai.enable=false` 时开 AI 合规判定等于让审核队列全红 |
| 29 | 新增场景 `moderation` = AI 调用的**第 7 个** scene（前 6 个：`scoring` / `metainfo` / `overwrite` / `diagnose` / `search_rewrite` / `search_answer`）；需同步 `GLOSSARY` 的 `CallMeta` 词条与 `docs/architecture/ai-integration.md:201` |

### 2.5 skills/

| # | 决定 |
| --- | --- |
| 30 | 位置仓库根 `skills/`（当前只有本项目消费；**格式**仍遵循开放规范）+ 运行时读盘；容器**挂载**；读不到**启动硬失败**；**不做 Go 兜底文本** |
| 31 | 格式遵循 **Agent Skills 开放规范**、**不自造字段**（产品私有信息只进 `metadata`，且只能是 `map[string]string`）；场景绑定放 `ai.scene_skills` 配置，**不进 skill 本体** |
| 32 | 只两份 skill：`jelly-voice`（捏捏措辞）、`moderation`（AI 审核规则与注意事项）；`prompt_version` = SKILL.md 字节 `sha256` 前 12 位 |

### 2.6 SEO

| # | 决定 |
| --- | --- |
| 33 | 第一期只做技术骨头，**不动 SSR 白名单**（合集/项目不参与排名） |
| 34 | 公开页**继续显示**质量总分（接受闸门上线后"全部及格"的区分度衰减） |

---

## 三、提交路径时序（实现时最容易写错的地方）

```
POST /api/articles {status:2}
  ├─ 事务外: 配额 Lua 原子占位 (INCR, 超限即拒)
  ├─ 事务外: 评分 (LLM, temperature 0)
  │    ├─ 无可用分数 → DECR 回滚配额 → article.score_unavailable
  │    ├─ 分数 < 阈值 或 落在灰带 → 文章以 草稿(1) 落库 → article.quality_rejected
  │    └─ 分数 ≥ 阈值 → 继续
  ├─ 事务内: 建/改文章 → Upsert 评分行 → (可选)建审核任务 → 发 outbox 事件
  └─ 事务内(机审开): ①词表 ②评分派生 → 审核列表标红；③AI 合规异步落 article_risk_models
```

三条必须守住的纪律：

1. **评分不进事务**。现在的 `Create` 全程在一个 `WithinTx` 里，LLM 一次 5–30 秒，占着事务等于钉死连接池。这条要写进 ADR，否则下一个实现者会顺手挪进去。
2. **配额先占位、失败回滚**。先 `INCR` 再评分，失败 `DECR`，否则"评分成功后才发现超限"会白烧一次调用。
3. **前端必须有"评分中"状态**。提交会变慢到 5–30 秒，没有 loading 会被当成卡死然后点第二次 → 双份配额 + 双篇文章。

---

## 四、零 DDL 影响面

| 变更 | 合规性 |
| --- | --- |
| 评分表改 Upsert | ✅ 只改代码，不加列 |
| `site.article.*` 新增子节（`quality_gate` / `moderation` / `daily_submit_limit`） | ✅ `runtime_config` 是 `(name, payload)` 整段 JSON，加字段零 DDL |
| 新表 `article_risk_models` | ⚠️ 需一条 ADR 指名 + `schema.sql` + testkit 三处登记（零 DDL 允许新建表） |
| 给 `article_models` / 审核任务表加风险列 | ❌ **禁止**；风险从 `article_risk_models` join |

`article_risk_models` 建议形状（一篇文章一行，与评分表同构、不存历史）：

```
id, article_id, verdict(clean|suspect|violation), risk_level(0-3),
categories_json, reasons_json, blocked_words_json, prompt_version,
model_name, checked_at, created_at, updated_at, deleted_at
```

---

## 五、`skills/` 与通用 skill 加载器

**格式遵循 [Agent Skills 开放规范](https://agentskills.io/specification)，不自造字段。** 目标是这两份文本与这套加载器脱离本项目也能用。

### 5.1 目录（仓库根 `skills/`）

```
skills/
├── jelly-voice/
│   └── SKILL.md              # 捏捏对话：口吻、称呼、禁用语
└── moderation/
    └── SKILL.md              # AI 审核：判定规则、注意事项、输出要求
```

放在仓库根，不放 `.agents/skills/`：**格式**遵循开放规范（因此内容可移植），但**位置**是我们自己的选择——当前只有本项目消费这两份 skill，没有第三方客户端要读，多一层点号目录只是增加解释成本。

要共享只需改一行：把 `BLOGX_SKILLS_DIRS` 的默认值指到 `.agents/skills` 即可，目录整体搬过去、不涉及任何代码改动。这条决定随时可逆。

### 5.2 SKILL.md（只用规范字段）

```yaml
---
name: jelly-voice
description: 读者站「捏捏」AI 对话的口吻与措辞约束。需要以"捏捏"身份回答读者问题、或调整助手语气与称呼时使用。
metadata:
  myblogx.scene: "search_answer"
  myblogx.revision: "1"
---
```

规范硬性约束（校验器逐条检查，不通过即启动失败）：

| 字段 | 约束 |
| --- | --- |
| `name` | 必填，≤64 字符，小写字母/数字/连字符，不以连字符开头或结尾，**必须等于父目录名** |
| `description` | 必填，1–1024 字符，非空；写"什么时候用它"，触发全靠它 |
| `license` / `compatibility` / `allowed-tools` | 可选；`compatibility` ≤500，`allowed-tools` 是空格分隔的工具名 |
| `metadata` | 可选，**只能是 `map[string]string`**——产品私有信息必须放这里，不能自造顶层字段 |

可选目录：`scripts/`（可执行代码）、`references/`（按需加载的详细文档）、`assets/`（模板/图片）、`evals/`（测试用例）。正文建议 <500 行 / <5000 token，超了拆进 `references/`。

### 5.3 场景绑定不进 skill

skill 必须保持可移植（任何人拿走都能直接用），"哪个场景用哪份 skill"是产品接线，放运行期配置：

```yaml
ai:
  scene_skills:
    search_answer: ["jelly-voice"]
    moderation: ["moderation"]
```

放在 `metadata` 里是**描述**，不是真相；真相在配置里，因为它要能热改（例如临时摘掉某份 skill 而不发版）。

### 5.4 加载器（`backend/internal/platform/skills`）

只依赖标准库 + YAML，**不 import 任何业务包**——将来要给别人用，整包移出去零改动（这也是它不进 `internal/<业务模块>/` 的原因：archcheck 会把业务模块位置当模块，受 R10/R11 约束）。

**位置定为 `internal/platform/skills`（已决，2026-11-07）。** Go 的 `internal` 规则意味着外部模块**不能** import 它，这是有意的取舍："通用"在本项目里指**格式合规 + 实现无业务耦合**，不指"对外发布成可 import 的库"；真要发布时整包移到独立 module 即可，零改动。

| API | 对应规范的哪一步 |
| --- | --- |
| `Load(roots []string) (*Catalog, error)`，扫 `<root>/*/SKILL.md` | Step 1 Discover |
| 只解析 frontmatter，正文延迟到 `Skill.Body()` 才读 | Step 2 Parse + 渐进披露第 1/2 层 |
| `Catalog.List()` 返回 name+description 目录 | Step 3 Disclose |
| `Skill.Body()` / `Resource(rel)` / `ListResources()` | Step 4 Activate |
| `Skill.Hash()` = SKILL.md 字节 sha256 前 12 位 | 版本可追溯 |

三条纪律：

1. **启动只读目录层，正文按需读**。渐进披露在服务端同样成立：20 份 skill 不该在启动时占 20 份正文的内存与日志体积。
2. **硬失败只对自家 skill**。随仓库走的两份校验不过 → 启动失败；将来若允许外部 root，走规范建议的宽松模式（跳过并 warn，不拦启动）。
3. **roots 进进程级配置**：`BLOGX_SKILLS_DIRS`（逗号分隔，默认 `skills`）。"读不到就不启动"影响进程怎么跑，属 `settings.yaml` + env 那一层。改成 `.agents/skills` 只是改这个默认值。

### 5.5 消费点

- `jelly-voice` → `internal/ai/search/ai_search_analyze.go` 的 system prompt（`search_answer`）
- `moderation` → 新场景 `moderation` 的 system prompt

### 5.6 补偿门禁

`scripts/gates/` 新增 `check-skills.mjs`：优先调官方参考校验器 `skills-ref validate`，取不到时用等价规则（frontmatter 必填、`name == 目录名`、`description` ≤1024、正文非空、`metadata` 值全为字符串）。

根目录 md 对 archcheck **完全不可见**（它只扫 `backend/**/*.go`），不补这道门禁，策略文本就是"永远绿的暗礁"。这也是把校验规则写成独立脚本而不是塞进 Go 门禁的原因：它要能在 CI 之外的任何地方跑。

---

## 六、SEO 第一期：白话解释 + 交付清单

### 6.1 先建立两层认知

| 层次 | 目标 | 对应项 |
| --- | --- | --- |
| **第一层：能被找到** | 搜索引擎得**知道**你有这页、**读得懂**内容、**不重复** | sitemap、robots、canonical、SSR |
| **第二层：被点得进** | 搜索结果里那一条**长什么样** | title/description、OG、JSON-LD |

还有一条比这两层都重要的**前提**：**爬虫基本不执行 JavaScript**。本站的渲染边界是"只有 `/` 与 `/article/**` 在服务端渲染 HTML"，其余页面是浏览器里才拼出来的——爬虫打开它们只看到空壳。所以"给合集/项目做 SEO"在扩 SSR 白名单之前是白做（决策 33）。

### 6.2 逐项：这一项是干什么的

**① `sitemap.xml` —— 主动交清单**
一个文件，列出"我站有哪些页面、最后什么时候改的"。爬虫靠顺着链接爬来发现新页面，新站与深页可能几周都爬不到；sitemap 是主动上报（`<url><loc>…</loc><lastmod>…</lastmod></url>`）。不做：新文章收录延迟以周计。

**② `robots.txt` —— 门口指示牌**
站根的一个纯文本，两件事：① 哪些目录别来爬（`Disallow`）；② sitemap 在哪（`Sitemap:` 行）。不做：爬虫会在创作区、消息中心里乱爬，浪费"爬取预算"（一个站一天能被爬的次数有限）。注意它只是**礼貌请求**，不是访问控制——真正的私密保护还得靠登录态。现状：该文件只有两行、全部放行、没有 sitemap 行。

**③ `noindex` —— 可以看，但别收录**
页面级标签 `<meta name="robots" content="noindex">`。与 `robots.txt` 的 `Disallow` 分工不同：`Disallow` 是"别来爬"，`noindex` 是"可以爬但别放进结果"。**私密页必须用 `noindex` 而不是 `Disallow`**——被禁止爬取的页面，爬虫看不到里面的 `noindex`；若别处有链接指向它，它仍可能以"裸 URL 无标题"的形式出现在结果里。`/search` 尤其要收：`/search?q=任意词` 有无限多个变体，全被收录会把站点稀释成几万条空结果页。

**④ `canonical` —— 给搜索引擎去重，不改页面行为**
写在 `<head>` 的一行 `<link rel="canonical" href="…/article/123">`，只说一句话："这一页的正式地址是这个。" 同一篇文章可能有五个变体（`?from=collection:5`、`?from=project:2`、`?utm_source=…`、结尾带不带 `/`），搜索引擎会当成五篇重复内容、权重摊薄。
**它不改地址栏、不改路由、不改页面任何行为。** 从合集会话点进文章，URL 仍是 `?from=collection:5`，前端 `useRoute().query.from` 照常读到，左侧抽屉目录照旧显示"当前文章 + 上一篇/下一篇"——**代码零改动**。它只是在 head 里额外写一句"正式地址不带这个参数"。

**⑤ OG 补全 —— 分享卡片**
链接被发到微信 / X / Discord / Slack 时，这些平台读 `og:title` / `og:description` / `og:image` 生成预览卡。现状：这三样**都已有**。缺 `og:type=article`（声明"是文章"而非"是网站"）、`og:url`（规范地址）、`article:published_time` / `article:author` / `article:tag`。不补：卡片照样出，但被当成普通网页、且平台不知道发布时间。

**⑥ JSON-LD —— 让结果里多几行字**
埋在页面里、人看不见、专给机器读的 JSON。`Article`（标题/时间/作者/封面）、`BreadcrumbList`（面包屑，结果里显示成 `site.com › 分类 › 标题`）、`WebSite` + `SearchAction`（声明站内有搜索）。Google 用它生成**富摘要**——普通结果是一条蓝链，富摘要会多显示作者、日期、路径，点击率明显更高。不做：还是普通蓝链。

### 6.3 交付清单（6 项）

| # | 交付物 | 落点 |
| --- | --- | --- |
| 1 | `sitemap.xml`（只含 `/` 与 `/article/{id}`；仅已发布 + 可见 + 过作者可见性开关） | 后端新端点 |
| 2 | `robots.txt` 加 `Sitemap:` 行；`Disallow` 私密路由 | `frontend/public/robots.txt` |
| 3 | `noindex`：`/studio/**`、`/history`、`/messages/**`、`/search` | 各页 `useSeoMeta({robots})` |
| 4 | canonical（文章页剥掉 `?from=collection:` 之类来路参数） | `article/[id].vue` |
| 5 | OG 补全：`og:type=article`、`og:url`、`article:published_time/author/tag` | 同上 |
| 6 | JSON-LD：`Article`、`BreadcrumbList`、`WebSite`+`SearchAction` | `article/[id].vue`、`index.vue` |

六项的共同点：**全是"只增不改"的纯收益**——不动 URL、不动页面结构、不动数据模型、不影响任何现有功能。风险接近零。

### 6.4 明确不做的两项（都是有意克制，不是遗漏）

- **合集/项目不做 sitemap**：它们是 client-only 页面，爬虫拿到空壳；列进 sitemap 只会让站长后台持续报"已提交，未收录"。
- **RSS/Atom 砍掉**：它是"读者订阅功能"，不是 SEO 功能。对收录的加速是间接且微弱的；没人用阅读器订阅时收益接近零。等真有人说"想订阅"再加。

### 6.5 实施记录（2026-11-08，与上表的四处偏差都有理由）

| 项 | 落点/形态变了什么 | 为什么 |
| --- | --- | --- |
| 2 | `robots.txt` 改由 `server/routes/robots.txt.get.ts` 按运行时 `NUXT_PUBLIC_SITE_URL` 现拼，删掉 `public/robots.txt` | `Sitemap:` 行必须是绝对地址，而 local / test / production 是三个不同域名；静态文件只能写死一个，测试环境会把爬虫指向生产站的 sitemap |
| 2 | 只 `Disallow: /api/`，**不** Disallow 私密页面 | 与 §6.2③ 同一条理由：被 Disallow 的页面爬虫进不去，读不到页面里的 `noindex`。私密页面一律走页面级标签 |
| 5 | `article:published_time` 取 `created_at` | 库里没有 `published_at` 列（本票零 DDL），记录时间就是读者能看到的那一个"发布"时刻 |
| 6 | `BreadcrumbList` 只有两级（首页 → 标题），分类只进 `Article.articleSection` | 站点没有分类页（无 `/category/*` 路由）。面包屑的每一层都要有可访问的 `item`，为凑三层编一条分类 URL 等于把爬虫送去一页 404 |

另外两条实施口径：`canonical` 的域名取运行时 `siteUrl` 而非请求头（同一篇文章的多个来路变体必须输出同一条 canonical，否则"去重"本身会失效）；`noindex` 的指令值收敛到 `useNoIndexPage()` 一处，`test/gates/seo-foundation.test.ts` 按路由清单逐页检查有没有漏。


---

## 七、被否决的方案与理由

| 被否方案 | 否掉的理由 |
| --- | --- |
| 评分失败就放行（fail-open） | 默认配置下闸门从未生效；主动探测到"AI 一挂就全放行"会被打穿 |
| AI 未配置 = 闸门不生效 | 与 fail-closed 语义自相矛盾 |
| 阈值必须锚定档界（防矛盾配置） | 会话选择保留完全自由，改从"不展示档名"处消解（该条最终也被否，见决策 10） |
| 分数历史落 `article_review_log_models` | 需要 ADR + 三处登记；会话选择接受无历史 |
| 配额账本 = 永远建审核任务、数任务行 | 会话选择 Redis 日桶；账本需要"每次提交都有时间线"这一前提，当前默认配置下任务根本不建 |
| 新建 `article_submit_log_models` | 同一个问题的第三条路，成本最高 |
| 机审另做独立合规场景、与评分派生二选一 | 会话选择两者都要，并额外加纯代码高危词扫描 |
| 自动拒交给全量词表 | 误杀率高（"六四"命中"六十四位"），收窄为只有"阻断级"能拒 |
| 驳回话术库进 `skills/` | 会话判断：驳回原因内置成枚举即可，`skills/` 只放捏捏措辞与 AI 审核规则 |
| 扩 SSR 白名单让合集/项目可被收录 | 要推翻 ADR-0019 决策 1 的一部分并重估缓存与性能，第一期不做 |
| 文章 slug 化（`/article/{id}` → `/article/{slug}`） | 破坏性变更：外链、ES 投影、`?from=` 来路参数都按数字 id 走，须单独评估 301 与观测 |

---

## 八、代价与已知风险

1. **误杀无法复盘**：无分数历史（决策 14）+ 三层都能拒（决策 23），只有"阻断级词表"这一层被收窄；评分误判仍是不可申诉的黑箱。
2. **可保存自相矛盾的配置**（决策 8）：作者可能看到"不合格文章 · 已通过"。
3. **策略不在镜像里**（决策 29）：服务器上那份 md 是"线上真相"，与 Git 可能分叉；hash 版本让分叉可见，不阻止分叉。
4. **Redis 计数归零**：AOF + 卷 + 告警都做了仍非绝对（`flushdb`、换机器）。
5. **提交变慢 5–30 秒**：需要前端"评分中"状态与幂等保护，否则双击会双扣配额。
6. **不采样**：边界仍有 2–3 分抖动；灰带降低"随机打回"概率，不消除。
7. **公开分区分度衰减**（决策 34）：闸门上线后公开分数线全部 ≥ 阈值，这个数字从"质量信号"退化为"及格证明"。
8. **机审默认关**：三层机审的价值依赖站点先配好 AI；未配 AI 的站点只能用第 ①② 层。

---

## 九、与既有约定的冲突（显式记录，不静默覆盖）

1. **`GLOSSARY.md` 的 `CallMeta（scene）` 词条写"固定 6 个场景"**。新增 `moderation` 后是 7 个，落地时必须同步该词条与 `docs/architecture/ai-integration.md:201`。
2. **`docs/README.md:10` 说"28 条 ADR"，实际 29 条**（ADR-0029 已存在）。顺手修正。
3. **`docs/agents/domain.md:14` 约定"文件不存在就静默跳过，不要预先建议创建它"**。本次明确要求新建根 `skills/` 目录，属于对该约定的有意偏离，理由是：策略文本必须有单一真相、必须能随仓库评审，而 `docs/` 下放运行时消费的文本会让"文档"与"线上行为"混在一起。
4. **`skills/` 与 `AGENTS.md:9` 的 `## Agent skills` 用同一套格式**（都遵循 Agent Skills 开放规范），差别只在消费者：`skills/` 里的两份被**产品运行时**读取并注入模型调用，`docs/agents/` 那三个文件是**编码 agent** 读本仓配置的入口。不算撞名，但 `skills/README.md` 必须写清"这份 skill 谁读、在什么场景下生效"，否则两边都会以为对方在用。
5. **目录位置在"根 `skills/`"与"`.agents/skills/`"之间来回过一次，最终定为根 `skills/`**。理由：通用的是**格式与加载器**，不是位置；当前没有任何第三方客户端要读这两份 skill，而多一层点号目录只增加解释成本。代价是可移植性少了一点——但那是改一行 `BLOGX_SKILLS_DIRS` 默认值就能补上的（把目录整体搬到 `.agents/skills/`，代码零改动），所以这条决定随时可逆、不必现在付成本。

---

## 十、待落地

**已合成一份 spec 发布到 issue tracker**：[#302 发布治理一期](https://github.com/Jenlybein/NewblogX/issues/302)（标签 `ready-for-agent`，2026-11-07），并已按 tracer bullet 拆成 12 张子 issue（#303–#314，均为 #302 的 sub-issue）。下面两条保留为实施建议，编号不对应具体票。

**ADR（4 条，各自可独立推翻）**

**编号按落地顺序分配，不是按这份清单的原始设想**——ADR 规则要求严格递增、不跳号，所以先落地的先拿号（下表是 2026-11-08 的实际分配）。

| 编号 | 标题 | 状态 |
| --- | --- | --- |
| 0030 | `skills/` 运行时策略文本（读盘、硬失败、hash 版本） | 已交付（#309） |
| 0031 | 投稿配额（Redis 日桶、按提交计次、故障不计） | 已交付（#304） |
| 0032 | 发布前的质量闸门（评分即闸门、失败关闭、事务外评分、阈值自由配置） | 已交付（#306） |
| 0033 | 模型合规判定与 `article_risk_models` | #312（进行中） |

**ticket 建议拆分（10 条）**：配额 · 评分闸门 · 评分表 Upsert · 词表与派生标签 · AI 合规 + 新表 + ADR · 驳回枚举与 reason 透传 · 审核 UI（正文预览/批量/排序/红点） · skills 加载器 + 门禁 + 两份 SKILL.md · 捏捏注入措辞 · SEO 技术骨头。

其中**"驳回理由透传"是一个真 bug**，与本次设计无关、可独立修：`backend/app/outbox_wiring.go:193` 把拒绝通知的 reason 写成空串，作者收到"被拒"但没有任何原因。审核任务里 `Reason` 字段是有的，只是没被传出去。

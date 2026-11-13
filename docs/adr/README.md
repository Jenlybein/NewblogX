# 架构决策记录（ADR）

本目录是决策入口，**一条决策一个文件，编号连续 0001–0030**。每条回答「为什么这样定、否决了什么、代价是什么、当前生效约束」。改某条规则或某个取舍前，先读对应的 ADR；后端架构的硬约束总表是 [`../architecture/overview.md`](../architecture/overview.md) §六，ADR 记录的是每条约束的**出处与理由**；前端呈现层的约束写在各自 ADR 的「当前生效约束」里。

> **整理口径（2026-11-13）**：同一主题前后冲突只保留最新一条。被取代的决策与修订型 ADR 已整体并入保留下来的条目，旧文件物理删除；正文一律直接写现行结论，不留「后续修订」「实施记录」「差异表」尾段，也不留 `已取代` 状态。编号因此压缩重排为 0001–0029（此后新增正常续号），**旧编号与本次之前的引用一律失效**；历史版本查 git。整理时笔记与归档目录一并删除，过程记录、实测数字与 v1 归档只存在于 git 历史。

## 索引

| 编号 | 标题 | 一句话结论 | 关联文档 |
| --- | --- | --- | --- |
| [0001](./0001-capability-interfaces-over-deps.md) | 用能力接口取代 `Deps` 结构体装箱 | 窄接口注入，`Deps` 全部删除；R2 只管跨模块依赖打包，模块内部私有接缝不算端口，端口由使用方定义 | [架构总纲](../architecture/overview.md) · [治理 §二](../architecture/governance.md) |
| [0002](./0002-vertical-modules-and-composition-root.md) | 垂直模块切分与合成根 | 模块内 `domain`/`app`/`adapter`，合成根唯一连线；合成根构造跨模块具体适配器，模块自己的 adapter 由模块装配入口构造 | [架构总纲](../architecture/overview.md) |
| [0003](./0003-generated-contract-single-source.md) | 前后端契约以代码生成为单一来源 | Go struct → openapi → TS 生成 | [契约](../architecture/contracts.md) |
| [0004](./0004-archcheck-ast-rules.md) | 架构守卫从符号匹配改为可执行 AST 规则 | `tools/archcheck`，AST + 类型信息；R6 单文件行数上限 2200 行 | [治理 §二](../architecture/governance.md) |
| [0005](./0005-bounded-async-log-pipeline.md) | 日志改为有界异步管道 + 类型化事件 | 有界队列 + 批量直写 ClickHouse + WAL | [治理 §四](../architecture/governance.md) |
| [0006](./0006-test-pyramid-testkit-stateful-fakes.md) | 测试体系改为金字塔 + testkit + 有状态 fake | 分层阈值，禁用 SQLite 冒充 MySQL | [治理 §三](../architecture/governance.md) |
| [0007](./0007-id-string-type-as-truth.md) | ID 类型统一为 string 底层（类型即真相） | `ctype.ID` 底层改 string，R12 守卫 | [契约 §7](../architecture/contracts.md) · [治理 §2.2](../architecture/governance.md) |
| [0008](./0008-dead-letter-replay.md) | 死信重投（DLQ Replay）闭环 | 人工触发、幂等执行器、`unsupported ≠ failed` | [契约](../architecture/contracts.md) |
| [0009](./0009-es-sync-article-projection-only.md) | 删除「通用规则」同步路径，ES 同步只留文章搜索投影 | 软删投影三层修复（删除语义 / 发件箱 / 读侧过滤）与死信重投共同保证索引不含已删文档 | [ADR-0008](./0008-dead-letter-replay.md) |
| [0010](./0010-jelly-replaces-traditional-ai-entry.md) | 捏捏（Jelly）常驻角色取代传统 AI 入口 | 角色常驻 + 面板同源生长，非遮罩 | [GLOSSARY · Jelly](../../GLOSSARY.md) |
| [0011](./0011-neutral-glass-visual-system.md) | 中性玻璃视觉系统与首页首屏例外 | 克莱因蓝退役；中性石墨阶 + 功能蓝；圆角阶梯；液态玻璃只给导航/控制层；动效档位 C；中文字体系统栈；正文 markdown 区由 `--md-*` 独立治理；首屏满幅色带 + 超大 h1 + 指针墨流、轮播并入首屏 | [GLOSSARY · 语义色 token](../../GLOSSARY.md) · 无 `architecture/` 专篇 |
| [0012](./0012-email-code-refuse-instead-of-silent-skip.md) | 邮箱验证码「发不出就明说」 | 放弃静默防枚举：状态与用途不匹配时返回明确错误码；前端只写用途名，数字只在一处映射 | 根 [README](../../README.md) |
| [0013](./0013-banner-framing-as-parameters.md) | 封面上传前定形 | 按 16:9 裁切 + 降采样后只上传新文件，原图不入库；比例不合规时弹裁剪窗由作者自选区域；不存任何取景/crop 参数；读者端固定居中；零依赖手写 canvas | [GLOSSARY · 头像定形](../../GLOSSARY.md) |
| [0014](./0014-deco-segment-and-cursor-charm.md) | 装饰配置段与光标挂件 | 装饰只放 `deco` 段 / `deco.charm` 子块；挂件只有一条 `requestAnimationFrame` + 后台 `fps_cap`；指针端带阻尼跟随 + 可配偏移；12 个数值参数由一张生成式参数表派生四端并进契约 | [配置分层](../architecture/configuration.md) · [GLOSSARY · 光标挂件](../../GLOSSARY.md) |
| [0015](./0015-publish-display-switches-and-hot-top-operator-slot.md) | 站点发布/展示开关与热门置顶运营位 | 删掉从未生效的站点模式；发布/草稿/展示拆成三个布尔；热门置顶改独立表 + 后台运营页；删 ES 里无人消费的 `admin_top`/`top` | [GLOSSARY · 热门置顶](../../GLOSSARY.md) · [配置分层](../architecture/configuration.md) |
| [0016](./0016-reader-rendering-and-cache-strategy.md) | 读者站渲染、缓存、用户态与计数真相 | SSR 白名单只留 `/` 与 `/article/**`，其余 SPA；中文字体系统字体栈；客户端 30s SWR + 匿名 SSR 30s 缓存；私有态独立取数、手工 refresh 必真发请求；写响应回填权威计数收进阅读页取数模块；点赞/收藏数是派生值，并账按 `COUNT(关系行)` 覆盖整列、读路径只认 `app.CountLedger` | [ADR-0003](./0003-generated-contract-single-source.md) · [GLOSSARY · 计数账本](../../GLOSSARY.md) |
| [0017](./0017-inline-svg-in-article-body.md) | 文章正文放行内联 SVG（两层白名单） | 后端消毒策略与前端 markdown-it 同时放开同一份白名单，禁止 `script` / `on*` / `foreignObject` | [#134](https://github.com/Jenlybein/NewblogX/issues/134) |
| [0018](./0018-chat-dual-channel-http-write-ws-notify.md) | 私信：双通道 + 发送资格 + 消息中心 | `POST /chat/messages` 事务内落库为唯一发送通道，WS 只做通知；发送资格一次判完、一律失败关闭；列表按 `send_time` 升序 + 复合游标、已读改整会话、清除水位用时间口径；`chat_block_models` 是唯一经 ADR 指名新增的私信表 | [GLOSSARY · 私信与会话](../../GLOSSARY.md) · [TODO](../TODO.md) |
| [0019](./0019-named-outline-trees-as-the-only-shape.md) | 合集/项目：目录树与分组编排 | 两张具名树表 + 两个具名端点是对外唯一形状；归属树读模型收口一处；空文件夹允许、拖拽用 sortablejs、编排入口是编辑弹窗抽屉；分组树是「实体之间」的唯一顺序真相，4 张新表、整树换序幂等整份覆盖、`compose` 一个事务写字段与整棵树 | [GLOSSARY · 合集目录树 / 项目目录树](../../GLOSSARY.md) · [契约](../architecture/contracts.md) |
| [0020](./0020-config-section-registry-and-named-sections.md) | 配置段注册表与十段具名化 | 十段（含 `sms`）运行时配置收成 `sectionRegistry` 唯一事实，漏登记在写库前失败；参数化 `/site/{name}` 与自由键值块退场，每段一对具名形状、两端类型从生成的契约派生；`sms` 机密经 `sealForStore` 加密落库 | [配置分层](../architecture/configuration.md) · [契约](../architecture/contracts.md) |
| [0021](./0021-runtime-policy-skills.md) | 运行时策略 skill 放仓库根 `skills/`，读不到就不启动 | 位置是我们自己的选择（启动读盘）；加载器只读目录层、正文按需读、版本号取 SKILL.md 字节 sha256 前 12 位；缺失或不合规**进程起不来**；`skills/**` 对 archcheck 不可见，另配 `check-skills.mjs` 门禁 | [GLOSSARY · 运行时策略 skill](../../GLOSSARY.md) · [Agent Skills 规范](https://agentskills.io/specification) |
| [0022](./0022-submit-quota.md) | 投稿配额按自然日计数、按提交动作计量 | Redis 日桶 `article:submit:daily:<yyyymmdd>:<uid>` + 26 小时 TTL；预占/释放两段式 Lua 调用；上限 `site.article.submit_daily_max` 默认 15、管理员固定豁免；被打回也计、自动重新送审也计 | [GLOSSARY · 投稿配额](../../GLOSSARY.md) · [#304](https://github.com/Jenlybein/NewblogX/issues/304) |
| [0023](./0023-quality-gate.md) | 发布前的质量闸门——木桶判定、事务外评分、策略文本 | 闸门与人工审核是两道独立开关；顺序「配额 → 评分 → 判定 → 落库 →（可选）建审核任务」；评分在事务外、温度 0、结果必须落库；放行 = 总分 ≥ `pass_score + gray_band_width` **且**六项每项 ≥ `pass_score`，灰带只加总分；`gap` 只表总分差额、`weak_dimensions` 只列未过线维度；评分宽严从 `skills/scoring/SKILL.md` 注入，代码按字数/段落/纯礼节形态硬封顶 | [GLOSSARY · 发布质量闸门 / 木桶判定](../../GLOSSARY.md) · [#306](https://github.com/Jenlybein/NewblogX/issues/306) |
| [0024](./0024-risk-models.md) | 第③层模型合规判定与 `article_risk_models` | 合规判定是新表的唯一用途（一篇文章一行、覆盖写）；走异步发件箱；结论三档 clean/suspect/violation、风险级别 0–3 独立；规则来自 `skills/moderation`；结论与依据只给管理员 | [GLOSSARY · 三层机审](../../GLOSSARY.md) · [#312](https://github.com/Jenlybein/NewblogX/issues/312) |
| [0025](./0025-risk-gated-publish.md) | 按风险分流的发布口径——标红直接拒、干净文章自动发 | 提交时同步跑①②层 `moderation.Evaluate`，`medium` 及以上直接 `REJECTED`；`none`/`low` 落 `Published + pending_review`，模型判 `clean` 才转 `visible`；模型自动拒门槛固定 `violation + risk_level ≥ 2`；`pending_review` 只能由系统写入且必须转终态；`skip_examining` 开启时全部直接真发布；作者只见枚举文案、不二次扣配额、草稿不被自动拒 | [GLOSSARY · 待可见 / 三层机审](../../GLOSSARY.md) |
| [0026](./0026-taxonomy-delete-semantics.md) | 标签与分类的删除语义：软删行占住唯一键、恢复与停用 | 删过的名字可重用靠创建时先恢复同名软删行（保留原 ID）；标签以停用为主、删除只对零引用开放；分类删除在同一事务内先解除文章归属再软删，置空经 CDC 自动重算搜索投影 | [GLOSSARY · 标签 / 分类](../../GLOSSARY.md) · [#319](https://github.com/Jenlybein/NewblogX/issues/319) |
| [0027](./0027-relation-views-and-remove-fan.md) | 关注关系收成三视图与单向移除粉丝 | 读端点合成 `GET /relations?type=follow\|fans\|mutual`，互关在 SQL 里先筛后分页；条目形状合成 `RelationItem`；新增 `/relations/counts` 与 `/relations/fans/{id}`（不通知、不阻止再次关注）；零 DDL | [GLOSSARY · 关系视图 / 移除粉丝](../../GLOSSARY.md) · [ADR-0003](./0003-generated-contract-single-source.md) |
| [0028](./0028-ai-daily-quota.md) | AI 每日额度按场景、按人、按自然日；额度耗尽次日自动重审 | 七个场景具名上限（`ai.daily_limits`，0=不限），收费点在 `ai.Chat`/`ai.ChatStream` 出口；Redis 日桶 + Lua 预占、失败关闭；合规判定被额度挡住时文章回「审核中」但不建人工任务，事件借 `available_at` 次日重投 | [GLOSSARY · AI 每日额度](../../GLOSSARY.md) · [AI 接入](../architecture/ai-integration.md) |
| [0029](./0029-admin-nav-three-levels-and-site-split.md) | 后台菜单引入三级；站点拆四页、媒体运营拆两页 | 侧栏支持「分组 → 分组 → 页面」，分组 key 改层级链并补 depth=2 缩进；站点四页共用 `site` 段端点，保存走「取最新整段 → 只覆盖本页块 → 写回」；媒体两页各自分页，`/media` 留跳转页；零后端变更 | [GLOSSARY · 后台三级菜单](../../GLOSSARY.md) · [ADR-0020](./0020-config-section-registry-and-named-sections.md) |
| [0030](./0030-async-quality-gate.md) | 发布质量闸门改为异步判定 | 提交立即返回；发件箱按「评分 → 质量判定 → 合规判定」串行执行；未达标退回草稿 + 站内通知（带分数与放行线）；拿不到分数失败关闭 | [ADR-0023](./0023-quality-gate.md) · [ADR-0025](./0025-risk-gated-publish.md) · [GLOSSARY · 发布质量闸门](../../GLOSSARY.md) |

## 来源与拆分说明

- 0001–0009 是 v1 后端架构决策，原本按「一条决策一个文件」从 v1 归档合集拆出；归档原文已删除，需要对照历史时查 git。前 9 条在 v1 判定下全部 `已交付`。
- 0010 起是 v2 阶段的呈现层、前端行为、装饰层、渲染与性能策略、私信、目录树、配置与发布治理决策；呈现层决策没有对应的 `architecture/` 专篇，链接落在 `GLOSSARY.md` 与代码路径。
- 每条 ADR 记录的是**判断本身**；现状若有偏离，以 `docs/architecture/` 与代码为准，并按下面的规则更新 ADR 正文，而不是改写历史。

## 新增 / 修改 ADR 的规则

1. **编号**：四位补零、连续递增到当前最大值 +1；整理时允许整体重排压缩，但重排后必须同步全仓引用。
2. **命名**：`NNNN-短横线小写英文.md`，名字写「决策」而不是「问题」（例：`0007-id-string-type-as-truth.md`）。
3. **状态取值**只有四个：`提议` / `已接受` / `已交付` / `已否决`。一份 ADR 同时只有一个状态。**前后冲突只保留最新一条**：直接按现行结论重写该 ADR 正文，或把旧条目物理删除、把仍生效的条款并进保留下来的 ADR；不保留 `已取代` 占位文件，不追加「后续修订」尾段。需要历史理由时查 git。
4. **正文结构**固定，顺序不变：
   1. `# ADR-NNNN：<标题>`
   2. `- 状态：` / `- 日期：` / `- 来源：`
   3. `## 背景与问题`
   4. `## 决策`
   5. `## 理由与已否决方案`
   6. `## 代价 / 不做什么`
   7. `## 当前生效约束`（不得为空）
   8. `## 相关代码与文档`（链接到对应的 `../architecture/` 文档与具体代码路径）
5. **登记**：新增后在本索引表补一行，并在「关联文档」列写清它落在 `docs/architecture/` 的哪一篇。呈现层等没有 `architecture/` 专篇的决策，链接到根 `GLOSSARY.md`，**不要**为它另造一篇 architecture 文档——概念只在 GLOSSARY 定义一次。

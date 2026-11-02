# 架构决策记录（ADR）

本目录是 v2 阶段的决策入口，**一条决策一个文件**：0001–0009 是 v1 后端架构决策，0010–0019 是 v2 阶段的呈现层、前端行为、装饰层与站点策略决策，0020 是 v2 阶段对后端硬约束（零 DDL）的一次例外授权与通用取景表立项。需要改某条规则或某个取舍前，先读这里对应的 ADR；后端架构的硬约束总表是 `docs/architecture/overview.md` §六，ADR 记录的是每条约束的**出处与理由**；前端呈现层的约束写在各自 ADR 的「当前生效约束」里。

## 索引

| 编号 | 标题 | 一句话结论 | 关联文档 |
| --- | --- | --- | --- |
| [0001](./0001-capability-interfaces-over-deps.md) | 用能力接口取代 `Deps` 结构体装箱 | 窄接口注入，`Deps` 全部删除 | [架构总纲](../architecture/overview.md) · [治理 §二](../architecture/governance.md) |
| [0002](./0002-vertical-modules-and-composition-root.md) | 垂直模块切分与合成根 | 模块内 `domain`/`app`/`adapter`，合成根唯一连线 | [架构总纲](../architecture/overview.md) |
| [0003](./0003-generated-contract-single-source.md) | 前后端契约以代码生成为单一来源 | Go struct → openapi → TS 生成 | [契约](../architecture/contracts.md) |
| [0004](./0004-archcheck-ast-rules.md) | 架构守卫从符号匹配改为可执行 AST 规则 | `tools/archcheck`，AST + 类型信息 | [治理 §二](../architecture/governance.md) |
| [0005](./0005-bounded-async-log-pipeline.md) | 日志改为有界异步管道 + 类型化事件 | 有界队列 + 批量直写 ClickHouse + WAL | [治理 §四](../architecture/governance.md) |
| [0006](./0006-test-pyramid-testkit-stateful-fakes.md) | 测试体系改为金字塔 + testkit + 有状态 fake | 分层阈值，禁用 SQLite 冒充 MySQL | [治理 §三](../architecture/governance.md) |
| [0007](./0007-id-string-type-as-truth.md) | ID 类型统一为 string 底层（类型即真相） | `ctype.ID` 底层改 string，R12 守卫 | [契约 §7](../architecture/contracts.md) · [治理 §2.2](../architecture/governance.md) |
| [0008](./0008-dead-letter-replay.md) | 死信重投（DLQ Replay）闭环 | 人工触发、幂等执行器、`unsupported ≠ failed` | [契约](../architecture/contracts.md) · 原文仅收录于归档 |
| [0009](./0009-es-sync-article-projection-only.md) | 删除"通用规则"同步路径 | ES 同步只留文章搜索投影 | [ADR-0008](./0008-dead-letter-replay.md) · 原文仅收录于归档 |
| [0010](./0010-visual-system-and-motion-budget.md) | 双前端视觉系统与动效预算 | 五色相锁死 + 两层语义 token；圆角 0；动效 120 / 200 / 320ms —— **已被 [0012](./0012-neutral-glass-visual-system.md) 取代** | [GLOSSARY · 语义色 token](../../GLOSSARY.md) · 无 `architecture/` 专篇 |
| [0011](./0011-jelly-replaces-traditional-ai-entry.md) | 捏捏（Jelly）常驻角色取代传统 AI 入口 | 角色常驻 + 面板同源生长，非遮罩 | [ADR-0010](./0010-visual-system-and-motion-budget.md) · [GLOSSARY · Jelly](../../GLOSSARY.md) |
| [0012](./0012-neutral-glass-visual-system.md) | 中性玻璃视觉系统 | 克莱因蓝退役为主色；中性石墨阶 + 功能蓝；圆角阶梯；液态玻璃仅用于导航/控制层；动效档位 C + 吸顶玻璃顶栏 | [GLOSSARY · 语义色 token](../../GLOSSARY.md) · [Apple 液态玻璃解构](../notes/2026-10-30-apple-liquid-glass-deconstruction.md) · [改造方案](../notes/2026-10-31-neutral-glass-redesign-plan.md) |
| [0013](./0013-editorial-hero-cover.md) | 首屏编辑部式封面 | 满幅色带 + 超大 h1 + 网格刻度，仅首页首屏例外；删 tagline 与三格事实，轮播并入首屏 | [ADR-0012](./0012-neutral-glass-visual-system.md) · [改造方案](../notes/2026-10-31-neutral-glass-redesign-plan.md) · [GLOSSARY · 首屏封面](../../GLOSSARY.md) |
| [0014](./0014-email-code-refuse-instead-of-silent-skip.md) | 邮箱验证码"发不出就明说" | 放弃静默防枚举：状态与用途不匹配时返回明确错误码；前端只写用途名，数字只在一处映射 | 本 ADR · [README §2.5](../../README.md) |
| [0015](./0015-banner-framing-as-parameters.md) | 轮播取景存参数、由 CSS 渲染 | 焦点 + 缩放三个数存参数，不存裁好的图；服务端不裁切，渲染在 CSS，面板比例固定 | [ADR-0013](./0013-editorial-hero-cover.md) · [GLOSSARY · 取景](../../GLOSSARY.md) · [spec #44](https://github.com/Jenlybein/NewblogX/issues/44) |
| [0016](./0016-deco-segment-and-cursor-charm.md) | 装饰配置段与光标挂件 | 第 8 段 `deco` + `deco.charm` 子块；参数后台可调、钳位不报错；Verlet 链 + 松弛驱动弧垂；SVG 覆盖层；z-index 9999 但 `pointer-events: none`；轻晃可回落零帧（帧率部分已由 [0018](./0018-charm-frame-cap-and-pointer-offset.md) 取代）；带形多边形收尾 | [ADR-0012](./0012-neutral-glass-visual-system.md) · [配置](../architecture/configuration.md) · [GLOSSARY · 光标挂件](../../GLOSSARY.md) |
| [0017](./0017-publish-display-switches-and-hot-top-operator-slot.md) | 站点发布/展示开关与热门置顶运营位 | 删掉从未生效的站点模式；发布/草稿/展示拆成三个布尔（默认开）；热门置顶改为独立表 + 后台运营页；删 ES 里无人消费的 `admin_top`/`top` | [GLOSSARY · 热门置顶](../../GLOSSARY.md) · [配置分层](../architecture/configuration.md) · [#54](https://github.com/Jenlybein/NewblogX/issues/54) |
| [0018](./0018-charm-frame-cap-and-pointer-offset.md) | 光标挂件的帧率上限交给后台 | 取消自适应降速（原 20fps 闸门），只剩一条 `requestAnimationFrame` + 后台 `fps_cap`（默认 60、30–160）；`pointermove` 不再自己发布；绳头加可配偏移（默认右下 3px，已配置化为 `pointer_offset_x` / `pointer_offset_y`，每轴 -24–24 且 `0` 合法）；新增第四条钳位规则 `normalizeSignedRange`；跨端门禁改为四源键集合相等 + 锚点交付状态位 | [ADR-0016](./0016-deco-segment-and-cursor-charm.md) · [配置分层](../architecture/configuration.md) · [GLOSSARY · 光标挂件](../../GLOSSARY.md) |
| [0019](./0019-reader-rendering-and-cache-strategy.md) | 读者站渲染与缓存策略 | SSR 白名单只留 `/` 与 `/article/**`（分享卡片与收录），其余读者站页面 SPA；中文字体改系统字体栈（首屏字体 < 100 KB，取代 ADR-0012 的中文字体条款）；取数并行 + 骨架优先；客户端 30s SWR + 匿名 SSR 30s 缓存（SSR 只输出匿名内容） | [ADR-0012](./0012-neutral-glass-visual-system.md) · [性能实测](../notes/2026-11-01-reader-site-performance-probe.md) · [#79](https://github.com/Jenlybein/NewblogX/issues/79) |
| [0020](./0020-zero-ddl-exception-and-generic-framing-table.md) | 零 DDL 例外授权与通用取景表 | 例外只到"新建表"：口径是零列变更 + 允许新增表；新建通用取景表 `picture_crop_models`（`target_type` / `target_id`），旧的轮播专属取景表 `banner_crop_models` 并入；契约字段 `crop` 与错误码 `banner.crop_invalid` 一律不动 | [GLOSSARY · 零 DDL / 取景](../../GLOSSARY.md) · [架构总纲 §六 A1](../architecture/overview.md) · [ADR-0015](./0015-banner-framing-as-parameters.md) |

## 来源与拆分说明

- **ADR-0001–0009** 原本是 v1 时期 `docs/architecture/adr/` 下的独立文件；v1 收尾时被合并进 [04 · ADR 合集（v1 归档）](../archive/v1/04-adr-collection.md)。那次合并的口径是：**只保留每条 ADR 的决策骨架 + 关键参数 + 当前生效约束**，被否决方案的长篇论证压缩为一句理由，与其他 v1 文档重复的论述删除。
- **ADR-0001–0009** 是**按"一条决策一个文件"从该合集重新拆出的**，正文以归档合集为准（未新增事实、未删"当前生效约束"）；归档合集仍在 `docs/archive/v1/` 保留原样，作为这些文件的出处。**ADR-0010 / 0011** 是 v2 前端视觉重构新增的决策（2026-10-30），直接写在本目录，不来自 v1 归档，也**没有对应的 `docs/architecture/` 专篇**——它们的正文链接落在 spec 笔记与 GLOSSARY；**ADR-0012**（2026-10-31）取代 ADR-0010 的视觉立场，同样走"无 architecture 专篇、链接 spec 笔记与 GLOSSARY"的口径。
- 前 9 条在 v1 判定下全部**已交付**。文件头 `- 状态：已交付` 表示决策已落地且当前生效，不是提案；ADR-0010 已落地（分票 #2–#13）但于 **2026-10-31 被 ADR-0012 取代**（状态改为 `已取代`，正文标注仍生效的条款）；ADR-0011 状态是**已接受**；ADR-0012 与 ADR-0013 状态是**已交付**（落地分票 #18–#25 已全部合入）；ADR-0013 是 ADR-0012 首页首屏的例外区，两者同时生效；ADR-0014 状态是**已接受**（邮箱验证码语义，随该修复落地）；ADR-0015 状态是**已交付**（按 spec #44 落地，分票 #45–#50 已全部合入）；ADR-0016 状态是**已交付**（落地分票 #36–#43 已全部合入），它记录装饰层与光标挂件的决策，视觉立场仍受 ADR-0012 约束；ADR-0017 状态是**已交付**（站点发布/展示开关与热门置顶运营位；落地票 #54 的分票 #60–#67 已全部合入，#68 是收尾票：文档与全量门禁）。ADR-0018 状态是**已交付**（帧率上限交给后台 + 可配的指针端偏移；落地分票 #72–#75 已全部合入，#76 是收尾票：ADR 状态与文档口径、全量门禁；它取代 ADR-0016 的决策 9、已否决段的「永远开着的 rAF 轻晃」、代价段第 4 条与当前生效约束 8 —— 其余条款继续生效，四处差异表在 ADR-0016 文末「后续修订」，落地偏离与最终数字在 ADR-0018 文末「实施记录」）。ADR-0019 状态是**已交付**（读者站渲染与缓存策略：SSR 白名单只留 `/` 与 `/article/**`、其余 SPA；中文字体改系统字体栈；取数并行 + 骨架；客户端 30s SWR + 匿名 SSR 30s 缓存。它取代 ADR-0012 决策 8 与约束 10 中"中文继续 Noto Sans SC"的部分，差异表在 ADR-0012 文末「后续修订」；**落地票 #80–#87 已全部合入**，#87 收尾：改前/改后对比、四项硬指标全部达标、ADR 状态与实测笔记口径收口。落地偏离（`routeRules.swr` 不生效改用 Nitro 钩子、页面过渡 200→120 ms、SSR 匿名取数）与最终数字在 ADR-0019 文末「实施记录」，改后复测见 [性能实测](../notes/2026-11-01-reader-site-performance-probe.md)）。ADR-0020 状态是**已接受**（零 DDL 的例外授权与通用取景表立项：授权只到"新建表"，通用取景表 `picture_crop_models` 是首个登记例外，旧的 `banner_crop_models` 并入其中而契约字段与错误码不动；建表、回填与旧表下线由落地分票 #119–#121 执行，尚未合入）。
- ADR-008 / ADR-009 在 v1 归档时没有被其他文档展开，是合集里"完整保留"的两条，因此这两份文件的信息量明显大于前七条。
- 每条 ADR 记录的是**当时的判断**；现状若有偏离，以 `docs/architecture/` 与代码为准，并考虑补一条新 ADR 而不是改写历史。

## 归档备注：仅具历史价值、v2 不需要再看

| 内容 | 原因 |
| --- | --- |
| ADR-001 / 002 / 003 / 005 / 006 的"现状病灶"数据与迁移路径 | 迁移已完成，数字（32 个 `Deps`、170 包、2,357 断言、14,462 行 json）只说明当时为何动手 |
| ADR-004 的存量违规基线（150+ 处、R6=4、R7=65）与"逐模块清零"节奏 | 存量已清零，基线当前值是 0 |
| ADR-007 的预估 vs 实际偏差表、3-4 轮收敛过程 | 已落地，教训本身已沉淀为"验证口径必须与门禁一致" |
| ADR-008 的四层架构图与 ADR-009 的删除清单逐项 | 代码已在位，清单只用于核对历史 diff |
| 被否决方案的完整论证（wire/dig、depguard、tRPC、SQLite 冒充 MySQL 等） | 已压缩为一行，v2 选型时不必重读论证 |

## 新增 ADR 的规则

1. **编号**：四位补零、严格递增，不复用、不跳号、不为插入而重排既有编号。
2. **命名**：`NNNN-短横线小写英文.md`，名字写"决策"而不是"问题"（例：`0007-id-string-type-as-truth.md`，而不是 `0007-id-type-bug.md`）。
3. **状态取值**只有五个：`提议` / `已接受` / `已交付` / `已取代`（superseded，[`docs/README.md`](../README.md) 里的写法）/ `已否决`。一份 ADR 同时只有一个状态；`已取代` 必须在正文写明被哪条取代（如"已取代（by ADR-0012）"）。已交付的 ADR **不改结论**——只能追加"后续修订"小节，或另开一条新 ADR 并在旧条目状态里标 `已取代`。
4. **正文结构**固定，顺序不变：
   1. `# ADR-NNNN：<标题>`
   2. `- 状态：` / `- 日期：` / `- 来源：`
   3. `## 背景与问题`
   4. `## 决策`
   5. `## 理由与已否决方案`
   6. `## 代价 / 不做什么`
   7. `## 当前生效约束`（不得为空）
   8. `## 相关代码与文档`（链接到对应的 `../architecture/` 文档与具体代码路径）
5. **登记**：新增后在本文索引表补一行，并在"关联文档"列写清它落在 `docs/architecture/` 的哪一篇。呈现层等没有 `architecture/` 专篇的决策，链接到 spec 笔记与根 `GLOSSARY.md`，**不要**为它另造一篇 architecture 文档——概念只在 GLOSSARY 定义一次，避免两处漂移。

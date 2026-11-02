# ADR-0020：零 DDL 例外授权与通用取景表

- 状态：已接受
- 日期：2026-11-02
- 来源：2026-11-02 设计评审（[grill session](../notes/2026-11-02-grill-session.md) 的 D28 / D52 与 §六·补三、§㉑㉓）· 父票 [#93](https://github.com/Jenlybein/NewblogX/issues/93) · 本票 [#118](https://github.com/Jenlybein/NewblogX/issues/118) · 落地分票 [#119](https://github.com/Jenlybein/NewblogX/issues/119)–#121

## 背景与问题

项目有一条零 DDL 硬约束（硬约束总表 A1）：**不改、不删既有表结构**，只允许 `CREATE TABLE IF NOT EXISTS` 新建表。它的真正理由不是"文档不好维护"，而是**CDC 按列名解析 binlog 行**：ES 文章投影监听 `article_models` / `user_models` / `category_models` / `tag_models` / `article_tag_models`（river 的表范围另含 `user_top_article_models` 与 `outbox_event_models`），image-ref 链路监听 `article_models` / `user_models` / `banner_models` / `favorite_models`；两条链路都从行快照里按**列名**取值（image-ref 的 `snapshot.RequireString("cover")`、行快照按 `id` / `deleted_at`）。改列名、删列、改类型都不会让编译红，只会让同步静默错位。以往文档把它写成"ES 投影按列名解析"，这个说法**不完备**——image-ref 同样按列名解析，准确表述是"**CDC 按列名解析**"。

取景（[ADR-0015](./0015-banner-framing-as-parameters.md)）目前只有轮播一处需求，落在横幅专属表 `banner_crop_models`（主键 `banner_id`，与 `banner_models` 一对一）。接下来有三件事同时压到这张表上：

1. 轮播封面要加标题（#119 的 `banner_models` 重建），取景的行结构不能跟着漂；
2. 文章封面也要锁 16:9 并可取景（#129），取景参数必须复用同一套换算；
3. 合集封面走同一条取景路径（#103 / #114），主体类型再加一类。

若照现状继续"一类主体一张取景表"，就要为文章封面、合集封面各建一张表：表数随主体类型线性增长，读路径分叉，后台那一个取景窗要按三种表各写一遍换算与校验，"预览即最终效果"这条 ADR-0015 的承诺也会分叉成三份实现。

于是需要一次正式立项：**在零 DDL 的口径里给"新建通用取景表"一个有名字的授权**，并把边界写死——它只覆盖"新建表"，不覆盖任何既有表的列变更。

## 决策

1. **把零 DDL 的准确边界写成"零列变更 + 允许新增表"**。既有表的列名 / 类型 / 列顺序 / 删列一律禁止（索引、外键、CH 的 `TTL`/`CODEC` 不受此限）；新增表本就在允许范围内，本 ADR 把"新增通用取景表"登记为**首个以 ADR 立项的例外**——从此"某个新增表"不再靠口头约定，而要有一条 ADR 指名。
2. **例外只到"新建表"为止**。本 ADR **不授权**任何既有表的结构变更：不加列、不改列名类型顺序、不删列，也不授权"先 DROP 再 CREATE"式的重建。`banner_models` 的 DROP 重建属 #119 的一次性现场操作，须按该票的备份与手工流程单独取得同意，**不得以本 ADR 为先例**。
3. **新建通用取景表 `picture_crop_models`**：键为 `(target_type, target_id)`（`target_type` 记主体类型，如 `banner` / `article` / `collection`；`target_id` 记该主体的 `ctype.ID`），取景三列 `focus_x` / `focus_y` / `zoom` 的名称、类型与语义**原样沿用** ADR-0015 的约束 1–5。
4. **旧的轮播专属取景表并入通用表**：`banner_crop_models` 的每一行以 `target_type = 'banner'`、`target_id = 原 banner_id` 回填进 `picture_crop_models`。旧表名从此属历史命名，正文一律写"通用取景表"。
5. **旧表的物理下线不走代码通道**：`schema.sql` 只增 `CREATE TABLE IF NOT EXISTS picture_crop_models`，**不写 DROP**。`banner_crop_models` 的回填与下线由 #119 按"备份 → 回填 → 手工 DROP"执行；保留一张空表也不违反本 ADR 的任何约束。
6. **命名与契约分层**：表名是存储层的事，契约字段照旧是 `crop`、错误码照旧是 `banner.crop_invalid`。改名合并**不动** `swagger.json` / `openapi.d.ts` / 两端前端类型，也不改错误码的域。
7. **新表照旧走三处登记**：`schema.sql` 加一段建表语句、`testkit/schema/rows.go` 加行结构、testkit 的 `groupModels` 把行结构挂进对应表组，并由列名契约测试（`schema.ColumnsDiff` / `AssertColumns`）守住。
8. **通用取景表不进任何 CDC 监听范围**（ES 文章投影、image-ref 都不监听它），`outbox` 也不为取景发事件——取景是读路径的强需求，走事件最终一致会让"刚取好的景"不生效。

## 理由与已否决方案

核心判断是：**取景是"图 + 一组相对数"的关系，不是某张业务表的附属列**。主体类型会持续增加，而取景的语义（焦点 0–1 + 缩放 1–2）只有一套；把它按主体类型拆表，等于把同一套语义复制 N 份，日后每改一次校验就要改 N 处。

零 DDL 这边，结论要落在"**列**"而不是"**表**"：CDC 解析的是行的列名，所以只有列的生命周期需要冻结，新增表不改变任何既有行的列集合。把两者混写成"零 DDL = 什么都不能动"，会让新增表也要每次口头请示（本项目的 `outbox_event_models`、`article_hot_top_models`、`banner_crop_models` 都是这样长出来的）；混写成"加新表随便"，又会被人拿去论证"重建既有表也没关系"——两种读法都要在文档里关掉。

被否决的方案：

- **继续用 `banner_crop_models`，另开 `article_cover_crop_models` / `collection_crop_models`**：每类主体一张表，表数线性增长、读路径分叉、取景窗换算写三份。**否决**——这正是要合并的原因。
- **给 `article_models` / `banner_models` 加取景列**：直接违反零列变更；`banner_models` 还在 image-ref 的监听范围里按 `cover` 列名取值，加列虽安全但改列不安全，而取景列迟早要改。**否决**。
- **把取景塞进 `runtime_config_models` 的 JSON 段**：每加一张图就重写一整段配置，多管理员并发保存互相覆盖，且没有"这张图还在不在"的引用可查。**否决**。
- **用 `ALTER TABLE ... RENAME` 做"改名"**：本项目没有 ALTER 通道（`-db` 只执行 `CREATE TABLE IF NOT EXISTS`），改名同样是 DDL 动作；走"新建 + 回填"是唯一能在仓库里 review 的路径。**否决**。
- **保留 `banner_id` 列名、只把主键换成泛型 id**：文章封面的 id 会被存进 `banner_id`，等于让列名撒谎，读路径与后来人都被误导。**否决**。
- **给取景表加"取景版本"或历史行**：多一层版本判别与清理逻辑，而取景是"当前值"语义（改取景只是改三个数），没有回看历史行的需求。**否决**。
- **把取景表接进 CDC，让缓存 / 搜索跟着更新**：取景不参与搜索与索引，接进去只是扩大冻结范围与故障面。**否决**。
- **为取景预留"按屏宽多份参数"的列**：ADR-0015 已否决（比例是设计常量，不随屏宽变化），通用表不为它预留列。**否决**。

## 代价 / 不做什么

- **多一层类型判别**：读路径变成"按 `target_type` 过滤"，删主体时要同步清对应取景行；目前没有外键能指回多张表，引用完整性只能靠应用层加一次性清理（重建流程里必须清一遍孤儿行）。
- **`target_id` 无法建外键**：一张表指向多个主体表，MySQL 没有多态外键；`target_type` 是字符串，没有枚举约束。写错类型＝静默没有取景（缺行即缺省，读路径退化成"居中、缩放 1.0"，**不会报错**）。
- **一次回填与一次现场操作**：旧表数据要回填、旧表要下线，两者都不写进仓库内的建表通道，只能按 #119 的流程手工执行，执行前后必须备份。
- **命名与契约会长期错位**：表叫 `picture_crop_models`，契约字段却还是 `crop`、错误码还是 `banner.crop_invalid`。这是有意的——改契约要重跑跨端生成并牵动两个前端，收益只有"名字更整齐"。代价是后来人看到 `banner.crop_invalid` 会以为取景只服务轮播，所以 GLOSSARY 的「取景」词条必须把这条拆开写清。
- **不做什么**：不授权任何既有表的列变更或重建（含以后想"顺手加一列"的冲动）；不改契约字段与错误码；不给取景表加外键 / 触发器 / 生成列；不做取景历史版本；不做按屏宽或按设备的多份取景；不把通用取景表接进 ES 投影、image-ref 或 outbox。

## 当前生效约束

1. **零 DDL 的准确口径 = 零列变更 + 允许新增表**。既有表的列名 / 类型 / 列顺序 / 删列一律禁止（索引、外键、CH 的 `TTL`/`CODEC` 不受此限）；新增表必须用 `CREATE TABLE IF NOT EXISTS` 登记进 `backend/internal/platform/mysql/ddl/schema.sql`。
2. **理由表述固定为"CDC 按列名解析"**，不得再写成"ES 投影按列名解析"：ES 文章投影与 image-ref 两条链路都从 binlog 行快照按列名取值（image-ref 取 `cover`，行快照取 `id` / `deleted_at`）。
3. **通用取景表 `picture_crop_models` 是首个登记例外，也是当前唯一一个**：键 `(target_type, target_id)`，取景三列 `focus_x` / `focus_y` / `zoom` 的名称、类型与语义沿用 ADR-0015 约束 1–5，不得改列。再开第二个"点名例外"必须另开 ADR。
4. **例外仅限新建表**：任何既有表的列变更仍一律禁止（含 ALTER、DROP 后重建、加列、删列、改类型、改列顺序）。本 ADR 不授权 `banner_models` 或其他既有表的任何结构变更。
5. **`banner_crop_models` 的退场不走代码通道**：`schema.sql` 只增通用表的建表语句、不写任何 `DROP`；旧表的回填与下线只能由 #119 按"备份 → 回填 → 手工 DROP"执行，且不得据此事后把 DROP 写进仓库内的建表流程。
6. **契约与错误码一律不动**：字段名仍是 `crop`，错误码仍是 `banner.crop_invalid`；表名不进契约、不进前端类型，改表名不需要重跑 `make openapi` / `pnpm openapi:types`。
7. **新表仍须三处登记加列契约测试**：`schema.sql` / `testkit/schema/rows.go` / testkit `groupModels`；`schema.ColumnsDiff` / `AssertColumns` 必须覆盖通用取景表。
8. **通用取景表不得进入任何 CDC 监听范围**（ES 文章投影、image-ref）**与 outbox 事件**；一旦接入，`target_type` / `target_id` 的列名即被冻结。
9. **旧表名属历史命名**：正文一律写"通用取景表"，`banner_crop_models` 只在"历史命名 / 迁移"语境里出现。

## 相关代码与文档

- 硬约束与口径：[overview §六 A1](../architecture/overview.md) · [GLOSSARY · 零 DDL / 取景](../../GLOSSARY.md) · [README §11.3 与 §十三](../../README.md)
- 决策上游：[ADR-0015](./0015-banner-framing-as-parameters.md)（取景存参数、CSS 渲染；本 ADR 只改"存在哪张表"，不改它的结论）· [ADR-0017](./0017-publish-display-switches-and-hot-top-operator-slot.md)（既有表冻结下另开新表的先例）
- 表结构：[backend/internal/platform/mysql/ddl/schema.sql](../../backend/internal/platform/mysql/ddl/schema.sql)（`banner_crop_models` 现状在这里，通用表的建表语句也加在这里）· [ddl.go](../../backend/internal/platform/mysql/ddl/ddl.go)（`Apply` 只做 `CREATE TABLE IF NOT EXISTS`）
- 行结构与契约测试：[backend/internal/testkit/schema/rows.go](../../backend/internal/testkit/schema/rows.go)（`BannerCropRow`）· [testkit/schema/schema.go](../../backend/internal/testkit/schema/schema.go)（`groupModels`）· `schema.ColumnsDiff` / `AssertColumns`
- 取景落点：[backend/internal/site/adapter/mysql/crop.go](../../backend/internal/site/adapter/mysql/crop.go)（`cropRow`，改名合并后随之迁到通用表）
- CDC 反证：[backend/internal/platform/cdc/imageref/river.go](../../backend/internal/platform/cdc/imageref/river.go)（`imageRefTables` 四张表 + 按列名取值）· [article_search_projection.go](../../backend/internal/platform/cdc/river/article_search_projection.go)（ES 文章投影监听的五张表）· [settings.yaml](../../backend/config/settings.yaml)（river 的表范围）
- 立项与执行：[#93](https://github.com/Jenlybein/NewblogX/issues/93)（父票）· [#118](https://github.com/Jenlybein/NewblogX/issues/118)（本 ADR）· [#119](https://github.com/Jenlybein/NewblogX/issues/119)（建表与回填）· [设计评审记录](../notes/2026-11-02-grill-session.md)（D28 / §六·补三 / §㉑㉓）

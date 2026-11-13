# ADR-0019：合集目录树与项目目录树成为唯一对外形状

- 状态：已交付
- 日期：2026-11-05
- 来源：parent spec [#272](https://github.com/Jenlybein/NewblogX/issues/272) 契约阶段 · 落地票 [#277](https://github.com/Jenlybein/NewblogX/issues/277)（拆树·收缩与出处链）

## 背景与问题

合集与项目的「层次」和「顺序」曾分散在三处真相里，读面因此出现分歧：

1. **层次**：产品要的是文档站 / Obsidian 文件树那种「项目 → 章节文件夹 → 文章页」，且文章阅读页左侧也要看到同一棵树；现有扁平收录关系只表达「第几篇」，往既有中间表加层次字段就是 ALTER，违反零 DDL 硬约束（`docs/architecture/overview.md` §六 A1）。合集与项目的四层实现又是逐行克隆，再各加一套树会让克隆面翻倍。
2. **对外形状**：迁移期间曾同时存在一张与 owner 无关的共用树表 `content_outline_node_models`、两张按实体命名的树表，以及 5 个旧共享端点与各自具名端点。两套端点、三张表并存时，同一份「PUT 具名树」会让两个列表卡片入口的篇数 / 封面一真一假；收缩必然让契约条目数下降。
3. **实体之间的编排**：后台合集 / 项目是一列平铺卡片，运营看不出哪几个属于同一批主题；顺序在 `collection_models.sort_order` / `project_models.sort_order`，而目录树里节点顺序在树表 `sort_order`，两者维度不同必须分清。编辑弹窗里「改标题」与「保存目录树」若分两次请求，任一失败都会留下半截状态。

于是需要一次决策：把「合集 / 项目内部」的层次与收录收成唯一一份树形状，把「实体之间」的分组编排收成唯一一份顺序真相，两者不同维度、不互相取代。

## 决策

1. **对外只留两个具名形状**：`GET|PUT /collections/{id}/outline-tree` 与 `GET|PUT /projects/{id}/outline-tree`。5 个旧共享端点（`GET|PUT /collections|/projects/{id}/outline*` 与 `GET /articles/{id}/outline-context`）从路由、handler、DTO、swag 注解、契约产物与三份 route guard 清单一并删除；旧共用表 `content_outline_node_models` 从 `schema.sql`、testkit `schema.All` / `Outline` 组、行结构、仓储与真库夹具中全部删除，不得回到 schema.sql / testkit / 读面。`schema.sql` 与 `schema.Models(schema.All)` 双向无差集仍是唯一权威。
2. **层次只存在于两张具名树表**：`collection_outline_node_models` / `project_outline_node_models` 是「合集 / 项目内部」层次与收录的唯一真相；不得在文章表、合集表、项目表或任何既有表上加层次字段。**归属树读模型收口一处**：列表卡片调用方只传「哪个 owner」，实现内部按归属选表（`NamedOutlineRepository`），读者详情、阅读页左栏与后台编辑器同样只经具名树仓储，事务入口一次给出归属；旧共用表不再有任何读取点。
3. **两种节点、一篇文章可进多棵树**：文件夹节点只分组、**不可读不可点击**（无封面 / 摘要 / 正文），文章节点指向一篇文章；任意深度嵌套，**空文件夹允许保留**（先搭章节骨架再挂文章）。一篇文章可同时出现在多棵树里，同一棵树内最多一个节点（唯一约束）。失效行口径沿用：文章后来不再公开时节点保留，读者侧靠 JOIN 过滤、后台标「已失效」可一键清理。
4. **阅读页左栏不再有回退分支**：归属只由 `?from=<kind>:<id>` 给出；来源缺失或无法解析时按「没有可展示的树」返回空读模型、不猜默认归属、不打任何请求。右栏标题大纲（`ArticleTocAnchor`）始终独立保留；`<1024px` 收进「目录」抽屉。读者共用呈现组件 / 版式仍只有一份，两个入口、两张主表、两套规则不变，不加兼容层、不加开关分支、不新增第二测试接缝。`/collections/**`、`/projects/**` 保持 SPA（树只在客户端取数、骨架占位），`/article/**` 的 SSR 边界不因本决策改动。
5. **编排入口只经树编辑器一个入口**：后台的收录与排序只在编辑弹窗里的抽屉组件 `components/articles/OutlineTreeEditor.vue`（按 `kind` 各挂一次）完成，不得再造第二套顺序真相；弹窗**新建后保持打开并拿到实体 ID**（没有 ID 时抽屉里只显示「先保存」提示），抽屉自带保存按钮与 dirty 标记、与信息卡的保存互不干扰。交互保留按钮式路径（上移 / 下移 / 缩进 / 移出 / 新建 / 重命名 / 批量收录）与整树一次性提交；拖拽排序改用成熟 sortablejs 方案（拖拽落点语义落在 `frontend_admin/app/utils/outlineArrange.ts` 的纯函数并有单测），不再依赖 HTML5 DnD。旧扁平中间表 `collection_article_models` / `project_article_models` 已下线，不读写、不迁移。
6. **编排层是 4 张新表、分组树是唯一顺序真相**：两个实体各一对分组 / 归属表——`collection_group_models` / `collection_group_item_models` / `project_group_models` / `project_group_item_models`（零 DDL：只新建，不改任何既有列）。`group_id = 0` 表示**未分组**，不建特殊行；归属表唯一键落在实体主键 `collection_id` / `project_id` 上（不是 `(group_id, x_id)`），一个合集最多进一个分组，且对实体主键有真实外键。**分组之间、组内条目、未分组段的顺序全由归属表 `sort_order` 表达**；`collection_models.sort_order` / `project_models.sort_order` 退出读面，仅作未分组历史条目的兜底排序（既没有归属行、也没被拖过）。目录树是「合集 / 项目内部」的层次与收录唯一真相，分组是「实体之间」的编排层，两者不同维度，不构成第二套真相。
7. **分组 CRUD 立即生效**：建 / 改名 / 删各是一次独立提交，不参与拖拽的 10 秒防抖。**整树换序幂等整份覆盖**：提交的一定是界面上最终看到的排列，集合与当前库不一致时（有人并发建 / 删了分组或条目）**整单拒绝**，前端据此刷新后重排。**删分组只删编排**：分组软删 + 其条目 `group_id` 置 0，合集 / 项目本身一行不动（否则条目既不在任何分组里、也不在未分组里，界面上直接消失）。
8. **一个原子 compose 端点**：`PUT /<kind>/{id}/compose` 在**一个事务**里写字段（标题 / 描述 / 封面）与整棵目录树；编辑弹窗里「改标题 + 拖完树」是一次保存动作，不允许两个请求各留半截状态。
9. **分组语义只写一份**：两个实体各有一对表，但领域规则、用例、handler 都只有一份，表名通过 `groupTables` / `grouping.Repository` 注入。
10. **零 DDL 不变**：代码里只有 `CREATE TABLE IF NOT EXISTS`，没有 DROP / ALTER / 运行时 AutoMigrate；`-db` 不会 DROP 旧表，旧共用表与旧中间表的物理下线是**部署时手工 DROP**。契约三份产物（`backend/docs/swagger.json` 与两个 app 的 `app/types/openapi.d.ts`）同批由生成器重新产出，不手改、不跨 app 共享；`scripts/contract-structures.json` 的 `minimumEntries` 以实测为独立事实源，判据实现不得为绕开条目变动而改。`OutlineNodeRequest` / `OutlineNodeResponse` / `OutlineArticleRefResponse` 等叶子 DTO 继续由两个具名形状复用，`OutlineTreeResponse` 仍由两个详情响应使用。

## 理由与已否决方案

最低成本的迁移是「一个维度只留一个真相」：内部层次与收录归具名树，实体之间的编排归分组归属表，对外形状只留两个具名端点。共用的领域规则、用例、handler 与呈现组件各一份，差异只留在按 `kind` / 表名注入的地方。

被否决的方案：

- **继续保留 5 个旧共享端点 + 旧共用表做兼容层**：两套端点、三张表并存时同一份 PUT 会让两个入口的篇数 / 封面一真一假，读面必然分裂。**否决**。
- **给旧扁平中间表加 `parent_id`**：加列就是 ALTER，零 DDL 下没有迁移通道；且按 `(collection_id, article_id)` 唯一的表表达不了同名文件夹、空文件夹、文件夹整体移动。**否决**。
- **一张与 owner 无关的共用树表长期服务两个实体**：会让具名形状与归属读面再次分叉；两张具名表配一份领域规则才同时满足「对外只有两个形状」与「实现只有一份」。**否决**。
- **详情页内嵌正文（文档站式）**：章节会失去 `/article/**` 的 SSR 与分享链接，SEO 与分享卡片一起丢。**否决**。
- **保留旧中间表做收录真相、新表只存分组**：顺序与成员出现两个真相，失效行清理与排序提交在两处打架。**否决**。
- **迁移旧收录**：上线时生产两张中间表均为 0 行，迁移只换来一次性脚本、一次回填验证与永久的新旧对照口径。**否决**。
- **从标题前缀自动推导层次**：`00-` / `01-` 是写作习惯不是契约，命名不统一整棵树就散架，也无法表达空文件夹。**否决**。
- **树里只允许「含文章或子文件夹」的文件夹**：与「空文件夹也能先搭章节骨架」冲突；空文件夹允许保留，读者侧也保留。**否决**。
- **HTML5 DnD 拖拽排序**：移动端与自动化测试里都脆；改用成熟 sortablejs，同时保留按钮式路径与整树一次性提交。**否决**。
- **把分组写成一个永远存在的特殊「未分组」行**：未分组是「还没被放进任何分组」的状态，不是一种分组。**否决**，用 `group_id = 0`。
- **归属表唯一键取 `(group_id, x_id)`**：同一合集能同时进两个分组，界面出现两份。**否决**，唯一键落在实体主键。
- **换序时「缺几个也照写」**：会把静默排丢的那条当成成功；整单拒绝 + 刷新重排与标签 / 项目 / 热门置顶的全量换序同一条语义。**否决**。
- **删分组只软删分组、不动条目**：条目会指向不存在的分组，读面里直接消失。**否决**。
- **分组 CRUD 参与 10 秒防抖**：点了删除却要等 10 秒才生效不可接受。**否决**。
- **改标题与拖树分两次请求**：任何一次失败都留下半截状态，用户没有任何线索知道该重试哪一个。**否决**，一个事务的 compose 端点。
- **两个实体各写一套分组领域规则 / 用例 / handler**：「未分组固定第一」「删分组不删条目」这些口径早晚会漂成两种。**否决**，表名注入、规则一份。
- **为绕开契约条目下降去改判据或加兼容开关**：判据「只增不减」逻辑与测试接缝不得动，基线本身以实测为独立事实源。**否决**。
- **在代码里 DROP / ALTER 任何表**：零 DDL 下不做，旧表的物理下线留给部署时手工 DROP。**否决**。

## 代价 / 不做什么

- **契约是破坏性收缩**：旧共享端点与旧共用形状不再出现在契约里，两个前端的派生类型同批重生成；仍按旧路径调用的客户端会 404。
- **`minimumEntries` 随批以实测重新登记**：「只增不减」判据逻辑继续生效，只是独立事实源里登记的数字随收缩 / 补字段变动；判据实现不得为绕开变动而改。
- **编排入口合并**：旧的独立排列页与菜单项删除，管理员要改用编辑弹窗里的抽屉；旁路环境需要手工重排。新建后弹窗必须保持打开并拿到实体 ID，抽屉保存与信息卡保存互不干扰。
- **4 张新表需要手动 `-db`**：`schema.sql` 里都是 `CREATE TABLE IF NOT EXISTS`；`-db` 会顺带跑 `InitES`，ES 未起时会 fatal，部署顺序见 [README §11.3](../../README.md) / [README §11.4.4](../../README.md)。旧表部署时手工 DROP，代码里不 DROP / ALTER。
- **不做兼容层、不做按开关二选一的分支、不新增源码文本断言或第二测试接缝**；既有 `schema.Models(schema.All)` ↔ `schema.sql` 双向无差集、`columns_static_test.go`、`schema_sql_integration_test.go`、`TestFlagDB` 与契约↔路由双向对账继续作为守卫。
- **不做什么**：不做文件夹的可读页（不做「章节概览」）；不做自动编号、树内搜索、多树合并视图（一篇文章属多棵树时阅读页只显示来路指定的那一棵）；不做旧收录的数据迁移；不做运行时 AutoMigrate、不新增 DDL；不为分组与目录树造第二套顺序真相。

## 当前生效约束

1. 层次只能存在于 `collection_outline_node_models` / `project_outline_node_models` 两张具名树表；不得在文章表、合集表、项目表或任何既有表上加层次字段，`content_outline_node_models` 不得回到 `schema.sql`、testkit 或任何读面；旧扁平中间表 `collection_article_models` / `project_article_models` 已下线，代码里不得新增任何读写，既不迁移也不 DROP。
2. 对外树端点只有 `GET|PUT /collections/{id}/outline-tree` 与 `GET|PUT /projects/{id}/outline-tree`；`GET /articles/{id}/outline-context` 与 `GET|PUT /collections|/projects/{id}/outline[/admin]` 不得重新登记。
3. 归属树读模型只有一处：调用方给出 owner，实现内部按归属选表（`NamedOutlineRepository`）；列表卡片篇数 / 封面必须与同一归属的具名树同步（真 MySQL 行为用例钉住「PUT 具名树 → 两处列表卡片同步」）。
4. 阅读页左栏只在 `?from=<kind>:<id>` 指向某棵具名树时出现；来源缺失 / 不合法时不猜默认归属、不发请求；右栏标题大纲始终保留。`/collections/**`、`/projects/**` 保持 SPA，`/article/**` 的 SSR 白名单不得因本决策改动。
5. 文件夹节点不可读、不可点击（无封面 / 摘要 / 正文）；**空文件夹允许保留**；一篇文章可同时进多棵树，但同一棵树内最多一个节点；文章不再公开时节点保留，读者侧 JOIN 过滤、后台标「已失效」可清理。
6. 读者共用组件与版式仍是一份，两个入口、两张主表、两套规则不变；不得引入兼容层、开关分支或第二个测试接缝。
7. 后台的收录与排序**只经树编辑器一个入口**（编辑弹窗里的抽屉组件 `OutlineTreeEditor.vue`，按 `kind` 各挂一次），不得造第二套顺序真相；弹窗新建后保持打开并拿到实体 ID，抽屉自带保存按钮与 dirty 标记；拖拽用成熟 sortablejs 方案，保留按钮式路径与整树一次性提交。
8. 两个实体各一对分组 / 归属表共 4 张新表；`group_id = 0` 表示未分组、不建特殊行；归属表唯一键落在实体主键上（一个合集最多进一个分组），并对实体主键有真实外键。
9. **分组树是分组的唯一顺序真相**：分组之间、组内条目、未分组段顺序全由归属表 `sort_order` 表达；`collection_models.sort_order` / `project_models.sort_order` 退出读面、仅作未分组历史条目兜底。目录树是「合集 / 项目内部」层次与收录的唯一真相，分组是「实体之间」的编排层，两者不同维度，不得互相改写或合并成第二套真相。
10. 分组 CRUD 立即生效，不参与 10 秒防抖；整树换序是幂等整份覆盖，集合与当前库不一致时整单拒绝（`order_mismatch` 同语义）；删分组只删编排（分组软删 + 条目 `group_id` 置 0，实体本身一行不动）。
11. `PUT /<kind>/{id}/compose` 在一个事务里写字段（标题 / 描述 / 封面）与整棵目录树；不允许把「改字段」与「保存树」拆成两次提交。
12. 领域规则 / 用例 / handler 只一份，表名经 `groupTables` / `grouping.Repository` 注入；不得为两个实体各克隆一份实现。
13. 零 DDL：代码里只有 `CREATE TABLE IF NOT EXISTS`，不 DROP / ALTER、无运行时 AutoMigrate；`-db` 只做加法，旧共用表与旧中间表由部署时手工 DROP。契约三份产物同批由生成器产出，`scripts/contract-structures.json` 为 `minimumEntries` 的独立事实源，判据实现不得为绕开条目变动而改。
14. `OutlineNodeRequest` / `OutlineNodeResponse` / `OutlineArticleRefResponse` 是两个具名形状复用的叶子 DTO，不因「名字带 Outline」删除；`OutlineTreeResponse` 仍由两个详情响应使用。

## 相关代码与文档

- 概念：[GLOSSARY · 合集目录树](../../GLOSSARY.md) · [GLOSSARY · 项目目录树](../../GLOSSARY.md) · [GLOSSARY · 合集](../../GLOSSARY.md) · [GLOSSARY · 项目](../../GLOSSARY.md)
- 上游决策：[ADR-0003](./0003-generated-contract-single-source.md)（契约以代码生成为单一来源）· [ADR-0015](./0015-publish-display-switches-and-hot-top-operator-slot.md)（失效行与运营位口径）· [ADR-0016](./0016-reader-rendering-and-cache-strategy.md)（渲染边界）· [ADR-0009](./0009-es-sync-article-projection-only.md)（合集 / 项目不进 ES）· [架构总纲 §六 A1](../architecture/overview.md)（零 DDL）· [契约](../architecture/contracts.md)
- 后端目录树：`backend/internal/article/app/named_outline.go`、`app/outline.go`、`app/outline_reader.go`、`app/collection.go`、`app/project.go`、`adapter/mysql/card_stats.go`、`adapter/mysql/collection_repo.go`、`adapter/mysql/project_repo.go`、`adapter/http/router.go`、`adapter/http/handler_named_outline.go`、`adapter/http/dto_outline.go`
- 后端分组：[grouping.go](../../backend/internal/article/domain/grouping/grouping.go)（纯规则 + 仓储契约）· [group_repo.go](../../backend/internal/article/adapter/mysql/group_repo.go)（一对表名 → 一份实现）· [app/grouping.go](../../backend/internal/article/app/grouping.go)（CRUD / 换序 / compose）· [grouping_handler.go](../../backend/internal/article/adapter/http/grouping_handler.go)
- 表与守卫：`backend/internal/platform/mysql/ddl/schema.sql`、`backend/internal/testkit/schema/schema.go`、`backend/docs/swagger.json`、`scripts/contract-structures.json`、`scripts/check-contract-freshness.mjs`、`backend/internal/platform/http/router/{retired_flat_include_test.go,retired_shared_outline_test.go,admin_gate_scan_test.go,contract_routes_test.go}`
- 前端：`frontend/app/services/outline.ts`、`frontend/app/utils/outlineContext.ts`、`frontend/app/pages/collections/[id].vue`、`frontend/app/pages/projects/[id].vue`、阅读页 `frontend/app/pages/article/[id].vue`（左栏与「目录」抽屉）、`frontend_admin/app/components/articles/OutlineTreeEditor.vue`、`frontend_admin/app/utils/outlineArrange.ts`、`frontend_admin/app/services/admin.ts`、两个 app 的 `app/types/openapi.d.ts`
- 部署与运行：[README §11.3](../../README.md) · [README §11.4](../../README.md) · [README §11.4.4](../../README.md)

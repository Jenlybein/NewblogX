# ADR-0026：标签与分类的删除语义（软删行占住唯一键、恢复与停用）

- 状态：已交付
- 日期：2026-11-09
- 来源：用户 2026-11-09 的批量决策（[#319](https://github.com/Jenlybein/NewblogX/issues/319) Implementation Decisions）· 落地票 [#322](https://github.com/Jenlybein/NewblogX/issues/322) / [#325](https://github.com/Jenlybein/NewblogX/issues/325) · 关联 [ADR-0009](./0009-es-sync-article-projection-only.md)（分类置空经 CDC 自动重算搜索投影）· 术语见 [GLOSSARY · 标签 / 分类](../../GLOSSARY.md)

## 背景与问题

分类与标签都是**软删**实体：行上带 `deleted_at`，删除走 GORM 软删（`backend/internal/article/adapter/mysql/rows.go:28-33` 的 `BaseRow`）。但库里的唯一索引不认识软删：

- 分类 `category_models`：`UNIQUE KEY uk_category_user_title (user_id, title)`（`backend/internal/platform/mysql/ddl/schema.sql:212`）。
- 标签 `tag_models`：`UNIQUE KEY idx_tag_models_title (title)`（同文件 `:584`；名字带 `idx_` 前缀，但它**是唯一键**）。

于是"删过的名字被永久占用"：查重（GORM 模型查询，自动带 `deleted_at IS NULL`）说没重复，INSERT 却撞 1062。此前没人踩到，只是因为个人端还不能删分类；分类管理接入个人主页后，这条路径立刻会被走到。

MySQL 没有部分索引 / 条件唯一索引，现行约束是零 DDL（不改既有表的列与索引），所以"只对活行唯一"在库层面表达不出来。**"删过的名字能不能重用"必须在应用层显式决定**，而不是留给下一个人偶遇。

第二个问题在标签侧：标签被删是一刀切，还是与"停用"分级？"一律允许删除"与"停用为主"能达成的效果不同，代价也不同。

## 决策

1. **删除保持软删，名字可以重用；重用的方式是"先恢复、再新建"。** 凡"唯一键 + 软删"的实体，创建时都走平台助手 `RestoreOrCreateUnique`（`backend/internal/platform/mysql/restore.go:29`）：先按匹配键找 `deleted_at IS NOT NULL` 的行并恢复（清 `deleted_at`、写回本次字段值、**保留原 ID**），找不到才插入。分类的匹配键是 `(user_id, title)`；并发下"唯一键冲突"等价于"已存在"，不算错误。**分类与标签的创建都已走这条**（标签侧在本批补齐，两个仓储的 `Save` 共用 `saveUniqueWithRestore`，`backend/internal/article/adapter/mysql/taxonomy_repo.go:129-155`）；改名走 UPDATE，不会去恢复另一行软删记录（见「已知限制」）。
2. **恢复保留原 ID，调用方不得依赖"新建必然有新 ID"。** 前端建完重拉列表，不拿返回的 ID 做后续写入。
3. **删除分类必须在同一事务内先解除文章归属、再软删分类。** 文章的分类字段置空（`article_models.category_id` 上有外键指向分类表，`backend/internal/platform/mysql/ddl/schema.sql:132`，但软删下外键永远不会触发，救不了这条引用），置空经 binlog 触发该文章的搜索投影重算（`backend/internal/platform/cdc/river/projection_replay.go:146`），不需要额外补偿任务。
4. **标签：停用是常规动作，删除只对零引用开放。** 停用后它不进可选列表（`Tag.Usable` / `FilterUsable`，`backend/internal/article/domain/taxonomy/taxonomy.go:230-263`），已打过它的文章不受影响；删除前按关联表**实时**统计引用数，被引用时返回可读错误并提示改用停用，不采信冗余计数列 `tag_models.article_count`（`backend/internal/platform/mysql/ddl/schema.sql:580`）。
5. **唯一键冲突一律翻译成领域可读文案。** 分类与标签的 `Save` 共用同一条翻译 `taxonomySaveError`（`backend/internal/article/adapter/mysql/taxonomy_repo.go:164-172`），两侧口径只有一份；不允许把 1062 原文透传成 500。
6. **删除分类时，回收站里的文章保留分类归属。** 置空 `article_models.category_id` 只作用于**活跃文章**（软删作用域），并且用 `UpdateColumn` 而不是 `Updates` —— 后者会顺带刷 `updated_at`，让所有原属该分类的文章在详情页集体显示一个新的"更新于"（`backend/internal/article/adapter/mysql/taxonomy_repo.go:192-199`，用例 `TestDeleteCategoryDetachesArticlesBeforeSoftDelete`）。为什么保留：这篇文章看不见，删除分类也不该偷偷改它的数据；而删除是软删、同名重建会**恢复同一行分类**，归属随之回来，不丢信息。

## 理由与已否决方案

- **否决"改成物理删除"**：物理删除确实能立刻腾出名字，但代价有三条——① 全仓软删约定被破一个口子，这些表的 `deleted_at` 退化成死列，"删除还能恢复"的能力丢失；② 不可逆：误删一个分类时要连着把 N 篇文章的归属一起写进同一次事务，没有回退路径；③ binlog 的 delete 事件与投影的重算假设会跟着变，CDC 侧要再审一遍。改为"恢复"只多一次 UPDATE，不动 schema、不动 CDC。
- **否决"删除时给标题加脏后缀"**（如 `旧名#deleted-<ts>`）：名字能让出来，但把脏数据写进了标题这一列——列表、审计、导出读到的都是脏名，恢复时还得再解析一遍后缀才能还原；后缀规则一旦改口径，历史行再也解释不清。恢复方案下标题始终是用户写的那一个。
- **否决"删除时把标题置空（NULL）"**：MySQL 唯一索引对 NULL 不判重，技术上也能腾出名字，但同样**不可逆地**毁掉"这个名字以前存在过"的信息，恢复只能得到一条无名行。
- **否决"标签一律允许删除（含被引用）"**：删掉一个被 30 篇文章使用的标签，等于一次性从 30 篇公开文章上摘掉一个主题，作者既没有预期也不会收到通知；而"让它不再可选"这个真实诉求用停用就能达到，且停用可逆。删除只留给零引用的垃圾条目（拼错的、空壳的），那时爆炸半径是 0。
- **否决"用冗余计数列判断能不能删"**：`tag_models.article_count` 可能过期，一次"看起来零引用"的删除会抹掉真实引用。按关联表实时统计是唯一可信的判据。

## 代价 / 不做什么

- **软删行一直留在表里**，包括"恢复过又删掉"的。本批的**唯一遗留**是一次性清理脚本（#319 明确不做）：新语义下同名创建会自动恢复，清理不再必要，但表会缓慢变大。
- **恢复的语义是"这条以前存在过"**：恢复出来的行继承旧 ID 与旧的创建时间，把它当"全新的一条"来用（例如按 ID 大小当创建顺序）会看到旧位置。
- **每次 `Save` 多一次"活行是否存在"的查询**：不看 `RowsAffected` 就得先查一次（对应 `saveUniqueWithRestore` 的第一段），换来的是"值没变的保存"不会被误判成新建。分类与标签的保存频率以人操作计，这点开销不值得优化掉。
- **不给分类加隐私开关、不做多级分类与排序**（要加列，违反零 DDL）；**不做标签合并 / 批量重命名 / 批量迁移**（#319 out of scope）。
- **停用标签不会因为"改一下描述"被悄悄放回可选**：编辑是读—改—写，没传启用状态就保持原状。

## 当前生效约束

1. 创建（新建路径）必须走 `RestoreOrCreateUnique`（或同语义的"先恢复再插入"）；禁止裸 INSERT 之后只靠 1062 兜底。分类与标签都已落地（共用 `saveUniqueWithRestore`）；补新实体时用同一个助手，禁止改成"给标题加后缀"。
2. 恢复不得更换 ID。
3. 删除分类 = 解除**活跃**文章归属 + 软删分类，两步在同一事务内完成；先解除归属、后删分类，顺序不得颠倒。
4. 标签删除前必须按关联表实时统计引用数；被引用时拒绝并提示改用停用。
5. 唯一键冲突不得以 500 或数据库原文的形式出现在响应里。
6. 删除分类只是"活跃文章解绑 + 软删分类"：回收站里的文章保留分类归属，且活跃文章的 `updated_at` 不得被这次解绑改动（用 `UpdateColumn`，不要用 `Updates`）。
7. 停用状态只能由显式的启用动作改回。

## 已知限制

- **把一个分类改名成"只被软删行占用"的名字，仍会报「名称重复」。** 恢复只发生在新建路径；改名是 `UPDATE category_models ... WHERE id = ?`，撞的仍是那条不含 `deleted_at` 的唯一索引。用户会看到"名称重复"，但列表里根本看不到那个名字。这是"零 DDL + 保留软删行"的必然结果，本批不修（修法要么改索引，要么改名时顺手恢复另一行——后者会把一条看不见的分类复活，更糟）。行为由 `TestRenameCategoryIntoSoftDeletedNameStillDuplicates` 钉住（`backend/internal/article/adapter/mysql/taxonomy_repo_test.go`）。
- **"删除分类后搜索侧的分类归属同步更新"没有直接用例。** #325 的这条验收目前只由 CDC 链路（ADR-0009：置空经 binlog 触发该文章的投影重算）与 ADR 保证，本批**没有**补"删分类 → 投影重算"的端到端用例。原因：要断言它必须把 MySQL binlog、river worker 与搜索索引串起来跑，而这一票改动的只是"置空 + 软删"这两条 SQL 语义（已由 `TestDeleteCategoryDetachesArticlesBeforeSoftDelete` 钉住），投影重算本身由 ADR-0009 的 golden 用例覆盖。要补的话落点应在 search 子域的集成用例，而不是分类仓储——把这个缺口写在这里，是为了不让它被误当成"已经测过"。

## 相关代码与文档

- 软删基类：`backend/internal/article/adapter/mysql/rows.go:28-33`（`BaseRow` 的 `gorm.DeletedAt`）
- 唯一索引与冗余计数列：`backend/internal/platform/mysql/ddl/schema.sql:204-215`（分类，`uk_category_user_title` 在 `:212`）· 同文件 `:572-587`（标签，`idx_tag_models_title` 在 `:584`，`article_count` 在 `:580`）
- 平台助手与先例：`backend/internal/platform/mysql/restore.go:29`（`RestoreOrCreateUnique`）· 点赞 `backend/internal/article/adapter/mysql/ports.go:170-189`
- 1062 翻译：`backend/internal/article/adapter/mysql/taxonomy_repo.go:164-172`（`taxonomySaveError`，分类与标签共用）· 共用写入形状 `:129-155`（`saveUniqueWithRestore`）· 两个仓储的 `Save`：`CategoryRepo.Save` `:103-113`、`TagRepo.Save` `:313-328`
- 用例：`backend/internal/article/app/subdomain.go:24`（`ListCategories`）· `:49`（`SaveCategory`）· `:99`（`DeleteCategories`）· `:143`（`ListTags`）· `:162`（`SaveTag`）· `:252`（`DeleteTags`）
- 外键与 CDC：`backend/internal/platform/mysql/ddl/schema.sql:132` · `backend/internal/platform/cdc/river/projection_replay.go:146`
- 术语：[GLOSSARY · 标签 / 分类](../../GLOSSARY.md)

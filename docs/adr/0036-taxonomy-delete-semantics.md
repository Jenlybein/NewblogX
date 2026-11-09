# ADR-0036：标签与分类的删除语义（软删行占住唯一键、恢复与停用）

- 状态：已接受（本批 #320–#325 合并后转「已交付」）
- 日期：2026-11-09
- 来源：用户 2026-11-09 的批量决策（[#319](https://github.com/Jenlybein/NewblogX/issues/319) Implementation Decisions）· 落地票 [#322](https://github.com/Jenlybein/NewblogX/issues/322) / [#325](https://github.com/Jenlybein/NewblogX/issues/325) · 关联 [ADR-0009](./0009-es-sync-article-projection-only.md)（分类置空经 CDC 自动重算搜索投影）· 术语见 [GLOSSARY · 标签 / 分类](../../GLOSSARY.md)

## 背景与问题

分类与标签都是**软删**实体：行上带 `deleted_at`，删除走 GORM 软删（`backend/internal/article/adapter/mysql/rows.go:28-33` 的 `BaseRow`）。但库里的唯一索引不认识软删：

- 分类 `category_models`：`UNIQUE KEY uk_category_user_title (user_id, title)`（`backend/internal/platform/mysql/ddl/schema.sql:212`）。
- 标签 `tag_models`：`UNIQUE KEY idx_tag_models_title (title)`（同文件 `:584`；名字带 `idx_` 前缀，但它**是唯一键**）。

于是"删过的名字被永久占用"：查重（GORM 模型查询，自动带 `deleted_at IS NULL`）说没重复，INSERT 却撞 1062。今天还没人踩到，只因为个人端还不能删分类——分类管理一上线它立刻会爆。

MySQL 没有部分索引 / 条件唯一索引，本批又是零 DDL（不改既有表的列与索引），所以"只对活行唯一"在库层面表达不出来。**"删过的名字能不能重用"必须在应用层显式决定**，而不是留给下一个人偶遇。

第二个问题在标签侧：标签被删是一刀切，还是与"停用"分级？"一律允许删除"与"停用为主"能达成的效果不同，代价也不同。

## 决策

1. **删除保持软删，名字可以重用；重用的方式是"先恢复、再新建"。** 凡"唯一键 + 软删"的实体，创建时都走平台助手 `RestoreOrCreateUnique`（`backend/internal/platform/mysql/restore.go:29`）：先按匹配键找 `deleted_at IS NOT NULL` 的行并恢复（清 `deleted_at`、写回本次字段值、**保留原 ID**），找不到才插入。分类的匹配键是 `(user_id, title)`；并发下"唯一键冲突"等价于"已存在"，不算错误。**本批落地的是分类创建**，标签侧的同名重建仍是已知遗留（见代价），补的时候照这条走。
2. **恢复保留原 ID，调用方不得依赖"新建必然有新 ID"。** 前端建完重拉列表，不拿返回的 ID 做后续写入。
3. **删除分类必须在同一事务内先解除文章归属、再软删分类。** 文章的分类字段置空（`article_models.category_id` 上有外键指向分类表，`backend/internal/platform/mysql/ddl/schema.sql:132`，但软删下外键永远不会触发，救不了这条引用），置空经 binlog 触发该文章的搜索投影重算（`backend/internal/platform/cdc/river/projection_replay.go:146`），不需要额外补偿任务。
4. **标签：停用是常规动作，删除只对零引用开放。** 停用后它不进可选列表（`Tag.Usable` / `FilterUsable`，`backend/internal/article/domain/taxonomy/taxonomy.go:205-239`），已打过它的文章不受影响；删除前按关联表**实时**统计引用数，被引用时返回可读错误并提示改用停用，不采信冗余计数列 `tag_models.article_count`（`backend/internal/platform/mysql/ddl/schema.sql:580`）。
5. **唯一键冲突一律翻译成领域可读文案。** 分类侧已有 `categorySaveError`（`backend/internal/article/adapter/mysql/taxonomy_repo.go:114-129`），标签侧必须补同等形状的一条；不允许把 1062 原文透传成 500。

## 理由与已否决方案

- **否决"改成物理删除"**：物理删除确实能立刻腾出名字，但代价有三条——① 全仓软删约定被破一个口子，这些表的 `deleted_at` 退化成死列，"删除还能恢复"的能力丢失；② 不可逆：误删一个分类时要连着把 N 篇文章的归属一起写进同一次事务，没有回退路径；③ binlog 的 delete 事件与投影的重算假设会跟着变，CDC 侧要再审一遍。改为"恢复"只多一次 UPDATE，不动 schema、不动 CDC。
- **否决"删除时给标题加脏后缀"**（如 `旧名#deleted-<ts>`）：名字能让出来，但把脏数据写进了标题这一列——列表、审计、导出读到的都是脏名，恢复时还得再解析一遍后缀才能还原；后缀规则一旦改口径，历史行再也解释不清。恢复方案下标题始终是用户写的那一个。
- **否决"删除时把标题置空（NULL）"**：MySQL 唯一索引对 NULL 不判重，技术上也能腾出名字，但同样**不可逆地**毁掉"这个名字以前存在过"的信息，恢复只能得到一条无名行。
- **否决"标签一律允许删除（含被引用）"**：删掉一个被 30 篇文章使用的标签，等于一次性从 30 篇公开文章上摘掉一个主题，作者既没有预期也不会收到通知；而"让它不再可选"这个真实诉求用停用就能达到，且停用可逆。删除只留给零引用的垃圾条目（拼错的、空壳的），那时爆炸半径是 0。
- **否决"用冗余计数列判断能不能删"**：`tag_models.article_count` 可能过期，一次"看起来零引用"的删除会抹掉真实引用。按关联表实时统计是唯一可信的判据。

## 代价 / 不做什么

- **软删行一直留在表里**，包括"恢复过又删掉"的。本批不做一次性清理脚本（#319 明确不做）：新语义下同名创建会自动恢复，清理不再必要，但表会缓慢变大。
- **恢复的语义是"这条以前存在过"**：恢复出来的行继承旧 ID 与旧的创建时间，把它当"全新的一条"来用（例如按 ID 大小当创建顺序）会看到旧位置。
- **标签侧本批只补 1062 的可读文案，没有做恢复**：一个被删除的标签名仍占着唯一键，重建同名会得到"名称重复"，而不是被恢复。原因是标签删除只对零引用开放、本批不接受"删了就要能重建同名"的诉求。后续若要修，走决策 1 的同一个助手，**不要**改标题。
- **不给分类加隐私开关、不做多级分类与排序**（要加列，违反零 DDL）；**不做标签合并 / 批量重命名 / 批量迁移**（#319 out of scope）。
- **停用标签不会因为"改一下描述"被悄悄放回可选**：编辑是读—改—写，没传启用状态就保持原状。

## 当前生效约束

1. 分类的创建必须走 `RestoreOrCreateUnique`（或同语义的"先恢复再插入"）；禁止裸 INSERT 之后只靠 1062 兜底。标签侧同名重建在本批仍是已知遗留（见代价），补的时候用同一个助手，禁止改成"给标题加后缀"。
2. 恢复不得更换 ID。
3. 删除分类 = 解除文章归属 + 软删分类，两步在同一事务内完成；先解除归属、后删分类，顺序不得颠倒。
4. 标签删除前必须按关联表实时统计引用数；被引用时拒绝并提示改用停用。
5. 唯一键冲突不得以 500 或数据库原文的形式出现在响应里。
6. 停用状态只能由显式的启用动作改回。

## 相关代码与文档

- 软删基类：`backend/internal/article/adapter/mysql/rows.go:28-33`（`BaseRow` 的 `gorm.DeletedAt`）
- 唯一索引与冗余计数列：`backend/internal/platform/mysql/ddl/schema.sql:204-215`（分类，`uk_category_user_title` 在 `:212`）· 同文件 `:572-587`（标签，`idx_tag_models_title` 在 `:584`，`article_count` 在 `:580`）
- 平台助手与先例：`backend/internal/platform/mysql/restore.go:29`（`RestoreOrCreateUnique`）· 点赞 `backend/internal/article/adapter/mysql/ports.go:170-189`
- 1062 翻译：`backend/internal/article/adapter/mysql/taxonomy_repo.go:114-129`（`categorySaveError`）；标签侧同文件 `TagRepo.Save`（`:206-231`）尚无等价翻译
- 用例：`backend/internal/article/app/subdomain.go:21`（`ListCategories`）· `:41`（`SaveCategory`）· `:82`（`DeleteCategories`）· `:117`（`ListTags`）· `:128`（`SaveTag`）· `:181`（`DeleteTags`）
- 外键与 CDC：`backend/internal/platform/mysql/ddl/schema.sql:132` · `backend/internal/platform/cdc/river/projection_replay.go:146`
- 术语：[GLOSSARY · 标签 / 分类](../../GLOSSARY.md)

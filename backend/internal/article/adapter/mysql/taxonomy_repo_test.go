package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/taxonomy"
	"myblogx/internal/kernel/ctype"
	"myblogx/test/testutil"
)

// TestSaveCategoryDuplicateTitleReturnsReadableError 钉住唯一键冲突的翻译。
//
// 场景：应用层先做 ExistsTitle 查重，但两次请求并发时仍可能同时通过查重、
// 最后由表上的 uk_category_user_title 拒绝后一次。如果适配层把
// gorm.ErrDuplicatedKey 原样返出去，handler 会把它当普通失败透传，
// 用户看到的就是 "UNIQUE constraint failed..." / MySQL 1062，而不是
// "名称重复"。
//
// 这里用与生产一致的表结构（composite unique index + TranslateError）复现冲突，
// 断言仓储把底层错误翻译成 taxonomy 的领域错误 —— 文案由 apperr 消息表提供。
func TestSaveCategoryDuplicateTitleReturnsReadableError(t *testing.T) {
	db := testutil.SetupSQLite(t, &categoryRow{})
	if err := db.Exec(
		"CREATE UNIQUE INDEX uk_category_user_title ON category_models(user_id, title)",
	).Error; err != nil {
		t.Fatalf("建唯一索引失败: %v", err)
	}

	repo := NewCategoryRepo(db)
	ctx := context.Background()
	userID := domain.UserID(ctype.NewID(100))

	first, err := taxonomy.NewCategory(domain.CategoryID(ctype.NewID(1)), userID, "前端")
	if err != nil {
		t.Fatalf("构造第一个分类失败: %v", err)
	}
	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("保存第一个分类失败: %v", err)
	}

	second, err := taxonomy.NewCategory(domain.CategoryID(ctype.NewID(2)), userID, "前端")
	if err != nil {
		t.Fatalf("构造第二个分类失败: %v", err)
	}
	if err := repo.Save(ctx, second); err != nil {
		if !errors.Is(err, taxonomy.ErrTitleDuplicated) {
			t.Fatalf("唯一键冲突必须翻译成领域错误，实际: %v", err)
		}
		if err.Error() != "名称重复" {
			t.Fatalf("面向用户的文案应为「名称重复」，实际: %q", err.Error())
		}
		return
	}
	t.Fatal("同名分类第二次保存应失败，实际成功了")
}

// TestSaveCategoryDuplicateTitleOnRenameReturnsReadableError 覆盖更新路径：
// 把已有分类改成一个同用户下已存在的标题，撞的也是同一个唯一索引。
func TestSaveCategoryDuplicateTitleOnRenameReturnsReadableError(t *testing.T) {
	db := testutil.SetupSQLite(t, &categoryRow{})
	if err := db.Exec(
		"CREATE UNIQUE INDEX uk_category_user_title ON category_models(user_id, title)",
	).Error; err != nil {
		t.Fatalf("建唯一索引失败: %v", err)
	}

	repo := NewCategoryRepo(db)
	ctx := context.Background()
	userID := domain.UserID(ctype.NewID(100))

	first, _ := taxonomy.NewCategory(domain.CategoryID(ctype.NewID(1)), userID, "前端")
	second, _ := taxonomy.NewCategory(domain.CategoryID(ctype.NewID(2)), userID, "后端")
	for _, c := range []*taxonomy.Category{first, second} {
		if err := repo.Save(ctx, c); err != nil {
			t.Fatalf("预置分类 %q 失败: %v", c.Title(), err)
		}
	}

	// 把 second 改名成 first 的标题。
	if err := second.Rename("前端"); err != nil {
		t.Fatalf("改名失败: %v", err)
	}
	if err := repo.Save(ctx, second); err != nil {
		if !errors.Is(err, taxonomy.ErrTitleDuplicated) {
			t.Fatalf("改名撞唯一键必须翻译成领域错误，实际: %v", err)
		}
		return
	}
	t.Fatal("改名成同名分类应失败，实际成功了")
}

// ── 标签仓储：唯一键翻译 × 软删后同名重建 × 实时引用数（票 322）──

// newTagRepoWithUniqueTitle 建一张与生产同构的 tag_models（title 上唯一索引）。
//
// 关联表与文章表一起建：列表的文章数是从 article_tag_models 现算的，
// 少了这两张表连"列出标签"都跑不起来。
func newTagRepoWithUniqueTitle(t *testing.T) (*TagRepo, *gorm.DB) {
	t.Helper()
	db := testutil.SetupSQLite(t, &tagRow{}, &articleRow{}, &articleTagRow{})
	if err := db.Exec("CREATE UNIQUE INDEX idx_tag_models_title ON tag_models(title)").Error; err != nil {
		t.Fatalf("建唯一索引失败: %v", err)
	}
	return NewTagRepo(db), db
}

func mustNewTag(t *testing.T, id uint64, title string) *taxonomy.Tag {
	t.Helper()
	tag, err := taxonomy.NewTag(domain.TagID(ctype.NewID(id)), title, domain.UserID(ctype.NewID(900)))
	if err != nil {
		t.Fatalf("构造标签 %q 失败: %v", title, err)
	}
	return tag
}

// TestSaveTagDuplicateTitleOnRenameReturnsReadableError 覆盖更新路径：
// 改名撞到另一个活标签的标题 → 领域错误 + 「名称重复」文案，而不是 MySQL 1062。
func TestSaveTagDuplicateTitleOnRenameReturnsReadableError(t *testing.T) {
	repo, _ := newTagRepoWithUniqueTitle(t)
	ctx := context.Background()

	goTag := mustNewTag(t, 1, "Go")
	sqlTag := mustNewTag(t, 2, "MySQL")
	for _, tag := range []*taxonomy.Tag{goTag, sqlTag} {
		if err := repo.Save(ctx, tag); err != nil {
			t.Fatalf("预置标签 %q 失败: %v", tag.Title(), err)
		}
	}

	if err := sqlTag.Rename("Go", 0, ""); err != nil {
		t.Fatalf("改名失败: %v", err)
	}
	err := repo.Save(ctx, sqlTag)
	if !errors.Is(err, taxonomy.ErrTitleDuplicated) {
		t.Fatalf("撞唯一键必须翻译成领域错误，实际: %v", err)
	}
	if err.Error() != "名称重复" {
		t.Fatalf("面向用户的文案应为「名称重复」，实际: %q", err.Error())
	}
}

// TestSaveTagNewPathWithExistingLiveTitleReturnsDuplicate 钉住 1062 在"新建路径"上的翻译。
//
// 场景：库里已有一条活标签 "Go"，另一个调用方（并发窗口里刚过查重的那一个，
// 或直接调仓储的用例）拿着**新 ID** 再存同名。RestoreOrCreateUnique 找不到可恢复的
// 软删行，插入撞唯一键，返回 (false, nil)。丢掉这个 bool 就会让 Save 返回 nil——
// 用户看到"保存成功"，库里却什么都没多出来；分类侧同场景返回 ErrTitleDuplicated。
func TestSaveTagNewPathWithExistingLiveTitleReturnsDuplicate(t *testing.T) {
	repo, db := newTagRepoWithUniqueTitle(t)
	ctx := context.Background()

	goTag := mustNewTag(t, 1, "Go")
	if err := repo.Save(ctx, goTag); err != nil {
		t.Fatalf("预置标签失败: %v", err)
	}

	dup := mustNewTag(t, 2, "Go")
	err := repo.Save(ctx, dup)
	if !errors.Is(err, taxonomy.ErrTitleDuplicated) {
		t.Fatalf("已有活行 + 新建路径的同名保存必须返回领域错误，实际: %v", err)
	}
	if err.Error() != "名称重复" {
		t.Fatalf("面向用户的文案应为「名称重复」，实际: %q", err.Error())
	}

	// 被拒的那次不能留下半条数据：复用不了软删行、插入也没成功，表里仍是一行。
	var n int64
	if err := db.Unscoped().Model(&tagRow{}).Count(&n).Error; err != nil {
		t.Fatalf("统计标签数失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("同名冲突后表里应仍只有 1 行，实际 %d 行", n)
	}
}

// TestSaveTagUnchangedUpdateIsNotADuplicate 钉住一个坑：MySQL 对"值没变"的
// UPDATE 返回 0 行变更。若拿 RowsAffected 判定"行不存在"，一次什么也没改的
// 编辑就会去 INSERT，然后撞上自己的唯一键，被翻成「名称重复」。
func TestSaveTagUnchangedUpdateIsNotADuplicate(t *testing.T) {
	repo, db := newTagRepoWithUniqueTitle(t)
	ctx := context.Background()

	tag := mustNewTag(t, 1, "Go")
	if err := repo.Save(ctx, tag); err != nil {
		t.Fatalf("新建标签失败: %v", err)
	}
	// 原样再存一次（后台编辑弹窗"只点了保存"的情形）。
	if err := repo.Save(ctx, tag); err != nil {
		t.Fatalf("原样保存不应报错，实际: %v", err)
	}
	var n int64
	if err := db.Model(&tagRow{}).Count(&n).Error; err != nil {
		t.Fatalf("统计标签数失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("不应多出一行，实际 %d 行", n)
	}
}

// TestSaveTagRecreatesAfterSoftDelete 是"删过的名字还能再建"。
//
// title 上的唯一索引不含 deleted_at，而删除是软删：裸 INSERT 会让重建同名
// 永远得到 1062。房规是恢复那条软删行（保留原 ID），与点赞 / 收藏一致。
func TestSaveTagRecreatesAfterSoftDelete(t *testing.T) {
	repo, db := newTagRepoWithUniqueTitle(t)
	ctx := context.Background()

	old := mustNewTag(t, 1, "Go")
	if err := repo.Save(ctx, old); err != nil {
		t.Fatalf("新建标签失败: %v", err)
	}
	if err := repo.Delete(ctx, old.ID()); err != nil {
		t.Fatalf("软删标签失败: %v", err)
	}
	// 删除是软删：行还在，只是 deleted_at 有值。
	if list, err := repo.List(ctx, true); err != nil || len(list) != 0 {
		t.Fatalf("软删后列表应为空，实际 %d 条 err=%v", len(list), err)
	}

	reborn := mustNewTag(t, 2, "Go")
	if err := reborn.Rename("Go", 5, "重建后的描述"); err != nil {
		t.Fatalf("设置字段失败: %v", err)
	}
	if err := repo.Save(ctx, reborn); err != nil {
		t.Fatalf("软删后重建同名标签失败（唯一键没走恢复）: %v", err)
	}

	list, err := repo.List(ctx, true)
	if err != nil {
		t.Fatalf("列出标签失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("恢复应复用那一条，列表里只该有一条，实际 %d 条", len(list))
	}
	if list[0].ID() != old.ID() {
		t.Fatalf("恢复应保留原 ID %s，实际 %s", old.ID(), list[0].ID())
	}
	if list[0].Sort() != 5 || list[0].Description() != "重建后的描述" {
		t.Fatalf("恢复应带上这次写入的字段，实际 sort=%d desc=%q",
			list[0].Sort(), list[0].Description())
	}
	// 落库只有一行（软删行被复用，而不是又插一条）。
	var total int64
	if err := db.Unscoped().Model(&tagRow{}).Count(&total).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("表里应只有一条（复用软删行），实际 %d 条", total)
	}
}

// TestTagArticleRefCountsComeFromJoinTable 钉住"文章数按关联表实时统计"。
//
// tag_models.article_count 没有维护者（真实数据里恒为 0），所以列表与删除门禁
// 都必须从 article_tag_models 现算；已软删的文章不计入引用。
func TestTagArticleRefCountsComeFromJoinTable(t *testing.T) {
	db := testutil.SetupSQLite(t, &tagRow{}, &articleRow{}, &articleTagRow{})
	if err := db.Exec("CREATE UNIQUE INDEX idx_tag_models_title ON tag_models(title)").Error; err != nil {
		t.Fatalf("建唯一索引失败: %v", err)
	}
	repo := NewTagRepo(db)
	ctx := context.Background()

	tag := mustNewTag(t, 1, "Go")
	if err := repo.Save(ctx, tag); err != nil {
		t.Fatalf("新建标签失败: %v", err)
	}
	// 冗余列故意写成 99：任何"读那一列"的实现都会被抓出来。
	if err := db.Exec("UPDATE tag_models SET article_count = 99 WHERE id = ?", ctype.ID(tag.ID())).Error; err != nil {
		t.Fatalf("写冗余列失败: %v", err)
	}

	for _, id := range []uint64{11, 12, 13} {
		if err := db.Create(&articleRow{BaseRow: BaseRow{ID: ctype.NewID(id)}}).Error; err != nil {
			t.Fatalf("预置文章 %d 失败: %v", id, err)
		}
		if err := db.Create(&articleTagRow{
			BaseRow:   BaseRow{ID: ctype.NewID(1000 + id)},
			ArticleID: ctype.NewID(id),
			TagID:     ctype.ID(tag.ID()),
		}).Error; err != nil {
			t.Fatalf("预置文章标签关联失败: %v", err)
		}
	}
	// 再给另一条已删文章挂同一个标签：文档已删，不该算引用。
	if err := db.Create(&articleRow{BaseRow: BaseRow{ID: ctype.NewID(14)}}).Error; err != nil {
		t.Fatalf("预置已删文章失败: %v", err)
	}
	if err := db.Delete(&articleRow{BaseRow: BaseRow{ID: ctype.NewID(14)}}).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}
	if err := db.Create(&articleTagRow{
		BaseRow:   BaseRow{ID: ctype.NewID(1014)},
		ArticleID: ctype.NewID(14),
		TagID:     ctype.ID(tag.ID()),
	}).Error; err != nil {
		t.Fatalf("预置已删文章的标签关联失败: %v", err)
	}

	list, err := repo.List(ctx, true)
	if err != nil {
		t.Fatalf("列出标签失败: %v", err)
	}
	if len(list) != 1 || list[0].ArticleCount() != 3 {
		t.Fatalf("列表里的文章数应按关联表实时统计为 3，实际 %d 条 / %d",
			len(list), list[0].ArticleCount())
	}

	refs, err := repo.CountArticleRefs(ctx, tag.ID())
	if err != nil {
		t.Fatalf("统计引用失败: %v", err)
	}
	if refs != 3 {
		t.Fatalf("引用数应为 3（已删文章不算），实际 %d", refs)
	}

	// 没有任何关联的标签是 0，删除门禁据此放行。
	other := mustNewTag(t, 2, "MySQL")
	if err := repo.Save(ctx, other); err != nil {
		t.Fatalf("新建第二个标签失败: %v", err)
	}
	if refs, err := repo.CountArticleRefs(ctx, other.ID()); err != nil || refs != 0 {
		t.Fatalf("零引用标签应为 0，实际 %d err=%v", refs, err)
	}
}

// ── 分类仓储：软删后同名重建 × 删除时解除文章归属（票 325）──

// newCategoryRepoWithUniqueTitle 建一张与生产同构的 category_models
// （(user_id, title) 上唯一索引），文章表一起建：删除分类要连带清文章的分类字段。
func newCategoryRepoWithUniqueTitle(t *testing.T) (*CategoryRepo, *gorm.DB) {
	t.Helper()
	// user_models 一起建：ListByUser 会 LEFT JOIN 出作者昵称 / 头像。
	db := testutil.SetupSQLite(t, &categoryRow{}, &articleRow{}, &userRow{})
	if err := db.Exec(
		"CREATE UNIQUE INDEX uk_category_user_title ON category_models(user_id, title)",
	).Error; err != nil {
		t.Fatalf("建唯一索引失败: %v", err)
	}
	return NewCategoryRepo(db), db
}

func mustNewCategory(t *testing.T, id uint64, userID uint64, title string) *taxonomy.Category {
	t.Helper()
	c, err := taxonomy.NewCategory(
		domain.CategoryID(ctype.NewID(id)),
		domain.UserID(ctype.NewID(userID)),
		title,
	)
	if err != nil {
		t.Fatalf("构造分类 %q 失败: %v", title, err)
	}
	return c
}

// TestSaveCategoryRecreatesAfterSoftDelete 是票 325 的核心：
// "删过的名字还能再建"。
//
// (user_id, title) 上的唯一索引不含 deleted_at，而删除是软删：裸 INSERT 会让
// 重建同名永远得到 1062（查重按活行看说没重复，写进去却撞上软删行）。
// 房规是恢复那条软删行（清 deleted_at、写回本次字段、保留原 ID），与点赞 / 标签一致。
func TestSaveCategoryRecreatesAfterSoftDelete(t *testing.T) {
	repo, db := newCategoryRepoWithUniqueTitle(t)
	ctx := context.Background()

	old := mustNewCategory(t, 1, 100, "前端")
	if err := repo.Save(ctx, old); err != nil {
		t.Fatalf("新建分类失败: %v", err)
	}
	if err := repo.Delete(ctx, old.ID()); err != nil {
		t.Fatalf("软删分类失败: %v", err)
	}
	// 删除是软删：活行里查不到，行本身还在。
	if list, err := repo.ListByUser(ctx, old.UserID()); err != nil || len(list) != 0 {
		t.Fatalf("软删后列表应为空，实际 %d 条 err=%v", len(list), err)
	}

	reborn := mustNewCategory(t, 2, 100, "前端")
	if err := repo.Save(ctx, reborn); err != nil {
		t.Fatalf("软删后重建同名分类失败（唯一键没走恢复）: %v", err)
	}

	list, err := repo.ListByUser(ctx, old.UserID())
	if err != nil {
		t.Fatalf("列出分类失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("恢复应复用那一条，列表里只该有一条，实际 %d 条", len(list))
	}
	if list[0].ID() != old.ID() {
		t.Fatalf("恢复应保留原 ID %s，实际 %s", old.ID(), list[0].ID())
	}
	if list[0].Title() != "前端" {
		t.Fatalf("恢复出来的标题应为「前端」，实际 %q", list[0].Title())
	}
	// 落库只有一行：软删行被复用，而不是又插一条。
	var total int64
	if err := db.Unscoped().Model(&categoryRow{}).Count(&total).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("表里应只有一条（复用软删行），实际 %d 条", total)
	}
}

// TestRenameCategoryIntoSoftDeletedNameStillDuplicates 钉住 ADR-0036 的「已知限制」。
//
// 恢复只发生在**新建**路径。改名走 UPDATE：目标名字如果只被一条软删行占着，
// 唯一索引照样拒绝——用户看到「名称重复」，而列表里根本看不到那个名字。
// 这是"零 DDL + 保留软删行"的必然结果；把它写进 ADR 而不是假装没有，
// 并且用这条用例把行为钉死，免得日后被当成 bug 顺手改成"改名也去恢复"。
func TestRenameCategoryIntoSoftDeletedNameStillDuplicates(t *testing.T) {
	repo, db := newCategoryRepoWithUniqueTitle(t)
	ctx := context.Background()

	keep := mustNewCategory(t, 1, 100, "前端")
	gone := mustNewCategory(t, 2, 100, "后端")
	for _, c := range []*taxonomy.Category{keep, gone} {
		if err := repo.Save(ctx, c); err != nil {
			t.Fatalf("预置分类 %q 失败: %v", c.Title(), err)
		}
	}
	// 软删「后端」：名字从此只被一条软删行占着。
	if err := repo.Delete(ctx, gone.ID()); err != nil {
		t.Fatalf("软删分类失败: %v", err)
	}

	// 查重只看活行，所以它说"没有重复"——这也是用户会觉得莫名其妙的原因。
	if dup, err := repo.ExistsTitle(ctx, keep.UserID(), "后端", keep.ID()); err != nil || dup {
		t.Fatalf("活行查重应说没有重复，实际 dup=%v err=%v", dup, err)
	}
	if err := keep.Rename("后端"); err != nil {
		t.Fatalf("改名失败: %v", err)
	}
	if err := repo.Save(ctx, keep); !errors.Is(err, taxonomy.ErrTitleDuplicated) {
		t.Fatalf("改名成一个只被软删行占用的名字应报「名称重复」，实际: %v", err)
	}

	// 两条行都还在：没有任何一条被这次失败顺手恢复或删掉。
	var total int64
	if err := db.Unscoped().Model(&categoryRow{}).Count(&total).Error; err != nil {
		t.Fatalf("统计分类数失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("表里应仍是 2 行（1 活 + 1 软删），实际 %d 行", total)
	}
}

// TestSaveCategoryUnchangedUpdateIsNotADuplicate 钉住"值没变"的 UPDATE。
//
// MySQL 对值没变的 UPDATE 返回 0 行变更。若拿 RowsAffected 判定"行不存在"，
// 一次什么都没改的保存就会去 INSERT，然后撞上自己那行、被翻成「名称重复」。
func TestSaveCategoryUnchangedUpdateIsNotADuplicate(t *testing.T) {
	repo, db := newCategoryRepoWithUniqueTitle(t)
	ctx := context.Background()

	c := mustNewCategory(t, 1, 100, "前端")
	if err := repo.Save(ctx, c); err != nil {
		t.Fatalf("新建分类失败: %v", err)
	}
	// 原样再存一次（个人主页弹窗"只点了保存"的情形）。
	if err := repo.Save(ctx, c); err != nil {
		t.Fatalf("原样保存不应报错，实际: %v", err)
	}
	var n int64
	if err := db.Model(&categoryRow{}).Count(&n).Error; err != nil {
		t.Fatalf("统计分类数失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("不应多出一行，实际 %d 行", n)
	}
}

// TestDeleteCategoryDetachesArticlesBeforeSoftDelete 钉住删除的两步语义（ADR-0036 约束 3）：
// 先把挂在该分类下的文章置空、再软删分类；文章本身不消失，别的分类不受影响。
//
// article_models.category_id 上有外键指向分类表，但删除是软删，外键永远不触发，
// 不显式置空就会留下"指向一条已删分类"的文章——详情页仍会显示那个分类。
func TestDeleteCategoryDetachesArticlesBeforeSoftDelete(t *testing.T) {
	repo, db := newCategoryRepoWithUniqueTitle(t)
	ctx := context.Background()

	doomed := mustNewCategory(t, 1, 100, "要删掉的分类")
	kept := mustNewCategory(t, 2, 100, "留着的分类")
	for _, c := range []*taxonomy.Category{doomed, kept} {
		if err := repo.Save(ctx, c); err != nil {
			t.Fatalf("预置分类 %q 失败: %v", c.Title(), err)
		}
	}
	doomedID := ctype.ID(doomed.ID())
	keptID := ctype.ID(kept.ID())
	for _, row := range []*articleRow{
		{BaseRow: BaseRow{ID: ctype.NewID(11)}, CategoryID: &doomedID},
		{BaseRow: BaseRow{ID: ctype.NewID(12)}, CategoryID: &doomedID},
		{BaseRow: BaseRow{ID: ctype.NewID(13)}, CategoryID: &keptID},
		{BaseRow: BaseRow{ID: ctype.NewID(14)}}, // 本来就没分类
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("预置文章 %s 失败: %v", row.ID, err)
		}
	}
	// 已软删的文章也挂着这个分类：它不展示，但置空它是无害的，不该报错。
	deletedID := ctype.ID(ctype.NewID(15))
	if err := db.Create(&articleRow{BaseRow: BaseRow{ID: deletedID}, CategoryID: &doomedID}).Error; err != nil {
		t.Fatalf("预置已删文章失败: %v", err)
	}
	if err := db.Delete(&articleRow{BaseRow: BaseRow{ID: deletedID}}).Error; err != nil {
		t.Fatalf("软删文章失败: %v", err)
	}

	// 打点：置空 category_id 不是"编辑文章"，不该刷 updated_at ——
	// 否则作者主页与文章详情页会集体显示一个新的"更新于"，而这只是分类被删了。
	frozen := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := db.Model(&articleRow{}).Where("id IN ?", []ctype.ID{ctype.NewID(11), ctype.NewID(12)}).
		UpdateColumn("updated_at", frozen).Error; err != nil {
		t.Fatalf("预置 updated_at 失败: %v", err)
	}

	if err := repo.Delete(ctx, doomed.ID()); err != nil {
		t.Fatalf("删除分类失败: %v", err)
	}

	// 文章还在，只是没有分类了。
	for _, id := range []uint64{11, 12} {
		var got *ctype.ID
		if err := db.Model(&articleRow{}).Select("category_id").
			Where("id = ?", ctype.NewID(id)).Scan(&got).Error; err != nil {
			t.Fatalf("读文章 %d 的分类失败: %v", id, err)
		}
		if got != nil {
			t.Fatalf("文章 %d 应变成未分类，实际 category_id=%s", id, got)
		}
	}

	// 决策（ADR-0036）：回收站里的文章**保留**分类归属。删除是软删、重建同名分类
	// 会恢复同一行，归属随之回来；这里顺手清掉反而丢掉了一条能回来的信息。
	var trashedCategory *ctype.ID
	if err := db.Unscoped().Model(&articleRow{}).Select("category_id").
		Where("id = ?", deletedID).Scan(&trashedCategory).Error; err != nil {
		t.Fatalf("读回收站文章的分类失败: %v", err)
	}
	if trashedCategory == nil || *trashedCategory != doomedID {
		t.Fatalf("回收站里的文章应保留分类归属，实际 %v", trashedCategory)
	}

	// 解绑不改文章的"更新于"。
	for _, id := range []uint64{11, 12} {
		var got time.Time
		if err := db.Model(&articleRow{}).Select("updated_at").
			Where("id = ?", ctype.NewID(id)).Scan(&got).Error; err != nil {
			t.Fatalf("读文章 %d 的 updated_at 失败: %v", id, err)
		}
		if !got.UTC().Equal(frozen) {
			t.Fatalf("文章 %d 的 updated_at 不该被改，期望 %s，实际 %s", id, frozen, got.UTC())
		}
	}
	// 别的分类下的文章不受影响。
	var keptCategory *ctype.ID
	if err := db.Model(&articleRow{}).Select("category_id").
		Where("id = ?", ctype.NewID(13)).Scan(&keptCategory).Error; err != nil {
		t.Fatalf("读文章 13 的分类失败: %v", err)
	}
	if keptCategory == nil || *keptCategory != keptID {
		t.Fatalf("别的分类下的文章不该被置空，实际 %v", keptCategory)
	}
	// 分类本身是软删：行还在。
	var live int64
	if err := db.Model(&categoryRow{}).Where("id = ?", doomedID).Count(&live).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if live != 0 {
		t.Fatalf("分类应已被软删，实际仍有 %d 条活行", live)
	}
	var total int64
	if err := db.Unscoped().Model(&categoryRow{}).Where("id = ?", doomedID).Count(&total).Error; err != nil {
		t.Fatalf("统计软删行失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("删除应为软删（行保留），实际 %d 条", total)
	}
}

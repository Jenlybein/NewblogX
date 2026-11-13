//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/domain/grouping"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"

	"gorm.io/gorm"
)

// 本文件守 ADR-0019 的四张新表。都必须在真 MySQL 上跑：
// 唯一键、外键、"删分组把条目退回未分组"都是 SQL 语义，内存 fake 无法证伪。

// migrateGroups 建出分组相关表：条目表有指向实体主键的外键，
// 所以实体主键表必须先存在。
func migrateGroups(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db,
		schema.UserCore, schema.ArticleCore,
		schema.Collection, schema.Project,
	)
	return db
}

// seedGroupCollection 插一个合集：条目表的外键指向它，不先建会撞 FK。
func seedGroupCollection(t *testing.T, db *gorm.DB, id, creatorID uint64, at time.Time) {
	t.Helper()
	row := schema.CollectionRow{
		BaseRow:     schema.BaseRow{ID: ctype.NewID(id), CreatedAt: at, UpdatedAt: at},
		Title:       "合集",
		Description: "描述",
		Cover:       "",
		IsDisplay:   true,
		CreatorID:   ctype.NewID(creatorID),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("插入合集 %d 失败: %v", id, err)
	}
}

// itemIDGen 发递增的条目行 ID：归属表主键唯一，固定值会撞 uk。
func itemIDGen(start uint64) func() ctype.ID {
	var n uint64
	return func() ctype.ID {
		n++
		return ctype.NewID(start + n)
	}
}
// TestGroupTablesAreRegistered 确认四张表真的建出来了（表清单一致性由 testkit 守，
// 这里守"能写能读"）。
func TestGroupTablesAreRegistered(t *testing.T) {
	db := migrateGroups(t)
	for _, table := range []string{
		"collection_group_models", "collection_group_item_models",
		"project_group_models", "project_group_item_models",
	} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("表 %s 没有建出来", table)
		}
	}
}

// TestReplaceOrderWritesGroupAndItems 锁住整树覆盖的落库形状。
func TestReplaceOrderWritesGroupAndItems(t *testing.T) {
	db := migrateGroups(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedGroupCollection(t, db, 8_001, 1, at)
	seedGroupCollection(t, db, 8_002, 1, at)

	repo := mysql.NewCollectionGroupRepo(db, itemIDGen(9_001))
	ctx := context.Background()

	g1 := grouping.ID(ctype.NewID(7_001))
	g2 := grouping.ID(ctype.NewID(7_002))
	for _, g := range []struct {
		id    grouping.ID
		title string
		order int
	}{{g1, "第一组", 1}, {g2, "第二组", 2}} {
		built, err := grouping.New(grouping.NewParams{ID: g.id, Title: g.title, SortOrder: g.order, Now: at})
		if err != nil {
			t.Fatalf("构造分组失败: %v", err)
		}
		if err := repo.SaveGroup(ctx, built); err != nil {
			t.Fatalf("保存分组失败: %v", err)
		}
	}

	tree := grouping.TreeOrder{
		Groups: []grouping.GroupOrder{
			{ID: g2, Items: []ctype.ID{ctype.NewID(8_002)}},
			{ID: g1, Items: []ctype.ID{ctype.NewID(8_001)}},
		},
	}
	if err := repo.ReplaceOrder(ctx, tree, at); err != nil {
		t.Fatalf("整树覆盖失败: %v", err)
	}

	groups, err := repo.ListGroups(ctx)
	if err != nil {
		t.Fatalf("列分组失败: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("应有 2 个分组，实际 %d", len(groups))
	}
	// 顺序以提交为准：g2 拿到 1。
	if groups[0].ID() != g2 {
		t.Fatalf("第一个分组应是 g2，实际 %s", groups[0].ID())
	}

	assignments, err := repo.ListAssignments(ctx)
	if err != nil {
		t.Fatalf("列归属失败: %v", err)
	}
	if len(assignments) != 2 {
		t.Fatalf("应有 2 条归属，实际 %d", len(assignments))
	}

	// 幂等：同一份输入再提交一次，行数不变。
	if err := repo.ReplaceOrder(ctx, tree, at); err != nil {
		t.Fatalf("重复覆盖应幂等，实际报错: %v", err)
	}
	assignments, err = repo.ListAssignments(ctx)
	if err != nil {
		t.Fatalf("列归属失败: %v", err)
	}
	if len(assignments) != 2 {
		t.Fatalf("重复覆盖不该产生新行，实际 %d 条", len(assignments))
	}
}

// TestDeleteGroupReturnsItemsToUngrouped 是这票的安全边界：
// 删分组只删编排，合集本身一行不动，条目退回未分组。
func TestDeleteGroupReturnsItemsToUngrouped(t *testing.T) {
	db := migrateGroups(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	seedGroupCollection(t, db, 8_101, 1, at)

	repo := mysql.NewCollectionGroupRepo(db, itemIDGen(9_101))
	ctx := context.Background()

	g := grouping.ID(ctype.NewID(7_101))
	built, err := grouping.New(grouping.NewParams{ID: g, Title: "待删", SortOrder: 1, Now: at})
	if err != nil {
		t.Fatalf("构造分组失败: %v", err)
	}
	if err := repo.SaveGroup(ctx, built); err != nil {
		t.Fatalf("保存分组失败: %v", err)
	}
	if err := repo.ReplaceOrder(ctx, grouping.TreeOrder{
		Groups: []grouping.GroupOrder{{ID: g, Items: []ctype.ID{ctype.NewID(8_101)}}},
	}, at); err != nil {
		t.Fatalf("整树覆盖失败: %v", err)
	}

	if err := repo.DeleteGroup(ctx, g, at); err != nil {
		t.Fatalf("删除分组失败: %v", err)
	}

	groups, err := repo.ListGroups(ctx)
	if err != nil {
		t.Fatalf("列分组失败: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("分组应已软删，实际还剩 %d 个", len(groups))
	}

	assignments, err := repo.ListAssignments(ctx)
	if err != nil {
		t.Fatalf("列归属失败: %v", err)
	}
	if len(assignments) != 1 {
		t.Fatalf("条目应仍在（退回未分组），实际 %d 条", len(assignments))
	}
	if !assignments[0].GroupID.IsZero() {
		t.Fatalf("条目应退回未分组，实际仍在 %s", assignments[0].GroupID)
	}

	// 合集本身一行不动。
	var alive int64
	if err := db.Table("collection_models").Where("id = ? AND deleted_at IS NULL", ctype.NewID(8_101)).
		Count(&alive).Error; err != nil {
		t.Fatalf("统计合集失败: %v", err)
	}
	if alive != 1 {
		t.Fatalf("删分组不得动合集本身，实际剩下 %d 行", alive)
	}
}

// TestProjectGroupIsSeparateFromCollection 锁住两套表互不干扰：
// 一篇文章可以同时进两个实体，两个分组也不能串。
func TestProjectGroupIsSeparateFromCollection(t *testing.T) {
	db := migrateGroups(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	collectionRepo := mysql.NewCollectionGroupRepo(db, itemIDGen(9_201))
	projectRepo := mysql.NewProjectGroupRepo(db, itemIDGen(9_202))
	ctx := context.Background()

	g := grouping.ID(ctype.NewID(7_201))
	built, err := grouping.New(grouping.NewParams{ID: g, Title: "只进合集", SortOrder: 1, Now: at})
	if err != nil {
		t.Fatalf("构造分组失败: %v", err)
	}
	if err := collectionRepo.SaveGroup(ctx, built); err != nil {
		t.Fatalf("保存分组失败: %v", err)
	}

	projectGroups, err := projectRepo.ListGroups(ctx)
	if err != nil {
		t.Fatalf("列项目分组失败: %v", err)
	}
	if len(projectGroups) != 0 {
		t.Fatalf("合集分组不该出现在项目表里，实际 %d 个", len(projectGroups))
	}
}

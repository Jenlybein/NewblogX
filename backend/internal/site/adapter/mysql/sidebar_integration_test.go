//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	sitemysql "myblogx/internal/site/adapter/mysql"
	"myblogx/internal/site/domain"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// TestSidebarRepoOrderingAndReplace 是真 MySQL 下的右栏卡片契约：
// 读侧只返回启用行且按 sort_order 升序；整表替换后行数与顺序一起变。
//
// 排序号在 SQL 里生效（不是靠调用方排序），这条用例钉住的就是"后台改完顺序、
// 读者站刷新即生效"的那条链路。
func TestSidebarRepoOrderingAndReplace(t *testing.T) {
	t.Parallel()

	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.Site)

	base := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	rows := []schema.SidebarCardRow{
		{
			BaseRow:   schema.BaseRow{ID: ctype.NewID(201), CreatedAt: base, UpdatedAt: base},
			Kind:      string(domain.CardKindTagCloud),
			Title:     "标签云",
			SortOrder: 2,
			Enabled:   true,
		},
		{
			BaseRow:   schema.BaseRow{ID: ctype.NewID(202), CreatedAt: base, UpdatedAt: base},
			Kind:      string(domain.CardKindHotArticles),
			Title:     "热门文章",
			SortOrder: 1,
			Enabled:   true,
		},
		{
			BaseRow:   schema.BaseRow{ID: ctype.NewID(203), CreatedAt: base, UpdatedAt: base},
			Kind:      string(domain.CardKindLatestComments),
			Title:     "停用的评论",
			SortOrder: 0,
			Enabled:   false,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("预置右栏卡片失败: %v", err)
	}

	repo := sitemysql.NewSidebar(db)
	got, err := repo.ListEnabled(context.Background())
	if err != nil {
		t.Fatalf("读取启用卡片失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("停用卡不该返回：期望 2 张，实际 %d", len(got))
	}
	if got[0].Kind != domain.CardKindHotArticles || got[1].Kind != domain.CardKindTagCloud {
		t.Fatalf("启用卡应按 sort_order 升序返回: %+v", got)
	}

	all, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatalf("读取全部卡片失败: %v", err)
	}
	if len(all) != 3 || all[0].Kind != domain.CardKindLatestComments {
		t.Fatalf("管理端应包含停用卡、顺序从 0 开始: %+v", all)
	}

	// 整表替换：空提交应把表清空（读者侧随后回退到旧右栏）。
	if err := repo.Replace(context.Background(), nil); err != nil {
		t.Fatalf("清空卡片失败: %v", err)
	}
	if remaining, err := repo.ListAll(context.Background()); err != nil || len(remaining) != 0 {
		t.Fatalf("清空后不应有行: rows=%+v err=%v", remaining, err)
	}

	// 重新写入两张卡：顺序由 sort_order 决定，与插入顺序无关。
	err = repo.Replace(context.Background(), []domain.SidebarCard{
		{Kind: domain.CardKindTagCloud, Title: "标签云", SortOrder: 1, Enabled: true},
		{Kind: domain.CardKindHotArticles, Title: "热门文章", SortOrder: 0, Enabled: true},
	})
	if err != nil {
		t.Fatalf("写入卡片失败: %v", err)
	}
	got, err = repo.ListEnabled(context.Background())
	if err != nil {
		t.Fatalf("替换后读取卡片失败: %v", err)
	}
	if len(got) != 2 || got[0].Kind != domain.CardKindHotArticles {
		t.Fatalf("替换后的顺序应按 sort_order: %+v", got)
	}
	if got[0].ID.IsZero() || got[1].ID.IsZero() {
		t.Fatalf("替换应生成主键: %+v", got)
	}
}

package mysql_test

import (
	"context"
	"testing"

	sitemysql "myblogx/internal/site/adapter/mysql"
	"myblogx/internal/site/domain"
	"myblogx/internal/testkit"
	"myblogx/internal/testkit/schema"

	"myblogx/internal/kernel/ctype"
)

// TestBannerDeleteOnlyTouchesBannerTable 证明删轮播只动 banner_models 一行。
//
// 取景体系已全量退场，取景表也从测试建模里消失（`schema.Site` 不再含它）。
// 如果删轮播的路径还残留"顺手清取景孤儿行"的跨表删除，这里会因为目标表不存在
// 直接报错——这正是本用例要拦的回归。
func TestBannerDeleteOnlyTouchesBannerTable(t *testing.T) {
	db := testkit.NewSQLite(t, schema.Models(schema.Site)...)
	repo := sitemysql.NewBanner(db)
	ctx := context.Background()

	for _, id := range []uint64{8001, 8002} {
		banner := domain.Banner{ID: ctype.NewID(id), Show: true, Cover: "/cover.png", Href: "/target"}
		if err := repo.Create(ctx, &banner); err != nil {
			t.Fatalf("创建轮播 %d 失败: %v", id, err)
		}
	}

	if _, err := repo.Delete(ctx, []uint64{8001}); err != nil {
		t.Fatalf("删除轮播失败（不应再去操作已下线的表）: %v", err)
	}

	if _, err := repo.Find(ctx, 8001); err == nil {
		t.Fatal("被删除的轮播仍能读到")
	}
	if _, err := repo.Find(ctx, 8002); err != nil {
		t.Fatalf("删除另一条轮播不该影响这一条: %v", err)
	}
}

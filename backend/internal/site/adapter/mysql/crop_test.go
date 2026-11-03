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

// 取景参数体系已在 #203 全量退场：本模块不再读写取景参数（没有 Load/Upsert、
// 也没有 attachCrops），表留到 #205 才物理下线。这里钉住留下来的那一小段：
// 删轮播时按 target_type=banner 真删它自己的孤儿行，不碰其它主体。

func TestBannerDeleteClearsOnlyItsOwnCropRows(t *testing.T) {
	db := testkit.NewSQLite(t, schema.Models(schema.Site)...)
	repo := sitemysql.NewBanner(db)
	ctx := context.Background()

	// 历史数据：同一个数值 id 在 banner 与 article 下各有一行（加取景时期写进去的）。
	for _, targetType := range []string{"banner", "article"} {
		if err := db.Create(&schema.PictureCropRow{
			TargetType: targetType,
			TargetID:   ctype.NewID(7001),
			FocusX:     0.4,
			FocusY:     0.6,
			Zoom:       1.3,
		}).Error; err != nil {
			t.Fatalf("预置 %s 取景行失败: %v", targetType, err)
		}
	}

	banner := domain.Banner{ID: ctype.NewID(7001), Show: true, Cover: "/cover.png", Href: "/target", Title: "轮播标题"}
	if err := repo.Create(ctx, &banner); err != nil {
		t.Fatalf("创建轮播失败: %v", err)
	}
	if _, err := repo.Delete(ctx, []uint64{7001}); err != nil {
		t.Fatalf("删除轮播失败: %v", err)
	}

	var left []schema.PictureCropRow
	if err := db.Where("target_id = ?", ctype.NewID(7001)).Find(&left).Error; err != nil {
		t.Fatalf("读回取景行失败: %v", err)
	}
	if len(left) != 1 || left[0].TargetType != "article" {
		t.Fatalf("删轮播只应清掉 target_type=banner 的行，实际剩 %+v", left)
	}
}

// 轮播仓储重建后仍照常读写标题等本表列（取景已是历史，不再参与）。
func TestBannerRepoRoundTripsTitle(t *testing.T) {
	db := testkit.NewSQLite(t, schema.Models(schema.Site)...)
	repo := sitemysql.NewBanner(db)
	ctx := context.Background()

	banner := domain.Banner{
		ID:    ctype.NewID(7002),
		Show:  true,
		Cover: "/cover.png",
		Href:  "/target",
		Title: "轮播标题",
	}
	if err := repo.Create(ctx, &banner); err != nil {
		t.Fatalf("创建轮播失败: %v", err)
	}

	got, err := repo.Find(ctx, 7002)
	if err != nil {
		t.Fatalf("读取轮播失败: %v", err)
	}
	if got.Title != "轮播标题" {
		t.Fatalf("标题没有读写回来：%q", got.Title)
	}
	if err := repo.Update(ctx, 7002, "新标题", "/cover2.png", "/target2", false); err != nil {
		t.Fatalf("更新轮播失败: %v", err)
	}
	got, err = repo.Find(ctx, 7002)
	if err != nil {
		t.Fatalf("更新后读取轮播失败: %v", err)
	}
	if got.Title != "新标题" || got.Cover != "/cover2.png" || got.Show {
		t.Fatalf("更新没有落库：%+v", got)
	}
}

// 未登记的主体类型一律拒绝：表里 target_type 没有枚举约束，写错会静默删错行。
func TestCropDeleteRejectsUnknownTargetType(t *testing.T) {
	db := testkit.NewSQLite(t, schema.Models(schema.Site)...)
	repo := sitemysql.NewCrop(db)
	ctx := context.Background()

	if err := repo.Delete(ctx, domain.CropTargetType("unknown"), []uint64{1}); err == nil {
		t.Fatal("未知 target_type 的删除应当被拒绝")
	}
}

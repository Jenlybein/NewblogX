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

// 通用取景表按 (target_type, target_id) 定位主体：同一个数值的 target_id
// 在轮播与文章下是两条互不覆盖的记录。这条用例钉住"表不写死轮播"这件事。
func TestCropRepoSeparatesTargetTypes(t *testing.T) {
	db := testkit.NewSQLite(t, schema.Models(schema.Site)...)
	repo := sitemysql.NewCrop(db)
	ctx := context.Background()

	const sameID = 9001
	bannerCrop := domain.Crop{FocusX: 0.2, FocusY: 0.8, Zoom: 1.5}
	articleCrop := domain.Crop{FocusX: 0.7, FocusY: 0.3, Zoom: 1.2}

	if err := repo.Upsert(ctx, domain.CropTargetBanner, sameID, bannerCrop); err != nil {
		t.Fatalf("写入轮播取景失败: %v", err)
	}
	if err := repo.Upsert(ctx, domain.CropTargetArticle, sameID, articleCrop); err != nil {
		t.Fatalf("写入文章取景失败: %v", err)
	}

	banners, err := repo.Load(ctx, domain.CropTargetBanner, []uint64{sameID, 1234})
	if err != nil {
		t.Fatalf("读轮播取景失败: %v", err)
	}
	if got := banners[sameID]; got != bannerCrop {
		t.Fatalf("轮播取景被覆盖了：want %+v got %+v", bannerCrop, got)
	}
	if _, ok := banners[1234]; ok {
		t.Fatal("没有取景记录的主体不应出现在返回 map 里")
	}

	articles, err := repo.Load(ctx, domain.CropTargetArticle, []uint64{sameID})
	if err != nil {
		t.Fatalf("读文章取景失败: %v", err)
	}
	if got := articles[sameID]; got != articleCrop {
		t.Fatalf("文章取景被覆盖了：want %+v got %+v", articleCrop, got)
	}

	// 覆盖轮播那一行不能碰同 ID 的文章行。
	updatedBanner := domain.Crop{FocusX: 0.1, FocusY: 0.1, Zoom: 1.1}
	if err := repo.Upsert(ctx, domain.CropTargetBanner, sameID, updatedBanner); err != nil {
		t.Fatalf("覆盖轮播取景失败: %v", err)
	}
	articles, err = repo.Load(ctx, domain.CropTargetArticle, []uint64{sameID})
	if err != nil {
		t.Fatalf("重新读文章取景失败: %v", err)
	}
	if got := articles[sameID]; got != articleCrop {
		t.Fatalf("覆盖轮播时把文章取景改掉了：want %+v got %+v", articleCrop, got)
	}

	// 删轮播那一行也不能碰文章行。
	if err := repo.Delete(ctx, domain.CropTargetBanner, []uint64{sameID}); err != nil {
		t.Fatalf("删除轮播取景失败: %v", err)
	}
	banners, err = repo.Load(ctx, domain.CropTargetBanner, []uint64{sameID})
	if err != nil {
		t.Fatalf("重读轮播取景失败: %v", err)
	}
	if _, ok := banners[sameID]; ok {
		t.Fatal("轮播取景应当已删除")
	}
	articles, err = repo.Load(ctx, domain.CropTargetArticle, []uint64{sameID})
	if err != nil {
		t.Fatalf("三读文章取景失败: %v", err)
	}
	if got := articles[sameID]; got != articleCrop {
		t.Fatalf("删轮播时把文章取景删掉了：want %+v got %+v", articleCrop, got)
	}
}

// 未知主体类型必须报错：ADR 明写 target_type 没有枚举约束，写错类型会静默
// 变成"没有取景"。仓储层在入口拒绝，避免把错类型写进表里。
func TestCropRepoRejectsUnknownTargetType(t *testing.T) {
	db := testkit.NewSQLite(t, schema.Models(schema.Site)...)
	repo := sitemysql.NewCrop(db)
	ctx := context.Background()

	if err := repo.Upsert(ctx, domain.CropTargetType("unknown"), 1, domain.DefaultCrop); err == nil {
		t.Fatal("未知 target_type 应当被拒绝")
	}
	if _, err := repo.Load(ctx, domain.CropTargetType("unknown"), []uint64{1}); err == nil {
		t.Fatal("未知 target_type 的读也应当被拒绝")
	}
}

// 轮播仓储重建后仍照常读写：标题列能落库读回，取景行落在通用表里，
// 且删除轮播只清它自己的取景行。
func TestBannerRepoRoundTripsTitleAndCrop(t *testing.T) {
	db := testkit.NewSQLite(t, schema.Models(schema.Site)...)
	repo := sitemysql.NewBanner(db)
	ctx := context.Background()

	banner := domain.Banner{
		ID:    ctype.NewID(7001),
		Show:  true,
		Cover: "/cover.png",
		Href:  "/target",
		Title: "轮播标题",
		Crop:  domain.Crop{FocusX: 0.25, FocusY: 0.75, Zoom: 1.8},
	}
	if err := repo.Create(ctx, &banner); err != nil {
		t.Fatalf("创建轮播失败: %v", err)
	}

	got, err := repo.Find(ctx, 7001)
	if err != nil {
		t.Fatalf("读取轮播失败: %v", err)
	}
	if got.Title != "轮播标题" {
		t.Fatalf("标题没有读写回来：%q", got.Title)
	}
	if got.Crop != banner.Crop {
		t.Fatalf("取景没有读写回来：want %+v got %+v", banner.Crop, got.Crop)
	}

	// 同一个数值 id 在文章主体下另有一条取景：删除轮播不能把它删掉。
	crops := sitemysql.NewCrop(db)
	articleCrop := domain.Crop{FocusX: 0.4, FocusY: 0.6, Zoom: 1.3}
	if err := crops.Upsert(ctx, domain.CropTargetArticle, 7001, articleCrop); err != nil {
		t.Fatalf("预置文章取景失败: %v", err)
	}
	if _, err := repo.Delete(ctx, []uint64{7001}); err != nil {
		t.Fatalf("删除轮播失败: %v", err)
	}
	left, err := crops.Load(ctx, domain.CropTargetArticle, []uint64{7001})
	if err != nil {
		t.Fatalf("读文章取景失败: %v", err)
	}
	if left[7001] != articleCrop {
		t.Fatalf("删轮播时误删了文章取景：%+v", left)
	}
	bannerLeft, err := crops.Load(ctx, domain.CropTargetBanner, []uint64{7001})
	if err != nil {
		t.Fatalf("重读轮播取景失败: %v", err)
	}
	if _, ok := bannerLeft[7001]; ok {
		t.Fatal("删除轮播后它自己的取景行应当一并清掉")
	}
}

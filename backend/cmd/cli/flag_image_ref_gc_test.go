package cli

import (
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"
	"myblogx/internal/testkit"
	"myblogx/internal/testkit/schema"

	"gorm.io/gorm"
)

// 本文件钉住一次性清理命令的双模语义：不带 -apply 只读、不写任何行；
// 带 -apply 才删孤儿引用。用例走内存 sqlite，不依赖 Docker。

func newImageRefGCDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testkit.NewSQLite(t, schema.Models(schema.ImageAssets, schema.Site)...)
}

func seedImageRefGCData(t *testing.T, db *gorm.DB) {
	t.Helper()
	at := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	alive := schema.BannerRow{
		BaseRow: schema.BaseRow{ID: ctype.NewID(1), CreatedAt: at, UpdatedAt: at},
		Show:    true,
		Cover:   "/cover.png",
	}
	if err := db.Create(&alive).Error; err != nil {
		t.Fatalf("造活轮播失败: %v", err)
	}
	deleted := schema.BannerRow{
		BaseRow: schema.BaseRow{ID: ctype.NewID(2), CreatedAt: at, UpdatedAt: at},
		Show:    true,
		Cover:   "/deleted.png",
	}
	if err := db.Create(&deleted).Error; err != nil {
		t.Fatalf("造软删轮播失败: %v", err)
	}
	if err := db.Model(&schema.BannerRow{}).Where("id = ?", ctype.NewID(2)).
		Update("deleted_at", at).Error; err != nil {
		t.Fatalf("软删轮播失败: %v", err)
	}

	refs := []schema.ImageRefRow{
		{BaseRow: schema.BaseRow{ID: ctype.NewID(11)}, ImageID: ctype.NewID(1), RefType: image_ref_enum.RefTypeBanner, OwnerID: ctype.NewID(1), Field: image_ref_enum.RefFieldBannerCover},
		{BaseRow: schema.BaseRow{ID: ctype.NewID(12)}, ImageID: ctype.NewID(1), RefType: image_ref_enum.RefTypeBanner, OwnerID: ctype.NewID(2), Field: image_ref_enum.RefFieldBannerCover},
		{BaseRow: schema.BaseRow{ID: ctype.NewID(13)}, ImageID: ctype.NewID(1), RefType: image_ref_enum.RefTypeBanner, OwnerID: ctype.NewID(999), Field: image_ref_enum.RefFieldBannerCover},
		{BaseRow: schema.BaseRow{ID: ctype.NewID(14)}, ImageID: ctype.NewID(1), RefType: image_ref_enum.RefTypeUser, OwnerID: ctype.NewID(999), Field: image_ref_enum.RefFieldUserAvatar},
	}
	if err := db.Omit("ImageModel").Create(&refs).Error; err != nil {
		t.Fatalf("造引用行失败: %v", err)
	}
}

func countImageRefs(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var total int64
	if err := db.Model(&schema.ImageRefRow{}).Count(&total).Error; err != nil {
		t.Fatalf("统计引用行失败: %v", err)
	}
	return total
}

// TestFlagImageRefGCOrphansDryRunDoesNotWrite 确认只读模式只报数、不落库。
func TestFlagImageRefGCOrphansDryRunDoesNotWrite(t *testing.T) {
	db := newImageRefGCDB(t)
	seedImageRefGCData(t, db)
	before := countImageRefs(t, db)

	if err := FlagImageRefGCOrphans(db, nil, false); err != nil {
		t.Fatalf("只读清单失败: %v", err)
	}
	if after := countImageRefs(t, db); after != before {
		t.Fatalf("只读模式不应写库：%d → %d", before, after)
	}
}

// TestFlagImageRefGCOrphansApplyDeletes 确认 -apply 才真正删除孤儿引用。
func TestFlagImageRefGCOrphansApplyDeletes(t *testing.T) {
	db := newImageRefGCDB(t)
	seedImageRefGCData(t, db)

	if err := FlagImageRefGCOrphans(db, nil, true); err != nil {
		t.Fatalf("清理孤儿引用失败: %v", err)
	}
	if got := countImageRefs(t, db); got != 2 {
		t.Fatalf("清理后应剩 2 行（活轮播 + 用户头像），实际 %d", got)
	}

	var left []schema.ImageRefRow
	if err := db.Order("id ASC").Find(&left).Error; err != nil {
		t.Fatalf("读剩余引用行失败: %v", err)
	}
	if left[0].RefType != image_ref_enum.RefTypeBanner || left[0].OwnerID.Uint64() != 1 {
		t.Fatalf("活轮播的引用行不应被删：%+v", left[0])
	}
	if left[1].RefType != image_ref_enum.RefTypeUser {
		t.Fatalf("别的 ref_type 不应被删：%+v", left[1])
	}
}

// TestFlagImageRefGCOrphansRejectsNilDB 确认没传库时给错误而不是 panic。
func TestFlagImageRefGCOrphansRejectsNilDB(t *testing.T) {
	if err := FlagImageRefGCOrphans(nil, nil, false); err == nil {
		t.Fatal("db 为 nil 时应当返回错误")
	}
}

package imageref_test

import (
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"
	"myblogx/internal/platform/cdc/imageref"
	"myblogx/internal/testkit"
	"myblogx/internal/testkit/schema"

	"gorm.io/gorm"
)

// 本文件钉住"重建轮播表后清理孤儿引用"的判定口径：ref_type 为轮播、
// 但对应轮播已经不存在（DROP 重建）或已软删的账本行要被识别出来，
// 且只读扫描绝不写库。用手边的内存 sqlite 跑，不依赖 Docker。

// newOrphanRefDB 建出账本与轮播两张表。
func newOrphanRefDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testkit.NewSQLite(t, schema.Models(schema.ImageAssets, schema.Site)...)
}

// seedOrphanBanner 放一行轮播；softDeleted 为真时填 deleted_at。
func seedOrphanBanner(t *testing.T, db *gorm.DB, id uint64, softDeleted bool) {
	t.Helper()
	at := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	row := schema.BannerRow{
		BaseRow: schema.BaseRow{ID: ctype.NewID(id), CreatedAt: at, UpdatedAt: at},
		Show:    true,
		Cover:   "/cover.png",
		Href:    "/href",
		Title:   "标题",
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("造轮播行失败: %v", err)
	}
	if softDeleted {
		if err := db.Model(&schema.BannerRow{}).Where("id = ?", ctype.NewID(id)).
			Update("deleted_at", at).Error; err != nil {
			t.Fatalf("软删轮播行失败: %v", err)
		}
	}
}

// seedOrphanRef 放一行图片引用账本。
func seedOrphanRef(t *testing.T, db *gorm.DB, id uint64, refType image_ref_enum.RefType, ownerID uint64) {
	t.Helper()
	row := schema.ImageRefRow{
		BaseRow: schema.BaseRow{ID: ctype.NewID(id)},
		ImageID: ctype.NewID(1),
		RefType: refType,
		OwnerID: ctype.NewID(ownerID),
		Field:   image_ref_enum.RefFieldBannerCover,
	}
	if err := db.Omit("ImageModel").Create(&row).Error; err != nil {
		t.Fatalf("造引用行失败: %v", err)
	}
}

// TestScanOrphanRefsFindsMissingAndSoftDeletedBanners 覆盖三种行：
// 活轮播（保留）、软删轮播（孤儿）、已 DROP 掉的轮播 id（孤儿）。
func TestScanOrphanRefsFindsMissingAndSoftDeletedBanners(t *testing.T) {
	db := newOrphanRefDB(t)
	seedOrphanBanner(t, db, 1, false)
	seedOrphanBanner(t, db, 2, true)

	seedOrphanRef(t, db, 11, image_ref_enum.RefTypeBanner, 1) // 活轮播，保留
	seedOrphanRef(t, db, 12, image_ref_enum.RefTypeBanner, 2) // 软删轮播，清理
	// 999 是"重建前存在、重建后消失"的轮播 id。
	seedOrphanRef(t, db, 13, image_ref_enum.RefTypeBanner, 999)
	// 别的 ref_type 不归本轮管。
	seedOrphanRef(t, db, 14, image_ref_enum.RefTypeUser, 999)

	before := countOrphanRefs(t, db)

	plan, err := imageref.ScanOrphanRefs(db, image_ref_enum.RefTypeBanner)
	if err != nil {
		t.Fatalf("扫描孤儿引用失败: %v", err)
	}
	if len(plan.OwnerIDs) != 2 {
		t.Fatalf("应当识别出 2 个孤儿主体，实际 %d：%v", len(plan.OwnerIDs), plan.OwnerIDs)
	}
	wantOwners := []uint64{2, 999}
	for i, want := range wantOwners {
		if got := plan.OwnerIDs[i].Uint64(); got != want {
			t.Fatalf("第 %d 个孤儿主体 want %d got %d（完整：%v）", i, want, got, plan.OwnerIDs)
		}
	}
	if plan.Rows != 2 {
		t.Fatalf("孤儿引用行数应当为 2，实际 %d", plan.Rows)
	}

	if after := countOrphanRefs(t, db); after != before {
		t.Fatalf("只读扫描不应写库：扫描前 %d 行，扫描后 %d 行", before, after)
	}
}

// TestCleanupOrphanRefsDeletesOnlyPlannedRows 验证 apply：只删计划里的行，
// 活轮播与别的 ref_type 都保留。
func TestCleanupOrphanRefsDeletesOnlyPlannedRows(t *testing.T) {
	db := newOrphanRefDB(t)
	seedOrphanBanner(t, db, 1, false)
	seedOrphanBanner(t, db, 2, true)
	seedOrphanRef(t, db, 11, image_ref_enum.RefTypeBanner, 1)
	seedOrphanRef(t, db, 12, image_ref_enum.RefTypeBanner, 2)
	seedOrphanRef(t, db, 13, image_ref_enum.RefTypeBanner, 999)
	seedOrphanRef(t, db, 14, image_ref_enum.RefTypeUser, 999)

	plan, err := imageref.ScanOrphanRefs(db, image_ref_enum.RefTypeBanner)
	if err != nil {
		t.Fatalf("扫描孤儿引用失败: %v", err)
	}
	deleted, err := imageref.CleanupOrphanRefs(db, plan)
	if err != nil {
		t.Fatalf("清理孤儿引用失败: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("应当删除 2 行，实际 %d", deleted)
	}

	var left []schema.ImageRefRow
	if err := db.Order("id ASC").Find(&left).Error; err != nil {
		t.Fatalf("读剩余引用行失败: %v", err)
	}
	if len(left) != 2 {
		t.Fatalf("剩余引用行应当为 2，实际 %d：%+v", len(left), left)
	}
	if left[0].RefType != image_ref_enum.RefTypeBanner || left[0].OwnerID.Uint64() != 1 {
		t.Fatalf("活轮播的引用行不应被删：%+v", left[0])
	}
	if left[1].RefType != image_ref_enum.RefTypeUser {
		t.Fatalf("别的 ref_type 不应被删：%+v", left[1])
	}

	// 再扫一次应当为空（幂等）。
	again, err := imageref.ScanOrphanRefs(db, image_ref_enum.RefTypeBanner)
	if err != nil {
		t.Fatalf("二次扫描失败: %v", err)
	}
	if len(again.OwnerIDs) != 0 || again.Rows != 0 {
		t.Fatalf("清理后不应再有孤儿：%+v", again)
	}
}

// TestScanOrphanRefsRejectsUnsupportedRefType 只支持已登记主体类型：
// 没登记的类型不能拿轮播表硬套，否则会误删别的模块的账本行。
func TestScanOrphanRefsRejectsUnsupportedRefType(t *testing.T) {
	db := newOrphanRefDB(t)
	if _, err := imageref.ScanOrphanRefs(db, image_ref_enum.RefTypeUser); err == nil {
		t.Fatal("未登记的主体类型应当报错")
	}
}

func countOrphanRefs(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var total int64
	if err := db.Model(&schema.ImageRefRow{}).Count(&total).Error; err != nil {
		t.Fatalf("统计引用行失败: %v", err)
	}
	return total
}

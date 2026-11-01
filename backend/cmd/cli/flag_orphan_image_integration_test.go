//go:build integration

package cli

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 本文件验证「孤儿图只读清单」（#139 / spec #138）。
//
// 为什么必须真库：判定要跨 image_models、image_ref_models 以及四个引用源表
// （文章正文/封面、轮播封面、收藏封面、用户头像），且 LIKE 行为与列类型都
// 依赖真实 DDL。内存 fake 表达不了"某列里是否真的出现了这个 object_key"。

const (
	imageStatusForTestPass      = 1
	imageStatusForTestReviewing = 4
	imageStatusForTestBlocked   = 5
)

// migrateOrphanSchema 迁移本清单要用到的表。
//
// 重扫真实引用会查 article_models / banner_models / favorite_models /
// user_models 四张表的列，所以它们都必须存在，否则计数查询直接报错。
func migrateOrphanSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	schema.Migrate(t, db,
		schema.UserCore, schema.ArticleCore, schema.ImageAssets, schema.Site, schema.Favorite)
}
// seedOrphanImage 插入一张图片行。createdAt 决定它是否已过冷静期。
func seedOrphanImage(
	t *testing.T, db *gorm.DB, id uint64, objectKey string, status int, createdAt time.Time,
) {
	t.Helper()
	row := &schema.ImageRow{
		BaseRow:   schema.BaseRow{ID: ctype.NewID(id), CreatedAt: createdAt, UpdatedAt: createdAt},
		UserID:    ctype.NewID(100),
		Provider:  "qiniu",
		Bucket:    "blogx",
		ObjectKey: objectKey,
		URL:       "https://cdn.example.com/" + objectKey,
		FileName:  "f.png",
		MimeType:  "image/png",
		Hash:      "hash-" + objectKey,
		Status:    enum.ImageStatus(status),
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("插入图片行失败: %v", err)
	}
}

// seedOrphanLedgerRef 给某张图建一条账本引用行。
func seedOrphanLedgerRef(t *testing.T, db *gorm.DB, id uint64, imageID uint64) {
	t.Helper()
	row := &schema.ImageRefRow{
		BaseRow:  schema.BaseRow{ID: ctype.NewID(id), CreatedAt: time.Now(), UpdatedAt: time.Now()},
		ImageID:  ctype.NewID(imageID),
		RefType:  1,
		OwnerID:  ctype.NewID(100),
		Field:    1,
		Position: 0,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("插入账本引用行失败: %v", err)
	}
}

// TestOrphanListSelectsZeroRefImagePastCooldown 钉住"真孤儿被选中"。
func TestOrphanListSelectsZeroRefImagePastCooldown(t *testing.T) {
	db := mysqlcontainer.DB(t)
	migrateOrphanSchema(t, db)
	seedReconcileFixture(t, db)

	old := time.Now().AddDate(0, 0, -30)
	seedOrphanImage(t, db, 9001, "images/orphan-a.png", imageStatusForTestPass, old)

	report, err := scanOrphanImages(context.Background(), db, 14, 200)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if report.Candidate != 1 {
		t.Fatalf("应选出 1 条候选，实际 %d（统计: %+v）", report.Candidate, report)
	}
	if len(report.Items) != 1 || report.Items[0].ObjectKey != "images/orphan-a.png" {
		t.Fatalf("候选内容不符: %+v", report.Items)
	}
	if report.Items[0].RealRefs != 0 {
		t.Fatalf("真孤儿的真实引用应为 0，实际 %d", report.Items[0].RealRefs)
	}
}

// TestOrphanListKeepsLedgerReferencedImage 有账本引用的图不是孤儿。
func TestOrphanListKeepsLedgerReferencedImage(t *testing.T) {
	db := mysqlcontainer.DB(t)
	migrateOrphanSchema(t, db)
	seedReconcileFixture(t, db)

	old := time.Now().AddDate(0, 0, -30)
	seedOrphanImage(t, db, 9002, "images/referenced.png", imageStatusForTestPass, old)
	seedOrphanLedgerRef(t, db, 9102, 9002)

	report, err := scanOrphanImages(context.Background(), db, 14, 200)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if report.Candidate != 0 {
		t.Fatalf("有账本引用的图不应入选，实际候选 %d", report.Candidate)
	}
}

// TestOrphanListRescansRealRefsAndFlagsInconsistency 是本票的核心：
// 账本零引用、但真实引用非零 → 不入选，且单独标记为"账本不一致"。
//
// 账本由 CDC 异步维护且有死信、引用靠从 URL 反解 object_key，所以"账本
// 零引用"不等于"没人用"。删图不可逆，必须重扫。
func TestOrphanListRescansRealRefsAndFlagsInconsistency(t *testing.T) {
	db := mysqlcontainer.DB(t)
	migrateOrphanSchema(t, db)
	seedReconcileFixture(t, db)

	old := time.Now().AddDate(0, 0, -30)
	seedOrphanImage(t, db, 9003, "images/still-used.png", imageStatusForTestPass, old)

	// 文章正文里确实还引用着这张图；但账本没有对应行（模拟账本漏记）。
	schema.InsertArticle(t, db, schema.ArticleSpec{
		ID:            3001,
		Title:         "仍在用这张图的文章",
		AuthorID:      100,
		Content:       `<img src="https://cdn.example.com/images/still-used.png">`,
		PublishStatus: schema.ArticleStatusPublished,
		Visibility:    schema.VisibilityVisible,
		CreatedAt:     old,
		UpdatedAt:     old,
	})

	report, err := scanOrphanImages(context.Background(), db, 14, 200)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if report.Candidate != 0 {
		t.Fatalf("真实仍被引用的图不应入选（否则会误删），实际候选 %d", report.Candidate)
	}
	if report.Inconsistent != 1 {
		t.Fatalf("应标记 1 条账本不一致，实际 %d", report.Inconsistent)
	}
	if len(report.Items) != 1 || !report.Items[0].LedgerInconsistent {
		t.Fatalf("不一致条目应带标记并被列出: %+v", report.Items)
	}
	if len(report.Items) == 1 && report.Items[0].RealRefs == 0 {
		t.Fatalf("不一致条目的真实引用数应大于 0，实际 %d", report.Items[0].RealRefs)
	}
}

// TestOrphanListSkipsFreshAndRestrictedStatus 冷静期与状态过滤。
func TestOrphanListSkipsFreshAndRestrictedStatus(t *testing.T) {
	db := mysqlcontainer.DB(t)
	migrateOrphanSchema(t, db)
	seedReconcileFixture(t, db)

	fresh := time.Now().AddDate(0, 0, -1)
	old := time.Now().AddDate(0, 0, -30)
	seedOrphanImage(t, db, 9004, "images/fresh.png", imageStatusForTestPass, fresh)
	seedOrphanImage(t, db, 9005, "images/reviewing.png", imageStatusForTestReviewing, old)
	seedOrphanImage(t, db, 9006, "images/blocked.png", imageStatusForTestBlocked, old)

	report, err := scanOrphanImages(context.Background(), db, 14, 200)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if report.Candidate != 0 {
		t.Fatalf("未过冷静期与审核中/封禁都不应入选，实际候选 %d", report.Candidate)
	}
	if report.SkippedFresh != 1 {
		t.Fatalf("应跳过 1 条未过冷静期的，实际 %d", report.SkippedFresh)
	}
	if report.SkippedStatus != 2 {
		t.Fatalf("应跳过 2 条审核中/封禁的，实际 %d", report.SkippedStatus)
	}
}

// TestOrphanListIsReadOnly 只读断言：跑完之后图片表、引用表都不变。
func TestOrphanListIsReadOnly(t *testing.T) {
	db := mysqlcontainer.DB(t)
	migrateOrphanSchema(t, db)
	seedReconcileFixture(t, db)

	old := time.Now().AddDate(0, 0, -30)
	seedOrphanImage(t, db, 9007, "images/readonly-a.png", imageStatusForTestPass, old)
	seedOrphanImage(t, db, 9008, "images/readonly-b.png", imageStatusForTestPass, old)

	var imagesBefore, refsBefore int64
	db.Model(&schema.ImageRow{}).Count(&imagesBefore)
	db.Model(&schema.ImageRefRow{}).Count(&refsBefore)

	if _, err := scanOrphanImages(context.Background(), db, 14, 200); err != nil {
		t.Fatalf("扫描失败: %v", err)
	}

	var imagesAfter, refsAfter int64
	db.Model(&schema.ImageRow{}).Count(&imagesAfter)
	db.Model(&schema.ImageRefRow{}).Count(&refsAfter)

	if imagesBefore != imagesAfter {
		t.Fatalf("只读清单不应改动图片表: %d -> %d", imagesBefore, imagesAfter)
	}
	if refsBefore != refsAfter {
		t.Fatalf("只读清单不应改动引用表: %d -> %d", refsBefore, refsAfter)
	}
}

// TestOrphanListRespectsLimit 超上限时截断并标记"还有剩余"。
func TestOrphanListRespectsLimit(t *testing.T) {
	db := mysqlcontainer.DB(t)
	migrateOrphanSchema(t, db)
	seedReconcileFixture(t, db)

	old := time.Now().AddDate(0, 0, -30)
	keys := []string{"images/many-a.png", "images/many-b.png", "images/many-c.png"}
	for index, key := range keys {
		seedOrphanImage(t, db, uint64(9010+index), key, imageStatusForTestPass, old)
	}

	report, err := scanOrphanImages(context.Background(), db, 14, 2)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(report.Items) != 2 {
		t.Fatalf("单次上限 2，实际列出 %d 条", len(report.Items))
	}
	if !report.Truncated {
		t.Fatalf("超过上限时应标记还有剩余")
	}
}

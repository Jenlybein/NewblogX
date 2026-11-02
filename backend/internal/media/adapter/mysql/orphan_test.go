package mysql_test

import (
	"context"
	"testing"
	"time"

	"myblogx/conf"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/kernel/enum/image_ref_enum"
	mediamysql "myblogx/internal/media/adapter/mysql"
	"myblogx/internal/media/domain"
	"myblogx/internal/testkit"
	"myblogx/internal/testkit/schema"

	"gorm.io/gorm"
)

// 本文件的用例钉住孤儿图清单的**取数口径**（issue #139）：
// 哪些行进检查范围、账本按图片怎么数、重扫真实引用怎么算。
// 用手边的内存 sqlite 跑，不依赖 Docker / 真库。

var testNow = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

const (
	testPrefix   = "myblogx"
	testBucket   = "bucket"
	testCDNHost  = "https://cdn.example.com/"
	otherCDNHost = "https://old.example.com/"
)

// newOrphanDB 建出清单用到的表：图片 + 账本 + 四类引用源。
func newOrphanDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testkit.NewSQLite(t, schema.Models(schema.ImageAssets)...)
	if err := db.AutoMigrate(&sourceArticleRow{}, &sourceUserRow{}, &sourceBannerRow{}, &sourceFavoriteRow{}); err != nil {
		t.Fatalf("建引用源表失败: %v", err)
	}
	return db
}

// seedImage 往图片表放一行。
func seedImage(t *testing.T, db *gorm.DB, id uint64, objectKey string, createdAt time.Time, status domain.Status) ctype.ID {
	t.Helper()
	row := schema.ImageRow{
		BaseRow:   schema.BaseRow{ID: ctype.NewID(id), CreatedAt: createdAt},
		UserID:    ctype.NewID(1),
		Provider:  enum.ImageProviderQiNiu,
		Bucket:    testBucket,
		ObjectKey: objectKey,
		FileName:  objectKey,
		URL:       testCDNHost + objectKey,
		MimeType:  "image/png",
		Hash:      objectKey,
		Status:    enum.ImageStatus(status),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("造图片行失败: %v", err)
	}
	return row.ID
}

// seedLedgerRef 往引用账本里放一行。
func seedLedgerRef(t *testing.T, db *gorm.DB, id uint64, imageID ctype.ID, refType image_ref_enum.RefType, field image_ref_enum.RefField, owner uint64, position int) {
	t.Helper()
	row := schema.ImageRefRow{
		BaseRow:  schema.BaseRow{ID: ctype.NewID(id)},
		ImageID:  imageID,
		RefType:  refType,
		OwnerID:  ctype.NewID(owner),
		Field:    field,
		Position: position,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("造账本行失败: %v", err)
	}
}

// ── 引用源行（只建本用例读到的列）────────────────────────────────

type sourceArticleRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	Content   string
	Cover     string
	DeletedAt gorm.DeletedAt
}

func (sourceArticleRow) TableName() string { return "article_models" }

type sourceUserRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	Avatar    string
	DeletedAt gorm.DeletedAt
}

func (sourceUserRow) TableName() string { return "user_models" }

type sourceBannerRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	Cover     string
	DeletedAt gorm.DeletedAt
}

func (sourceBannerRow) TableName() string { return "banner_models" }

type sourceFavoriteRow struct {
	ID        ctype.ID `gorm:"primaryKey;autoIncrement:false"`
	Cover     string
	DeletedAt gorm.DeletedAt
}

func (sourceFavoriteRow) TableName() string { return "favorite_models" }

// ── ListCandidates ──────────────────────────────────────────────

func TestOrphanStoreListCandidatesFiltersStatusAndGrace(t *testing.T) {
	db := newOrphanDB(t)
	store := mediamysql.NewOrphanStore(db, func() conf.QiNiu { return conf.QiNiu{Prefix: testPrefix} })

	seedImage(t, db, 1, testPrefix+"/images/20260101/old-pass", testNow.Add(-20*24*time.Hour), domain.StatusPass)
	seedImage(t, db, 2, testPrefix+"/images/20260101/recent", testNow.Add(-3*24*time.Hour), domain.StatusPass)
	seedImage(t, db, 3, testPrefix+"/images/20260101/reviewing", testNow.Add(-20*24*time.Hour), domain.StatusReviewing)
	seedImage(t, db, 4, testPrefix+"/images/20260101/blocked", testNow.Add(-20*24*time.Hour), domain.StatusBlocked)
	seedImage(t, db, 5, testPrefix+"/images/20260101/oldest-pass", testNow.Add(-30*24*time.Hour), domain.StatusPass)

	spec := domain.OrphanScanSpec{
		UploadedBefore: testNow.Add(-14 * 24 * time.Hour),
		Statuses:       domain.CleanableStatuses(),
		Limit:          2,
	}
	images, total, err := store.ListCandidates(context.Background(), spec)
	if err != nil {
		t.Fatalf("取候选失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("符合条件总数应为 2（两张过了冷静期的 pass 图），实际 %d", total)
	}
	if len(images) != 2 {
		t.Fatalf("单页条数应被上限卡住，实际 %d", len(images))
	}
	// 老图先出。
	if images[0].ID.Uint64() != 5 || images[1].ID.Uint64() != 1 {
		t.Fatalf("应按上传时间从旧到新，实际顺序 %s, %s", images[0].ID, images[1].ID)
	}
	if images[0].ObjectKey != testPrefix+"/images/20260101/oldest-pass" {
		t.Fatalf("清单条目必须带对象键，实际 %q", images[0].ObjectKey)
	}
	if !images[0].CreatedAt.Equal(testNow.Add(-30 * 24 * time.Hour)) {
		t.Fatalf("清单条目必须带上传时间，实际 %s", images[0].CreatedAt)
	}
}

func TestOrphanStoreListCandidatesRejectsEmptyStatusFilter(t *testing.T) {
	db := newOrphanDB(t)
	store := mediamysql.NewOrphanStore(db, nil)

	seedImage(t, db, 1, testPrefix+"/images/20260101/old-pass", testNow.Add(-20*24*time.Hour), domain.StatusPass)

	// 状态白名单为空等于"哪张都不许进"，不能静默退化成整表扫描。
	images, total, err := store.ListCandidates(context.Background(), domain.OrphanScanSpec{
		UploadedBefore: testNow.Add(-14 * 24 * time.Hour),
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("取候选失败: %v", err)
	}
	if len(images) != 0 || total != 0 {
		t.Fatalf("状态白名单为空时不该返回任何图: 页=%d 总数=%d", len(images), total)
	}
}

// ── LedgerSnapshot ─────────────────────────────────────────────

func TestOrphanStoreLedgerSnapshotCountsAndFlagsAvatars(t *testing.T) {
	db := newOrphanDB(t)
	store := mediamysql.NewOrphanStore(db, nil)

	first := seedImage(t, db, 1, testPrefix+"/images/20260101/a", testNow.Add(-20*24*time.Hour), domain.StatusPass)
	second := seedImage(t, db, 2, testPrefix+"/images/20260101/b", testNow.Add(-20*24*time.Hour), domain.StatusPass)

	seedLedgerRef(t, db, 101, first, image_ref_enum.RefTypeArticle, image_ref_enum.RefFieldArticleCover, 7, 0)
	seedLedgerRef(t, db, 102, first, image_ref_enum.RefTypeArticle, image_ref_enum.RefFieldArticleContent, 7, 0)
	seedLedgerRef(t, db, 103, first, image_ref_enum.RefTypeUser, image_ref_enum.RefFieldUserAvatar, 8, 0)
	seedLedgerRef(t, db, 104, second, image_ref_enum.RefTypeBanner, image_ref_enum.RefFieldBannerCover, 9, 0)

	snapshot, err := store.LedgerSnapshot(context.Background(), []domain.ImageID{first, second})
	if err != nil {
		t.Fatalf("读账本失败: %v", err)
	}
	if got := snapshot.Refs[first]; got != 3 {
		t.Fatalf("第一张图账本应有 3 条引用行，实际 %d", got)
	}
	if !snapshot.Avatars[first] {
		t.Fatal("第一张图被用作头像，应被标出")
	}
	if got := snapshot.Refs[second]; got != 1 {
		t.Fatalf("第二张图账本应有 1 条引用行，实际 %d", got)
	}
	if snapshot.Avatars[second] {
		t.Fatal("第二张图不是头像，不该被标出")
	}
}

// ── LiveRefs ───────────────────────────────────────────────────

func TestOrphanStoreLiveRefsRescansFourSources(t *testing.T) {
	db := newOrphanDB(t)
	store := mediamysql.NewOrphanStore(db, func() conf.QiNiu { return conf.QiNiu{Prefix: testPrefix} })

	keyA := testPrefix + "/images/20260101/a"
	urlA := testCDNHost + keyA
	// 历史域名/前缀不匹配的 URL：反解不出对象键，会被丢弃 —— 这正是账本
	// 可能漏记的典型原因，所以它不该给出任何真实引用数。
	keyB := "legacy/images/20260101/b"
	urlB := otherCDNHost + keyB

	// 文章：正文里两次出现 + 封面一次 = 3 处。
	if err := db.Create(&sourceArticleRow{
		ID:      ctype.NewID(1),
		Content: "![a](" + urlA + ")\n\n![b](" + urlA + ")",
		Cover:   urlA,
	}).Error; err != nil {
		t.Fatalf("造文章行失败: %v", err)
	}
	// 已软删的文章不算引用。
	if err := db.Create(&sourceArticleRow{
		ID:        ctype.NewID(2),
		Content:   "![a](" + urlA + ")",
		DeletedAt: gorm.DeletedAt{Time: testNow, Valid: true},
	}).Error; err != nil {
		t.Fatalf("造已删文章行失败: %v", err)
	}
	// 用户头像、轮播封面、收藏封面各一处。
	if err := db.Create(&sourceUserRow{ID: ctype.NewID(3), Avatar: urlA}).Error; err != nil {
		t.Fatalf("造用户行失败: %v", err)
	}
	if err := db.Create(&sourceBannerRow{ID: ctype.NewID(4), Cover: urlA}).Error; err != nil {
		t.Fatalf("造轮播行失败: %v", err)
	}
	if err := db.Create(&sourceFavoriteRow{ID: ctype.NewID(5), Cover: urlA}).Error; err != nil {
		t.Fatalf("造收藏行失败: %v", err)
	}
	// 旧前缀的引用：真实存在，但按反解规则匹配不上。
	if err := db.Create(&sourceBannerRow{ID: ctype.NewID(6), Cover: urlB}).Error; err != nil {
		t.Fatalf("造旧前缀轮播行失败: %v", err)
	}

	live, err := store.LiveRefs(context.Background(), []string{keyA, keyB})
	if err != nil {
		t.Fatalf("重扫真实引用失败: %v", err)
	}
	if got := live[keyA]; got != 6 {
		t.Fatalf("真实引用应为 6（正文 2 + 封面 1 + 头像 1 + 轮播 1 + 收藏 1），实际 %d", got)
	}
	if got := live[keyB]; got != 0 {
		t.Fatalf("前缀对不上的 URL 应被丢弃（这是账本漏记的典型原因），实际 %d", got)
	}
}

func TestOrphanStoreLiveRefsOnlyCountsRequestedKeys(t *testing.T) {
	db := newOrphanDB(t)
	store := mediamysql.NewOrphanStore(db, func() conf.QiNiu { return conf.QiNiu{Prefix: testPrefix} })

	wanted := testPrefix + "/images/20260101/wanted"
	other := testPrefix + "/images/20260101/other"
	if err := db.Create(&sourceArticleRow{
		ID:      ctype.NewID(1),
		Content: "![x](" + testCDNHost + other + ")",
	}).Error; err != nil {
		t.Fatalf("造文章行失败: %v", err)
	}

	live, err := store.LiveRefs(context.Background(), []string{wanted})
	if err != nil {
		t.Fatalf("重扫真实引用失败: %v", err)
	}
	if len(live) != 0 {
		t.Fatalf("没被问到的对象键不该出现在结果里: %+v", live)
	}
}

// ── DeletedArticleRefs ──────────────────────────────────────────

// 指向已删除文章的账本行只统计、不进入自动删除：真删前的保护读数
// 必须能区分"文章已进回收站"和"文章还活着"。
func TestOrphanStoreDeletedArticleRefsOnlyCountsTrashedArticles(t *testing.T) {
	db := newOrphanDB(t)
	store := mediamysql.NewOrphanStore(db, nil)

	first := seedImage(t, db, 1, testPrefix+"/images/20260101/trashed-ref", testNow.Add(-20*24*time.Hour), domain.StatusPass)
	second := seedImage(t, db, 2, testPrefix+"/images/20260101/live-ref", testNow.Add(-20*24*time.Hour), domain.StatusPass)

	// 文章 200 在回收站：账本行 201 指向它，属于受保护引用。
	if err := db.Create(&sourceArticleRow{
		ID:        ctype.NewID(200),
		DeletedAt: gorm.DeletedAt{Time: testNow, Valid: true},
	}).Error; err != nil {
		t.Fatalf("造已删除文章失败: %v", err)
	}
	seedLedgerRef(t, db, 201, first, image_ref_enum.RefTypeArticle, image_ref_enum.RefFieldArticleContent, 200, 0)

	// 文章 100 还活着：账本行指向它是正常引用，不属于保护范围。
	if err := db.Create(&sourceArticleRow{ID: ctype.NewID(100)}).Error; err != nil {
		t.Fatalf("造存活文章失败: %v", err)
	}
	seedLedgerRef(t, db, 202, second, image_ref_enum.RefTypeArticle, image_ref_enum.RefFieldArticleCover, 100, 0)

	// 用户头像引用不属于文章引用，不能被算成"已删除文章引用"。
	seedLedgerRef(t, db, 203, first, image_ref_enum.RefTypeUser, image_ref_enum.RefFieldUserAvatar, 300, 0)

	got, err := store.DeletedArticleRefs(context.Background(), []domain.ImageID{first, second, ctype.NewID(999)})
	if err != nil {
		t.Fatalf("读受保护引用失败: %v", err)
	}
	if got[first] != 1 {
		t.Fatalf("第一张图应统计 1 条指向已删除文章的引用行，实际 %d", got[first])
	}
	if _, ok := got[second]; ok {
		t.Fatalf("指向存活文章的引用行不该被算成受保护引用: %+v", got)
	}
	if len(got) != 1 {
		t.Fatalf("只应返回有受保护引用的图片: %+v", got)
	}
}

package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/image_ref_enum"
	"myblogx/internal/media/app"
	"myblogx/internal/media/apptest"
	"myblogx/internal/media/domain"
)

// 本文件的用例对应 issue #139 的判定口径：
//   - 候选 = 真实引用为 0 且早于冷静期，头像/审核中/封禁一律不动；
//   - 账本引用数为 0 但真实引用非零的条目单独标出（账本漏记的证据）；
//   - 全程只读，扫描结束后图片表、引用账本、存储侧都没有任何变化。

const (
	testBucket = "bucket"
	testPrefix = "myblogx"
)

// scanEnv 是一个只读清单用例的运行环境。
type scanEnv struct {
	store   *apptest.OrphanStore
	scanner *app.OrphanScanner
	log     *apptest.Recorder
}

// fixedNow 是 apptest.Clock 的固定时刻：所有"多少天前"都相对它算。
func fixedNow() time.Time { return apptest.Clock{}.Now() }

// daysAgo 返回固定时刻往前若干天。
func daysAgo(days int) time.Time {
	return fixedNow().Add(-time.Duration(days) * 24 * time.Hour)
}

// newScanEnv 用固定配置搭一个环境。
func newScanEnv(t *testing.T, cfg domain.OrphanScanConfig) *scanEnv {
	t.Helper()
	return newScanEnvWithConfig(t, func() domain.OrphanScanConfig { return cfg })
}

// newScanEnvWithConfig 用配置读取函数搭一个环境（用于验证"配置现读"）。
func newScanEnvWithConfig(t *testing.T, cfg func() domain.OrphanScanConfig) *scanEnv {
	t.Helper()
	store := apptest.NewOrphanStore()
	log := apptest.NewRecorder()
	return &scanEnv{
		store:   store,
		log:     log,
		scanner: app.NewOrphanScanner(store, apptest.Clock{}, log, cfg),
	}
}

// seedImage 往图片表放一张图，返回落库后的图片（Create 会分配 ID）。
func seedImage(t *testing.T, store *apptest.OrphanStore, objectKey string, createdAt time.Time, status domain.Status) domain.Image {
	t.Helper()
	img := domain.Image{
		UserID:    ctype.NewID(1),
		Provider:  domain.ProviderQiniu,
		Bucket:    testBucket,
		ObjectKey: objectKey,
		URL:       "https://cdn.example.com/" + testPrefix + "/" + objectKey,
		FileName:  objectKey,
		MimeType:  "image/png",
		Hash:      objectKey,
		Size:      1,
		Width:     1,
		Height:    1,
		Status:    status,
		CreatedAt: createdAt,
	}
	if err := store.Create(context.Background(), &img); err != nil {
		t.Fatalf("造图片数据失败: %v", err)
	}
	return img
}

func TestOrphanScanSelectsZeroReferenceImage(t *testing.T) {
	env := newScanEnv(t, domain.OrphanScanConfig{})

	img := seedImage(t, env.store, "myblogx/images/20260101/zero", daysAgo(20), domain.StatusPass)

	res, err := env.scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("应选中 1 条零引用图，实际 %d 条: %+v", len(res.Candidates), res.Candidates)
	}

	candidate := res.Candidates[0]
	if candidate.ID != img.ID {
		t.Fatalf("选中的图片标识不对: want=%s got=%s", img.ID, candidate.ID)
	}
	if candidate.ObjectKey != "myblogx/images/20260101/zero" {
		t.Fatalf("清单必须带存储对象键，实际 %q", candidate.ObjectKey)
	}
	if !candidate.CreatedAt.Equal(daysAgo(20)) {
		t.Fatalf("清单必须带上传时间，实际 %s", candidate.CreatedAt)
	}
	if candidate.Status != domain.StatusPass {
		t.Fatalf("清单必须带状态，实际 %s", candidate.Status)
	}
	if candidate.LedgerRefs != 0 || candidate.LiveRefs != 0 {
		t.Fatalf("零引用条目两种引用数都应显示 0: 账本=%d 真实=%d", candidate.LedgerRefs, candidate.LiveRefs)
	}
	if res.Scanned != 1 || res.Excluded != 0 || res.Remaining != 0 {
		t.Fatalf("统计不对: 检查=%d 排除=%d 剩余=%d", res.Scanned, res.Excluded, res.Remaining)
	}
}

func TestOrphanScanKeepsLiveReferencedImageEvenWhenLedgerMissedIt(t *testing.T) {
	env := newScanEnv(t, domain.OrphanScanConfig{})

	// 账本与真实引用一致：账本 2 条、真实 2 处。有引用，不是候选。
	consistent := seedImage(t, env.store, "myblogx/images/20260101/used", daysAgo(20), domain.StatusPass)
	env.store.AddLedgerRef(consistent.ID, image_ref_enum.RefFieldArticleContent)
	env.store.AddLedgerRef(consistent.ID, image_ref_enum.RefFieldArticleCover)
	env.store.SetLiveRefs(consistent.ObjectKey, 2)

	// 账本漏记：账本 0 条，但真实引用有 1 处。不是候选，且必须单独标出。
	missed := seedImage(t, env.store, "myblogx/images/20260101/missed", daysAgo(20), domain.StatusPass)
	env.store.SetLiveRefs(missed.ObjectKey, 1)

	res, err := env.scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("真实引用非零的图不该进候选: %+v", res.Candidates)
	}

	if len(res.Mismatches) != 1 {
		t.Fatalf("应单独标出 1 条账本不一致，实际 %d 条: %+v", len(res.Mismatches), res.Mismatches)
	}
	mismatch := res.Mismatches[0]
	if mismatch.ID != missed.ID {
		t.Fatalf("标出的不一致条目不对: want=%s got=%s", missed.ID, mismatch.ID)
	}
	if mismatch.LedgerRefs != 0 {
		t.Fatalf("不一致条目的账本引用数应为 0，实际 %d", mismatch.LedgerRefs)
	}
	if mismatch.LiveRefs != 1 {
		t.Fatalf("不一致条目的真实引用数应为 1，实际 %d", mismatch.LiveRefs)
	}

	// 两边都非零是"一致"，不属于不一致，也不该被标出。
	if res.Scanned != 2 || res.Excluded != 2 || res.RefExcluded != 2 {
		t.Fatalf("统计不对: 检查=%d 排除=%d 因引用排除=%d", res.Scanned, res.Excluded, res.RefExcluded)
	}
}

func TestOrphanScanSkipsImagesInsideGracePeriod(t *testing.T) {
	env := newScanEnv(t, domain.OrphanScanConfig{})

	// 3 天前的图还在冷静期内：不检查、不选中。
	seedImage(t, env.store, "myblogx/images/20260101/fresh", daysAgo(3), domain.StatusPass)
	// 20 天前的图过了冷静期，作为对照被选中。
	old := seedImage(t, env.store, "myblogx/images/20260101/old", daysAgo(20), domain.StatusPass)

	res, err := env.scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if res.Scanned != 1 {
		t.Fatalf("冷静期内的图不该进入检查范围: 检查=%d", res.Scanned)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].ID != old.ID {
		t.Fatalf("只应选中过了冷静期的那张: %+v", res.Candidates)
	}
}

func TestOrphanScanSkipsUserAvatars(t *testing.T) {
	env := newScanEnv(t, domain.OrphanScanConfig{})

	// 账本记着"用户头像"，但重扫真实引用匹配不上（例如换过 CDN 域名）。
	// 头像一律不动，不能因为重扫为 0 就被当成孤儿。
	avatar := seedImage(t, env.store, "myblogx/images/20260101/avatar", daysAgo(20), domain.StatusPass)
	env.store.AddLedgerRef(avatar.ID, image_ref_enum.RefFieldUserAvatar)

	res, err := env.scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("用户头像不该进候选: %+v", res.Candidates)
	}
	if res.AvatarExcluded != 1 {
		t.Fatalf("应记录 1 条因头像被排除，实际 %d", res.AvatarExcluded)
	}
}

func TestOrphanScanSkipsReviewingAndBlockedImages(t *testing.T) {
	env := newScanEnv(t, domain.OrphanScanConfig{})

	reviewing := seedImage(t, env.store, "myblogx/images/20260101/reviewing", daysAgo(20), domain.StatusReviewing)
	blocked := seedImage(t, env.store, "myblogx/images/20260101/blocked", daysAgo(20), domain.StatusBlocked)
	pass := seedImage(t, env.store, "myblogx/images/20260101/pass", daysAgo(20), domain.StatusPass)

	res, err := env.scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].ID != pass.ID {
		t.Fatalf("只应选中审核通过的那张: %+v", res.Candidates)
	}
	for _, candidate := range res.Candidates {
		if candidate.ID == reviewing.ID || candidate.ID == blocked.ID {
			t.Fatalf("审核中/封禁的图不该被选中: %+v", candidate)
		}
	}
	// 审核中/封禁在取数时就过滤掉了，连"检查过"都不算。
	if res.Scanned != 1 {
		t.Fatalf("审核中/封禁不该进入检查范围: 检查=%d", res.Scanned)
	}
}

func TestOrphanScanIsReadOnly(t *testing.T) {
	env := newScanEnv(t, domain.OrphanScanConfig{})

	img := seedImage(t, env.store, "myblogx/images/20260101/zero", daysAgo(20), domain.StatusPass)
	env.store.AddLedgerRef(img.ID, image_ref_enum.RefFieldArticleCover)
	env.store.SetLiveRefs("myblogx/images/20260101/other", 1)

	// 把存储删除的失败注入打开：只要扫描碰了存储，用例就会以"删除失败"收场。
	// 只读模式连一次删除调用都不该发出。
	storage := apptest.NewCountingStorage()
	storage.FailNextDelete(errors.New("只读清单不该调用存储删除"))

	before := env.store.Snapshot()

	res, err := env.scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("本用例要先选中一条才谈得上'只是看': %+v", res.Candidates)
	}

	// 图片表、引用账本、真实引用三份事实都必须与扫描前逐字节一致。
	if after := env.store.Snapshot(); after != before {
		t.Fatalf("只读扫描改动了数据:\nbefore=%s\nafter=%s", before, after)
	}
	if calls := storage.DeleteCalls(); calls != 0 {
		t.Fatalf("只读扫描调用了存储删除 %d 次", calls)
	}
	// 图片行与账本行仍然各自都在：没有被软删或清空。
	if got := env.store.Snapshot(); !strings.Contains(got, string(img.ID)) {
		t.Fatalf("图片行在扫描后不见了: %s", got)
	}
	if count := len(env.store.Snapshot()); count == 0 {
		t.Fatal("账本快照为空，用例的只读断言失去意义")
	}
}

func TestOrphanScanReportsRemainingOverBatchLimit(t *testing.T) {
	env := newScanEnv(t, domain.OrphanScanConfig{BatchLimit: 2})

	first := seedImage(t, env.store, "myblogx/images/20260101/first", daysAgo(30), domain.StatusPass)
	second := seedImage(t, env.store, "myblogx/images/20260101/second", daysAgo(20), domain.StatusPass)
	seedImage(t, env.store, "myblogx/images/20260101/third", daysAgo(15), domain.StatusPass)

	res, err := env.scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if res.Scanned != 2 {
		t.Fatalf("单次检查条数应被上限卡住: 检查=%d", res.Scanned)
	}
	if len(res.Candidates) != 2 {
		t.Fatalf("本轮应输出 2 条候选，实际 %d 条", len(res.Candidates))
	}
	if res.Candidates[0].ID != first.ID || res.Candidates[1].ID != second.ID {
		t.Fatalf("超限时应按上传时间从旧到新接着扫: %+v", res.Candidates)
	}
	if res.Remaining != 1 {
		t.Fatalf("超限时应报告剩余 1 条，实际 %d", res.Remaining)
	}
}

func TestOrphanScanReadsRuntimeConfigEachRun(t *testing.T) {
	// 冷静期由配置函数现读：同一个扫描器两次调用之间改配置，下一次立即生效。
	cfg := domain.OrphanScanConfig{GraceDays: 30, BatchLimit: 10}
	env := newScanEnvWithConfig(t, func() domain.OrphanScanConfig { return cfg })

	seedImage(t, env.store, "myblogx/images/20260101/recent", daysAgo(20), domain.StatusPass)

	first, err := env.scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(first.Candidates) != 0 {
		t.Fatalf("冷静期 30 天时 20 天前的图不该入选: %+v", first.Candidates)
	}
	if first.GraceDays != 30 {
		t.Fatalf("结果应带本轮生效的冷静期，实际 %d", first.GraceDays)
	}

	cfg.GraceDays = 1
	second, err := env.scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(second.Candidates) != 1 {
		t.Fatalf("冷静期改成 1 天后同一张图应立即入选: %+v", second.Candidates)
	}
	if second.GraceDays != 1 {
		t.Fatalf("改配置后应现读生效，实际冷静期 %d", second.GraceDays)
	}
}

func TestOrphanScanLogsConclusion(t *testing.T) {
	env := newScanEnv(t, domain.OrphanScanConfig{})

	seedImage(t, env.store, "myblogx/images/20260101/zero", daysAgo(20), domain.StatusPass)
	seedImage(t, env.store, "myblogx/images/20260101/recent", daysAgo(2), domain.StatusPass)

	if _, err := env.scanner.Scan(context.Background()); err != nil {
		t.Fatalf("扫描失败: %v", err)
	}

	last := env.log.Last()
	if !strings.HasPrefix(last, "info ") {
		t.Fatalf("结论应写到 info 日志，实际最后一条是 %q", last)
	}
	for _, want := range []string{"检查=1", "选中=1", "排除=0", "剩余=0"} {
		if !strings.Contains(last, want) {
			t.Fatalf("结论日志缺少 %q: %q", want, last)
		}
	}
}

func TestOrphanScanPropagatesStoreFailure(t *testing.T) {
	env := newScanEnv(t, domain.OrphanScanConfig{})
	seedImage(t, env.store, "myblogx/images/20260101/zero", daysAgo(20), domain.StatusPass)

	env.store.FailNext("LedgerSnapshot", errors.New("账本查询失败"))

	if _, err := env.scanner.Scan(context.Background()); err == nil {
		t.Fatal("账本读取失败时扫描应报错，而不是当成零引用继续")
	}
}

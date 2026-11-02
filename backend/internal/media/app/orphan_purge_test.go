package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"myblogx/internal/kernel/enum/image_ref_enum"
	"myblogx/internal/media/app"
	"myblogx/internal/media/apptest"
	"myblogx/internal/media/domain"
)

// 本文件的用例对应 issue #140「真删回收」：
//   - 二次确认（重扫真实引用）通过才删；有引用则跳过；
//   - 先删存储对象、确认成功才删本地行；对象删失败时行必须留着；
//   - 复用既有 DeleteWithRefs：行删掉时账本里指向它的行一并清掉；
//   - 同一条目反复执行不造成二次损坏；
//   - dry-run 只确认不写；
//   - 账本里指向已删除文章的引用行只统计、不自动删（否则用户恢复文章后图没了）。

const purgeBucket = testBucket

// purgeEnv 是一次真删用例的运行环境。
type purgeEnv struct {
	store   *apptest.OrphanStore
	storage *apptest.CountingStorage
	log     *apptest.Recorder
	scanner *app.OrphanScanner
	purger  *app.OrphanPurger
}

// newPurgeEnv 用固定配置搭一个环境。
func newPurgeEnv(t *testing.T, cfg domain.OrphanPurgeConfig) *purgeEnv {
	t.Helper()
	return newPurgeEnvWithConfig(t, func() domain.OrphanPurgeConfig { return cfg })
}

// newPurgeEnvWithConfig 用配置读取函数搭环境（用于验证开关现读）。
func newPurgeEnvWithConfig(t *testing.T, cfg func() domain.OrphanPurgeConfig) *purgeEnv {
	t.Helper()
	store := apptest.NewOrphanStore()
	storage := apptest.NewCountingStorage()
	log := apptest.NewRecorder()

	scanner := app.NewOrphanScanner(store, apptest.Clock{}, log, func() domain.OrphanScanConfig {
		got := cfg()
		return domain.OrphanScanConfig{GraceDays: got.GraceDays, BatchLimit: got.BatchLimit}
	})
	remover := app.NewImageRemover(store.Repo, storage)
	purger := app.NewOrphanPurger(store, store, remover, apptest.Clock{}, log, cfg)

	return &purgeEnv{store: store, storage: storage, log: log, scanner: scanner, purger: purger}
}

// scanCandidates 跑一次清单，拿到待确认的候选。
func scanCandidates(t *testing.T, env *purgeEnv) []domain.OrphanCandidate {
	t.Helper()
	res, err := env.scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("清单扫描失败: %v", err)
	}
	return res.Candidates
}

// skipReason 判断结果里是否有一条以 reason 跳过的记录。
func skipReason(t *testing.T, res domain.OrphanPurgeResult, reason domain.PurgeSkipReason) bool {
	t.Helper()
	for _, skip := range res.Skips {
		if skip.Reason == reason {
			return true
		}
	}
	return false
}

// hasWarnLog 判断日志里是否有告警。
func hasWarnLog(recorder *apptest.Recorder) bool {
	for _, entry := range recorder.All() {
		if strings.HasPrefix(entry, "warn ") {
			return true
		}
	}
	return false
}

func TestOrphanPurgeDeletesConfirmedZeroReferenceImage(t *testing.T) {
	env := newPurgeEnv(t, domain.OrphanPurgeConfig{Enabled: true})

	img := seedImage(t, env.store, testPrefix+"/images/20260101/zero", daysAgo(20), domain.StatusPass)
	candidates := scanCandidates(t, env)
	if len(candidates) != 1 {
		t.Fatalf("前置条件：清单应选中 1 条零引用图，实际 %d 条", len(candidates))
	}

	res, err := env.purger.Purge(context.Background(), candidates, false)
	if err != nil {
		t.Fatalf("真删失败: %v", err)
	}
	if res.Deleted != 1 {
		t.Fatalf("应删成功 1 条，实际 %d 条（跳过 %d: %+v）", res.Deleted, res.Skipped, res.Skips)
	}
	if res.Selected != 1 || res.Skipped != 0 || res.WouldDelete != 1 {
		t.Fatalf("统计不对: 选中=%d 计划=%d 删除=%d 跳过=%d", res.Selected, res.WouldDelete, res.Deleted, res.Skipped)
	}

	// 先删对象：存储删除必须真的被调用。
	if calls := env.storage.DeleteCalls(); calls != 1 {
		t.Fatalf("应调用存储删除 1 次，实际 %d 次", calls)
	}
	// 对象删成功才删行：本地图片行必须已经消失。
	if env.store.Has(img.ID) {
		t.Fatalf("图片行在真删后仍然存在: id=%s", img.ID)
	}

	last := env.log.Last()
	for _, want := range []string{"选中=1", "删除成功=1", "跳过=0"} {
		if !strings.Contains(last, want) {
			t.Fatalf("结论日志缺少 %q: %q", want, last)
		}
	}
}

func TestOrphanPurgeClearsLedgerRowsWithDeletedImage(t *testing.T) {
	env := newPurgeEnv(t, domain.OrphanPurgeConfig{Enabled: true})

	// 账本记着一条引用（真实引用重扫为 0）。真删时账本行必须随行一起清掉。
	img := seedImage(t, env.store, testPrefix+"/images/20260101/ledger", daysAgo(20), domain.StatusPass)
	env.store.AddLedgerRef(img.ID, image_ref_enum.RefFieldArticleContent)
	candidates := scanCandidates(t, env)
	if len(candidates) != 1 {
		t.Fatalf("前置条件：应选中 1 条，实际 %d 条", len(candidates))
	}
	if candidates[0].LedgerRefs != 1 || candidates[0].LiveRefs != 0 {
		t.Fatalf("前置条件：账本 1 条 / 真实 0 条，实际 %d / %d", candidates[0].LedgerRefs, candidates[0].LiveRefs)
	}

	res, err := env.purger.Purge(context.Background(), candidates, false)
	if err != nil {
		t.Fatalf("真删失败: %v", err)
	}
	if res.Deleted != 1 {
		t.Fatalf("应删成功 1 条，实际 %d 条: %+v", res.Deleted, res.Skips)
	}
	if count := env.store.LedgerRefCount(img.ID); count != 0 {
		t.Fatalf("图片行删掉后账本里仍留下 %d 条引用行", count)
	}
}

func TestOrphanPurgeSkipsWhenSecondConfirmationFindsLiveRefs(t *testing.T) {
	env := newPurgeEnv(t, domain.OrphanPurgeConfig{Enabled: true})

	img := seedImage(t, env.store, testPrefix+"/images/20260101/raced", daysAgo(20), domain.StatusPass)
	candidates := scanCandidates(t, env)
	if len(candidates) != 1 {
		t.Fatalf("前置条件：清单应选中 1 条，实际 %d 条", len(candidates))
	}

	// 清单列出时真实引用为 0；真删前重扫发现有人引用了（例如刚保存了文章）。
	// 不得信任清单/账本，必须跳过。
	env.store.SetLiveRefs(img.ObjectKey, 1)

	res, err := env.purger.Purge(context.Background(), candidates, false)
	if err != nil {
		t.Fatalf("真删失败: %v", err)
	}
	if res.Deleted != 0 || res.Skipped != 1 {
		t.Fatalf("二次确认发现引用时应跳过: 删除=%d 跳过=%d", res.Deleted, res.Skipped)
	}
	if !skipReason(t, res, domain.PurgeSkipLiveRefs) {
		t.Fatalf("跳过原因应记录为真实引用: %+v", res.Skips)
	}
	if calls := env.storage.DeleteCalls(); calls != 0 {
		t.Fatalf("跳过时不得调用存储删除，实际 %d 次", calls)
	}
	if !env.store.Has(img.ID) {
		t.Fatalf("跳过时本地图片行必须保留: id=%s", img.ID)
	}
}

func TestOrphanPurgeKeepsRowWhenStorageDeleteFails(t *testing.T) {
	env := newPurgeEnv(t, domain.OrphanPurgeConfig{Enabled: true})

	img := seedImage(t, env.store, testPrefix+"/images/20260101/storage-fail", daysAgo(20), domain.StatusPass)
	candidates := scanCandidates(t, env)

	// 对象删不掉时整条跳过：绝不能把本地行删了留下孤儿对象。
	env.storage.FailNextDelete(errors.New("storage delete failed"))

	res, err := env.purger.Purge(context.Background(), candidates, false)
	if err != nil {
		t.Fatalf("真删失败: %v", err)
	}
	if res.Deleted != 0 || res.Skipped != 1 {
		t.Fatalf("对象删失败时应跳过整条: 删除=%d 跳过=%d", res.Deleted, res.Skipped)
	}
	if !skipReason(t, res, domain.PurgeSkipStorageDelete) {
		t.Fatalf("跳过原因应记录为存储删除失败: %+v", res.Skips)
	}
	// 必须证明"确实试过删对象"，而不是看起来跳过其实什么都没做。
	if calls := env.storage.DeleteCalls(); calls != 1 {
		t.Fatalf("应调用存储删除 1 次，实际 %d 次", calls)
	}
	if !env.store.Has(img.ID) {
		t.Fatalf("对象删除失败时本地图片行必须保留: id=%s", img.ID)
	}
	if !hasWarnLog(env.log) {
		t.Fatalf("存储删除失败必须告警，实际日志: %v", env.log.All())
	}
}

func TestOrphanPurgeIsIdempotentOnRepeatRun(t *testing.T) {
	env := newPurgeEnv(t, domain.OrphanPurgeConfig{Enabled: true})

	img := seedImage(t, env.store, testPrefix+"/images/20260101/idempotent", daysAgo(20), domain.StatusPass)
	candidates := scanCandidates(t, env)

	first, err := env.purger.Purge(context.Background(), candidates, false)
	if err != nil {
		t.Fatalf("第一次真删失败: %v", err)
	}
	if first.Deleted != 1 || env.storage.DeleteCalls() != 1 {
		t.Fatalf("第一次应删 1 条并调用存储删除 1 次: 删除=%d 调用=%d", first.Deleted, env.storage.DeleteCalls())
	}

	// 同一条目再执行一次：行已经没了，不能再删对象、再写任何东西。
	second, err := env.purger.Purge(context.Background(), candidates, false)
	if err != nil {
		t.Fatalf("第二次真删失败: %v", err)
	}
	if second.Deleted != 0 {
		t.Fatalf("重复执行不应再删成功任何条目，实际 %d", second.Deleted)
	}
	if calls := env.storage.DeleteCalls(); calls != 1 {
		t.Fatalf("重复执行对已删对象不得再发删除调用，累计 %d 次", calls)
	}
	if !skipReason(t, second, domain.PurgeSkipAlreadyGone) {
		t.Fatalf("已不存在的条目应按 already_gone 跳过: %+v", second.Skips)
	}
	if env.store.Has(img.ID) {
		t.Fatalf("图片行不应被重复执行复活: id=%s", img.ID)
	}
}

func TestOrphanPurgeDryRunWritesNothing(t *testing.T) {
	env := newPurgeEnv(t, domain.OrphanPurgeConfig{Enabled: true})

	img := seedImage(t, env.store, testPrefix+"/images/20260101/dry-run", daysAgo(20), domain.StatusPass)
	candidates := scanCandidates(t, env)

	before := env.store.Snapshot()

	res, err := env.purger.Purge(context.Background(), candidates, true)
	if err != nil {
		t.Fatalf("dry-run 失败: %v", err)
	}
	if !res.DryRun {
		t.Fatalf("结果应标记为 dry-run")
	}
	if res.WouldDelete != 1 || res.Deleted != 0 {
		t.Fatalf("dry-run 应只报计划删除 1 条、实际删除 0 条，实际 计划=%d 删除=%d", res.WouldDelete, res.Deleted)
	}
	if calls := env.storage.DeleteCalls(); calls != 0 {
		t.Fatalf("dry-run 不得调用存储删除，实际 %d 次", calls)
	}
	if after := env.store.Snapshot(); after != before {
		t.Fatalf("dry-run 改动了数据:\nbefore=%s\nafter=%s", before, after)
	}
	if !env.store.Has(img.ID) {
		t.Fatalf("dry-run 不得删除图片行: id=%s", img.ID)
	}
}

func TestOrphanPurgeCountsButKeepsRefsToDeletedArticle(t *testing.T) {
	env := newPurgeEnv(t, domain.OrphanPurgeConfig{Enabled: true})

	// 账本里有一行指向一篇已删除的文章：真实引用重扫为 0（软删文章不参与重扫），
	// 所以它会进清单。但删了图，用户恢复文章后就会裂图 —— 只能统计，不能自动删。
	img := seedImage(t, env.store, testPrefix+"/images/20260101/deleted-article", daysAgo(20), domain.StatusPass)
	env.store.AddDeletedArticleRef(img.ID, image_ref_enum.RefFieldArticleContent)
	candidates := scanCandidates(t, env)
	if len(candidates) != 1 {
		t.Fatalf("前置条件：应有 1 条候选，实际 %d 条", len(candidates))
	}

	res, err := env.purger.Purge(context.Background(), candidates, false)
	if err != nil {
		t.Fatalf("真删失败: %v", err)
	}
	if res.Deleted != 0 || res.Skipped != 1 {
		t.Fatalf("指向已删除文章的引用应受保护: 删除=%d 跳过=%d", res.Deleted, res.Skipped)
	}
	if !skipReason(t, res, domain.PurgeSkipDeletedArticleRefs) {
		t.Fatalf("跳过原因应记录为已删除文章引用: %+v", res.Skips)
	}
	if res.ProtectedRefRows != 1 {
		t.Fatalf("应统计 1 条受保护的引用行，实际 %d", res.ProtectedRefRows)
	}
	if calls := env.storage.DeleteCalls(); calls != 0 {
		t.Fatalf("受保护条目不得调用存储删除，实际 %d 次", calls)
	}
	if !env.store.Has(img.ID) {
		t.Fatalf("受保护条目不得删除图片行: id=%s", img.ID)
	}
	if count := env.store.LedgerRefCount(img.ID); count != 1 {
		t.Fatalf("受保护的账本行不得被自动删掉，实际剩 %d 条", count)
	}
}

func TestOrphanPurgeRespectsBatchLimit(t *testing.T) {
	env := newPurgeEnv(t, domain.OrphanPurgeConfig{Enabled: true, BatchLimit: 2})

	first := seedImage(t, env.store, testPrefix+"/images/20260101/limit-1", daysAgo(30), domain.StatusPass)
	second := seedImage(t, env.store, testPrefix+"/images/20260101/limit-2", daysAgo(25), domain.StatusPass)
	third := seedImage(t, env.store, testPrefix+"/images/20260101/limit-3", daysAgo(20), domain.StatusPass)

	// 直接构造候选，单独验证真删自己的条数上限（清单上限已在 #139 用例里钉住）。
	candidates := []domain.OrphanCandidate{
		{Image: first},
		{Image: second},
		{Image: third},
	}
	res, err := env.purger.Purge(context.Background(), candidates, false)
	if err != nil {
		t.Fatalf("真删失败: %v", err)
	}
	if res.Selected != 2 || res.Deleted != 2 {
		t.Fatalf("本轮应只处理 2 条: 选中=%d 删除=%d", res.Selected, res.Deleted)
	}
	if res.Remaining != 1 {
		t.Fatalf("超限时应报告剩余 1 条，实际 %d", res.Remaining)
	}
	if env.store.Has(third.ID) == false {
		t.Fatalf("超限的第 3 条不应被删")
	}
}

func TestOrphanPurgeSkipsAvatarAndUncleanableStatus(t *testing.T) {
	env := newPurgeEnv(t, domain.OrphanPurgeConfig{Enabled: true})

	avatar := seedImage(t, env.store, testPrefix+"/images/20260101/avatar", daysAgo(20), domain.StatusPass)
	env.store.AddLedgerRef(avatar.ID, image_ref_enum.RefFieldUserAvatar)

	reviewing := seedImage(t, env.store, testPrefix+"/images/20260101/reviewing", daysAgo(20), domain.StatusReviewing)
	blocked := seedImage(t, env.store, testPrefix+"/images/20260101/blocked", daysAgo(20), domain.StatusBlocked)

	// 绕过清单直接给候选：真删必须自己再守一遍"头像 / 审核中 / 封禁"。
	candidates := []domain.OrphanCandidate{
		{Image: avatar},
		{Image: reviewing},
		{Image: blocked},
	}
	res, err := env.purger.Purge(context.Background(), candidates, false)
	if err != nil {
		t.Fatalf("真删失败: %v", err)
	}
	if res.Deleted != 0 || res.Skipped != 3 {
		t.Fatalf("头像/审核中/封禁都应跳过: 删除=%d 跳过=%d", res.Deleted, res.Skipped)
	}
	if !skipReason(t, res, domain.PurgeSkipAvatar) {
		t.Fatalf("应记录头像跳过原因: %+v", res.Skips)
	}
	if !skipReason(t, res, domain.PurgeSkipStatus) {
		t.Fatalf("应记录状态跳过原因: %+v", res.Skips)
	}
	if calls := env.storage.DeleteCalls(); calls != 0 {
		t.Fatalf("跳过时不得调用存储删除，实际 %d 次", calls)
	}
}

func TestOrphanPurgeReadsRuntimeSwitchEachRun(t *testing.T) {
	cfg := domain.OrphanPurgeConfig{Enabled: false, GraceDays: 14, BatchLimit: 10}
	env := newPurgeEnvWithConfig(t, func() domain.OrphanPurgeConfig { return cfg })

	img := seedImage(t, env.store, testPrefix+"/images/20260101/switch", daysAgo(20), domain.StatusPass)
	candidates := scanCandidates(t, env)

	res, err := env.purger.Purge(context.Background(), candidates, false)
	if err != nil {
		t.Fatalf("真删失败: %v", err)
	}
	if res.Enabled || res.Deleted != 0 {
		t.Fatalf("开关关闭时不得删除任何东西: enabled=%v 删除=%d", res.Enabled, res.Deleted)
	}
	if calls := env.storage.DeleteCalls(); calls != 0 {
		t.Fatalf("开关关闭时不得调用存储删除，实际 %d 次", calls)
	}
	if !env.store.Has(img.ID) {
		t.Fatalf("开关关闭时图片行必须保留")
	}

	// 改配置后同一次进程内的下一次执行立即生效（不必重启）。
	cfg.Enabled = true
	again, err := env.purger.Purge(context.Background(), candidates, false)
	if err != nil {
		t.Fatalf("真删失败: %v", err)
	}
	if !again.Enabled || again.Deleted != 1 {
		t.Fatalf("打开开关后应立即删掉 1 条: enabled=%v 删除=%d", again.Enabled, again.Deleted)
	}
	if calls := env.storage.DeleteCalls(); calls != 1 {
		t.Fatalf("打开开关后应调用存储删除 1 次，累计 %d 次", calls)
	}
}

func TestOrphanPurgeLogsSkipsWithReasons(t *testing.T) {
	env := newPurgeEnv(t, domain.OrphanPurgeConfig{Enabled: true})

	img := seedImage(t, env.store, testPrefix+"/images/20260101/logged", daysAgo(20), domain.StatusPass)
	candidates := scanCandidates(t, env)
	env.store.SetLiveRefs(img.ObjectKey, 1)

	if _, err := env.purger.Purge(context.Background(), candidates, false); err != nil {
		t.Fatalf("真删失败: %v", err)
	}
	last := env.log.Last()
	if !strings.Contains(last, "跳过=1") || !strings.Contains(last, "真实引用") {
		t.Fatalf("结论日志应说明跳过数量与原因: %q", last)
	}
}

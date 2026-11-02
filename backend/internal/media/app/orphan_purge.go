package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/media/domain"
)

// OrphanPurger 是"确认后的孤儿图真删"用例（issue #140）。
//
// 它把不可逆删除拆成两道闸：
//  1. 清单（#139）先按真实引用选出候选；
//  2. 真删前再重扫一次真实引用、重读账本头像与"已删除文章引用"，
//     任一不通过就跳过整条。账本只说零引用不够，必须重扫。
//
// 删除本身不在这里实现：调用 domain.OrphanPurgeStore.Remove（由既有
// Service.Remove 提供），保证顺序永远是"先删对象、成功后才删行"。
type OrphanPurger struct {
	store   domain.OrphanStore
	guard   domain.OrphanPurgeGuard
	remover domain.OrphanPurgeStore
	clock   domain.Clock
	logger  domain.Logger
	cfg     func() domain.OrphanPurgeConfig
}

// NewOrphanPurger 构造真删用例。
//
// cfg 是函数而不是值：停手开关与冷静期/上限都每次现读，改配置不必重启。
func NewOrphanPurger(
	store domain.OrphanStore,
	guard domain.OrphanPurgeGuard,
	remover domain.OrphanPurgeStore,
	clock domain.Clock,
	logger domain.Logger,
	cfg func() domain.OrphanPurgeConfig,
) *OrphanPurger {
	if store == nil {
		panic("media/app: 孤儿图真删缺少清单数据源")
	}
	if guard == nil {
		panic("media/app: 孤儿图真删缺少已删除文章引用读数")
	}
	if remover == nil {
		panic("media/app: 孤儿图真删缺少删除能力")
	}
	if clock == nil {
		panic("media/app: 孤儿图真删缺少时钟")
	}
	if cfg == nil {
		panic("media/app: 孤儿图真删缺少配置")
	}
	return &OrphanPurger{store: store, guard: guard, remover: remover, clock: clock, logger: logger, cfg: cfg}
}

// Purge 对清单给出的候选做二次确认并真删。
//
// dryRun=true 时只做确认与统计，不碰存储、不写图片行/账本。
// 除全局取数失败（无法确认）外不返回 error：单条的失败按跳过处理，
// 一条失败不影响其余条目，且必须留下"为什么没删"的记录。
func (p *OrphanPurger) Purge(ctx context.Context, candidates []domain.OrphanCandidate, dryRun bool) (domain.OrphanPurgeResult, error) {
	cfg := p.cfg()
	scanCfg := cfg.ScanConfig()
	res := domain.OrphanPurgeResult{
		Enabled:    cfg.Enabled,
		DryRun:     dryRun,
		GraceDays:  scanCfg.GraceDays,
		BatchLimit: scanCfg.BatchLimit,
	}

	if !cfg.Enabled {
		p.logConclusion(res)
		return res, nil
	}

	selected := candidates
	if len(selected) > scanCfg.BatchLimit {
		selected = selected[:scanCfg.BatchLimit]
		res.Remaining = len(candidates) - scanCfg.BatchLimit
	}
	res.Selected = len(selected)
	if len(selected) == 0 {
		p.logConclusion(res)
		return res, nil
	}

	ids, keys := purgeIDsAndKeys(selected)
	// 二次确认的三个独立读数：账本头像、真实引用、指向已删除文章的引用。
	// 任一读数失败都不能"当成没有"继续删。
	ledger, err := p.store.LedgerSnapshot(ctx, ids)
	if err != nil {
		p.confirmFailed(err)
		return res, err
	}
	live, err := p.store.LiveRefs(ctx, keys)
	if err != nil {
		p.confirmFailed(err)
		return res, err
	}
	protected, err := p.guard.DeletedArticleRefs(ctx, ids)
	if err != nil {
		p.confirmFailed(err)
		return res, err
	}
	// "指向已删除文章的引用行"只统计、不自动删。放在确认之前先汇总，
	// 免得同一张图因为另一条跳过原因而漏报。
	for _, count := range protected {
		res.ProtectedRefRows += count
	}

	cutoff := p.clock.Now().Add(-scanCfg.GracePeriod())
	for _, candidate := range selected {
		img := candidate.Image
		if reason, ok := p.confirm(img, cutoff, ledger, live, protected); !ok {
			res.Skips = append(res.Skips, domain.OrphanPurgeSkip{Image: img, Reason: reason})
			continue
		}

		res.WouldDelete++
		if dryRun {
			continue
		}

		deleted, err := p.remover.Remove(ctx, []domain.ImageID{img.ID})
		if err != nil {
			res.Skips = append(res.Skips, p.skipForRemove(img, err))
			continue
		}
		res.Deleted += deleted
	}

	res.Skipped = len(res.Skips)
	p.logConclusion(res)
	return res, nil
}

// confirm 是逐条的二次确认。返回 ok=false 时 reason 说明为什么跳过。
func (p *OrphanPurger) confirm(
	img domain.Image,
	cutoff time.Time,
	ledger domain.LedgerSnapshot,
	live map[string]int,
	protected map[domain.ImageID]int,
) (domain.PurgeSkipReason, bool) {
	switch {
	case !cleanableStatus(img.Status):
		return domain.PurgeSkipStatus, false
	case ledger.Avatars[img.ID]:
		// 头像可能只在账本里记着（换过 CDN 域名时重扫匹配不上），无条件排除。
		return domain.PurgeSkipAvatar, false
	case strings.TrimSpace(img.ObjectKey) == "":
		// 没有对象键就没法重扫真实引用；确认不了就不删。
		return domain.PurgeSkipObjectKey, false
	case !img.CreatedAt.Before(cutoff):
		// 调用方给的候选可能已经过时；冷静期是误删的最后一道防线。
		return domain.PurgeSkipGrace, false
	case live[strings.TrimSpace(img.ObjectKey)] > 0:
		return domain.PurgeSkipLiveRefs, false
	case protected[img.ID] > 0:
		// 账本里还有指向已删除文章的引用行：只统计不删。
		// 用户可能恢复文章，图先没了就补不回来。
		return domain.PurgeSkipDeletedArticleRefs, false
	}
	return "", true
}

// skipForRemove 把删除链路的错误翻译成跳过原因。
func (p *OrphanPurger) skipForRemove(img domain.Image, err error) domain.OrphanPurgeSkip {
	skip := domain.OrphanPurgeSkip{Image: img, Detail: err.Error()}
	switch {
	case apperr.IsCode(err, apperr.CodeImageUnavailable):
		// 重复执行：行已经没了。这是幂等的正常结果，不告警。
		skip.Reason = domain.PurgeSkipAlreadyGone
		return skip
	case apperr.IsCode(err, apperr.CodeImageDeleteFailed):
		// 存储对象没删掉；Service.Remove 在删行之前就返回，图片行仍在。
		skip.Reason = domain.PurgeSkipStorageDelete
	default:
		skip.Reason = domain.PurgeSkipDelete
	}
	p.warnf("孤儿图回收跳过: image=%s key=%s 原因=%s err=%v", img.ID, img.ObjectKey, skip.Reason, err)
	return skip
}

// confirmFailed 记录"无法完成二次确认"并停下本轮。
func (p *OrphanPurger) confirmFailed(err error) {
	p.warnf("孤儿图回收停止: 二次确认读取失败，本轮不删除任何条目: %v", err)
}

// logConclusion 把结论写进日志：选了多少、删成功多少、跳过多少（含原因）。
//
// 删除不可逆，事后只能靠日志回答"这条为什么没了/为什么还在"。
func (p *OrphanPurger) logConclusion(res domain.OrphanPurgeResult) {
	if p.logger == nil {
		return
	}
	if !res.Enabled {
		p.logger.Infof("孤儿图回收未执行: 任务已关闭（image_gc.enabled=false）")
		return
	}
	p.logger.Infof(
		"孤儿图回收完成: dry_run=%v 冷静期=%d天 上限=%d 选中=%d 计划删除=%d 删除成功=%d 跳过=%d(%s) 指向已删除文章的引用行=%d 剩余=%d",
		res.DryRun, res.GraceDays, res.BatchLimit, res.Selected, res.WouldDelete,
		res.Deleted, res.Skipped, purgeSkipSummary(res.Skips), res.ProtectedRefRows, res.Remaining,
	)
}

func (p *OrphanPurger) warnf(format string, args ...any) {
	if p.logger != nil {
		p.logger.Warnf(format, args...)
	}
}

// cleanableStatus 复用清单的状态白名单（审核中/封禁/未知一律不删）。
func cleanableStatus(status domain.Status) bool {
	for _, allowed := range domain.CleanableStatuses() {
		if allowed == status {
			return true
		}
	}
	return false
}

// purgeIDsAndKeys 取本批候选的图片标识与对象键，空键与重复键都去掉。
func purgeIDsAndKeys(candidates []domain.OrphanCandidate) ([]domain.ImageID, []string) {
	ids := make([]domain.ImageID, 0, len(candidates))
	keys := make([]string, 0, len(candidates))
	seenIDs := make(map[domain.ImageID]struct{}, len(candidates))
	seenKeys := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seenIDs[candidate.ID]; !ok {
			seenIDs[candidate.ID] = struct{}{}
			ids = append(ids, candidate.ID)
		}
		key := strings.TrimSpace(candidate.ObjectKey)
		if key == "" {
			continue
		}
		if _, ok := seenKeys[key]; ok {
			continue
		}
		seenKeys[key] = struct{}{}
		keys = append(keys, key)
	}
	return ids, keys
}

// purgeSkipSummary 把跳过原因汇总成"真实引用=2 头像=1 ..."。
//
// 固定按原因名排序，日志才好对比两次执行；不按出现顺序，免得同一批数据
// 因为候选顺序变化就看起来"结果不同"。
func purgeSkipSummary(skips []domain.OrphanPurgeSkip) string {
	counts := make(map[domain.PurgeSkipReason]int, len(skips))
	for _, skip := range skips {
		counts[skip.Reason]++
	}
	if len(counts) == 0 {
		return "无"
	}
	reasons := make([]string, 0, len(counts))
	for reason := range counts {
		reasons = append(reasons, string(reason))
	}
	sort.Strings(reasons)
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, fmt.Sprintf("%s=%d", purgeSkipLabel(domain.PurgeSkipReason(reason)), counts[domain.PurgeSkipReason(reason)]))
	}
	return strings.Join(parts, " ")
}

// purgeSkipLabel 是跳过原因在日志里的中文名。
func purgeSkipLabel(reason domain.PurgeSkipReason) string {
	switch reason {
	case domain.PurgeSkipLiveRefs:
		return "真实引用"
	case domain.PurgeSkipAvatar:
		return "头像"
	case domain.PurgeSkipStatus:
		return "状态"
	case domain.PurgeSkipGrace:
		return "冷静期"
	case domain.PurgeSkipObjectKey:
		return "缺对象键"
	case domain.PurgeSkipDeletedArticleRefs:
		return "已删除文章引用"
	case domain.PurgeSkipStorageDelete:
		return "存储删除失败"
	case domain.PurgeSkipDelete:
		return "删除失败"
	case domain.PurgeSkipAlreadyGone:
		return "已不存在"
	default:
		return string(reason)
	}
}

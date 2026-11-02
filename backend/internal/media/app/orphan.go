package app

import (
	"context"
	"strings"

	"myblogx/internal/media/domain"
)

// OrphanScanner 是"无人引用的候选图清单"的只读用例（issue #139）。
//
// 它只依赖 domain.OrphanStore —— 一个没有写方法的读端口。删除能力
// （issue #140）将来另开用例、另注入带写能力的端口，不在这条链路上。
type OrphanScanner struct {
	store  domain.OrphanStore
	clock  domain.Clock
	logger domain.Logger
	cfg    func() domain.OrphanScanConfig
}

// NewOrphanScanner 构造清单用例。
//
// cfg 是函数而不是值：冷静期是运行时配置，每次扫描现读，改配置不必重启。
func NewOrphanScanner(store domain.OrphanStore, clock domain.Clock, logger domain.Logger, cfg func() domain.OrphanScanConfig) *OrphanScanner {
	if store == nil {
		panic("media/app: 孤儿图数据源未注入")
	}
	if clock == nil {
		panic("media/app: 时钟未注入")
	}
	if cfg == nil {
		panic("media/app: 孤儿图扫描配置未注入")
	}
	return &OrphanScanner{store: store, clock: clock, logger: logger, cfg: cfg}
}

// Scan 列出本轮的候选孤儿图。**只读：不写图片行、不写账本、不碰存储。**
//
// 判定顺序刻意如此：
//  1. 先按"状态可清理 + 过了冷静期"取一页（账本之外最便宜的过滤）；
//  2. 再读账本：它的行数用来**排除头像**、并在候选里并列展示；
//  3. 最后重扫真实引用。候选判定只看真实引用 —— 账本漏记时它可能给不出
//     证据，所以它只能缩小范围、不能单独定生死。
func (s *OrphanScanner) Scan(ctx context.Context) (domain.OrphanScanResult, error) {
	cfg := s.cfg().Normalize()
	cutoff := s.clock.Now().Add(-cfg.GracePeriod())

	spec := domain.OrphanScanSpec{
		UploadedBefore: cutoff,
		Statuses:       domain.CleanableStatuses(),
		Limit:          cfg.BatchLimit,
	}

	images, total, err := s.store.ListCandidates(ctx, spec)
	if err != nil {
		return domain.OrphanScanResult{}, err
	}
	if total < int64(len(images)) {
		// 数据源给出"总数小于本页"只可能是它自己算错；按本页兜底，
		// 免得"剩余"出现负数把运维的进度判断带偏。
		total = int64(len(images))
	}

	res := domain.OrphanScanResult{
		GraceDays:  cfg.GraceDays,
		BatchLimit: cfg.BatchLimit,
		Cutoff:     cutoff,
		Scanned:    len(images),
		Remaining:  int(total) - len(images),
	}
	if len(images) == 0 {
		s.logConclusion(res, cfg)
		return res, nil
	}

	ids := make([]domain.ImageID, 0, len(images))
	for _, img := range images {
		ids = append(ids, img.ID)
	}
	ledger, err := s.store.LedgerSnapshot(ctx, ids)
	if err != nil {
		return res, err
	}

	keys := objectKeysOf(images)
	live, err := s.store.LiveRefs(ctx, keys)
	if err != nil {
		return res, err
	}

	for _, img := range images {
		ledgerRefs := ledger.Refs[img.ID]
		liveRefs := live[img.ObjectKey]

		switch {
		case ledger.Avatars[img.ID]:
			// 头像：可能只在账本里记着（重扫因域名/前缀变化没匹配上），
			// 但删了会让用户长期裂图，所以无条件排除。
			res.Excluded++
			res.AvatarExcluded++
		case liveRefs > 0:
			// 还有真实引用：不论账本记没记，都不是孤儿。
			res.Excluded++
			res.RefExcluded++
			if ledgerRefs == 0 {
				// 账本说没有、重扫说有 —— 这是账本漏记的证据，单独标出。
				res.Mismatches = append(res.Mismatches, domain.OrphanMismatch{
					Image:      img,
					LedgerRefs: ledgerRefs,
					LiveRefs:   liveRefs,
				})
			}
		default:
			res.Candidates = append(res.Candidates, domain.OrphanCandidate{
				Image:      img,
				LedgerRefs: ledgerRefs,
				LiveRefs:   0,
			})
		}
	}

	s.logConclusion(res, cfg)
	return res, nil
}

// objectKeysOf 取本页图片的对象键，空键与重复键都去掉。
func objectKeysOf(images []domain.Image) []string {
	seen := make(map[string]struct{}, len(images))
	keys := make([]string, 0, len(images))
	for _, img := range images {
		key := strings.TrimSpace(img.ObjectKey)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

// logConclusion 把结论写进日志：跑过没有、检查多少、选中多少、排除多少、剩多少。
//
// 运维事后只能看日志 —— 「任务跑过没有」本身就是需要被回答的问题。
func (s *OrphanScanner) logConclusion(res domain.OrphanScanResult, cfg domain.OrphanScanConfig) {
	if s.logger == nil {
		return
	}
	s.logger.Infof(
		"孤儿图清单扫描完成: 只读=true 冷静期=%d天 上限=%d 检查=%d 选中=%d 排除=%d(头像=%d 仍被引用=%d) 不一致=%d 剩余=%d",
		cfg.GraceDays, cfg.BatchLimit, res.Scanned, len(res.Candidates),
		res.Excluded, res.AvatarExcluded, res.RefExcluded, len(res.Mismatches), res.Remaining,
	)
}

package cli

// 本文件是「孤儿图只读清单」（#139 / spec #138）。
//
// 引用账本（image_ref_models）由 image-ref river 从 binlog 维护，但它**只写不读**
// ——全仓没有任何代码按 image_id 反查引用数。这里是第一段读它的代码。
//
// #139 只做「看」：列出候选、暴露账本不一致，**不改任何数据**（图片行、引用行、
// 存储对象都不动）。真删在 #140，且真删前还要重扫一次真实引用。
//
// 已核实、不需要再排查的事实：
//   - 图片表 image_models：object_key 唯一、有 status 与软删列
//   - 引用账本 image_ref_models：image_id / ref_type / owner_id / field / position
//   - 引用源共五列：article_models.content 与 cover、banner_models.cover、
//     favorite_models.cover、user_models.avatar
//   - 站点配置 / 分类 / 通知**不存图**，所以没有"账本没覆盖的引用源"

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"myblogx/conf"
	"myblogx/internal/kernel/ctype"
	mediaDomain "myblogx/internal/media/domain"
)

// 默认参数（可用命令行覆盖）。
const (
	// defaultOrphanCooldownDays 是冷静期默认值。
	//
	// 14 天而非 7 天：要覆盖"周更节奏（本周传图、下周才发文）"以及 CDC 积压的
	// 余量。删错的代价远大于多存几天。
	defaultOrphanCooldownDays = 14
	// defaultOrphanLimit 是单次扫描的条数上限，避免一次扫爆。
	defaultOrphanLimit = 200
)

// orphanImageRow 是本脚本要读的图片行（只选需要的列）。
//
// ID 一律用 ctype.ID：雪花 ID 超出 JS 安全整数，裸整型会让契约印成 integer
// （archcheck R12）。
type orphanImageRow struct {
	ID        ctype.ID `gorm:"column:id"`
	CreatedAt time.Time
	UpdatedAt time.Time
	ObjectKey string
	URL       string
	UserID    ctype.ID
	Status    uint
}

// TableName 固定表名。
func (orphanImageRow) TableName() string { return "image_models" }

// ledgerRefRow 是引用账本行的按图聚合结果。
type ledgerRefRow struct {
	ImageID ctype.ID `gorm:"column:image_id"`
	Refs    int64    `gorm:"column:refs"`
}

// TableName 固定表名。
func (ledgerRefRow) TableName() string { return "image_ref_models" }

// orphanCandidate 是一条候选（只读清单的一行）。
type orphanCandidate struct {
	ID         ctype.ID
	ObjectKey  string
	CreatedAt  time.Time
	Status     uint
	LedgerRefs int64
	RealRefs   int64
	// LedgerInconsistent 表示"账本记零引用、但真实扫描发现仍被引用"。
	// 这是账本漏记的证据（CDC 有死信、引用靠 URL 反解，都可能漏）。
	LedgerInconsistent bool
}

// orphanImageReport 是一次清单的结论。
type orphanImageReport struct {
	Scanned              int
	Candidate            int
	SkippedFresh         int // 未过冷静期
	SkippedStatus        int // 审核中 / 封禁
	SkippedStatusDeleted int // 已删除状态
	Inconsistent         int // 账本零引用但真实有引用
	Truncated            bool
	Items                []orphanCandidate
}

// FlagOrphanImageList 打印孤儿图只读清单。
//
// 两个参数**优先取运行时配置**（cfg 来自 site_default_settings.yaml 的
// image_gc 段，执行时现读、改配置不必重启），命令行 flag 只作为一次性覆盖。
// 本命令**永远只读**：不会改图片行、引用行或存储对象。
func FlagOrphanImageList(
	db *gorm.DB, logger *logrus.Logger, cfg conf.ImageGC,
	cooldownOverride, limitOverride int,
) error {
	if db == nil {
		return fmt.Errorf("孤儿图清单失败: 数据库未初始化")
	}

	cooldownDays := resolveCooldownDays(cfg, cooldownOverride)
	limit := resolveBatchLimit(cfg, limitOverride)

	ctx := context.Background()
	report, err := scanOrphanImages(ctx, db, cooldownDays, limit)
	if err != nil {
		return err
	}

	logger.Infof(
		"孤儿图清单(只读): 扫描=%d 候选=%d 账本不一致=%d 跳过(未过冷静期=%d 审核中/封禁=%d 已删除=%d) 超限=%v",
		report.Scanned, report.Candidate, report.Inconsistent,
		report.SkippedFresh, report.SkippedStatus, report.SkippedStatusDeleted, report.Truncated,
	)

	for _, item := range report.Items {
		logger.Infof(
			"  候选 id=%s object_key=%s 上传=%s 状态=%d 账本引用=%d 真实引用=%d%s",
			item.ID, item.ObjectKey, item.CreatedAt.Format(time.RFC3339), item.Status,
			item.LedgerRefs, item.RealRefs,
			map[bool]string{true: " [账本不一致: 账本零引用但真实有引用]"}[item.LedgerInconsistent],
		)
	}
	if report.Candidate == 0 {
		logger.Infof("  没有符合条件的候选（这通常是好消息）")
	}
	if report.Truncated {
		logger.Infof("  已达单次上限 %d，还有剩余，下次继续", limit)
	}

	return nil
}

// resolveCooldownDays 把"配置 → flag 覆盖 → 兜底默认值"的优先级收敛成一个纯函数。
//
// 为什么必须先配置：冷静期是运营节奏量，改它不该要求重新发版或改命令行；
// flag 只是为了一次性试验（例如临时放宽到 30 天看看候选规模）。
func resolveCooldownDays(cfg conf.ImageGC, override int) int {
	if override > 0 {
		return override
	}
	if cfg.CooldownDays > 0 {
		return cfg.CooldownDays
	}
	return defaultOrphanCooldownDays
}

// resolveBatchLimit 同 resolveCooldownDays，作用于单次条数上限。
func resolveBatchLimit(cfg conf.ImageGC, override int) int {
	if override > 0 {
		return override
	}
	if cfg.BatchLimit > 0 {
		return cfg.BatchLimit
	}
	return defaultOrphanLimit
}

// scanOrphanImages 执行扫描并分类。纯读，不开事务、不写任何行。
func scanOrphanImages(
	ctx context.Context, db *gorm.DB, cooldownDays, limit int,
) (orphanImageReport, error) {
	var report orphanImageReport

	// 1. 候选范围：图片表全量（软删行由 gorm 默认 scope 排除）。
	var images []orphanImageRow
	if err := db.WithContext(ctx).
		Select("id", "created_at", "object_key", "url", "user_id", "status").
		Find(&images).Error; err != nil {
		return report, err
	}
	report.Scanned = len(images)
	if len(images) == 0 {
		return report, nil
	}

	// 2. 一次查全账本引用数（避免逐图查库）。
	var ledgerRows []ledgerRefRow
	if err := db.WithContext(ctx).
		Model(&ledgerRefRow{}).
		Select("image_id", "COUNT(*) AS refs").
		Group("image_id").
		Scan(&ledgerRows).Error; err != nil {
		return report, err
	}
	ledgerCount := make(map[ctype.ID]int64, len(ledgerRows))
	for _, row := range ledgerRows {
		ledgerCount[row.ImageID] = row.Refs
	}

	cutoff := time.Now().AddDate(0, 0, -cooldownDays)
	report.Items = make([]orphanCandidate, 0, limit)

	for _, image := range images {
		// 状态过滤：只处理"审核通过"的图。
		// 审核中尚未定性、封禁可能要作证据保留——都不替审核流程做决定。
		switch mediaDomain.Status(image.Status) {
		case mediaDomain.StatusPass:
			// 继续
		case mediaDomain.StatusReviewing:
			report.SkippedStatus++
			continue
		case mediaDomain.StatusBlocked:
			report.SkippedStatus++
			continue
		case mediaDomain.StatusDeleted:
			report.SkippedStatusDeleted++
			continue
		default:
			report.SkippedStatus++
			continue
		}

		// 冷静期：刚上传、可能马上要被引用的图不动。
		if image.CreatedAt.After(cutoff) {
			report.SkippedFresh++
			continue
		}

		refs := ledgerCount[image.ID]

		// 账本已有引用 → 不是孤儿（也天然保护了"当前头像"）。
		if refs > 0 {
			continue
		}

		// 账本零引用 → 必须重扫真实引用，不能只信账本。
		// 账本由 CDC 异步维护且有死信，引用靠从 URL 反解 object_key，
		// 换存储域名/前缀会大面积匹配不上；而删图不可逆。
		realRefs, err := countRealImageRefs(ctx, db, image.ObjectKey)
		if err != nil {
			return report, err
		}

		if realRefs > 0 {
			// 账本零引用但真实有引用：账本漏记的证据，单独暴露。
			report.Inconsistent++
			if len(report.Items) < limit {
				report.Items = append(report.Items, orphanCandidate{
					ID:                 image.ID,
					ObjectKey:          image.ObjectKey,
					CreatedAt:          image.CreatedAt,
					Status:             image.Status,
					LedgerRefs:         refs,
					RealRefs:           realRefs,
					LedgerInconsistent: true,
				})
			} else {
				report.Truncated = true
			}
			continue
		}

		// 真孤儿。
		if len(report.Items) < limit {
			report.Items = append(report.Items, orphanCandidate{
				ID:         image.ID,
				ObjectKey:  image.ObjectKey,
				CreatedAt:  image.CreatedAt,
				Status:     image.Status,
				LedgerRefs: refs,
				RealRefs:   realRefs,
			})
			report.Candidate++
		} else {
			report.Truncated = true
		}
	}

	// 候选计数要把"超限后漏掉的真孤儿"也算进去，否则日志会低估规模。
	return report, nil
}

// realRefSource 描述一个引用源列。
type realRefSource struct {
	Table  any
	Column string
}

// realRefSources 是全部引用源。与引用账本覆盖的四类一致：
// 文章正文/封面、轮播封面、收藏封面、用户头像。
//
// 用"列里是否包含该 object_key"直接判定，而不是把 URL 反解成 key 再比对：
// 前者是正向证据（内容里确实出现了这个 key），后者依赖 URL 形状
// （存储前缀一变就大面积匹配不上）。判定孤儿需要的是前者。
func realRefSources() []realRefSource {
	return []realRefSource{
		// articleRefRow 复用 flag_reconcile_digg.go 里的定义（同一张表、同一种用途）。
		{Table: &articleRefRow{}, Column: "content"},
		{Table: &articleRefRow{}, Column: "cover"},
		{Table: &orphanBannerRefRow{}, Column: "cover"},
		{Table: &orphanFavoriteRefRow{}, Column: "cover"},
		{Table: &orphanUserRefRow{}, Column: "avatar"},
	}
}

// countRealImageRefs 数出"当前数据里有多少行真的引用了这个 object_key"。
//
// 逐列 LIKE 查询：候选集通常很小（且有条数上限），可接受。
func countRealImageRefs(ctx context.Context, db *gorm.DB, objectKey string) (int64, error) {
	key := strings.TrimSpace(objectKey)
	if key == "" {
		return 0, nil
	}

	var total int64
	for _, source := range realRefSources() {
		// LIKE 的元字符要转义：object_key 里理论上可能含 % 或 _。
		pattern := "%" + escapeLikePattern(key) + "%"
		var count int64
		if err := db.WithContext(ctx).
			Model(source.Table).
			Where(source.Column+" LIKE ?", pattern).
			Count(&count).Error; err != nil {
			return 0, err
		}
		total += count
	}
	return total, nil
}

// escapeLikePattern 转义 LIKE 通配符，避免把 key 里的 % _ 当成通配。
func escapeLikePattern(raw string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(raw)
}

// orphanBannerRefRow / orphanFavoriteRefRow / orphanUserRefRow 只用于按列计数，
// 不需要完整字段。命名带 orphan 前缀，避免与其它运维脚本的行结构重名。
type orphanBannerRefRow struct {
	ID ctype.ID `gorm:"column:id"`
}

// TableName 固定表名。
func (orphanBannerRefRow) TableName() string { return "banner_models" }

type orphanFavoriteRefRow struct {
	ID ctype.ID `gorm:"column:id"`
}

// TableName 固定表名。
func (orphanFavoriteRefRow) TableName() string { return "favorite_models" }

type orphanUserRefRow struct {
	ID ctype.ID `gorm:"column:id"`
}

// TableName 固定表名。
func (orphanUserRefRow) TableName() string { return "user_models" }

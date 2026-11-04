package mysql

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// 本文件承载浏览历史的**过期清理**（#241 的后端缺口 4）。
//
// 为什么放在 adapter 而不是 cron 装配处：清洗 SQL 必须和 row 定义待在同一个包，
// 表名与列名才受类型约束；cron 侧只负责"什么时候跑、用什么时钟"。
// 与 counter_flush.go 是同一套分工。

// PurgeViewHistoryBefore 软删浏览时间早于 cutoff 的历史记录。
//
// 返回影响行数（清理任务只在这个数 > 0 时打日志，见 scheduler.Job）。
//
// 软删而不是硬删：与用户"移出历史"（RemoveViewHistory）保持同一语义，
// 误删可回滚；代价是同一 (article_id, user_id) 的软删行会一直占着唯一键
// ——这正是 ViewHistoryRepo.Upsert 用 Unscoped 查并复活旧行的理由。
//
// cutoff 由调用方传入（生产传 viewhistory.PurgeCutoff(time.Now())，用例传固定时钟）：
// SQL 里绝不取 now()，否则 90 天边界无法被钉死，也无法在事务外补跑历史区间。
func PurgeViewHistoryBefore(ctx context.Context, db *gorm.DB, cutoff time.Time) (int64, error) {
	if db == nil {
		return 0, nil
	}
	// GORM 看到 DeletedAt 字段会自动把它写成软删（UPDATE deleted_at），
	// 因此这里必须显式只命中未删行，避免把已清理的行反复刷时间戳。
	res := db.WithContext(ctx).
		Where("updated_at < ? AND deleted_at IS NULL", cutoff).
		Delete(&userArticleViewHistoryRow{})
	return res.RowsAffected, res.Error
}

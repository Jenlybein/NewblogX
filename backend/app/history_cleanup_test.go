package app_test

import (
	"context"
	"testing"
	"time"

	"myblogx/app"
	"myblogx/internal/testkit"
	"myblogx/internal/testkit/schema"
)

// TestPurgeViewHistoryUsesNinetyDayRetentionAtFixedClock 钉住每日清理任务的**口径**。
//
// 为什么在装配层（app）也要一条用例：adapter 的固定时钟用例守的是 SQL 的边界
// （严格小于 cutoff），但"调度任务究竟按多少天算"发生在 app/cron_wiring.go。
// 这个任务会真实抹掉用户的历史（软删），把 90 改成 30 或把 now 换成时区偏移的
// 时刻都不会让 SQL 用例红，只有这里会。
//
// 用内存 sqlite 而不是真 MySQL：要验证的是"装配层的时间与保留期"，不是 SQL
// 在 MySQL 上的行为（那由 adapter 的集成用例守）。
func TestPurgeViewHistoryUsesNinetyDayRetentionAtFixedClock(t *testing.T) {
	db := testkit.NewSQLite(t, &schema.UserArticleViewHistoryRow{})

	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	// 三条记录：91 天前（应清理）、恰好 90 天前（保留，边界严格小于）、89 天前（保留）。
	schema.InsertViewHistory(t, db, 1, 100, 1001, now.Add(-91*24*time.Hour))
	schema.InsertViewHistory(t, db, 2, 100, 1002, now.Add(-90*24*time.Hour))
	schema.InsertViewHistory(t, db, 3, 100, 1003, now.Add(-89*24*time.Hour))

	affected, err := app.PurgeViewHistory(context.Background(), db, now)
	if err != nil {
		t.Fatalf("清理浏览历史失败: %v", err)
	}
	if affected != 1 {
		t.Fatalf("固定时钟下应只清理 1 条（now-91d），实际 %d", affected)
	}

	// 软删：失效行仍在库里（可回溯），但不再出现在可见列表里。
	var visible int64
	if err := db.Model(&schema.UserArticleViewHistoryRow{}).Count(&visible).Error; err != nil {
		t.Fatalf("统计可见历史失败: %v", err)
	}
	if visible != 2 {
		t.Fatalf("清理后应还剩 2 条，实际 %d", visible)
	}
	var total int64
	if err := db.Unscoped().Model(&schema.UserArticleViewHistoryRow{}).Count(&total).Error; err != nil {
		t.Fatalf("统计全部历史失败: %v", err)
	}
	if total != 3 {
		t.Fatalf("软删不应物理删除，库里应仍有 3 行，实际 %d", total)
	}

	// 同一时刻再跑一次：幂等（0 条），也不会把已软删行的 deleted_at 再刷一遍。
	again, err := app.PurgeViewHistory(context.Background(), db, now)
	if err != nil {
		t.Fatalf("重复清理失败: %v", err)
	}
	if again != 0 {
		t.Fatalf("重复清理应 0 条，实际 %d", again)
	}
}

package app_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/app"
	filterdomain "myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/testkit"
	"myblogx/internal/testkit/schema"
)

// 关键词风险记录的保留期清理（ADR-0032 决策 10）。
//
// 三条要钉住：边界（严格小于 cutoff）、保留天数来自调用方（不是写死的 90）、
// 物理删除（软删会让那些上下文片段继续留在库里，与"保留 N 天"的承诺相反）。

// insertHit 写一条命中记录（时间由用例指定）。
func insertHit(t *testing.T, db *gorm.DB, id uint64, at time.Time) {
	t.Helper()
	row := schema.ContentFilterHitRow{
		BaseRow:   schema.BaseRow{ID: ctype.NewID(id), CreatedAt: at, UpdatedAt: at},
		Surface:   string(filterdomain.SurfaceArticle),
		ContentID: ctype.NewID(1),
		UserID:    ctype.NewID(7),
		HitCount:  1,
		Status:    string(filterdomain.StatusPending),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("插入命中记录失败: %v", err)
	}
}

func TestPurgeContentFilterHitsUsesConfiguredRetention(t *testing.T) {
	db := testkit.NewSQLite(t, &schema.ContentFilterHitRow{})
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

	// 保留 90 天：91 天前该删、恰好 90 天前保留（边界严格小于）、89 天前保留。
	insertHit(t, db, 1, now.Add(-91*24*time.Hour))
	insertHit(t, db, 2, now.Add(-90*24*time.Hour))
	insertHit(t, db, 3, now.Add(-89*24*time.Hour))

	affected, err := app.PurgeContentFilterHits(context.Background(), db, now, 90)
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if affected != 1 {
		t.Fatalf("保留 90 天时只该删 1 条（now-91d），实际 %d", affected)
	}

	var total int64
	if err := db.Model(&schema.ContentFilterHitRow{}).Count(&total).Error; err != nil {
		t.Fatalf("统计记录失败: %v", err)
	}
	if total != 2 {
		t.Fatalf("清理后应剩 2 条，实际 %d", total)
	}

	// 同一时刻再跑一次：幂等。
	again, err := app.PurgeContentFilterHits(context.Background(), db, now, 90)
	if err != nil {
		t.Fatalf("重复清理失败: %v", err)
	}
	if again != 0 {
		t.Fatalf("重复清理应是 0 条，实际 %d", again)
	}

	// 保留天数由调用方给：改成 1 天时剩下两条（89 / 90 天前）都该被删。
	affected, err = app.PurgeContentFilterHits(context.Background(), db, now, 1)
	if err != nil {
		t.Fatalf("短保留期清理失败: %v", err)
	}
	if affected != 2 {
		t.Fatalf("保留 1 天时应删掉剩下的 2 条，实际 %d", affected)
	}
}

func TestPurgeContentFilterHitsFallsBackToDomainRetention(t *testing.T) {
	db := testkit.NewSQLite(t, &schema.ContentFilterHitRow{})
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

	// 0 与负数表示"配置没写"，用领域兜底值 90 天。
	for i, days := range []int{0, -1} {
		insertHit(t, db, uint64(i+1), now.Add(-91*24*time.Hour))
		affected, err := app.PurgeContentFilterHits(context.Background(), db, now, days)
		if err != nil {
			t.Fatalf("retainDays=%d 清理失败: %v", days, err)
		}
		if affected != 1 {
			t.Fatalf("retainDays=%d 应按 %d 天兜底并删 1 条，实际 %d",
				days, filterdomain.DefaultRetainDays, affected)
		}
	}
}

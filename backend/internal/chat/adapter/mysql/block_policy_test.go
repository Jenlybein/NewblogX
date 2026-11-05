package mysql

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/test/testutil"
)

// 屏蔽 / 拉黑（ADR-0026）：方向性、软删解除、拉黑清单。
func TestBlockPolicyLevelIsDirectionalAndSoftDeletable(t *testing.T) {
	db := testutil.SetupSQLite(t, &chatBlockRow{})
	p := NewBlockPolicy(db)
	ctx := context.Background()
	a, b := ctype.NewID(1), ctype.NewID(2)

	row := chatBlockRow{
		ID: ctype.NewID(10), OwnerID: a, BlockedID: b,
		Level: uint8(domain.BlockLevelSoft), CreatedAt: time.Now(),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("建屏蔽记录失败: %v", err)
	}

	if lvl, err := p.Level(ctx, a, b); err != nil || lvl != domain.BlockLevelSoft {
		t.Fatalf("A→B 应为屏蔽，实际 level=%v err=%v", lvl, err)
	}
	if lvl, err := p.Level(ctx, b, a); err != nil || lvl != domain.BlockLevelNone {
		t.Fatalf("方向必须独立，B→A 应为 None，实际 level=%v err=%v", lvl, err)
	}

	// 升级为拉黑：Level 必须如实返回 level=2。
	if err := db.Model(&chatBlockRow{}).Where("id = ?", 10).Update("level", uint8(domain.BlockLevelHard)).Error; err != nil {
		t.Fatalf("升级拉黑失败: %v", err)
	}
	if lvl, err := p.Level(ctx, a, b); err != nil || lvl != domain.BlockLevelHard {
		t.Fatalf("A→B 应为拉黑，实际 level=%v err=%v", lvl, err)
	}

	// 软删 = 解除：查询走 GORM 默认范围，不再命中。
	if err := db.Delete(&chatBlockRow{}, "id = ?", 10).Error; err != nil {
		t.Fatalf("软删失败: %v", err)
	}
	if lvl, err := p.Level(ctx, a, b); err != nil || lvl != domain.BlockLevelNone {
		t.Fatalf("解除后应为 None，实际 level=%v err=%v", lvl, err)
	}
}

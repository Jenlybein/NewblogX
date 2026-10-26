package outboxrelay_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"

	"gorm.io/gorm"
)

func seed(t *testing.T, db *gorm.DB, status string, attempts int) ctype.ID {
	t.Helper()
	row := &schema.OutboxRow{
		EventName:     "article.published",
		AggregateType: "article",
		AggregateID:   7,
		Status:        status,
		Attempts:      attempts,
		AvailableAt:   time.Now().Add(-time.Hour),
		LastError:     "上次失败原因",
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("插入发件箱事件失败: %v", err)
	}
	return row.ID
}

func load(t *testing.T, db *gorm.DB, id ctype.ID) schema.OutboxRow {
	t.Helper()
	var row schema.OutboxRow
	if err := db.Where("id = ?", id).First(&row).Error; err != nil {
		t.Fatalf("读取发件箱事件失败: %v", err)
	}
	return row
}

// dead 的事件退回 pending，attempts 归零 —— 这是"人工重投"的核心语义：
// 重新给一轮重试机会，而不是在原计数上继续（否则下次失败立刻又变 dead）。
func TestReplayResetsDeadToPending(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.OutboxRow{})
	id := seed(t, db, outboxrelay.OutboxStatusDead, 5)

	r := outboxrelay.NewReplayer(db)
	err := r.Replay(context.Background(), deadletter.Record{
		Stream:   outboxrelay.ReplayStream,
		CdcJobID: fmt.Sprintf("outbox:%s", id.String()),
	})
	if err != nil {
		t.Fatalf("重投不应报错: %v", err)
	}

	got := load(t, db, id)
	if got.Status != outboxrelay.OutboxStatusPending {
		t.Fatalf("状态应为 pending，实际 %s", got.Status)
	}
	if got.Attempts != 0 {
		t.Fatalf("attempts 应归零，实际 %d", got.Attempts)
	}
	if got.LastError != "" {
		t.Fatalf("last_error 应清空，实际 %q", got.LastError)
	}
	if got.AvailableAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("available_at 应立即可领取，实际 %v", got.AvailableAt)
	}
}

// 不是 dead 的事件（已经 done，或中继正在处理）不该被改成 pending —— 那会把
// 一条已投递成功的事件重新投一遍。
func TestReplaySkipsNonDead(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.OutboxRow{})
	id := seed(t, db, outboxrelay.OutboxStatusDone, 1)

	r := outboxrelay.NewReplayer(db)
	err := r.Replay(context.Background(), deadletter.Record{
		Stream:   outboxrelay.ReplayStream,
		CdcJobID: fmt.Sprintf("outbox:%s", id.String()),
	})
	if !errors.Is(err, deadletter.ErrSkipped) {
		t.Fatalf("期望 ErrSkipped，实际 %v", err)
	}
	if got := load(t, db, id); got.Status != outboxrelay.OutboxStatusDone {
		t.Fatalf("done 状态不该被改动，实际 %s", got.Status)
	}
}

// cdc_job_id 格式不对时明确报错，不要"猜一个主键"。
func TestReplayRejectsBadJobID(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.OutboxRow{})
	r := outboxrelay.NewReplayer(db)
	for _, jobID := range []string{"", "outbox:", "es_river:foo", "outbox:not-a-number"} {
		if err := r.Replay(context.Background(), deadletter.Record{CdcJobID: jobID}); err == nil {
			t.Fatalf("%q 应报错", jobID)
		}
	}
}

// 没有数据库时明确失败，不能静默当成功（否则死信会被误标 resolved）。
func TestReplayWithoutDBFails(t *testing.T) {
	r := outboxrelay.NewReplayer(nil)
	if err := r.Replay(context.Background(), deadletter.Record{CdcJobID: "outbox:1"}); err == nil {
		t.Fatal("未注入数据库时应报错")
	}
}

//go:build integration

package outboxrelay_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// migrate 建 outbox 表与死信表（本组测试只关心这两张）。
func migrate(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	if err := db.AutoMigrate(
		&schema.OutboxRow{},
		&deadletter.Record{},
	); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return db
}

// seedEvent 插一条 pending 事件，返回它的 ID。
func seedEvent(t *testing.T, db *gorm.DB, name string, aggregateID uint64) ctype.ID {
	t.Helper()
	now := time.Now()
	row := schema.OutboxRow{
		EventName:     name,
		AggregateType: "article",
		AggregateID:   aggregateID,
		Payload:       `{"article_id":1}`,
		Status:        outboxrelay.OutboxStatusPending,
		AvailableAt:   now,
	}
	row.CreatedAt = now
	row.UpdatedAt = now
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("插入事件失败: %v", err)
	}
	return row.ID
}

func reload(t *testing.T, db *gorm.DB, id ctype.ID) schema.OutboxRow {
	t.Helper()
	var row schema.OutboxRow
	if err := db.Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatalf("读取事件失败: %v", err)
	}
	return row
}

// waitClaimable 等到这一行真的可以被再次领取。
//
// 为什么必须有这一步：失败后 markFailed 会把 available_at 设成
// now + BackoffMS*attempts，而领取条件是 available_at <= now。
// 本组测试用 BackoffMS=1 想让重试"几乎立刻"发生，但第二次失败后的窗口是 2ms ——
// 往往比"读一次行 + 发一次投递"还短，于是第三次 Dispatch 直接落空
// （它返回 nil 且不改状态，这是注释里写明的正常竞争，不是故障）。
// 结果是状态停在 failed，看起来像"重试耗尽没转 dead"。
// 这类竞态在慢机器上不容易撞到，CI 上却会稳定复现，所以显式把窗口等过去。
func waitClaimable(t *testing.T, db *gorm.DB, id ctype.ID) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for {
		row := reload(t, db, id)
		if !row.AvailableAt.After(time.Now()) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("事件 %s 在 3s 内始终不可领取（available_at=%s）",
				id, row.AvailableAt.Format(time.RFC3339Nano))
		}
		time.Sleep(time.Millisecond)
	}
}

// TestRelayDispatchSuccess 成功投递：订阅者被调用一次，事件标记为 done。
func TestRelayDispatchSuccess(t *testing.T) {
	db := migrate(t)
	relay := outboxrelay.New(db, nil, outboxrelay.Options{})

	var calls int32
	var gotName string
	relay.On("article.published", func(ctx context.Context, e outboxrelay.Event) error {
		atomic.AddInt32(&calls, 1)
		gotName = e.EventName
		return nil
	})

	id := seedEvent(t, db, "article.published", 1001)
	if err := relay.Dispatch(context.Background(), id); err != nil {
		t.Fatalf("投递返回错误: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("订阅者调用次数: got=%d want=1", got)
	}
	if gotName != "article.published" {
		t.Fatalf("事件名: got=%q want=%q", gotName, "article.published")
	}

	row := reload(t, db, id)
	if row.Status != outboxrelay.OutboxStatusDone {
		t.Fatalf("状态: got=%s want=%s", row.Status, outboxrelay.OutboxStatusDone)
	}
	if row.DispatchedAt == nil {
		t.Fatal("dispatched_at 未写入")
	}
}

// TestRelayDispatchIsIdempotent 幂等：重复投递同一条事件，订阅者只执行一次。
//
// 这是 at-least-once 语义下最关键的性质：第二次领取会被"条件 UPDATE"挡掉，
// 因为状态已经不是 pending/failed。
func TestRelayDispatchIsIdempotent(t *testing.T) {
	db := migrate(t)
	relay := outboxrelay.New(db, nil, outboxrelay.Options{})

	var calls int32
	relay.On("article.published", func(ctx context.Context, e outboxrelay.Event) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	id := seedEvent(t, db, "article.published", 1002)
	for i := 0; i < 3; i++ {
		if err := relay.Dispatch(context.Background(), id); err != nil {
			t.Fatalf("第 %d 次投递返回错误: %v", i+1, err)
		}
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("订阅者调用次数: got=%d want=1（重复投递必须被挡掉）", got)
	}
}

// TestRelayNoSubscriberMarksDone 没人订阅的事件不算失败，直接标记完成。
func TestRelayNoSubscriberMarksDone(t *testing.T) {
	db := migrate(t)
	relay := outboxrelay.New(db, nil, outboxrelay.Options{})

	id := seedEvent(t, db, "article.unknown", 1003)
	if err := relay.Dispatch(context.Background(), id); err != nil {
		t.Fatalf("投递返回错误: %v", err)
	}
	if row := reload(t, db, id); row.Status != outboxrelay.OutboxStatusDone {
		t.Fatalf("状态: got=%s want=%s", row.Status, outboxrelay.OutboxStatusDone)
	}
}

// TestRelayRetryThenDeadLetter 失败重试：耗尽重试后标记 dead 并写死信。
func TestRelayRetryThenDeadLetter(t *testing.T) {
	db := migrate(t)
	// BackoffMS=1（毫秒）让失败后几乎立刻可重试，不必在测试里等真实退避。
	// 注意不能写 0：withDefaults 会把 0 当成"未设置"并填成 1000ms。
	relay := outboxrelay.New(db, nil, outboxrelay.Options{MaxAttempts: 3, BackoffMS: 1})

	var calls int32
	relay.On("article.published", func(ctx context.Context, e outboxrelay.Event) error {
		atomic.AddInt32(&calls, 1)
		return errors.New("下游炸了")
	})

	id := seedEvent(t, db, "article.published", 1004)

	for i := 1; i <= 2; i++ {
		// 第二次之前要等第一次失败的退避窗口过去，否则这次投递会落空
		if i > 1 {
			waitClaimable(t, db, id)
		}
		if err := relay.Dispatch(context.Background(), id); err != nil {
			t.Fatalf("第 %d 次投递返回错误: %v", i, err)
		}
		row := reload(t, db, id)
		if row.Status != outboxrelay.OutboxStatusFailed {
			t.Fatalf("第 %d 次后状态: got=%s want=%s", i, row.Status, outboxrelay.OutboxStatusFailed)
		}
		if row.Attempts != i {
			t.Fatalf("第 %d 次后 attempts: got=%d want=%d", i, row.Attempts, i)
		}
	}

	// 第三次：重试耗尽 → dead + 死信
	waitClaimable(t, db, id)
	if err := relay.Dispatch(context.Background(), id); err != nil {
		t.Fatalf("第 3 次投递返回错误: %v", err)
	}
	row := reload(t, db, id)
	if row.Status != outboxrelay.OutboxStatusDead {
		t.Fatalf("耗尽后状态: got=%s want=%s", row.Status, outboxrelay.OutboxStatusDead)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("订阅者调用次数: got=%d want=3", got)
	}

	var dlq int64
	if err := db.Model(&deadletter.Record{}).
		Where("stream = ?", "outbox_relay").Count(&dlq).Error; err != nil {
		t.Fatalf("查询死信失败: %v", err)
	}
	if dlq != 1 {
		t.Fatalf("死信条数: got=%d want=1", dlq)
	}

	// dead 之后不再被领取
	if err := relay.Dispatch(context.Background(), id); err != nil {
		t.Fatalf("dead 后再投递返回错误: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("dead 后仍被投递: calls=%d", got)
	}
}

// TestRelaySweepEnqueues 补偿扫描能把 pending 事件捞出来。
func TestRelaySweepEnqueues(t *testing.T) {
	db := migrate(t)
	relay := outboxrelay.New(db, nil, outboxrelay.Options{SweepLimit: 10})

	seedEvent(t, db, "article.created", 2001)
	seedEvent(t, db, "article.created", 2002)

	n, err := relay.Sweep(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("入队条数: got=%d want=2", n)
	}
}

// TestRelaySweepSkipsFuture 还没到可领取时间的事件不会被扫描到。
func TestRelaySweepSkipsFuture(t *testing.T) {
	db := migrate(t)
	relay := outboxrelay.New(db, nil, outboxrelay.Options{})

	now := time.Now()
	row := schema.OutboxRow{
		EventName:   "article.published",
		AggregateID: 3001,
		Status:      outboxrelay.OutboxStatusFailed,
		AvailableAt: now.Add(time.Hour), // 退避中
	}
	row.CreatedAt = now
	row.UpdatedAt = now
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("插入事件失败: %v", err)
	}

	n, err := relay.Sweep(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if n != 0 {
		t.Fatalf("入队条数: got=%d want=0（未到点的应被跳过）", n)
	}
}

// TestRelayRecoversExpiredLease 租约恢复：
// worker 领了事件但崩溃（状态卡在 processing），租约到期后必须能被重新捞起。
// 没有这个机制，崩溃一次就永久丢一条事件。
func TestRelayRecoversExpiredLease(t *testing.T) {
	db := migrate(t)
	relay := outboxrelay.New(db, nil, outboxrelay.Options{LeaseDuration: time.Minute})

	var calls int32
	relay.On("article.published", func(ctx context.Context, e outboxrelay.Event) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	id := seedEvent(t, db, "article.published", 4001)

	// 模拟"领了但没做完"：直接把状态改成 processing，并让租约看起来已过期。
	stale := time.Now().Add(-5 * time.Minute)
	if err := db.Model(&schema.OutboxRow{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":     outboxrelay.OutboxStatusProcessing,
			"updated_at": stale,
		}).Error; err != nil {
		t.Fatalf("制造陈旧 processing 失败: %v", err)
	}

	n, err := relay.Sweep(context.Background())
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("过期租约未被捞回: 入队=%d want=1", n)
	}
	if err := relay.Dispatch(context.Background(), id); err != nil {
		t.Fatalf("投递失败: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("订阅者调用次数: got=%d want=1", got)
	}
	if row := reload(t, db, id); row.Status != outboxrelay.OutboxStatusDone {
		t.Fatalf("状态: got=%s want=%s", row.Status, outboxrelay.OutboxStatusDone)
	}
}

// TestRelayRunAndClose 启动后 worker 会自动消费队列里的事件。
func TestRelayRunAndClose(t *testing.T) {
	db := migrate(t)
	relay := outboxrelay.New(db, nil, outboxrelay.Options{
		Workers:       2,
		SweepInterval: 50 * time.Millisecond,
	})

	var calls int32
	relay.On("article.published", func(ctx context.Context, e outboxrelay.Event) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	id := seedEvent(t, db, "article.published", 5001)
	relay.Run()
	relay.EnqueueID(id)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if reload(t, db, id).Status == outboxrelay.OutboxStatusDone {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	relay.Close()

	if row := reload(t, db, id); row.Status != outboxrelay.OutboxStatusDone {
		t.Fatalf("worker 未消费: 状态=%s", row.Status)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("订阅者调用次数: got=%d want=1", got)
	}
}

// seedStaleRow 造一条"状态为 status、最后更新时间在 age 之前"的事件行。
//
// updated_at 必须用原始 SQL 回填：GORM 对 UpdatedAt 字段有 autoUpdateTime
// 回调，用 Create/Updates 都改写不成"陈旧"值。
func seedStaleRow(t *testing.T, db *gorm.DB, status string, age time.Duration) ctype.ID {
	t.Helper()
	now := time.Now()
	row := schema.OutboxRow{
		EventName:     "article.published",
		AggregateType: "article",
		AggregateID:   9001,
		Payload:       "{}",
		Status:        status,
		AvailableAt:   now,
	}
	row.CreatedAt = now
	row.UpdatedAt = now
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("插入事件失败: %v", err)
	}

	if err := db.Exec(
		"UPDATE outbox_event_models SET updated_at = ? WHERE id = ?",
		now.Add(-age), row.ID,
	).Error; err != nil {
		t.Fatalf("回填 updated_at 失败: %v", err)
	}
	return row.ID
}

// TestRelayCleanup 保留期清理：
//   - 只删过期的 done / dead；
//   - 未过期的 done / dead 保留；
//   - pending / failed / processing 无论多老都不删（还欠一次投递）；
//   - 是物理删除（Unscoped 也查不到），否则回收不了磁盘。
func TestRelayCleanup(t *testing.T) {
	db := migrate(t)
	relay := outboxrelay.New(db, nil, outboxrelay.Options{
		RetentionDone: 24 * time.Hour,
		RetentionDead: 72 * time.Hour,
		CleanupLimit:  10,
	})

	expiredDone := seedStaleRow(t, db, outboxrelay.OutboxStatusDone, 48*time.Hour)
	obsoleteDone := seedStaleRow(t, db, outboxrelay.OutboxStatusDone, time.Hour)
	expiredDead := seedStaleRow(t, db, outboxrelay.OutboxStatusDead, 100*time.Hour)
	obsoleteDead := seedStaleRow(t, db, outboxrelay.OutboxStatusDead, time.Hour)
	// 这三条即使"很老"也不能删：它们还欠一次投递。
	oldPending := seedStaleRow(t, db, outboxrelay.OutboxStatusPending, 1000*time.Hour)
	oldFailed := seedStaleRow(t, db, outboxrelay.OutboxStatusFailed, 1000*time.Hour)
	oldProcessing := seedStaleRow(t, db, outboxrelay.OutboxStatusProcessing, 1000*time.Hour)

	n, err := relay.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("删除条数: got=%d want=2", n)
	}

	countUnscoped := func(id ctype.ID) int64 {
		t.Helper()
		var cnt int64
		if err := db.Unscoped().Model(&schema.OutboxRow{}).
			Where("id = ?", id).Count(&cnt).Error; err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		return cnt
	}

	for _, id := range []ctype.ID{expiredDone, expiredDead} {
		if got := countUnscoped(id); got != 0 {
			t.Fatalf("过期的终结行未被物理删除: id=%s count=%d", id, got)
		}
	}
	for _, id := range []ctype.ID{obsoleteDone, obsoleteDead, oldPending, oldFailed, oldProcessing} {
		if got := countUnscoped(id); got != 1 {
			t.Fatalf("行被误删: id=%s count=%d", id, got)
		}
	}
}

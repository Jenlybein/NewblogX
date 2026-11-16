package app

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/testkit/schema"
	usermysql "myblogx/internal/user/adapter/mysql"
)

// 禁言到期对账（票 399 / ADR-0032 决策 8）：
//   - 注册为每小时的独立任务（spec 要求"每小时对账"，与计数并账的周期无关）；
//   - 任务体：到期行清空、未到期行不动；
//   - 重跑幂等（0 行）；
//   - 与惰性判定结论一致 —— 对账只是把已经到期的列擦掉。

// muteJob 取出并账任务；没有就说明装配漏了这条任务。
func (s *recordingScheduler) muteJob(t *testing.T) recordedJob {
	t.Helper()
	for _, e := range s.every {
		if e.job.Name == "禁言到期对账" {
			return e
		}
	}
	t.Fatalf("必须注册「禁言到期对账」任务，实际注册了 %d 个 Every 任务", len(s.every))
	return recordedJob{}
}

// seedMutedUser 插入一个带禁言到期时间的账号。
func seedMutedUser(t *testing.T, infra *Infra, id uint64, username string, until time.Time) {
	t.Helper()
	schema.InsertUser(t, infra.DB, schema.UserSpec{
		ID:        id,
		Username:  username,
		Nickname:  username,
		MuteUntil: &until,
	})
}

// readMuteUntil 回读账号的禁言列（NULL 时返回 nil）。
func readMuteUntil(t *testing.T, infra *Infra, id uint64) *time.Time {
	t.Helper()
	var row schema.UserRow
	if err := infra.DB.First(&row, "id = ?", id).Error; err != nil {
		t.Fatalf("回查用户失败: %v", err)
	}
	return row.MuteUntil
}

func TestMuteReconcileJobRegisteredHourly(t *testing.T) {
	t.Parallel()

	infra := newCronTestInfra(t, 5, schema.UserCore)
	sched := wireRecordingCron(t, infra)

	job := sched.muteJob(t)
	if job.interval != time.Hour {
		t.Fatalf("禁言对账应每小时一次，实际 %s", job.interval)
	}
	if job.job.LockKey == "" {
		t.Fatal("禁言对账必须带锁（多实例 cron 下不能并发跑）")
	}
}

func TestMuteReconcileClearsExpiredOnlyAndIsIdempotent(t *testing.T) {
	t.Parallel()

	infra := newCronTestInfra(t, 30, schema.UserCore)
	// 任务体自己取 time.Now()，所以这里的"已到期 / 未到期"也必须相对真实时间造，
	// 不能写死日期（否则测试在某个时区/某一天会假红）。
	now := time.Now()
	expired := now.Add(-time.Minute)
	future := now.Add(48 * time.Hour)
	seedMutedUser(t, infra, 1001, "expired", expired)
	seedMutedUser(t, infra, 1002, "future", future)
	schema.InsertUser(t, infra.DB, schema.UserSpec{ID: 1003, Username: "never", Nickname: "never"})

	sched := wireRecordingCron(t, infra)
	job := sched.muteJob(t)

	// 到期账号 ID 列表给"自动恢复"留审计记录用（AC：解除与到期都留下记录）。
	ids, err := usermysql.ExpiredMuteUserIDs(context.Background(), infra.DB, time.Now())
	if err != nil {
		t.Fatalf("读到期禁言账号失败: %v", err)
	}
	if len(ids) != 1 || ids[0] != 1001 {
		t.Fatalf("到期账号应只有 1001，实际 %v", ids)
	}

	affected, err := job.job.Run(context.Background())
	if err != nil {
		t.Fatalf("对账不应失败: %v", err)
	}
	if affected != 1 {
		t.Fatalf("应清空 1 个已到期账号，实际 %d", affected)
	}
	if got := readMuteUntil(t, infra, 1001); got != nil {
		t.Fatalf("已到期账号的禁言应被清空，实际 %v", got)
	}
	if got := readMuteUntil(t, infra, 1002); got == nil || !got.Equal(future) {
		t.Fatalf("未到期账号的禁言应保持 %v，实际 %v", future, got)
	}

	// 重跑幂等：没有到期行时 affected=0。
	again, err := job.job.Run(context.Background())
	if err != nil || again != 0 {
		t.Fatalf("重跑对账应幂等（0 行），实际 %d, %v", again, err)
	}
}

// TestReconcileExpiredMutesWithoutInfra 无 DB 的进程（比如纯 web 静态进程）安全返回 0。
func TestReconcileExpiredMutesWithoutInfra(t *testing.T) {
	t.Parallel()

	if n, err := ReconcileExpiredMutes(context.Background(), nil, time.Now()); err != nil || n != 0 {
		t.Fatalf("无 infra 时应返回 0, nil，实际 %d, %v", n, err)
	}
}

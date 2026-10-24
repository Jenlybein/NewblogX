package mysql_test

import (
	"context"
	"errors"
	"testing"

	"myblogx/internal/platform/mysql"
	"myblogx/testkit"

	"gorm.io/gorm"
)

// item 是事务测试用表。
type item struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

func countItems(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&item{}).Count(&n).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	return n
}

// 回调返回 nil → 提交，写入可见。
func TestTxManagerCommit(t *testing.T) {
	t.Parallel()

	db := testkit.NewSQLite(t, &item{})
	tm := mysql.NewTxManager(db)

	err := tm.WithinTx(context.Background(), func(ctx context.Context, tx *gorm.DB) error {
		return tx.Create(&item{Name: "committed"}).Error
	})
	if err != nil {
		t.Fatalf("事务应成功: %v", err)
	}
	if got := countItems(t, db); got != 1 {
		t.Fatalf("提交后应有 1 行，实际 %d", got)
	}
}

// 回调返回 error → 回滚，写入不可见。
func TestTxManagerRollback(t *testing.T) {
	t.Parallel()

	db := testkit.NewSQLite(t, &item{})
	tm := mysql.NewTxManager(db)

	boom := errors.New("boom")
	err := tm.WithinTx(context.Background(), func(ctx context.Context, tx *gorm.DB) error {
		if err := tx.Create(&item{Name: "rolled-back"}).Error; err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("应原样返回回调错误，实际 %v", err)
	}
	if got := countItems(t, db); got != 0 {
		t.Fatalf("回滚后应为 0 行，实际 %d", got)
	}
}

// 未注入连接时必须报错而不是 panic。
func TestTxManagerNilDB(t *testing.T) {
	t.Parallel()

	tm := mysql.NewTxManager(nil)
	err := tm.WithinTx(context.Background(), func(ctx context.Context, tx *gorm.DB) error { return nil })
	if !errors.Is(err, mysql.ErrNilDB) {
		t.Fatalf("期望 ErrNilDB，实际 %v", err)
	}
}

// 空回调必须被拒绝。
func TestTxManagerNilFunc(t *testing.T) {
	t.Parallel()

	db := testkit.NewSQLite(t, &item{})
	tm := mysql.NewTxManager(db)
	if err := tm.WithinTx(context.Background(), nil); !errors.Is(err, mysql.ErrNilTxFunc) {
		t.Fatalf("期望 ErrNilTxFunc，实际 %v", err)
	}
}

// fake 必须如实统计提交与回滚。
func TestFakeTxManagerStats(t *testing.T) {
	t.Parallel()

	fake := mysql.NewFakeTxManager(nil)

	if err := fake.WithinTx(context.Background(), func(context.Context, *gorm.DB) error { return nil }); err != nil {
		t.Fatalf("提交应成功: %v", err)
	}
	boom := errors.New("x")
	_ = fake.WithinTx(context.Background(), func(context.Context, *gorm.DB) error { return boom })

	st := fake.Stats()
	if st.Calls != 2 || st.Commits != 1 || st.Rollbacks != 1 {
		t.Fatalf("统计不符: %+v", st)
	}
}

// FailNext 必须跳过回调并计入 Forced。
func TestFakeTxManagerForceFailure(t *testing.T) {
	t.Parallel()

	fake := mysql.NewFakeTxManager(nil)
	want := errors.New("injected")
	fake.FailNext(want)

	executed := false
	err := fake.WithinTx(context.Background(), func(context.Context, *gorm.DB) error {
		executed = true
		return nil
	})
	if !errors.Is(err, want) {
		t.Fatalf("期望注入错误，实际 %v", err)
	}
	if executed {
		t.Fatal("强制失败时不应执行回调")
	}
	if st := fake.Stats(); st.Forced != 1 || st.Commits != 0 {
		t.Fatalf("统计不符: %+v", st)
	}

	fake.ClearFailure()
	if err := fake.WithinTx(context.Background(), func(context.Context, *gorm.DB) error { return nil }); err != nil {
		t.Fatalf("清除失败后应恢复正常: %v", err)
	}
}

// fake 接真实连接时，回滚必须真的撤销写入。
func TestFakeTxManagerRealRollback(t *testing.T) {
	t.Parallel()

	db := testkit.NewSQLite(t, &item{})
	fake := mysql.NewFakeTxManager(db)

	err := fake.WithinTx(context.Background(), func(ctx context.Context, tx *gorm.DB) error {
		if err := tx.Create(&item{Name: "nope"}).Error; err != nil {
			return err
		}
		return errors.New("rollback please")
	})
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if got := countItems(t, db); got != 0 {
		t.Fatalf("fake 回滚应撤销写入，实际 %d 行", got)
	}
}

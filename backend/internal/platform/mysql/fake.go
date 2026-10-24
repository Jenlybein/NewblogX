package mysql

import (
	"context"
	"sync"

	"gorm.io/gorm"
)

// TxStats 是 FakeTxManager 的调用统计，供测试断言。
type TxStats struct {
	Calls     int // 总调用次数
	Commits   int // 回调返回 nil 的次数
	Rollbacks int // 回调返回 error 的次数
	Forced    int // 被 FailNext 强制失败的次数
}

// FakeTxManager 是可观测、可注入失败的事务管理器，只用于测试。
//
// 两种用法：
//   - 传 nil 连接：纯逻辑测试，回调拿到的 tx 为 nil，只验证“提交/回滚/强制失败”分支；
//   - 传真实连接（如 testkit.NewSQLite）：回调在真事务里执行，回滚会真的撤销写入，
//     同时保留调用统计。
type FakeTxManager struct {
	mu       sync.Mutex
	tx       *gorm.DB
	forceErr error
	stats    TxStats
}

// NewFakeTxManager 创建 fake。tx 可为 nil。
func NewFakeTxManager(tx *gorm.DB) *FakeTxManager {
	return &FakeTxManager{tx: tx}
}

// FailNext 让后续调用直接返回 err 且不执行回调，直到 ClearFailure。
func (f *FakeTxManager) FailNext(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forceErr = err
}

// ClearFailure 取消强制失败。
func (f *FakeTxManager) ClearFailure() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forceErr = nil
}

// Stats 返回当前统计快照。
func (f *FakeTxManager) Stats() TxStats {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stats
}

// WithinTx 执行回调并记录结果。
func (f *FakeTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context, tx *gorm.DB) error) error {
	f.mu.Lock()
	f.stats.Calls++
	forced := f.forceErr
	if forced != nil {
		f.stats.Forced++
	}
	tx := f.tx
	f.mu.Unlock()

	if forced != nil {
		return forced
	}
	if fn == nil {
		return ErrNilTxFunc
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var err error
	if tx != nil {
		err = tx.WithContext(ctx).Transaction(func(t *gorm.DB) error { return fn(ctx, t) })
	} else {
		err = fn(ctx, nil)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		f.stats.Rollbacks++
		return err
	}
	f.stats.Commits++
	return nil
}

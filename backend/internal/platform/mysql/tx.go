// Package mysql 提供 MySQL/GORM 的技术适配层。
//
// 这里只放“数据库怎么用”的技术能力（连接、事务边界），不放业务语义。
// 业务层依赖本包的窄接口，而不是 *gorm.DB，这样才可能用 fake 做单测。
package mysql

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var (
	// ErrNilDB 表示构造时传入的 *gorm.DB 为空。
	ErrNilDB = errors.New("mysql: nil *gorm.DB")
	// ErrNilTxFunc 表示事务回调为空。
	ErrNilTxFunc = errors.New("mysql: nil tx func")
)

// TxManager 定义一次事务的边界。
//
// 用法（业务层只依赖这个接口）：
//
//	err := tx.WithinTx(ctx, func(ctx context.Context, tx *gorm.DB) error {
//	    // 事务内的读写
//	    return nil
//	})
//
// 回调返回 error 则回滚，返回 nil 则提交。ctx 会透传给 GORM。
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx *gorm.DB) error) error
}

// gormTxManager 是基于 *gorm.DB.Transaction 的实现。
type gormTxManager struct {
	db *gorm.DB
}

// NewTxManager 用给定连接构造事务管理器。
// db 为空时不 panic，而是让 WithinTx 返回 ErrNilDB——启动期错误应尽早暴露，
// 但不该在装配阶段因为一个 nil 直接杀死进程难以定位。
func NewTxManager(db *gorm.DB) TxManager {
	return &gormTxManager{db: db}
}

// WithinTx 在事务中执行 fn。
func (m *gormTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context, tx *gorm.DB) error) error {
	if m == nil || m.db == nil {
		return ErrNilDB
	}
	if fn == nil {
		return ErrNilTxFunc
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, tx)
	})
}

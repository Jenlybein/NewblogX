//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"myblogx/internal/testkit/schema"
	"testing"

	"gorm.io/gorm"

	articlemysql "myblogx/internal/article/adapter/mysql"
	"myblogx/internal/article/app"
	"myblogx/internal/kernel/ctype"
)

// TestOutboxWriterTxCommit 事务提交后，事件行必须存在。
func TestOutboxWriterTxCommit(t *testing.T) {
	db := migrate(t)

	err := db.Transaction(func(tx *gorm.DB) error {
		w := articlemysql.NewOutboxWriter(tx)
		return w.Emit(context.Background(), app.Event{
			Name:          app.EventArticlePublished,
			Payload:       []byte(`{"article_id":7001}`),
			AggregateID:   ctype.NewID(7001),
			AggregateType: "article",
		})
	})
	if err != nil {
		t.Fatalf("提交事务失败: %v", err)
	}

	var row schema.OutboxRow
	if err := db.Where("aggregate_id = ?", 7001).Take(&row).Error; err != nil {
		t.Fatalf("提交后查不到事件行: %v", err)
	}
	if row.EventName != app.EventArticlePublished {
		t.Fatalf("事件名: got=%q want=%q", row.EventName, app.EventArticlePublished)
	}
	if row.Status != schema.OutboxStatusPending {
		t.Fatalf("初始状态: got=%s want=%s", row.Status, schema.OutboxStatusPending)
	}
	if row.Payload != `{"article_id":7001}` {
		t.Fatalf("payload: got=%q", row.Payload)
	}
}

// TestOutboxWriterTxRollback 事务回滚后，事件行必须不存在。
//
// 这是事务型发件箱最关键的性质：事件存在 ⇔ 业务已提交。
// 若这条不成立，就会出现"文章没发出来却通知了用户"这类幽灵副作用。
func TestOutboxWriterTxRollback(t *testing.T) {
	db := migrate(t)

	wantErr := errors.New("故意让事务回滚")
	err := db.Transaction(func(tx *gorm.DB) error {
		w := articlemysql.NewOutboxWriter(tx)
		if emitErr := w.Emit(context.Background(), app.Event{
			Name:        app.EventArticleDeleted,
			Payload:     []byte(`{"article_id":7002}`),
			AggregateID: ctype.NewID(7002),
		}); emitErr != nil {
			return emitErr
		}
		return wantErr // 让事务回滚
	})
	if err == nil {
		t.Fatal("事务应当因错误回滚")
	}

	var count int64
	if err := db.Model(&schema.OutboxRow{}).
		Where("aggregate_id = ?", 7002).Count(&count).Error; err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("回滚后仍有事件行: count=%d", count)
	}
}

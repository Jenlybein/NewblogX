//go:build integration

package deadletter_test

import (
	"testing"
	"time"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"

	"gorm.io/gorm"
)

// 死信删除（票 356）必须在真 MySQL 上验：
// 它是一条物理删除语句，而"只删死信、不动 outbox 的 dead 行"是这票最关键的
// 安全边界 —— 内存 fake 无法证伪 relay 会不会因此把某条消息重新投递。

func migrate(t *testing.T) *gorm.DB {
	t.Helper()
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.DeadLetter, schema.Outbox)
	return db
}

func seedDeadLetter(t *testing.T, db *gorm.DB, id uint64, jobID string) {
	t.Helper()
	row := schema.CdcDeadLetterRow{
		ID:          ctype.NewID(id),
		Stream:      "es_river",
		CdcJobID:    jobID,
		SourceTable: "article_models",
		Action:      "update",
		TargetKey:   "1",
		PayloadJSON: "{}",
		Status:      "pending",
		ErrorCode:   "es_timeout",
		ErrorMsg:    "超时",
		CreatedAt:   time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("插入死信 %d 失败: %v", id, err)
	}
}

// TestDeleteByIDsRemovesOnlyGivenRows 锁住"只删点出来的那几条"。
func TestDeleteByIDsRemovesOnlyGivenRows(t *testing.T) {
	db := migrate(t)
	seedDeadLetter(t, db, 101, "job-a")
	seedDeadLetter(t, db, 102, "job-b")
	seedDeadLetter(t, db, 103, "job-c")

	deleted, err := deadletter.DeleteByIDs(db, []string{ctype.NewID(101).String(), ctype.NewID(103).String()})
	if err != nil {
		t.Fatalf("删除死信失败: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("应删掉 2 行，实际 %d", deleted)
	}

	var left []schema.CdcDeadLetterRow
	if err := db.Find(&left).Error; err != nil {
		t.Fatalf("读回死信失败: %v", err)
	}
	if len(left) != 1 {
		t.Fatalf("应只剩 1 行，实际 %d", len(left))
	}
	if left[0].CdcJobID != "job-b" {
		t.Fatalf("剩下的应该是没被点中的 job-b，实际 %s", left[0].CdcJobID)
	}
}

// TestDeleteByIDsDoesNotTouchOutboxDeadRows 是这票的安全边界。
//
// outbox 的 dead 行不会被 relay 回收（它只取 pending / 到期的行），所以删死信
// 不会让某条消息被重新投递；这条用例把"删了死信之后 outbox 的 dead 行还在"钉住，
// 免得以后有人"顺手清理"把两边一起删，导致消息永久消失。
func TestDeleteByIDsDoesNotTouchOutboxDeadRows(t *testing.T) {
	db := migrate(t)
	seedDeadLetter(t, db, 201, "job-x")

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	outbox := schema.OutboxRow{
		BaseRow:       schema.BaseRow{ID: ctype.NewID(301), CreatedAt: at, UpdatedAt: at},
		EventName:     "article.updated",
		AggregateType: "article",
		AggregateID:   1,
		Payload:       "{}",
		Status:        schema.OutboxStatusDead,
		AvailableAt:   at,
	}
	if err := db.Create(&outbox).Error; err != nil {
		t.Fatalf("插入 outbox 失败: %v", err)
	}

	if _, err := deadletter.DeleteByIDs(db, []string{ctype.NewID(201).String()}); err != nil {
		t.Fatalf("删除死信失败: %v", err)
	}

	var alive int64
	if err := db.Model(&schema.OutboxRow{}).Where("id = ? AND status = ?", ctype.NewID(301), schema.OutboxStatusDead).
		Count(&alive).Error; err != nil {
		t.Fatalf("统计 outbox 失败: %v", err)
	}
	if alive != 1 {
		t.Fatalf("删死信不得动 outbox 的 dead 行，实际剩下 %d 行", alive)
	}
}

// TestDeleteByIDsEmptyInputIsNoop 空输入必须是空操作（前端清空勾选后会走到这里）。
func TestDeleteByIDsEmptyInputIsNoop(t *testing.T) {
	db := migrate(t)
	seedDeadLetter(t, db, 401, "job-e")

	deleted, err := deadletter.DeleteByIDs(db, nil)
	if err != nil {
		t.Fatalf("空输入不该报错: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("空输入不该删任何行，实际 %d", deleted)
	}

	var count int64
	if err := db.Model(&schema.CdcDeadLetterRow{}).Count(&count).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("死信应仍是 1 行，实际 %d", count)
	}
}

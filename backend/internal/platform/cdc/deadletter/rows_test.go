package deadletter_test

import (
	"context"
	"testing"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/cdc/deadletter"
	"myblogx/internal/platform/idgen"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 这一组用例守的是"生产结构体"本身，不是 testkit/schema 里的替身。
//
// 起因（2026-11-13 线上）：`deadletter.Record` 漏了 BeforeCreate，主键恒为 0，
// 而 testkit/schema 的 CdcDeadLetterRow **有这个钩子** —— 用例插的行主键正常，
// 生产行却全是 0。结果重投时 `id IN ('')` 撞上 bigint，MySQL 报
// `1292 Truncated incorrect INTEGER value: ''`，页面上只看到一个红条。
//
// 所以这里刻意**不用** schema 替身：用生产结构体建表、插行、断言主键。

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	if !idgen.MustReady() {
		if err := idgen.Init(1); err != nil {
			t.Fatalf("初始化 idgen 失败: %v", err)
		}
	}
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&deadletter.Record{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return db
}

// Record 落库必须自带雪花主键，不能是零值。
func TestRecord_写入时生成主键(t *testing.T) {
	db := newDB(t)

	row := &deadletter.Record{
		Stream:      "outbox_relay",
		CdcJobID:    "outbox:1",
		SourceTable: "outbox_event_models",
		Action:      "dispatch",
		PayloadJSON: "{}",
		Status:      deadletter.StatusPending,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("插入死信失败: %v", err)
	}

	if row.ID.IsZero() {
		t.Fatal("主键仍是零值：BeforeCreate 没生效，线上重投会再次报 1292")
	}
}

// 两条死信必须拿到不同主键：主键是唯一键，都写 0 时第二条会撞车。
func TestRecord_两条死信主键不重复(t *testing.T) {
	db := newDB(t)

	ids := make([]ctype.ID, 0, 2)
	for _, job := range []string{"outbox:2", "outbox:3"} {
		row := &deadletter.Record{
			Stream:      "outbox_relay",
			CdcJobID:    job,
			SourceTable: "outbox_event_models",
			Action:      "dispatch",
			PayloadJSON: "{}",
			Status:      deadletter.StatusPending,
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("插入死信 %s 失败: %v", job, err)
		}
		ids = append(ids, row.ID)
	}

	if ids[0] == ids[1] {
		t.Fatalf("两条死信拿到了同一个主键 %q", ids[0].String())
	}
}

// 空 ID 不能进 SQL：这是 1292 的直接来源。
// 剔除之后"一条都不剩"要返回空表 + nil，而不是报错。
func TestByIDs_空ID被剔除而不是报错(t *testing.T) {
	db := newDB(t)

	got, err := deadletter.ByIDs(db, []ctype.ID{"", ctype.ID("")})
	if err != nil {
		t.Fatalf("空 ID 列表不该报错，实际 %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("空 ID 不该查出任何行，实际 %d 行", len(got))
	}
}

// 混入一个空 ID 时，正常 ID 仍要能查到。
func TestByIDs_空ID混在正常ID里只丢空的那条(t *testing.T) {
	db := newDB(t)

	row := &deadletter.Record{
		Stream:      "image_ref_river",
		CdcJobID:    "image:9",
		SourceTable: "article_models",
		Action:      "delete",
		PayloadJSON: "{}",
		Status:      deadletter.StatusPending,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("插入死信失败: %v", err)
	}

	got, err := deadletter.ByIDs(db, []ctype.ID{"", row.ID})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(got) != 1 || got[0].ID != row.ID {
		t.Fatalf("应只查到那一条正常记录，实际 %+v", got)
	}
}

var _ = context.Background

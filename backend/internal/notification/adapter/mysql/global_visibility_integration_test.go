//go:build integration

package mysql_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/global_notif_enum"
	"myblogx/internal/notification/adapter/mysql"
	"myblogx/internal/notification/domain"
	"myblogx/internal/testkit/mysqlcontainer"
	"myblogx/internal/testkit/schema"
)

// 这份用例验「线上查询逐字对齐」：domain.VisibilityPredicate.SQL() 生成的
// 完整可见性表达式（过期总闸 + 三段 OR）必须在真 MySQL 上给出与内存
// Matches 相同的可见集，并覆盖刚过期 / 注册同秒 / 新用户 / 老用户四边界。
// 不用 SQLite——它不负责生产 SQL 的比较符与 NULL / 时区行为。

// visibilitySeed 是一条写死种子。
type visibilitySeed struct {
	id      uint64
	title   string
	rule    int8
	created time.Time
	expire  time.Time
	// wantMemory 是这条公告在同一份规则的内存判定下的独立期望值（手工算好）。
	wantMemory bool
}

func TestGlobalRepoVisibleSetMatchesSharedPredicate(t *testing.T) {
	db := mysqlcontainer.DB(t)
	schema.Migrate(t, db, schema.UserCore, schema.Site, schema.Chat)
	if err := db.AutoMigrate(&schema.GlobalNotifRow{}, &schema.UserGlobalNotifRow{}); err != nil {
		t.Fatalf("建通知表失败: %v", err)
	}

	// 真库查询用 time.Now() 作为"此刻"，所以种子时间必须相对当前时刻写死，
	// 而不是写死一个已经过去的绝对年份（那会让所有公告都过期）。
	// 相对偏移仍是固定字面量：ID、注册时刻、发布时刻、过期时刻逐字可复现。
	// 生产里 userCreatedAt 是从库里读出来的，与公告 created_at 同精度；
	// 这里也钉到秒，避免把 Go 的纳秒精度塞进 DATETIME(3) 造成的假分叉。
	now := time.Now().Truncate(time.Second)
	userCreatedAt := now.Add(-48 * time.Hour)
	published := now.Add(-time.Hour)
	expire := now.Add(24 * time.Hour)

	rows := []visibilitySeed{
		{101, "所有用户", int8(domain.VisibleAll), published, expire, true},
		{102, "老用户", int8(domain.VisibleRegistered), published, expire, true},
		{103, "新用户", int8(domain.VisibleNew), published, expire, false},
		{104, "刚过期", int8(domain.VisibleAll), published, now.Add(-time.Second), false},
		{105, "未知规则", 0, published, expire, false},
		{106, "注册同秒-老用户", int8(domain.VisibleRegistered), userCreatedAt, expire, true},
		{107, "注册同秒-新用户", int8(domain.VisibleNew), userCreatedAt, expire, false},
	}
	for _, r := range rows {
		row := &schema.GlobalNotifRow{
			BaseRow:         schema.BaseRow{ID: ctype.NewID(r.id), CreatedAt: r.created, UpdatedAt: r.created},
			UserVisibleRule: global_notif_enum.Type(r.rule),
			Title:           r.title,
			Content:         "内容",
			ExpireTime:      r.expire,
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("播种公告 %q 失败: %v", r.title, err)
		}
	}

	// 用户级软删：102 被该用户删掉，可见集里必须排除它（个人态不是可见规则）。
	softDeleted := &schema.UserGlobalNotifRow{
		BaseRow: schema.BaseRow{
			ID: ctype.NewID(901), CreatedAt: now, UpdatedAt: now,
			DeletedAt: gorm.DeletedAt{Time: now, Valid: true},
		},
		MsgID:  ctype.NewID(102),
		UserID: ctype.NewID(1),
	}
	if err := db.Create(softDeleted).Error; err != nil {
		t.Fatalf("播种用户级软删失败: %v", err)
	}

	repo := mysql.NewGlobal(db)
	got, _, err := repo.ListVisible(context.Background(), 1, userCreatedAt, domain.Page{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("查可见集失败: %v", err)
	}
	gotSet := make(map[uint64]bool, len(got))
	for _, n := range got {
		gotSet[n.ID.Uint64()] = true
	}

	// 独立期望：手工按「过期总闸 + 三段 OR + 用户级软删」算出的可见集。
	wantSet := map[uint64]bool{
		101: true, // 所有用户
		106: true, // 注册同秒落在 >= 老用户分支
	}
	for id := range wantSet {
		if !gotSet[id] {
			t.Fatalf("线上可见集缺公告 %d，实际 %v", id, gotSet)
		}
	}
	for id := range gotSet {
		if !wantSet[id] {
			t.Fatalf("线上可见集多出公告 %d，期望 %v，实际 %v", id, wantSet, gotSet)
		}
	}

	// 同一份规则在内存里的判定逐条与真库结果对齐（排除用户级软删那一条，
	// 它是个人态、由仓储叠加，不属于可见规则）。
	predicate := domain.VisibilityPredicate{Now: now, UserCreatedAt: userCreatedAt}
	for _, r := range rows {
		n := domain.GlobalNotif{
			ID:          ctype.NewID(r.id),
			VisibleRule: domain.VisibleRule(r.rule),
			ExpireAt:    r.expire,
			CreatedAt:   r.created,
		}
		if gotMemory := predicate.Matches(n); gotMemory != r.wantMemory {
			t.Fatalf("内存判定与独立期望分叉：%q got=%v want=%v", r.title, gotMemory, r.wantMemory)
		}
		if r.id == 102 {
			continue // 用户级软删只影响线上查询，内存规则里它本应可见
		}
		if r.wantMemory != gotSet[r.id] {
			t.Fatalf("内存判定与真库可见集分叉：%q memory=%v mysql=%v", r.title, r.wantMemory, gotSet[r.id])
		}
	}
}

package mysql

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/chat/domain"
	"myblogx/internal/chat/domain/msgid"
	"myblogx/internal/kernel/ctype"
	"myblogx/test/testutil"
)

// 本文件守 GET /chat/messages 的复合游标与"清空水位按时间算"两条口径：
//
//  1. ORDER BY send_time DESC, id DESC —— 相同时间下 id 只当确定性定序键；
//  2. 取 limit+1 判断 has_more，next_cursor 指向本页最老一条；
//  3. 跟着游标翻页不重不漏，且**新消息插入后旧页不位移**（offset 分页的老毛病）；
//  4. clear_before_msg_id 解析成 send_time，ID 是 FNV 哈希式的大数也照挡不误。
//
// 与 chat_integration_test.go 的分工：这里用 sqlite 验证"排序/游标/水位算得对"，
// MySQL 容器用例守真实外键与事务。
//
// R12：所有 ID 语义字段一律用 ctype.ID，绝不写裸 uint64。

const cursorTestSession = "chat:100:200"

// cursorTestDB 建齐消息、会话、用户级删除状态三张表。
func cursorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.SetupSQLite(t, &sessionRow{}, &msgRow{}, &stateRow{})
}

// seedCursorMsg 造一条消息。hashID 允许用 [2^62,2^63) 区间的"客户端幂等键式"主键，
// 用来证明 id 大小与消息先后完全无关。
func seedCursorMsg(t *testing.T, db *gorm.DB, id ctype.ID, at time.Time, content string) {
	t.Helper()
	row := msgRow{
		ID:         id,
		SessionID:  cursorTestSession,
		SenderID:   ctype.NewID(200),
		ReceiverID: ctype.NewID(100),
		Content:    content,
		SendTime:   at,
		MsgStatus:  int8(domain.StatusSent),
		MsgType:    int8(domain.TypeText),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建消息 %s 失败: %v", id, err)
	}
}

// hashLikeID 造一个"大而与时序无关"的主键，模拟幂等键派生出来的主键。
//
// 这里写死一个测试本地的大数，而不是引用派生模块的号段常量：本文件验的是
// 「排序 / 水位 / 分页不读 ID 大小」，即主键多大都不该影响先后；号段本身是
// msgid 包的实现细节（#279），生产代码里已不再导出。
func hashLikeID(n uint64) ctype.ID {
	return ctype.NewID((1 << 62) + n)
}

func contentsOf(result domain.MessageQueryResult) []string {
	out := make([]string, 0, len(result.Messages))
	for _, msg := range result.Messages {
		out = append(out, msg.Content)
	}
	return out
}

func assertContents(t *testing.T, result domain.MessageQueryResult, want ...string) {
	t.Helper()
	got := contentsOf(result)
	if len(got) != len(want) {
		t.Fatalf("列表内容不符: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("列表内容不符: got %v want %v", got, want)
		}
	}
}

// TestListMessagesOrdersBySendTimeThenID 相同 send_time 时按 id DESC 定序。
// id 只是定序键，不是时序键：这里故意让大哈希 ID 与小 ID 混排。
func TestListMessagesOrdersBySendTimeThenID(t *testing.T) {
	db := cursorTestDB(t)
	repo := New(db)
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	seedCursorMsg(t, db, ctype.NewID(11), at, "小ID")
	seedCursorMsg(t, db, hashLikeID(900), at, "哈希大ID")
	seedCursorMsg(t, db, hashLikeID(300), at, "哈希中ID")
	seedCursorMsg(t, db, ctype.NewID(12), at.Add(time.Second), "更新一条")

	got, err := repo.ListMessages(context.Background(), domain.MessageQuery{
		UserID: ctype.NewID(100), SessionID: cursorTestSession, Page: domain.Page{Page: 1, Limit: 10},
	})
	if err != nil {
		t.Fatalf("查消息失败: %v", err)
	}
	assertContents(t, got, "更新一条", "哈希大ID", "哈希中ID", "小ID")
	if got.HasMore || got.Total != 4 {
		t.Fatalf("4 条全部返回时不应有 has_more: has_more=%v total=%d", got.HasMore, got.Total)
	}
}

// TestListMessagesCursorPagesAreStable 覆盖 (b)(c)(d)：第一页 newest 2 + has_more，
// 跟游标拿 older 2 不重复，且第一页之后插入新消息不会让旧页位移。
func TestListMessagesCursorPagesAreStable(t *testing.T) {
	db := cursorTestDB(t)
	repo := New(db)
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	// 故意让旧消息有更大的哈希 ID：id 顺序与时间顺序相反。
	seedCursorMsg(t, db, hashLikeID(1), at, "m1")
	seedCursorMsg(t, db, hashLikeID(2), at.Add(time.Minute), "m2")
	seedCursorMsg(t, db, hashLikeID(3), at.Add(2*time.Minute), "m3")
	seedCursorMsg(t, db, hashLikeID(4), at.Add(3*time.Minute), "m4")

	query := func(cursor domain.MessageCursor) domain.MessageQueryResult {
		t.Helper()
		got, err := repo.ListMessages(ctx, domain.MessageQuery{
			UserID: ctype.NewID(100), SessionID: cursorTestSession,
			Page: domain.Page{Page: 1, Limit: 2}, Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("查消息失败: %v", err)
		}
		return got
	}

	first := query(domain.MessageCursor{})
	assertContents(t, first, "m4", "m3")
	if !first.HasMore || first.Total != 4 {
		t.Fatalf("第一页应有 has_more=true、total=4，实际 has_more=%v total=%d", first.HasMore, first.Total)
	}
	if first.NextCursor.BeforeSendTime == "" || first.NextCursor.BeforeID.IsZero() {
		t.Fatalf("第一页必须给可用的 next_cursor，实际 %+v", first.NextCursor)
	}
	if want := at.Add(2 * time.Minute).Format(time.RFC3339Nano); first.NextCursor.BeforeSendTime != want {
		t.Fatalf("next_cursor 时间应为本页最老一条 m3: got %s want %s", first.NextCursor.BeforeSendTime, want)
	}
	if first.NextCursor.BeforeID != hashLikeID(3) {
		t.Fatalf("next_cursor id 应为 m3 的 %s，实际 %s", hashLikeID(3), first.NextCursor.BeforeID)
	}

	// 模拟"翻页期间收到一条新消息"：偏移分页会把 m3 重复推到第二页，游标不会。
	seedCursorMsg(t, db, hashLikeID(5), at.Add(4*time.Minute), "new")

	second := query(first.NextCursor)
	assertContents(t, second, "m2", "m1")
	if second.HasMore || second.NextCursor.BeforeSendTime != "" || !second.NextCursor.BeforeID.IsZero() {
		t.Fatalf("最后一页应 has_more=false 且 next_cursor 为零值，实际 has_more=%v cursor=%+v",
			second.HasMore, second.NextCursor)
	}
	if second.Total != 5 {
		t.Fatalf("count 应是会话当前可见总数 5（含新消息），实际 %d", second.Total)
	}
}

// TestListMessagesOrdersDerivedIDsBySendTime 幂等消息的派生主键不因 ID 大而排到前面。
//
// #279 把「幂等键 → 主键」收进 msgid 模块后，这条约束仍必须靠行为守着：
// 派生主键比普通雪花主键大得多，但它是最早的一条消息，所以列表 DESC 要把它
// 放在最后、分页也要按时间翻到它 —— 排序与分页都不许读 ID 大小。
func TestListMessagesOrdersDerivedIDsBySendTime(t *testing.T) {
	db := cursorTestDB(t)
	repo := New(db)
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	derived := msgid.FromClientKey(ctype.NewID(200), "client-key-old")
	if derived.Uint64() < hashLikeID(0).Uint64() {
		t.Fatalf("派生主键应落在 2^62 以上的号段才会暴露「按 ID 排序」的错误，实际 %s", derived)
	}

	seedCursorMsg(t, db, derived, at, "派生的最早一条")
	seedCursorMsg(t, db, ctype.NewID(11), at.Add(time.Minute), "普通第二条")
	seedCursorMsg(t, db, ctype.NewID(12), at.Add(2*time.Minute), "普通第三条")

	ctx := context.Background()
	query := func(cursor domain.MessageCursor) domain.MessageQueryResult {
		t.Helper()
		got, err := repo.ListMessages(ctx, domain.MessageQuery{
			UserID: ctype.NewID(100), SessionID: cursorTestSession,
			Page: domain.Page{Page: 1, Limit: 2}, Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("查消息失败: %v", err)
		}
		return got
	}

	first := query(domain.MessageCursor{})
	assertContents(t, first, "普通第三条", "普通第二条")
	if !first.HasMore {
		t.Fatal("还有一条更早的消息，第一页必须给出 has_more")
	}

	// 第二页要能翻到"ID 最大、时间最早"的那条：分页键是时间，不是 ID。
	assertContents(t, query(first.NextCursor), "派生的最早一条")
}

// TestListMessagesRejectsBadCursor 半个游标 / 坏时间戳不能静默退回第一页。
func TestListMessagesRejectsBadCursor(t *testing.T) {
	db := cursorTestDB(t)
	repo := New(db)
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	seedCursorMsg(t, db, ctype.NewID(11), at, "m1")

	for _, cursor := range []domain.MessageCursor{
		{BeforeSendTime: at.Format(time.RFC3339Nano)},
		{BeforeID: ctype.NewID(11)},
		{BeforeSendTime: "not-a-time", BeforeID: ctype.NewID(11)},
	} {
		if _, err := repo.ListMessages(context.Background(), domain.MessageQuery{
			UserID: ctype.NewID(100), SessionID: cursorTestSession,
			Page: domain.Page{Page: 1, Limit: 2}, Cursor: cursor,
		}); err == nil {
			t.Fatalf("非法游标 %+v 必须报错，不能退回第一页", cursor)
		}
	}
}

// TestListMessagesClearWatermarkUsesSendTimeNotID 清空水位按时间解析。
//
// 三条消息的 ID 故意与时序相反（水位行 ID 最大、更晚的消息 ID 反而更小），
// 旧实现 `id > clear_before_msg_id` 会把水位之前的 m1 放出来、把之后的 m3 挡掉；
// 新实现只认 send_time，所以恰好剩下 m3。
func TestListMessagesClearWatermarkUsesSendTimeNotID(t *testing.T) {
	db := cursorTestDB(t)
	repo := New(db)
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	cleared := hashLikeID(900)
	seedCursorMsg(t, db, hashLikeID(800), at, "m1-old")
	seedCursorMsg(t, db, cleared, at.Add(time.Minute), "m2-cleared")
	seedCursorMsg(t, db, hashLikeID(100), at.Add(2*time.Minute), "m3-new")

	// 会话行存的是"清到哪条"的水位；软删水位那条消息本身，验证 Unscoped 子查询。
	session := sessionRow{
		ID:               ctype.NewID(1),
		SessionID:        cursorTestSession,
		UserID:           ctype.NewID(100),
		ReceiverID:       ctype.NewID(200),
		ClearBeforeMsgID: cleared,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}
	if err := db.Delete(&msgRow{}, "id = ?", cleared).Error; err != nil {
		t.Fatalf("软删水位消息失败: %v", err)
	}

	got, err := repo.ListMessages(context.Background(), domain.MessageQuery{
		UserID: ctype.NewID(100), SessionID: cursorTestSession,
		ClearBefore: cleared, Page: domain.Page{Page: 1, Limit: 10},
	})
	if err != nil {
		t.Fatalf("查消息失败: %v", err)
	}
	assertContents(t, got, "m3-new")
	if got.Total != 1 || got.HasMore {
		t.Fatalf("可见总数应为 1 且无下一页，实际 total=%d has_more=%v", got.Total, got.HasMore)
	}
}

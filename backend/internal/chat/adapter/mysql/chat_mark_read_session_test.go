package mysql

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"myblogx/internal/chat/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/test/testutil"
)

// 整会话已读（POST /chat/read 的 session_id 模式）在 sqlite 上的行为守卫：
//
//   - 命中该会话里所有"发给我且未读"的消息，按（发送人, 会话）发已读事件；
//   - 第二次调用命中 0 条、不报错（前端静默重试的前提）；
//   - 清空水位按 send_time 算，软删的水位行也照样生效，历史不被"复活"；
//   - 用户级删除的消息不参与。
//
// R12：所有 ID 语义字段一律用 ctype.ID。

func markReadTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.SetupSQLite(t, &sessionRow{}, &msgRow{}, &stateRow{}, &outboxRow{})
}

func seedMarkReadMsg(t *testing.T, db *gorm.DB, id ctype.ID, sessionID string, at time.Time, status domain.MsgStatus) {
	t.Helper()
	row := msgRow{
		ID:         id,
		SessionID:  sessionID,
		SenderID:   ctype.NewID(200),
		ReceiverID: ctype.NewID(100),
		Content:    "hello",
		SendTime:   at,
		MsgStatus:  int8(status),
		MsgType:    int8(domain.TypeText),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建消息 %s 失败: %v", id, err)
	}
}

func statusOf(t *testing.T, db *gorm.DB, id ctype.ID) domain.MsgStatus {
	t.Helper()
	var row msgRow
	if err := db.Unscoped().Where("id = ?", id.Uint64()).Take(&row).Error; err != nil {
		t.Fatalf("读消息 %s 失败: %v", id, err)
	}
	return domain.MsgStatus(row.MsgStatus)
}

func readEventCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&outboxRow{}).
		Where("event_name = ?", domain.EventMessagesRead).
		Count(&n).Error; err != nil {
		t.Fatalf("统计已读事件失败: %v", err)
	}
	return n
}

// TestMarkReadSessionMarksAllUnreadAndIsIdempotent 整会话模式：一次全中、二次 0 条。
func TestMarkReadSessionMarksAllUnreadAndIsIdempotent(t *testing.T) {
	db := markReadTestDB(t)
	repo := New(db)
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	first := domain.MessageIDFromClientKey(ctype.NewID(200), "k1")
	second := ctype.NewID(22)
	third := ctype.NewID(23)

	seedSession(t, db, 1, cursorTestSession, 100, 200, 2, 0)
	seedMarkReadMsg(t, db, first, cursorTestSession, at, domain.StatusSent)
	seedMarkReadMsg(t, db, second, cursorTestSession, at.Add(time.Minute), domain.StatusSent)
	// 已读的不该被重复计入。
	seedMarkReadMsg(t, db, third, cursorTestSession, at.Add(2*time.Minute), domain.StatusRead)
	// 另一个会话的消息不该被顺手标已读。
	seedMarkReadMsg(t, db, ctype.NewID(99), "chat:100:300", at, domain.StatusSent)

	msgs, err := repo.MarkReadSession(ctx, ctype.NewID(100), cursorTestSession)
	if err != nil {
		t.Fatalf("整会话已读失败: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("应命中 2 条未读，实际 %d", len(msgs))
	}
	if statusOf(t, db, first) != domain.StatusRead || statusOf(t, db, second) != domain.StatusRead {
		t.Fatal("本会话的未读消息都应变成已读")
	}
	if statusOf(t, db, ctype.NewID(99)) != domain.StatusSent {
		t.Fatal("别的会话不该被标记已读")
	}
	if got := mustUnread(t, db, 1); got != 0 {
		t.Fatalf("未读应归零，实际 %d", got)
	}
	if got := readEventCount(t, db); got != 1 {
		t.Fatalf("同一（发送人, 会话）应聚合成 1 条已读事件，实际 %d", got)
	}

	again, err := repo.MarkReadSession(ctx, ctype.NewID(100), cursorTestSession)
	if err != nil {
		t.Fatalf("重复整会话已读不该报错: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("重复调用应命中 0 条，实际 %d", len(again))
	}
	if got := mustUnread(t, db, 1); got != 0 {
		t.Fatalf("重复调用后未读不该变负，实际 %d", got)
	}
	if got := readEventCount(t, db); got != 1 {
		t.Fatalf("重复调用不该再发已读事件，实际 %d", got)
	}
}

// TestMarkReadSessionRespectsTimeWatermarkAndUserDeletes 水位行（软删）之前的历史、
// 以及用户级删除的消息，都不会被整会话已读"复活"。
func TestMarkReadSessionRespectsTimeWatermarkAndUserDeletes(t *testing.T) {
	db := markReadTestDB(t)
	repo := New(db)
	ctx := context.Background()
	at := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	cleared := domain.MessageIDFromClientKey(ctype.NewID(200), "clear-key")
	old := ctype.NewID(31)
	fresh := ctype.NewID(32)
	deleted := ctype.NewID(33)

	seedSession(t, db, 2, cursorTestSession, 100, 200, 3, cleared.Uint64())
	seedMarkReadMsg(t, db, old, cursorTestSession, at, domain.StatusSent)
	seedMarkReadMsg(t, db, cleared, cursorTestSession, at.Add(time.Minute), domain.StatusSent)
	seedMarkReadMsg(t, db, fresh, cursorTestSession, at.Add(2*time.Minute), domain.StatusSent)
	seedMarkReadMsg(t, db, deleted, cursorTestSession, at.Add(3*time.Minute), domain.StatusSent)
	// 水位行被软删：Unscoped 子查询仍要能解出它的 send_time。
	if err := db.Delete(&msgRow{}, "id = ?", cleared.Uint64()).Error; err != nil {
		t.Fatalf("软删水位消息失败: %v", err)
	}
	state := stateRow{
		ID:        ctype.NewID(41),
		MsgID:     deleted,
		UserID:    ctype.NewID(100),
		SessionID: cursorTestSession,
		DeletedAt: gorm.DeletedAt{Time: at.Add(4 * time.Minute), Valid: true},
	}
	if err := db.Create(&state).Error; err != nil {
		t.Fatalf("创建用户级删除状态失败: %v", err)
	}

	msgs, err := repo.MarkReadSession(ctx, ctype.NewID(100), cursorTestSession)
	if err != nil {
		t.Fatalf("整会话已读失败: %v", err)
	}
	if len(msgs) != 1 || msgs[0].ID != fresh {
		t.Fatalf("只有水位之后、且未删除的 fresh 应命中，实际 %+v", msgs)
	}
	if statusOf(t, db, old) != domain.StatusSent || statusOf(t, db, deleted) != domain.StatusSent {
		t.Fatal("清空的水位历史与用户级删除的消息不该被标记已读")
	}
	if statusOf(t, db, fresh) != domain.StatusRead {
		t.Fatal("水位之后可见的未读消息应被标记已读")
	}
}

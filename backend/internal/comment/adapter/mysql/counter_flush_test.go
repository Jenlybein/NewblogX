package mysql

import (
	"testing"

	"myblogx/test/testutil"
)

// 包内测试：直接用 commentRow 建表，与 adapter 内部同一份表定义。

func TestFlushCommentCountApplies(t *testing.T) {
	t.Parallel()
	db := testutil.SetupSQLite(t, &commentRow{})

	row := commentRow{Content: "c1", ArticleID: 1, UserID: 1}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建评论失败: %v", err)
	}
	id := row.ID

	if _, err := FlushCommentCount(db, CommentReplyCount, id, 3); err != nil {
		t.Fatalf("回复数落库失败: %v", err)
	}
	if _, err := FlushCommentCount(db, CommentDiggCount, id, 2); err != nil {
		t.Fatalf("点赞数落库失败: %v", err)
	}

	var got commentRow
	if err := db.First(&got, id).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.ReplyCount != 3 || got.DiggCount != 2 {
		t.Fatalf("落库结果不符: reply=%d digg=%d", got.ReplyCount, got.DiggCount)
	}
}

// TestFlushCommentCountFloorAtZero 取消点赞过多时不出现负计数。
func TestFlushCommentCountFloorAtZero(t *testing.T) {
	t.Parallel()
	db := testutil.SetupSQLite(t, &commentRow{})

	row := commentRow{Content: "c1", ArticleID: 1, UserID: 1}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("创建评论失败: %v", err)
	}
	id := row.ID

	if _, err := FlushCommentCount(db, CommentDiggCount, id, 1); err != nil {
		t.Fatalf("加 1 失败: %v", err)
	}
	if _, err := FlushCommentCount(db, CommentDiggCount, id, -9); err != nil {
		t.Fatalf("减 9 失败: %v", err)
	}

	var got commentRow
	if err := db.First(&got, id).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.DiggCount != 0 {
		t.Fatalf("计数应停在 0，实际=%d", got.DiggCount)
	}
}

// TestFlushCommentCountMissingRow 评论已删除时返回 hit=false（不是错误），
// 调用方据此丢弃增量，避免无限重试同一行。
func TestFlushCommentCountMissingRow(t *testing.T) {
	t.Parallel()
	db := testutil.SetupSQLite(t, &commentRow{})

	hit, err := FlushCommentCount(db, CommentReplyCount, 999999, 1)
	if err != nil {
		t.Fatalf("评论不存在不应返回 error，实际=%v", err)
	}
	if hit {
		t.Fatal("评论不存在时应返回 hit=false")
	}
}

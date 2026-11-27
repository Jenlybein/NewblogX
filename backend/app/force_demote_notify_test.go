package app

import (
	"context"
	"strings"
	"testing"

	articleapp "myblogx/internal/article/app"
	"myblogx/internal/kernel/ctype"
	notificationapp "myblogx/internal/notification/app"
	notificationmysql "myblogx/internal/notification/adapter/mysql"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
)

// 强制退回的站内信有两条硬要求，都在这里钉住：
//
//  1. 正文必须带上管理员填的原因 —— 作者只知道"文章被退回了"却说不出为什么，
//     比不退回更糟；
//  2. 同一篇文章 + 同一条原因只发一条 —— 两笔并发事务可能都看到"已发布"，
//     各自都会调到这里；消息表不能加去重列（零 DDL），去重只能按
//     "接收者 + 链接 + 正文"判，而链接里带文章 ID、正文里带原因。
func TestForceDemoteNoticeCarriesReasonAndDedupes(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ArticleMessageRow{})

	// 用真实的站内信服务（写库版仓储），这样"去重查的是真表"而不是内存 fake。
	restore := swapNotificationService(notificationapp.New(
		notificationmysql.NewMessage(db), nil, nil, nil, nil, nil, nil))
	t.Cleanup(restore)

	notifier := &forceDemoteNotifier{db: db}
	articleID := ctype.NewID(700000000000000001)
	authorID := ctype.NewID(700000000000000002)
	operatorID := ctype.NewID(700000000000000003)
	notice := articleapp.ForceDemoteNotice{
		ArticleID:  articleID,
		AuthorID:   authorID,
		Title:      "被误放行的文章",
		Reason:     "评分 6 分，未达到发布线",
		OperatorID: operatorID,
	}

	if err := notifier.NotifyForceDemoted(context.Background(), notice); err != nil {
		t.Fatalf("发通知失败: %v", err)
	}

	var rows []schema.ArticleMessageRow
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("读取站内信失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("应发一条站内信，实际 %d 条", len(rows))
	}
	got := rows[0]
	if got.ReceiverID != authorID {
		t.Fatalf("应发给作者 %s，实际 %s", authorID, got.ReceiverID)
	}
	if !strings.Contains(got.Content, "退回草稿") || !strings.Contains(got.Content, notice.Reason) {
		t.Fatalf("正文应说明已退回草稿并带原因，实际 %q", got.Content)
	}
	if want := "/article/" + articleID.String(); got.LinkHerf != want {
		t.Fatalf("链接应指向文章详情 %q，实际 %q", want, got.LinkHerf)
	}
	if got.ActionUserID == nil || *got.ActionUserID != operatorID {
		t.Fatalf("操作人应记下执行处置的管理员，实际 %v", got.ActionUserID)
	}

	// 同一篇文章 + 同一条原因：重复调用不该产生第二条（并发重复请求的兜底）。
	if err := notifier.NotifyForceDemoted(context.Background(), notice); err != nil {
		t.Fatalf("重复发通知不该报错: %v", err)
	}
	rows = nil
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("读取站内信失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("同一文章 + 同一原因只应有一条，实际 %d 条", len(rows))
	}

	// 换了原因就是另一次处置：作者需要看到新的那句话。
	notice.Reason = "作者申诉后确认是误判"
	if err := notifier.NotifyForceDemoted(context.Background(), notice); err != nil {
		t.Fatalf("换原因后发通知失败: %v", err)
	}
	rows = nil
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("读取站内信失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("换了原因应再发一条，实际 %d 条", len(rows))
	}
}

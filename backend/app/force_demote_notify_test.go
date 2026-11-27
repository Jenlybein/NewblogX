package app

import (
	"context"
	"strings"
	"testing"

	"myblogx/internal/kernel/ctype"
	notificationmysql "myblogx/internal/notification/adapter/mysql"
	notificationapp "myblogx/internal/notification/app"
	outboxrelay "myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
)

// 强制退回的站内信有三条硬要求，都在这里钉住：
//
//  1. 正文必须带上管理员填的原因 —— 作者只知道"文章被退回了"却说不出为什么，
//     比不退回更糟；
//  2. 同一篇文章 + 同一条原因只发一条 —— 中继是 at-least-once，重投不该让作者
//     收到两条一样的通知；消息表不能加去重列（零 DDL），去重只能按
//     "接收者 + 链接 + 正文"判，而链接里带文章 ID、正文里带原因；
//  3. **事件重投能补发** —— 这是它从"同步发信"改成"发件箱事件"的理由：
//     同步发信失败后管理员重试，文章已是草稿、用例提前返回，通知就永久丢了。
func TestForceDemoteNoticeSubscriber(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ArticleMessageRow{})

	// 用真实的站内信服务（写库版仓储），这样"去重查的是真表"而不是内存 fake。
	restore := swapNotificationService(notificationapp.New(
		notificationmysql.NewMessage(db), nil, nil, nil, nil, nil, nil))
	t.Cleanup(restore)

	infra := &Infra{DB: db, Logger: testutil.Logger()}
	subscriber := forceDemoteNoticeSubscriber(infra)
	event := outboxrelay.Event{
		ID:          ctype.NewID(700000000000000010),
		AggregateID: ctype.NewID(700000000000000001),
		Payload: `{"article_id":"700000000000000001","author_id":"700000000000000002",` +
			`"operator_id":"700000000000000003","title":"被误放行的文章",` +
			`"reason":"评分 6 分，未达到发布线"}`,
	}

	if err := subscriber(context.Background(), event); err != nil {
		t.Fatalf("投递通知失败: %v", err)
	}

	var sent []schema.ArticleMessageRow
	if err := db.Find(&sent).Error; err != nil {
		t.Fatalf("读站内信失败: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("应发出一条站内信，实际 %d 条", len(sent))
	}
	if !strings.Contains(sent[0].Content, "评分 6 分，未达到发布线") {
		t.Fatalf("正文里必须带着管理员填的原因，实际 %q", sent[0].Content)
	}
	if !strings.Contains(sent[0].Content, "被误放行的文章") {
		t.Fatalf("正文里必须带文章标题，实际 %q", sent[0].Content)
	}

	// 重投（中继是 at-least-once）：不产生第二条。
	if err := subscriber(context.Background(), event); err != nil {
		t.Fatalf("重投不该报错: %v", err)
	}
	var again []schema.ArticleMessageRow
	if err := db.Find(&again).Error; err != nil {
		t.Fatalf("读站内信失败: %v", err)
	}
	if len(again) != 1 {
		t.Fatalf("重投后仍应只有一条，实际 %d 条", len(again))
	}
}

// 事件体坏了：不重试、不 panic —— 重试多少次体都是坏的，只会把队列塞满死信。
func TestForceDemoteNoticeSubscriberIgnoresBrokenPayload(t *testing.T) {
	infra := &Infra{DB: testutil.SetupSQLite(t, &schema.ArticleMessageRow{}), Logger: testutil.Logger()}
	subscriber := forceDemoteNoticeSubscriber(infra)
	if err := subscriber(context.Background(), outboxrelay.Event{Payload: "not json"}); err != nil {
		t.Fatalf("坏事件体不该返回错误（那会让中继无限重试）: %v", err)
	}
}

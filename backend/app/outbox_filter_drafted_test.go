package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	articleapp "myblogx/internal/article/app"
	"myblogx/internal/kernel/ctype"
	notificationapp "myblogx/internal/notification/app"
	notificationapptest "myblogx/internal/notification/apptest"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
)

// 2026-11-28 / ADR-0037 决策 D9：命中词处置 = 退回草稿时给作者一条站内通知。
//
// 这条用例守三件事，每一件单独看都很容易做错：
//
//  1. 通知**真的发出去**（订阅者是独立条目 `article.filter_drafted`，
//     不复用 `article.back_to_draft` —— 后者作者自己撤稿时也会发，
//     接上去等于每次自己撤稿都被通知一遍）；
//  2. 正文**不出现命中的词 / 档位 / 类目**（回显等于把词表送给作者，
//     ADR-0032 决策 9 同一条口径）；
//  3. 正文要说清"配额已计入"（这是作者最容易误解的一点：文章退了，
//     额度没退）。
func TestFilterDraftedNotice(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ArticleRow{}, &schema.ArticleMessageRow{}, &schema.OutboxRow{})

	articleID := ctype.NewID(304000000000000001)
	authorID := ctype.NewID(304000000000000002)
	const blockedWord = "代开发票"
	// 标题本身是干净的：命中词在正文里，被过滤器替换成星号后入库。
	// 通知里绝不能出现这个词。
	if err := db.Create(&schema.ArticleRow{
		BaseRow:  schema.BaseRow{ID: articleID},
		Title:    "一篇学术讨论",
		AuthorID: authorID,
	}).Error; err != nil {
		t.Fatalf("写入文章失败: %v", err)
	}

	messages := notificationapptest.NewMessages()
	restore := swapNotificationService(notificationapp.New(messages, nil, nil, nil, nil, nil, nil))
	t.Cleanup(restore)

	infra := &Infra{DB: db, Logger: testutil.Logger()}
	relay := outboxrelay.New(db, infra.Logger, outboxrelay.Options{})
	registerOutboxSubscribers(relay, infra)
	t.Cleanup(relay.Close)

	event := &schema.OutboxRow{
		EventName:     articleapp.EventArticleFilterDrafted,
		AggregateType: "article",
		AggregateID:   articleID.Uint64(),
		Payload:       fmt.Sprintf(`{"article_id":%d}`, articleID.Uint64()),
		Status:        outboxrelay.OutboxStatusPending,
		AvailableAt:   time.Now().Add(-time.Second),
	}
	if err := db.Create(event).Error; err != nil {
		t.Fatalf("写入发件箱事件失败: %v", err)
	}
	if err := relay.Dispatch(context.Background(), event.ID); err != nil {
		t.Fatalf("投递命中退回事件失败: %v", err)
	}

	sent := messages.All()
	if len(sent) != 1 {
		t.Fatalf("应给作者发一条退回草稿通知，实际 %d 条", len(sent))
	}

	notice := sent[0]
	if !strings.Contains(notice.Content, "已退回草稿") {
		t.Fatalf("正文应说明文章已退回草稿，实际 %q", notice.Content)
	}
	if !strings.Contains(notice.Content, "配额") {
		t.Fatalf("正文应说明这次提交照常计入配额（作者最容易误解的一点），实际 %q", notice.Content)
	}
	if !strings.Contains(notice.Content, "一篇学术讨论") {
		t.Fatalf("正文应带文章标题，实际 %q", notice.Content)
	}
	// 不泄露：命中词、档位名、类目名一个都不能出现。
	for _, leak := range []string{blockedWord, "拦截词", "观察词", "drugs", "block_action"} {
		if strings.Contains(notice.Content, leak) {
			t.Fatalf("正文不能出现 %q（等于把词表送给作者），实际 %q", leak, notice.Content)
		}
	}
	if want := fmt.Sprintf("/article/%d", articleID.Uint64()); notice.LinkHref != want {
		t.Fatalf("链接应指向该文章 %q，实际 %q", want, notice.LinkHref)
	}
}

// 命中退回草稿事件的订阅者必须**只发一条**：投递是 at-least-once，
// 重复投递同一条事件（例如崩溃重放）不能再给作者发第二遍。
func TestFilterDraftedNoticeIsIdempotent(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ArticleRow{}, &schema.ArticleMessageRow{}, &schema.OutboxRow{})

	articleID := ctype.NewID(304000000000000011)
	authorID := ctype.NewID(304000000000000012)
	if err := db.Create(&schema.ArticleRow{
		BaseRow:  schema.BaseRow{ID: articleID},
		Title:    "另一篇讨论",
		AuthorID: authorID,
	}).Error; err != nil {
		t.Fatalf("写入文章失败: %v", err)
	}

	messages := notificationapptest.NewMessages()
	restore := swapNotificationService(notificationapp.New(messages, nil, nil, nil, nil, nil, nil))
	t.Cleanup(restore)

	infra := &Infra{DB: db, Logger: testutil.Logger()}
	relay := outboxrelay.New(db, infra.Logger, outboxrelay.Options{})
	registerOutboxSubscribers(relay, infra)
	t.Cleanup(relay.Close)

	event := &schema.OutboxRow{
		EventName:     articleapp.EventArticleFilterDrafted,
		AggregateType: "article",
		AggregateID:   articleID.Uint64(),
		Payload:       fmt.Sprintf(`{"article_id":%d}`, articleID.Uint64()),
		Status:        outboxrelay.OutboxStatusPending,
		AvailableAt:   time.Now().Add(-time.Second),
	}
	if err := db.Create(event).Error; err != nil {
		t.Fatalf("写入发件箱事件失败: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := relay.Dispatch(context.Background(), event.ID); err != nil {
			t.Fatalf("第 %d 次投递失败: %v", i+1, err)
		}
	}

	if got := len(messages.All()); got != 1 {
		t.Fatalf("重复投递只能产生一条通知，实际 %d 条", got)
	}
}

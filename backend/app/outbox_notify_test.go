package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	articleapp "myblogx/internal/article/app"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum/article_review_enum"
	notificationapp "myblogx/internal/notification/app"
	notificationapptest "myblogx/internal/notification/apptest"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
)

// 驳回通知的正文必须带着审核员写的那句话。
//
// 断言方式是"看作者真的收到了什么"：驱动一次真实投递，读站内信正文。
// 这里不查"订阅者读了几张表"——换一种取数方式（比如把原因塞进事件体）
// 时，本用例应该继续成立。
func TestRejectNoticeCarriesReviewReason(t *testing.T) {
	cases := []struct {
		name string
		// taskReason 是审核任务行上的原因；空串 = 本票之前留下的历史数据。
		taskReason string
		// noTask 表示连审核任务行都没有（例如任务已被清掉）。
		noTask bool
		// want 是正文里必须出现的那句话。
		want string
	}{
		{name: "有原因", taskReason: "内容不完整", want: "内容不完整"},
		{name: "历史任务没填原因", taskReason: "", want: "审核员未填写具体原因"},
		{name: "没有审核任务行", noTask: true, want: "审核员未填写具体原因"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := testutil.SetupSQLite(t, &schema.ArticleRow{}, &schema.ArticleReviewTaskRow{},
				&schema.ArticleMessageRow{}, &schema.OutboxRow{})
			articleID := ctype.NewID(303000000000000001)
			authorID := ctype.NewID(303000000000000002)
			if err := db.Create(&schema.ArticleRow{
				BaseRow:  schema.BaseRow{ID: articleID},
				Title:    "待审文章",
				AuthorID: authorID,
			}).Error; err != nil {
				t.Fatalf("写入文章失败: %v", err)
			}

			if !c.noTask {
				reviewedAt := time.Now()
				if err := db.Create(&schema.ArticleReviewTaskRow{
					BaseRow:      schema.BaseRow{ID: ctype.NewID(303000000000000003)},
					ArticleID:    articleID,
					AuthorID:     authorID,
					ArticleTitle: "待审文章",
					AuthorName:   "作者",
					Stage:        article_review_enum.StageManual,
					Source:       article_review_enum.SourceCreate,
					Status:       article_review_enum.StatusRejected,
					Reason:       c.taskReason,
					ReviewedAt:   &reviewedAt,
				}).Error; err != nil {
					t.Fatalf("写入审核任务失败: %v", err)
				}
			}

			messages := notificationapptest.NewMessages()
			restore := swapNotificationService(notificationapp.New(messages, nil, nil, nil, nil, nil, nil))
			t.Cleanup(restore)

			infra := &Infra{DB: db, Logger: testutil.Logger()}
			relay := outboxrelay.New(db, infra.Logger, outboxrelay.Options{})
			registerOutboxSubscribers(relay, infra)
			t.Cleanup(relay.Close)

			event := &schema.OutboxRow{
				EventName:     articleapp.EventArticleRejected,
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
				t.Fatalf("投递驳回事件失败: %v", err)
			}

			sent := messages.All()
			if len(sent) != 1 {
				t.Fatalf("应给作者发一条驳回通知，实际 %d 条", len(sent))
			}
			got := sent[0]
			if got.Receiver != authorID.Uint64() {
				t.Fatalf("驳回通知应发给作者 %d，实际 %d", authorID.Uint64(), got.Receiver)
			}
			if !strings.Contains(got.Content, "审核失败") {
				t.Fatalf("正文应说明审核失败，实际 %q", got.Content)
			}
			if !strings.Contains(got.Content, c.want) {
				t.Fatalf("正文应包含 %q，实际 %q", c.want, got.Content)
			}
			// "失败原因："后面必须还有话——这正是本票修的空白。
			const label = "失败原因："
			at := strings.Index(got.Content, label)
			if at < 0 {
				t.Fatalf("正文应写明失败原因，实际 %q", got.Content)
			}
			if tail := strings.TrimSpace(got.Content[at+len(label):]); tail == "" {
				t.Fatalf("失败原因后面不能是空白，实际 %q", got.Content)
			}
		})
	}
}

// TestRejectNoticeUsesLatestRejection 一篇文章被驳回过多次时，
// 通知带的是**最近这一次**的原因：上一轮那句话对作者已经不成立了。
func TestRejectNoticeUsesLatestRejection(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ArticleRow{}, &schema.ArticleReviewTaskRow{},
		&schema.ArticleMessageRow{}, &schema.OutboxRow{})
	articleID := ctype.NewID(303000000000000011)
	authorID := ctype.NewID(303000000000000012)
	if err := db.Create(&schema.ArticleRow{
		BaseRow:  schema.BaseRow{ID: articleID},
		Title:    "改过一版的文章",
		AuthorID: authorID,
	}).Error; err != nil {
		t.Fatalf("写入文章失败: %v", err)
	}

	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-time.Hour)
	for i, row := range []*schema.ArticleReviewTaskRow{
		{
			BaseRow:      schema.BaseRow{ID: ctype.NewID(303000000000000013)},
			ArticleID:    articleID,
			AuthorID:     authorID,
			ArticleTitle: "改过一版的文章",
			AuthorName:   "作者",
			Stage:        article_review_enum.StageManual,
			Source:       article_review_enum.SourceCreate,
			Status:       article_review_enum.StatusRejected,
			Reason:       "第一轮：排版太乱",
			ReviewedAt:   &older,
		},
		{
			BaseRow:      schema.BaseRow{ID: ctype.NewID(303000000000000014)},
			ArticleID:    articleID,
			AuthorID:     authorID,
			ArticleTitle: "改过一版的文章",
			AuthorName:   "作者",
			Stage:        article_review_enum.StageManual,
			Source:       article_review_enum.SourceResubmit,
			Status:       article_review_enum.StatusRejected,
			Reason:       "第二轮：论证缺证据",
			ReviewedAt:   &newer,
		},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("写入第 %d 条审核任务失败: %v", i+1, err)
		}
	}

	messages := notificationapptest.NewMessages()
	restore := swapNotificationService(notificationapp.New(messages, nil, nil, nil, nil, nil, nil))
	t.Cleanup(restore)

	infra := &Infra{DB: db, Logger: testutil.Logger()}
	relay := outboxrelay.New(db, infra.Logger, outboxrelay.Options{})
	registerOutboxSubscribers(relay, infra)
	t.Cleanup(relay.Close)

	event := &schema.OutboxRow{
		EventName:     articleapp.EventArticleRejected,
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
		t.Fatalf("投递驳回事件失败: %v", err)
	}

	sent := messages.All()
	if len(sent) != 1 {
		t.Fatalf("应给作者发一条驳回通知，实际 %d 条", len(sent))
	}
	if !strings.Contains(sent[0].Content, "第二轮：论证缺证据") {
		t.Fatalf("通知应带最近一次驳回的原因，实际 %q", sent[0].Content)
	}
	if strings.Contains(sent[0].Content, "第一轮：排版太乱") {
		t.Fatalf("通知不该带上一轮的原因，实际 %q", sent[0].Content)
	}
}

// swapNotificationService 临时替换包级通知服务，返回还原函数。
//
// 用替换而不是真的装配通知模块：本包只关心"通知正文是什么"，
// 站内信存储换内存实现后断言更直接，也不必为通知模块建表。
func swapNotificationService(next *notificationapp.Service) func() {
	prev := notificationService
	notificationService = next
	return func() { notificationService = prev }
}

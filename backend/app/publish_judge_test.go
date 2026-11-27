package app

import (
	"context"
	"testing"

	articleapp "myblogx/internal/article/app"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/kernel/enum"
	"myblogx/internal/platform/cdc/outboxrelay"
	"myblogx/internal/testkit/schema"
	"myblogx/test/testutil"
)

// 回归：2026-11-27 线上那篇 6 分文章被放行。
//
// 当时质量闸门还没读到评分（并发的评分刚提交），于是报
// `article.score_unavailable` 让事件重试；而合规判定是**另一个**订阅者，
// 中继在错误后仍会执行它，它判 clean 之后把文章推成 visible。重试回来时
// 闸门只看「已发布待可见」，文章已不是待可见，闸门从此对它永不再判。
//
// 现在两步在同一个函数里：拿不到分数就直接返回错误，**不可能**走到合规判定。
// 本用例钉住的就是这条 —— 分数不可用时文章的可见性一个字节都不能变。
func TestJudgePublish_拿不到分数时绝不推进可见性(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ArticleRow{}, &schema.ArticleMessageRow{})
	articleID := ctype.NewID(304000000000000001)
	if err := db.Create(&schema.ArticleRow{
		BaseRow:          schema.BaseRow{ID: articleID},
		Title:            "我是好文章",
		AuthorID:         ctype.NewID(304000000000000002),
		PublishStatus:    enum.ArticleStatusPublished,
		VisibilityStatus: enum.ArticleVisibilityPendingReview,
	}).Error; err != nil {
		t.Fatalf("写入文章失败: %v", err)
	}

	// 评分通道不可用：aiService 为空，ensureScoreReady 必然失败。
	previous := aiService
	aiService = nil
	t.Cleanup(func() { aiService = previous })

	infra := &Infra{DB: db, Logger: testutil.Logger()}
	err := judgePublish(context.Background(), infra, outboxrelay.Event{
		AggregateID: articleID,
		Attempts:    1,
	}, enabledGatePolicy())
	if err == nil {
		t.Fatal("拿不到分数时必须报错让事件重试，绝不静默放行")
	}

	var got schema.ArticleRow
	if err := db.First(&got, "id = ?", articleID).Error; err != nil {
		t.Fatalf("读回文章失败: %v", err)
	}
	if got.VisibilityStatus != enum.ArticleVisibilityPendingReview {
		t.Fatalf("拿不到分数时可见性必须保持 pending_review，实际 %s", got.VisibilityStatus)
	}
	if got.PublishStatus != enum.ArticleStatusPublished {
		t.Fatalf("拿不到分数时状态必须保持已发布，实际 %d", got.PublishStatus)
	}
}

// 重试到上限仍拿不到分数：转人工，不把文章永久停在「已发布待可见」。
//
// 选转人工而不是退回草稿，是因为作者没做错任何事 —— 让一个模型故障变成
// 「请你重写一遍」是惩罚错人（ADR-0030 的失败语义）。
func TestJudgePublish_分数持续不可用时转人工(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ArticleRow{}, &schema.ArticleMessageRow{},
		&schema.ArticleReviewTaskRow{})
	articleID := ctype.NewID(304000000000000011)
	if err := db.Create(&schema.ArticleRow{
		BaseRow:          schema.BaseRow{ID: articleID},
		Title:            "等不到评分的文章",
		AuthorID:         ctype.NewID(304000000000000012),
		PublishStatus:    enum.ArticleStatusPublished,
		VisibilityStatus: enum.ArticleVisibilityPendingReview,
	}).Error; err != nil {
		t.Fatalf("写入文章失败: %v", err)
	}

	previous := aiService
	aiService = nil
	t.Cleanup(func() { aiService = previous })

	// 转人工要真的写库（改状态 + 建任务），单元测试里的 Infra 撑不起完整装配，
	// 所以替换掉这一跳，只钉"到了上限就转人工且不再报错"这条决策。
	var escalated []ctype.ID
	previousEscalate := escalateToManualReview
	escalateToManualReview = func(_ context.Context, _ *Infra, article noticeArticleRow) error {
		escalated = append(escalated, article.ID)
		return nil
	}
	t.Cleanup(func() { escalateToManualReview = previousEscalate })

	infra := &Infra{DB: db, Logger: testutil.Logger()}
	if err := judgePublish(context.Background(), infra, outboxrelay.Event{
		AggregateID: articleID,
		Attempts:    scoreEscalateAttempts,
	}, enabledGatePolicy()); err != nil {
		t.Fatalf("达到重试上限后应转人工并收尾事件，不该继续报错: %v", err)
	}
	if len(escalated) != 1 || escalated[0].Uint64() != articleID.Uint64() {
		t.Fatalf("应把该文章转人工一次，实际 %v", escalated)
	}
}

// 上限之前必须继续重试，不能提前转人工 —— 否则一次短暂抖动就把文章推给人。
func TestJudgePublish_未到上限时继续重试(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ArticleRow{}, &schema.ArticleMessageRow{})
	articleID := ctype.NewID(304000000000000031)
	if err := db.Create(&schema.ArticleRow{
		BaseRow:          schema.BaseRow{ID: articleID},
		Title:            "抖动中的文章",
		AuthorID:         ctype.NewID(304000000000000032),
		PublishStatus:    enum.ArticleStatusPublished,
		VisibilityStatus: enum.ArticleVisibilityPendingReview,
	}).Error; err != nil {
		t.Fatalf("写入文章失败: %v", err)
	}

	previous := aiService
	aiService = nil
	t.Cleanup(func() { aiService = previous })

	escalated := 0
	previousEscalate := escalateToManualReview
	escalateToManualReview = func(context.Context, *Infra, noticeArticleRow) error {
		escalated++
		return nil
	}
	t.Cleanup(func() { escalateToManualReview = previousEscalate })

	infra := &Infra{DB: db, Logger: testutil.Logger()}
	if err := judgePublish(context.Background(), infra, outboxrelay.Event{
		AggregateID: articleID,
		Attempts:    scoreEscalateAttempts - 1,
	}, enabledGatePolicy()); err == nil {
		t.Fatal("未到上限时必须报错让事件重试")
	}
	if escalated != 0 {
		t.Fatalf("未到上限不该转人工，实际转了 %d 次", escalated)
	}
}

// 已不是「已发布待可见」的文章，这条链路不该再碰它。
func TestJudgePublish_只管待可见的文章(t *testing.T) {
	db := testutil.SetupSQLite(t, &schema.ArticleRow{}, &schema.ArticleMessageRow{})
	articleID := ctype.NewID(304000000000000021)
	if err := db.Create(&schema.ArticleRow{
		BaseRow:          schema.BaseRow{ID: articleID},
		Title:            "已公开的文章",
		AuthorID:         ctype.NewID(304000000000000022),
		PublishStatus:    enum.ArticleStatusPublished,
		VisibilityStatus: enum.ArticleVisibilityVisible,
	}).Error; err != nil {
		t.Fatalf("写入文章失败: %v", err)
	}

	infra := &Infra{DB: db, Logger: testutil.Logger()}
	if err := judgePublish(context.Background(), infra, outboxrelay.Event{
		AggregateID: articleID,
		Attempts:    1,
	}, enabledGatePolicy()); err != nil {
		t.Fatalf("已公开的文章不该被这条链路判定: %v", err)
	}
}

// enabledGatePolicy 是"闸门开着、放行线 52"的最小策略。
func enabledGatePolicy() articleapp.QualityGatePolicy {
	return articleapp.QualityGatePolicy{
		Enabled:            true,
		Deferred:           true,
		PassScore:          50,
		GrayBandWidth:      2,
		BandPoorMax:        30,
		BandUnqualifiedMax: 59,
		BandNormalMax:      80,
		BandGoodMax:        89,
	}
}

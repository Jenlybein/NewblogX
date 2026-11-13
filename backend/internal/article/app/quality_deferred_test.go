package app_test

import (
	"context"
	"testing"
	"time"

	"myblogx/internal/apperr"
	"myblogx/internal/article/app"
	"myblogx/internal/article/apptest"
	"myblogx/internal/article/domain"
	"myblogx/internal/article/domain/moderation"
	"myblogx/internal/kernel/ctype"
)

// deferredPolicy 返回异步口径的闸门配置（与 HTTP 层一致：Deferred=true）。
func deferredPolicy() app.QualityGatePolicy {
	return app.QualityGatePolicy{
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

// TestAssessDeferredQualityReadsExistingScore 钉住异步判定只用**已有评分记录**，
// 不再触发模型调用：分数低于发布线时返回未通过，并带上分数与放行线。
func TestAssessDeferredQualityReadsExistingScore(t *testing.T) {
	articleID := domain.ArticleID(ctype.NewID(5001))
	scores := newFakeScores().Seed(articleID, moderation.ScoreFacts{
		TotalScore:     10,
		DimensionsJSON: `[{"name":"clarity","score":10}]`,
	})
	svc := app.New(
		apptest.NewTx(apptest.NewRepos()),
		apptest.NewClock(apptest.DefaultTime),
		apptest.NewIDGen(1000),
		nil, scores, nil, nil,
	)

	got, err := svc.AssessDeferredQuality(context.Background(), articleID, deferredPolicy())
	if err != nil {
		t.Fatalf("读取已有评分失败: %v", err)
	}
	if got.Passed {
		t.Fatalf("10 分不应通过发布线")
	}
	if got.Score != 10 || got.Required != 52 {
		t.Fatalf("分数/放行线不对: score=%d required=%d", got.Score, got.Required)
	}
}

// TestAssessDeferredQualityPassesWhenScoreReachesLine 钉住"够线就放行"。
func TestAssessDeferredQualityPassesWhenScoreReachesLine(t *testing.T) {
	articleID := domain.ArticleID(ctype.NewID(5002))
	scores := newFakeScores().Seed(articleID, moderation.ScoreFacts{
		TotalScore:     80,
		DimensionsJSON: `[{"name":"clarity","score":70},{"name":"structure","score":70}]`,
	})
	svc := app.New(
		apptest.NewTx(apptest.NewRepos()),
		apptest.NewClock(apptest.DefaultTime),
		apptest.NewIDGen(1000),
		nil, scores, nil, nil,
	)

	got, err := svc.AssessDeferredQuality(context.Background(), articleID, deferredPolicy())
	if err != nil {
		t.Fatalf("读取已有评分失败: %v", err)
	}
	if !got.Passed {
		t.Fatalf("80 分应通过发布线 52")
	}
}

// TestAssessDeferredQualityWithoutScoreFailsClosed 钉住失败关闭：
// 没有评分记录时返回 article.score_unavailable，而不是当干净放行。
func TestAssessDeferredQualityWithoutScoreFailsClosed(t *testing.T) {
	svc := app.New(
		apptest.NewTx(apptest.NewRepos()),
		apptest.NewClock(apptest.DefaultTime),
		apptest.NewIDGen(1000),
		nil, newFakeScores(), nil, nil,
	)

	_, err := svc.AssessDeferredQuality(context.Background(), domain.ArticleID(ctype.NewID(5003)), deferredPolicy())
	code, ok := apperr.CodeOf(err)
	if !ok || code != apperr.CodeArticleScoreUnavailable {
		t.Fatalf("应返回 score_unavailable，实际 %v ok=%v", err, ok)
	}
}

// TestDemotePendingForQualityReturnsDraft 钉住异步未达标的落点：从「已发布待可见」
// 退回草稿，作者可以改完再投；重复调用是幂等的（第二次返回 false）。
func TestDemotePendingForQualityReturnsDraft(t *testing.T) {
	articleID := domain.ArticleID(ctype.NewID(5004))
	now := apptest.DefaultTime
	a, err := domain.NewArticle(domain.NewArticleParams{
		ID:         articleID,
		AuthorID:   domain.UserID(ctype.NewID(100)),
		Title:      "标题",
		Content:    "正文",
		Status:     domain.StatusPublished,
		Visibility: domain.VisibilityPendingReview,
		Now:        now,
	})
	if err != nil {
		t.Fatalf("构造文章失败: %v", err)
	}
	repos := apptest.NewRepos()
	repos.FakeArticles().Seed(a)
	svc := app.New(
		apptest.NewTx(repos),
		apptest.NewClock(time.Time(now)),
		apptest.NewIDGen(1000),
		nil, nil, nil, nil,
	)

	demoted, err := svc.DemotePendingForQuality(context.Background(), articleID, "文章质量未达标")
	if err != nil {
		t.Fatalf("退回草稿失败: %v", err)
	}
	if !demoted {
		t.Fatalf("应标记为已退回")
	}
	stored, err := repos.Articles().Find(context.Background(), articleID)
	if err != nil {
		t.Fatalf("读取文章失败: %v", err)
	}
	if stored.Status() != domain.StatusDraft {
		t.Fatalf("状态应为草稿，实际 %s", stored.Status())
	}
	if stored.Visibility() != domain.VisibilityVisible {
		t.Fatalf("可见性应清成 visible，实际 %s", stored.Visibility())
	}

	// 幂等：再调一次不再变化。
	again, err := svc.DemotePendingForQuality(context.Background(), articleID, "文章质量未达标")
	if err != nil {
		t.Fatalf("第二次退回草稿不应报错: %v", err)
	}
	if again {
		t.Fatalf("第二次调用应是幂等的 false")
	}
}

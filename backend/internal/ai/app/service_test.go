package app_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"myblogx/conf"
	"myblogx/internal/ai/apptest"
	"myblogx/internal/ai/domain"
	aisearch "myblogx/internal/ai/search"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	searchdomain "myblogx/internal/search/domain"
)

// newEnv 构造带一篇文章的替身。
func newEnv() *apptest.Env {
	env := apptest.NewEnv(conf.AI{Enable: true, Active: &conf.AIProvider{
		Vendor: "openai", Model: "test-model",
		Fields: map[string]string{"api_key": "test-key"},
	}})
	env.Scores.Articles[100] = domain.ArticleSnapshot{
		ID:        ctype.NewID(100),
		AuthorID:  ctype.NewID(7),
		Title:     "标题",
		Content:   "正文内容",
		UpdatedAt: time.Now(),
		Published: true,
	}
	return env
}

func TestScoreSummaryWithoutRecord(t *testing.T) {
	env := newEnv()
	view, err := env.Service.ScoreSummary(context.Background(), 100)
	if err != nil {
		t.Fatalf("读评分摘要失败: %v", err)
	}
	if view.HasScore || view.ArticleID.Uint64() != 100 {
		t.Fatalf("无记录时应返回 has_score=false 且带上文章 ID: %+v", view)
	}
}

func TestScoreDetailRequiresAuthor(t *testing.T) {
	env := newEnv()
	ctx := context.Background()

	if _, err := env.Service.ScoreDetail(ctx, 0, 100); !apperr.IsCode(err, apperr.CodeAuthRequired) {
		t.Fatalf("未登录应报错，得到 %v", err)
	}
	if _, err := env.Service.ScoreDetail(ctx, 8, 100); !apperr.IsCode(err, apperr.CodeAIPermissionDenied) {
		t.Fatalf("非作者应报权限错误，得到 %v", err)
	}
	if _, err := env.Service.ScoreDetail(ctx, 7, 999); !apperr.IsCode(err, apperr.CodeAIArticleMissing) {
		t.Fatalf("不存在的文章应报错，得到 %v", err)
	}
}

func TestSummaryHidesAdvice(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	dimensions, _ := json.Marshal([]map[string]any{{"name": "结构", "score": 80, "reason": "原因"}})
	issues, _ := json.Marshal([]map[string]any{{"title": "问题"}})
	env.Scores.Seed(domain.ScoreRecord{
		ID:             ctype.NewID(1),
		ArticleID:      ctype.NewID(100),
		UserID:         ctype.NewID(7),
		AITotalScore:   85,
		TotalScore:     82,
		ScoreLevel:     "B",
		ArticleType:    "教程",
		DimensionsJSON: string(dimensions),
		MainIssuesJSON: string(issues),
		OverallComment: "总体不错",
		CreatedAt:      time.Now(),
	})

	summary, err := env.Service.ScoreSummary(ctx, 100)
	if err != nil {
		t.Fatalf("读摘要失败: %v", err)
	}
	if summary.AITotalScore != 0 || summary.ArticleType != "" || summary.OverallComment != "" {
		t.Fatalf("摘要不应暴露总分/类型/评语: %+v", summary)
	}
	if len(summary.MainIssues) != 0 {
		t.Fatalf("摘要不应带回问题列表: %+v", summary.MainIssues)
	}
	if summary.TotalScore != 82 || summary.ScoreLevel != "B" {
		t.Fatalf("摘要应保留总分与等级: %+v", summary)
	}
	if len(summary.Dimensions) != 1 || summary.Dimensions[0].Reason != "" {
		t.Fatalf("摘要的维度不应带原因: %+v", summary.Dimensions)
	}

	detail, err := env.Service.ScoreDetail(ctx, 7, 100)
	if err != nil {
		t.Fatalf("读详情失败: %v", err)
	}
	if detail.AITotalScore != 85 || detail.ArticleType != "教程" || detail.OverallComment != "总体不错" {
		t.Fatalf("详情应暴露完整信息: %+v", detail)
	}
	if len(detail.Dimensions) != 1 || detail.Dimensions[0].Reason != "原因" {
		t.Fatalf("详情的维度应带原因: %+v", detail.Dimensions)
	}
	if len(detail.MainIssues) != 1 {
		t.Fatalf("详情应带回问题列表: %+v", detail.MainIssues)
	}
}

func TestEnsureScoreIfMissingSkips(t *testing.T) {
	ctx := context.Background()

	t.Run("文章不存在", func(t *testing.T) {
		env := newEnv()
		if err := env.Service.EnsureScoreIfMissing(ctx, 999); err != nil {
			t.Fatalf("文章不存在不应报错: %v", err)
		}
	})

	t.Run("未发布", func(t *testing.T) {
		env := newEnv()
		article := env.Scores.Articles[100]
		article.Published = false
		env.Scores.Articles[100] = article
		if err := env.Service.EnsureScoreIfMissing(ctx, 100); err != nil {
			t.Fatalf("未发布不应报错: %v", err)
		}
		if len(env.Scores.All()) != 0 {
			t.Fatal("未发布不应产生评分记录")
		}
	})

	t.Run("AI 未启用", func(t *testing.T) {
		env := apptest.NewEnv(conf.AI{Enable: false})
		env.Scores.Articles[100] = domain.ArticleSnapshot{ID: ctype.NewID(100), AuthorID: ctype.NewID(7), Published: true}
		if err := env.Service.EnsureScoreIfMissing(ctx, 100); err != nil {
			t.Fatalf("AI 未启用不应报错: %v", err)
		}
		if len(env.Scores.All()) != 0 {
			t.Fatal("AI 未启用不应产生评分记录")
		}
	})
}

func TestRewriteSearchRequiresContent(t *testing.T) {
	env := newEnv()
	if _, err := env.Service.RewriteSearch(context.Background(), "   "); !apperr.IsCode(err, apperr.CodeAIContentRequired) {
		t.Fatalf("空内容应报错，得到 %v", err)
	}
}

func TestSearchArticlesDeduplicates(t *testing.T) {
	env := newEnv()
	env.Searcher.Results = func(req searchdomain.Request) searchdomain.Response {
		if len(req.LegacyTagList) > 0 {
			return searchdomain.Response{List: []searchdomain.Item{
				{ID: ctype.NewID(1)}, {ID: ctype.NewID(2)},
			}}
		}
		return searchdomain.Response{List: []searchdomain.Item{
			{ID: ctype.NewID(2)}, {ID: ctype.NewID(3)},
		}}
	}
	rewrite := aisearchRewrite("Go", []string{"Go"}, 2)
	result, err := env.Service.SearchArticles(context.Background(), rewrite)
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if len(result.List) != 3 {
		t.Fatalf("应去重后剩 3 条，得到 %d", len(result.List))
	}
	if len(env.Searcher.Calls) != 2 {
		t.Fatalf("应先带标签再不带标签各搜一次，实际 %d 次", len(env.Searcher.Calls))
	}
}

// aisearchRewrite 构造一个搜索改写结果。
func aisearchRewrite(key string, tags []string, sort int8) *aisearch.ArticleSearchRewrite {
	return &aisearch.ArticleSearchRewrite{
		Intent:  "search",
		Query:   []string{key},
		TagList: tags,
		Sort:    sort,
	}
}

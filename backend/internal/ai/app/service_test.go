package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"myblogx/conf"
	"myblogx/internal/ai/app"
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

// TestRescoreOverwritesStoredScore 钉住 #305：已评分的文章再评分时存储被覆盖，
// 读者页与作者页读到的都是最新一次结果，而且库里仍然只有一行。
func TestRescoreOverwritesStoredScore(t *testing.T) {
	env := newEnv()
	ctx := context.Background()
	// 把评分请求打到桩模型：第一次 41 分，第二次 88 分（模拟作者改稿后重算）。
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		score := 41
		if atomic.AddInt32(&calls, 1) > 1 {
			score = 88
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(scoreReply(score)))
	}))
	defer server.Close()
	env.AI.Active.Fields["base_url"] = server.URL

	in := app.RescoreInput{
		ArticleID: ctype.NewID(100),
		Title:     "标题",
		Content:   "第一版正文：讲清楚了一件事，但论证单薄，例子也不够具体。",
	}

	first, err := env.Service.Rescore(ctx, 7, in)
	if err != nil {
		t.Fatalf("首次评分失败: %v", err)
	}
	if first.AITotalScore != 41 {
		t.Fatalf("首次评分总分 = %d，期望 41", first.AITotalScore)
	}

	in.Content = "改稿后的正文：补了例子、压了重复表述，论证完整，结构清晰，读起来顺畅。"
	second, err := env.Service.Rescore(ctx, 7, in)
	if err != nil {
		t.Fatalf("再次评分失败: %v", err)
	}
	if second.AITotalScore != 88 {
		t.Fatalf("再次评分应返回新分数 88，实际 %d", second.AITotalScore)
	}
	if second.TotalScore <= first.TotalScore {
		t.Fatalf("再次评分的加权总分 %d 没有高于首次的 %d", second.TotalScore, first.TotalScore)
	}
	if second.RecordID != first.RecordID {
		t.Fatalf("再次评分应覆盖同一行，record_id 从 %v 变成了 %v", first.RecordID, second.RecordID)
	}

	// 作者读详情、读者读摘要，都必须是最新一次的结果。
	detail, err := env.Service.ScoreDetail(ctx, 7, 100)
	if err != nil {
		t.Fatalf("读作者评分详情失败: %v", err)
	}
	if detail.AITotalScore != 88 || detail.TotalScore != second.TotalScore {
		t.Fatalf("作者读到的是旧结果: %+v", detail)
	}
	summary, err := env.Service.ScoreSummary(ctx, 100)
	if err != nil {
		t.Fatalf("读读者评分摘要失败: %v", err)
	}
	if summary.TotalScore != second.TotalScore {
		t.Fatalf("读者读到的总分 = %d，期望 %d", summary.TotalScore, second.TotalScore)
	}

	if records := env.Scores.All(); len(records) != 1 {
		t.Fatalf("同一篇文章的评分记录数 = %d，期望 1", len(records))
	}
}

// scoreReply 拼一段 OpenAI 风格的评分回复，六个维度统一给 score 分。
func scoreReply(score int) string {
	names := []string{"clarity", "structure", "completeness", "readability", "persuasiveness", "language"}
	dimensions := make([]map[string]any, 0, len(names))
	for _, name := range names {
		dimensions = append(dimensions, map[string]any{"name": name, "score": score, "reason": "理由"})
	}
	body, err := json.Marshal(map[string]any{
		"ai_total_score":  score,
		"total_score":     score,
		"score_level":     "B",
		"article_type":    "教程",
		"dimensions":      dimensions,
		"main_issues":     []any{},
		"overall_comment": "整体不错",
	})
	if err != nil {
		panic(err)
	}
	reply, err := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"index": 0, "message": map[string]any{"role": "assistant", "content": "```json\n" + string(body) + "\n```"}},
		},
	})
	if err != nil {
		panic(err)
	}
	return string(reply)
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

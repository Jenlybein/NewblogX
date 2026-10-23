package ai_scoring

import (
	"encoding/json"
	"myblogx/conf"
	"myblogx/service/ai_service"
	"myblogx/test/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestScoreArticleQualityShortArticle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ai_service.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("解析 AI 请求失败: %v", err)
		}
		if len(req.Messages) == 0 || !strings.Contains(req.Messages[0].Content, "总体质量评分") {
			t.Fatalf("短文评分 prompt 不符合预期: %+v", req.Messages)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role": "assistant",
						"content": "```json\n{\n" +
							`"ai_total_score":95,` +
							`"total_score":95,` +
							`"score_level":"精品文章",` +
							`"article_type":"说明文",` +
							`"dimensions":[` +
							`{"name":"clarity","score":84,"reason":"表达比较清楚"},` +
							`{"name":"structure","score":82,"reason":"结构较完整"},` +
							`{"name":"completeness","score":80,"reason":"内容基本完整"},` +
							`{"name":"readability","score":81,"reason":"阅读节奏尚可"},` +
							`{"name":"persuasiveness","score":79,"reason":"论证还有提升空间"},` +
							`{"name":"language","score":86,"reason":"语言较规范"}` +
							`],` +
							`"main_issues":[{"positions":[{"paragraph":2,"quote":"论证部分略显单薄"}],"reason":"论证支撑不够充分","suggestion":"补充一个具体例子"}],` +
							`"overall_comment":"整体完成度不错，建议优先补强论证，并顺手压缩重复表述。"` +
							"\n}\n```",
					},
				},
			},
		})
	}))
	defer server.Close()

	testutil.SetConfig(&conf.Config{
		AI: conf.AI{
			Enable:    true,
			SecretKey: "test-key",
			BaseURL:   server.URL,
			ChatModel: "test-model",
		},
	})

	resp, err := ScoreArticleQuality(testutil.Config().AI, ArticleScoreRequest{
		Title:   "Go 写作实践",
		Content: "# Go 写作实践\n\n这是一篇完整的文章。\n\n它讨论了表达、结构和示例。",
	})
	if err != nil {
		t.Fatalf("短文评分失败: %v", err)
	}

	if resp.ArticleType != ArticleTypeExpository {
		t.Fatalf("文章类型应由 AI 识别并规范化: %+v", resp)
	}
	if resp.AITotalScore != 95 {
		t.Fatalf("应保留 AI 原始总分: %+v", resp)
	}
	if resp.TotalScore != calculateWeightedScore(resp.Dimensions, ArticleTypeExpository) {
		t.Fatalf("总分应由后端重算: %+v", resp)
	}
	if resp.ScoreLevel != scoreLevel(resp.TotalScore) {
		t.Fatalf("评分等级错误: %+v", resp)
	}
	if len(resp.MainIssues) != 1 {
		t.Fatalf("主要问题数量错误: %+v", resp.MainIssues)
	}
}

func TestScoreArticleQualityLongArticle(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		var req ai_service.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("解析 AI 请求失败: %v", err)
		}
		if len(req.Messages) == 0 {
			t.Fatalf("AI 请求消息不能为空")
		}
		content := req.Messages[0].Content

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(content, "初始化一个“暂定评分状态”"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"content": `{
								"article_type":"教程",
								"provisional_dimensions":[
									{"name":"clarity","score":82,"evidence":"整体可理解"},
									{"name":"structure","score":80,"evidence":"顺序基本合理"},
									{"name":"completeness","score":78,"evidence":"说明尚可"},
									{"name":"readability","score":79,"evidence":"句式平稳"},
									{"name":"persuasiveness","score":77,"evidence":"例证偏少"},
									{"name":"language","score":84,"evidence":"语法较稳定"}
								],
								"chunk_summary":"第一段主要介绍背景与目标。",
								"global_summary":"文章在说明一个实践方案。",
								"main_issues":[
									{"positions":[{"paragraph":3,"quote":"这里缺少具体例子"}],"reason":"例证偏少","suggestion":"补一个案例"}
								],
								"overall_comment":"整体方向对，但应先补案例并强化论证。",
								"covered_chunk_index":1,
								"covered_chunk_count":2
							}`,
						},
					},
				},
			})
		case strings.Contains(content, "最后一段"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"content": `{
								"ai_total_score":88,
								"total_score":88,
								"score_level":"优质文章",
								"article_type":"教程",
								"dimensions":[
									{"name":"clarity","score":83,"reason":"整体表达清楚"},
									{"name":"structure","score":82,"reason":"结构较顺"},
									{"name":"completeness","score":84,"reason":"内容完整"},
									{"name":"readability","score":80,"reason":"阅读体验稳定"},
									{"name":"persuasiveness","score":79,"reason":"论证较稳但仍可补强"},
									{"name":"language","score":85,"reason":"语言规范"}
								],
								"main_issues":[
									{"positions":[{"paragraph":3,"quote":"这里缺少具体例子"}],"reason":"例证偏少","suggestion":"补一个案例"},
									{"positions":[{"paragraph":3,"quote":"这里缺少具体例子"}],"reason":"例证偏少","suggestion":"补一个案例"}
								],
								"overall_comment":"整体不错，建议先补例证，再把标题写得更聚焦。"
							}`,
						},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role": "assistant",
							"content": `{
								"article_type":"教程",
								"provisional_dimensions":[
									{"name":"clarity","score":83,"evidence":"新增内容没有破坏理解"},
									{"name":"structure","score":81,"evidence":"顺序仍然较顺"},
									{"name":"completeness","score":80,"evidence":"信息继续补全"},
									{"name":"readability","score":80,"evidence":"可读性稳定"},
									{"name":"persuasiveness","score":78,"evidence":"论证略有增强"},
									{"name":"language","score":84,"evidence":"语言保持稳定"}
								],
								"chunk_summary":"中间段继续展开主体内容。",
								"global_summary":"文章围绕实践方案持续展开，信息逐步完整。",
								"main_issues":[
									{"positions":[{"paragraph":3,"quote":"这里缺少具体例子"}],"reason":"例证偏少","suggestion":"补一个案例"}
								],
								"overall_comment":"主体完整，但仍建议继续补案例。",
								"covered_chunk_index":2,
								"covered_chunk_count":3
							}`,
						},
					},
				},
			})
		}
	}))
	defer server.Close()

	testutil.SetConfig(&conf.Config{
		AI: conf.AI{
			Enable:    true,
			SecretKey: "test-key",
			BaseURL:   server.URL,
			ChatModel: "test-model",
		},
	})

	var builder strings.Builder
	builder.WriteString("# 超长文章标题\n\n")
	for i := 0; i < 3400; i++ {
		builder.WriteString("这是用于测试超长文章评分流程的段落内容，它会重复出现以触发长文模式，并验证多轮状态收口是否正常。\n")
	}

	resp, err := ScoreArticleQuality(testutil.Config().AI, ArticleScoreRequest{
		Title:   "超长文章标题",
		Content: builder.String(),
	})
	if err != nil {
		t.Fatalf("长文评分失败: %v", err)
	}

	if atomic.LoadInt32(&callCount) < 2 {
		t.Fatalf("长文流程至少应调用两次 AI, got=%d", callCount)
	}
	if len(resp.MainIssues) != 2 {
		t.Fatalf("问题列表应按模型输出保留，不再去重: %+v", resp.MainIssues)
	}
	if resp.ArticleType != ArticleTypeTutorial {
		t.Fatalf("长文应保留 AI 识别出的文章类型: %+v", resp)
	}
	if resp.AITotalScore != 88 {
		t.Fatalf("长文应保留 AI 原始总分: %+v", resp)
	}
	if resp.OverallComment == "" {
		t.Fatalf("最终建议应合并到 overall_comment: %+v", resp)
	}
}

func TestNormalizeFinalResponseKeepsLowScores(t *testing.T) {
	resp := normalizeFinalResponse(&ArticleScoreResponse{
		ArticleType: ArticleTypeArgument,
		Dimensions: []ArticleScoreDimension{
			{Name: DimensionClarity, Score: 20, Reason: "表达混乱"},
			{Name: DimensionStructure, Score: 25, Reason: "结构失衡"},
			{Name: DimensionCompleteness, Score: 30, Reason: "信息缺失"},
			{Name: DimensionReadability, Score: 28, Reason: "可读性较差"},
			{Name: DimensionPersuasiveness, Score: 18, Reason: "论证不足"},
			{Name: DimensionLanguage, Score: 22, Reason: "语病较多"},
		},
	}, "")

	if resp.TotalScore >= 60 {
		t.Fatalf("低分文章不应再被抬到 60 分以上: %+v", resp)
	}
	if resp.ScoreLevel != "较差文章" {
		t.Fatalf("低分档位错误: %+v", resp)
	}
}

func TestScoreLevelRanges(t *testing.T) {
	cases := []struct {
		score int
		level string
	}{
		{0, "较差文章"},
		{30, "较差文章"},
		{31, "不合格文章"},
		{59, "不合格文章"},
		{60, "常规文章"},
		{80, "常规文章"},
		{81, "优质文章"},
		{89, "优质文章"},
		{90, "精品文章"},
		{100, "精品文章"},
	}

	for _, item := range cases {
		if got := scoreLevel(item.score); got != item.level {
			t.Fatalf("scoreLevel(%d)=%s, want=%s", item.score, got, item.level)
		}
	}
}

func TestNormalizeFinalResponseKeepsIssueOrderAndCapsByScore(t *testing.T) {
	resp := normalizeFinalResponse(&ArticleScoreResponse{
		ArticleType:  ArticleTypeArgument,
		AITotalScore: 45,
		Dimensions: []ArticleScoreDimension{
			{Name: DimensionClarity, Score: 45, Reason: "表达一般"},
			{Name: DimensionStructure, Score: 48, Reason: "结构一般"},
			{Name: DimensionCompleteness, Score: 44, Reason: "内容一般"},
			{Name: DimensionReadability, Score: 46, Reason: "可读性一般"},
			{Name: DimensionPersuasiveness, Score: 43, Reason: "说服力一般"},
			{Name: DimensionLanguage, Score: 47, Reason: "语言一般"},
		},
		MainIssues: []ArticleScoreIssue{
			{Reason: "问题1", Suggestion: "建议1"},
			{Reason: "问题2", Suggestion: "建议2"},
			{Reason: "问题3", Suggestion: "建议3"},
			{Reason: "问题4", Suggestion: "建议4"},
			{Reason: "问题5", Suggestion: "建议5"},
			{Reason: "问题6", Suggestion: "建议6"},
			{Reason: "问题7", Suggestion: "建议7"},
			{Reason: "问题8", Suggestion: "建议8"},
		},
	}, "")

	if len(resp.MainIssues) != 7 {
		t.Fatalf("45 分文章的问题上限应为 7 条: %+v", resp.MainIssues)
	}
	if resp.MainIssues[0].Reason != "问题1" || resp.MainIssues[6].Reason != "问题7" {
		t.Fatalf("问题列表应保持原顺序裁剪: %+v", resp.MainIssues)
	}
}

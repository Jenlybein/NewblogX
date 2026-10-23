package ai_api_test

import (
	"encoding/json"
	"myblogx/api/ai_api"
	"myblogx/conf"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/ai_service"
	"myblogx/service/ai_service/ai_scoring"
	"myblogx/test/testutil"
	"myblogx/utils/jwts"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/gorm"
)

func TestAIArticleScoringType1ReadsSummaryFromCache(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.ArticleModel{},
		&models.ArticleAIScoreRecordModel{},
		&models.RuntimeSiteConfigModel{},
	)
	author := models.UserModel{Username: "score_author", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("创建作者失败: %v", err)
	}
	article := models.ArticleModel{Title: "评分文章", Content: "内容", AuthorID: author.ID}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	record := seedArticleAIScoreRecord(t, db, article.ID, author.ID)

	api := newAIApi(t)
	c, w := newAICtx()
	c.Set("requestJson", ai_api.AIArticleScoringRequest{
		Type:      1,
		ArticleID: &article.ID,
	})

	api.AIArticleScoringView(c)

	if code := readAICode(t, w); code != 0 {
		t.Fatalf("读取评分摘要失败, body=%s", w.Body.String())
	}
	var body struct {
		Code int                             `json:"code"`
		Data ai_api.AIArticleScoringResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}
	if !body.Data.HasScore || body.Data.RecordID == nil || *body.Data.RecordID != record.ID {
		t.Fatalf("摘要应返回已有评分记录: %+v", body.Data)
	}
	if body.Data.TotalScore != record.TotalScore || body.Data.ScoreLevel != record.ScoreLevel {
		t.Fatalf("摘要总分或等级错误: %+v", body.Data)
	}
	if len(body.Data.Dimensions) != 6 {
		t.Fatalf("摘要维度数量错误: %+v", body.Data.Dimensions)
	}
	if body.Data.Dimensions[0].Reason != "" {
		t.Fatalf("type=1 不应返回建议理由: %+v", body.Data.Dimensions)
	}
	if body.Data.OverallComment != "" || len(body.Data.MainIssues) != 0 {
		t.Fatalf("type=1 不应返回完整建议: %+v", body.Data)
	}
}

func TestAIArticleScoringType2ReadsFullCacheForAuthor(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.ArticleModel{},
		&models.ArticleAIScoreRecordModel{},
		&models.RuntimeSiteConfigModel{},
	)
	author := models.UserModel{Username: "score_author2", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("创建作者失败: %v", err)
	}
	article := models.ArticleModel{Title: "评分文章", Content: "内容", AuthorID: author.ID}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	record := seedArticleAIScoreRecord(t, db, article.ID, author.ID)

	api := newAIApi(t)
	c, w := newAICtx()
	c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: author.ID, Role: enum.RoleUser, Username: author.Username}})
	c.Set("requestJson", ai_api.AIArticleScoringRequest{
		Type:      2,
		ArticleID: &article.ID,
	})

	api.AIArticleScoringView(c)

	if code := readAICode(t, w); code != 0 {
		t.Fatalf("读取完整评分失败, body=%s", w.Body.String())
	}
	var body struct {
		Code int                             `json:"code"`
		Data ai_api.AIArticleScoringResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}
	if !body.Data.HasScore || body.Data.RecordID == nil || *body.Data.RecordID != record.ID {
		t.Fatalf("完整评分应返回已有评分记录: %+v", body.Data)
	}
	if body.Data.AITotalScore != record.AITotalScore || body.Data.ArticleType != record.ArticleType {
		t.Fatalf("完整评分缺少 AI 原始分或文章类型: %+v", body.Data)
	}
	if len(body.Data.MainIssues) != 1 || body.Data.OverallComment == "" {
		t.Fatalf("完整评分建议缺失: %+v", body.Data)
	}
	if len(body.Data.Dimensions) != 6 || body.Data.Dimensions[0].Reason == "" {
		t.Fatalf("type=2 应返回完整维度建议: %+v", body.Data.Dimensions)
	}
}

func TestAIArticleScoringType2RejectsNonAuthor(t *testing.T) {
	db := testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.ArticleModel{},
		&models.ArticleAIScoreRecordModel{},
		&models.RuntimeSiteConfigModel{},
	)
	author := models.UserModel{Username: "score_author3", Password: "x", Role: enum.RoleUser}
	reader := models.UserModel{Username: "score_reader3", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("创建作者失败: %v", err)
	}
	if err := db.Create(&reader).Error; err != nil {
		t.Fatalf("创建访客失败: %v", err)
	}
	article := models.ArticleModel{Title: "评分文章", Content: "内容", AuthorID: author.ID}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}
	seedArticleAIScoreRecord(t, db, article.ID, author.ID)

	api := newAIApi(t)
	c, w := newAICtx()
	c.Set("claims", &jwts.MyClaims{Claims: jwts.Claims{UserID: reader.ID, Role: enum.RoleUser, Username: reader.Username}})
	c.Set("requestJson", ai_api.AIArticleScoringRequest{
		Type:      2,
		ArticleID: &article.ID,
	})

	api.AIArticleScoringView(c)
	if code := readAICode(t, w); code == 0 {
		t.Fatalf("非作者读取完整评分应失败, body=%s", w.Body.String())
	}
}

func TestAIArticleScoringType3ScoresAndPersists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ai_service.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("解析 AI 请求失败: %v", err)
		}
		if len(req.Messages) == 0 {
			t.Fatalf("AI 请求消息不能为空")
		}

		w.Header().Set("Content-Type", "application/json")
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
							"article_type":"说明文",
							"dimensions":[
								{"name":"clarity","score":84,"reason":"表达比较清楚"},
								{"name":"structure","score":83,"reason":"结构较顺"},
								{"name":"completeness","score":82,"reason":"内容完整度较好"},
								{"name":"readability","score":80,"reason":"整体较顺畅"},
								{"name":"persuasiveness","score":79,"reason":"论证还有提升空间"},
								{"name":"language","score":85,"reason":"语言较规范"}
							],
							"main_issues":[
								{"positions":[{"paragraph":2,"quote":"这部分论证稍显单薄"}],"reason":"论证支撑不足","suggestion":"补充案例或数据"}
							],
							"overall_comment":"文章整体不错，优先补强论证部分，再顺手压缩重复表述并收束结尾。"
						}`,
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

	db := testutil.SetupSQLite(t,
		&models.UserModel{},
		&models.ArticleModel{},
		&models.ArticleAIScoreRecordModel{},
		&models.RuntimeSiteConfigModel{},
	)
	author := models.UserModel{Username: "score_author4", Password: "x", Role: enum.RoleUser}
	if err := db.Create(&author).Error; err != nil {
		t.Fatalf("创建作者失败: %v", err)
	}
	article := models.ArticleModel{
		Title:    "文章标题",
		Content:  "# 文章标题\n\n这是一篇测试文章。\n\n它包含完整的正文内容。",
		AuthorID: author.ID,
	}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("创建文章失败: %v", err)
	}

	api := newAIApi(t)
	c, w := newAICtx()
	c.Set("claims", &jwts.MyClaims{
		Claims: jwts.Claims{
			UserID:   author.ID,
			Role:     enum.RoleUser,
			Username: author.Username,
		},
	})
	c.Set("requestJson", ai_api.AIArticleScoringRequest{
		Type:      3,
		ArticleID: &article.ID,
	})

	api.AIArticleScoringView(c)

	if code := readAICode(t, w); code != 0 {
		t.Fatalf("文章评分接口应成功, body=%s", w.Body.String())
	}

	var body struct {
		Code int                             `json:"code"`
		Data ai_api.AIArticleScoringResponse `json:"data"`
		Msg  string                          `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v body=%s", err, w.Body.String())
	}

	if !body.Data.HasScore || body.Data.TotalScore <= 0 || len(body.Data.Dimensions) != 6 {
		t.Fatalf("评分返回结构错误: %+v", body.Data)
	}
	if body.Data.AITotalScore != 88 || body.Data.ArticleType != "说明文" {
		t.Fatalf("AI 原始总分或文章类型返回错误: %+v", body.Data)
	}
	if len(body.Data.MainIssues) != 1 {
		t.Fatalf("主要问题返回错误: %+v", body.Data.MainIssues)
	}

	var record models.ArticleAIScoreRecordModel
	if err := db.Order("created_at desc").Take(&record, "article_id = ?", article.ID).Error; err != nil {
		t.Fatalf("评分记录应落库: %v", err)
	}
	if record.AITotalScore != 88 || record.TotalScore <= 0 || record.ModelName != "test-model" {
		t.Fatalf("评分记录内容异常: %+v", record)
	}
}

func seedArticleAIScoreRecord(t *testing.T, db *gorm.DB, articleID ctype.ID, userID ctype.ID) *models.ArticleAIScoreRecordModel {
	t.Helper()
	dimensionsJSON, err := json.Marshal([]ai_scoring.ArticleScoreDimension{
		{Name: "clarity", Score: 84, Reason: "表达比较清楚"},
		{Name: "structure", Score: 83, Reason: "结构较顺"},
		{Name: "completeness", Score: 82, Reason: "内容完整度较好"},
		{Name: "readability", Score: 80, Reason: "整体较顺畅"},
		{Name: "persuasiveness", Score: 79, Reason: "论证还有提升空间"},
		{Name: "language", Score: 85, Reason: "语言较规范"},
	})
	if err != nil {
		t.Fatalf("序列化评分维度失败: %v", err)
	}
	mainIssuesJSON, err := json.Marshal([]ai_scoring.ArticleScoreIssue{
		{
			Positions:  []ai_scoring.ArticleScorePosition{{Paragraph: 2, Quote: "这部分论证稍显单薄"}},
			Reason:     "论证支撑不足",
			Suggestion: "补充案例或数据",
		},
	})
	if err != nil {
		t.Fatalf("序列化主要问题失败: %v", err)
	}

	record := &models.ArticleAIScoreRecordModel{
		ArticleID:      articleID,
		UserID:         userID,
		TitleSnapshot:  "评分文章",
		ContentHash:    "hash-1",
		ContentLength:  120,
		AITotalScore:   88,
		TotalScore:     88,
		ScoreLevel:     "优质文章",
		ArticleType:    "说明文",
		DimensionsJSON: string(dimensionsJSON),
		MainIssuesJSON: string(mainIssuesJSON),
		OverallComment: "文章整体不错，优先补强论证部分，再顺手压缩重复表述并收束结尾。",
		Provider:       "llm",
		ModelName:      "test-model",
		PromptVersion:  "article-scoring-v1",
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("创建评分记录失败: %v", err)
	}
	return record
}

package ai_api

import (
	"encoding/json"
	"errors"
	"strings"

	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/ai_service/ai_scoring"
	"myblogx/service/ai_service/article_score_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AIArticleScoringView 读取或生成文章质量评分。
// type=1 读取公开评分摘要；type=2 读取作者可见完整评分；type=3 重新评分并落库。
func (h AIApi) AIArticleScoringView(c *gin.Context) {
	cr := middleware.GetBindJson[AIArticleScoringRequest](c)
	if cr.ArticleID == nil || *cr.ArticleID == 0 {
		res.FailWithMsg("article_id 不能为空", c)
		return
	}

	switch cr.Type {
	case 1:
		h.readArticleScoreSummary(c, *cr.ArticleID)
	case 2:
		h.readArticleScoreDetail(c, *cr.ArticleID)
	case 3:
		h.regenerateArticleScore(c, cr)
	default:
		res.FailWithMsg("评分类型错误", c)
	}
}

func (h AIApi) readArticleScoreSummary(c *gin.Context, articleID ctype.ID) {
	record, err := article_score_service.LoadLatestArticleAIScoreRecord(h.App.DB, articleID)
	if err != nil {
		res.FailWithMsg("读取文章评分失败", c)
		return
	}
	if record == nil {
		res.OkWithData(AIArticleScoringResponse{
			HasScore:  false,
			ArticleID: &articleID,
		}, c)
		return
	}

	resp, err := buildArticleAIScoreResponse(record, false)
	if err != nil {
		res.FailWithMsg("读取文章评分失败", c)
		return
	}
	res.OkWithData(resp, c)
}

func (h AIApi) readArticleScoreDetail(c *gin.Context, articleID ctype.ID) {
	claims, article, ok := h.requireArticleAuthor(c, articleID)
	if !ok {
		_ = claims
		return
	}
	_ = article

	record, err := article_score_service.LoadLatestArticleAIScoreRecord(h.App.DB, articleID)
	if err != nil {
		res.FailWithMsg("读取文章评分失败", c)
		return
	}
	if record == nil {
		res.OkWithData(AIArticleScoringResponse{
			HasScore:  false,
			ArticleID: &articleID,
		}, c)
		return
	}

	resp, err := buildArticleAIScoreResponse(record, true)
	if err != nil {
		res.FailWithMsg("读取文章评分失败", c)
		return
	}
	res.OkWithData(resp, c)
}

func (h AIApi) regenerateArticleScore(c *gin.Context, cr AIArticleScoringRequest) {
	if h.App.RuntimeSite == nil {
		res.FailWithMsg("运行时配置服务未初始化", c)
		return
	}

	claims, article, ok := h.requireArticleAuthor(c, *cr.ArticleID)
	if !ok {
		return
	}

	title := strings.TrimSpace(cr.Title)
	content := strings.TrimSpace(cr.Content)
	if title == "" {
		title = strings.TrimSpace(article.Title)
	}
	if content == "" {
		content = strings.TrimSpace(article.Content)
	}

	scoreResp, err := ai_scoring.ScoreArticleQuality(h.App.RuntimeSite.GetRuntimeAI(), ai_scoring.ArticleScoreRequest{
		Title:   title,
		Content: content,
	})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	record, err := article_score_service.PersistArticleAIScoreRecord(h.App.DB, article, claims.UserID, title, content, scoreResp, h.App.RuntimeSite.GetRuntimeAI())
	if err != nil {
		res.FailWithMsg("保存文章评分失败", c)
		return
	}

	resp, err := buildArticleAIScoreResponse(record, true)
	if err != nil {
		res.FailWithMsg("保存文章评分失败", c)
		return
	}
	res.OkWithData(resp, c)
}

func (h AIApi) requireArticleAuthor(c *gin.Context, articleID ctype.ID) (*jwts.MyClaims, *models.ArticleModel, bool) {
	claims := jwts.GetClaimsByGin(c)
	if claims == nil || claims.UserID == 0 {
		res.FailWithMsg("请先登录", c)
		return nil, nil, false
	}

	article, err := article_score_service.LoadArticleForScoring(h.App.DB, articleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			res.FailWithMsg("文章不存在", c)
		} else {
			res.FailWithMsg("读取文章失败", c)
		}
		return nil, nil, false
	}
	if article.AuthorID != claims.UserID {
		res.FailWithMsg("权限错误", c)
		return nil, nil, false
	}
	return claims, article, true
}

func buildArticleAIScoreResponse(record *models.ArticleAIScoreRecordModel, includeAdvice bool) (AIArticleScoringResponse, error) {
	if record == nil {
		return AIArticleScoringResponse{}, nil
	}

	var rawDimensions []ai_scoring.ArticleScoreDimension
	if strings.TrimSpace(record.DimensionsJSON) != "" {
		if err := json.Unmarshal([]byte(record.DimensionsJSON), &rawDimensions); err != nil {
			return AIArticleScoringResponse{}, err
		}
	}

	resp := AIArticleScoringResponse{
		HasScore:     true,
		RecordID:     &record.ID,
		ArticleID:    &record.ArticleID,
		AITotalScore: record.AITotalScore,
		TotalScore:   record.TotalScore,
		ScoreLevel:   record.ScoreLevel,
		ArticleType:  record.ArticleType,
		CreatedAt:    &record.CreatedAt,
		Dimensions:   make([]AIArticleScoreDimension, 0, len(rawDimensions)),
	}
	for _, item := range rawDimensions {
		respDim := AIArticleScoreDimension{
			Name:  item.Name,
			Score: item.Score,
		}
		if includeAdvice {
			respDim.Reason = item.Reason
		}
		resp.Dimensions = append(resp.Dimensions, respDim)
	}

	if !includeAdvice {
		resp.AITotalScore = 0
		resp.ArticleType = ""
		return resp, nil
	}

	if strings.TrimSpace(record.MainIssuesJSON) != "" {
		if err := json.Unmarshal([]byte(record.MainIssuesJSON), &resp.MainIssues); err != nil {
			return AIArticleScoringResponse{}, err
		}
	}
	resp.OverallComment = record.OverallComment
	return resp, nil
}

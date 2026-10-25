// Package http 是 AI 模块的 HTTP 适配器。
package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/common/res"
	"myblogx/internal/ai/app"
	"myblogx/internal/ai/diagnose"
	"myblogx/internal/ai/overwrite"
	"myblogx/internal/apperr"
	"myblogx/internal/kernel/ctype"
	"myblogx/utils/jwts"
)

// Handler 聚合 AI 相关接口。
type Handler struct {
	svc *app.Service
}

// NewHandler 构造处理器。
func NewHandler(svc *app.Service) *Handler {
	if svc == nil {
		panic("ai/http: 用例服务未注入")
	}
	return &Handler{svc: svc}
}

// userID 从 claims 取当前登录用户。
func userID(c *gin.Context) uint64 {
	claims := jwts.GetClaimsByGin(c)
	if claims == nil {
		return 0
	}
	return uint64(claims.UserID)
}

// prepareSSE 设置 SSE 响应头。
func prepareSSE(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
}

// Scoring 读取或生成文章质量评分。
// type=1 公开摘要；type=2 作者完整视图；type=3 重新评分并落库。
// @Summary 读取或生成文章质量评分
// @Tags ai
// @Accept json
// @Produce json
// @Param body body ScoringRequest true "请求体"
// @Success 200 {object} object{code=int,data=ScoringResponse,msg=string}
// @Router /ai/scoring/article [post]
func (h *Handler) Scoring(c *gin.Context) {
	cr, ok := bindJSON[ScoringRequest](c)
	if !ok {
		return
	}
	if cr.ArticleID == nil || *cr.ArticleID == 0 {
		res.FailWithMsg("article_id 不能为空", c)
		return
	}
	articleID := uint64(*cr.ArticleID)
	ctx := c.Request.Context()

	switch cr.Type {
	case 1:
		view, err := h.svc.ScoreSummary(ctx, articleID)
		if err != nil {
			res.FailWithMsg("读取文章评分失败", c)
			return
		}
		res.OkWithData(toScoringResponse(view), c)
	case 2:
		view, err := h.svc.ScoreDetail(ctx, userID(c), articleID)
		if err != nil {
			failScore(c, err)
			return
		}
		res.OkWithData(toScoringResponse(view), c)
	case 3:
		view, err := h.svc.Rescore(ctx, userID(c), app.RescoreInput{
			ArticleID: articleID,
			Title:     cr.Title,
			Content:   cr.Content,
		})
		if err != nil {
			switch {
			case apperr.IsCode(err, apperr.CodeAuthRequired):
				res.FailWithMsg("请先登录", c)
			case apperr.IsCode(err, apperr.CodeAIPermissionDenied):
				res.FailWithMsg("权限错误", c)
			case apperr.IsCode(err, apperr.CodeAIArticleMissing):
				res.FailWithMsg("文章不存在", c)
			case apperr.IsCode(err, apperr.CodeAIScoreSaveFailed):
				res.FailWithMsg("保存文章评分失败", c)
			default:
				res.FailWithMsg(err.Error(), c)
			}
			return
		}
		res.OkWithData(toScoringResponse(view), c)
	default:
		res.FailWithMsg("评分类型错误", c)
	}
}

// failScore 把评分读取错误翻译成旧文案。
func failScore(c *gin.Context, err error) {
	switch {
	case apperr.IsCode(err, apperr.CodeAuthRequired):
		res.FailWithMsg("请先登录", c)
	case apperr.IsCode(err, apperr.CodeAIPermissionDenied):
		res.FailWithMsg("权限错误", c)
	case apperr.IsCode(err, apperr.CodeAIArticleMissing):
		res.FailWithMsg("文章不存在", c)
	default:
		res.FailWithMsg("读取文章评分失败", c)
	}
}

// MetaInfo 根据正文生成文章标题、摘要、分类与标签。
// @Summary 生成文章元信息
// @Tags ai
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body BaseRequest true "请求体"
// @Success 200 {object} object{code=int,data=MetaInfoResponse,msg=string}
// @Router /ai/metainfo [post]
func (h *Handler) MetaInfo(c *gin.Context) {
	cr, ok := bindJSON[BaseRequest](c)
	if !ok {
		return
	}
	data, err := h.svc.Metainfo(c.Request.Context(), userID(c), cr.Content)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(data, c)
}

// Overwrite 对选中文本执行改写，SSE 流式输出。
// @Summary AI 文本改写（SSE 流式）
// @Tags ai
// @Accept json
// @Produce text/event-stream
// @Security BearerAuth
// @Param body body OverwriteRequest true "请求体"
// @Success 200 {string} string "SSE 流"
// @Router /ai/overwrite [post]
func (h *Handler) Overwrite(c *gin.Context) {
	cr, ok := bindJSON[OverwriteRequest](c)
	if !ok {
		return
	}
	prepareSSE(c)

	contentChan, errChan, err := h.svc.RewriteStream(overwrite.RewriteRequest{
		Mode:          cr.Mode,
		SelectionText: cr.SelectionText,
		PrefixText:    cr.PrefixText,
		SuffixText:    cr.SuffixText,
		ArticleTitle:  cr.ArticleTitle,
		TargetStyle:   cr.TargetStyle,
	})
	if err != nil {
		res.SSEFail(err.Error(), c)
		return
	}
	drainStream(c, contentChan, errChan)
}

// Diagnose 对选中文本做结构化诊断，SSE 返回最终结果。
// @Summary AI 文本诊断（SSE 流式）
// @Tags ai
// @Accept json
// @Produce text/event-stream
// @Security BearerAuth
// @Param body body DiagnoseRequest true "请求体"
// @Success 200 {string} string "SSE 流"
// @Router /ai/diagnose [post]
func (h *Handler) Diagnose(c *gin.Context) {
	cr, ok := bindJSON[DiagnoseRequest](c)
	if !ok {
		return
	}
	prepareSSE(c)

	data, err := h.svc.Diagnose(diagnose.DiagnoseRequest{
		SelectionText: cr.SelectionText,
		PrefixText:    cr.PrefixText,
		SuffixText:    cr.SuffixText,
		ArticleTitle:  cr.ArticleTitle,
	})
	if err != nil {
		res.SSEFail(err.Error(), c)
		return
	}
	res.SSEOk(data, c)
}

// SearchList 通过 AI 意图识别改写搜索词并返回文章列表。
// @Summary AI 文章搜索（列表）
// @Tags ai
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body BaseRequest true "请求体"
// @Success 200 {object} object{code=int,data=object,msg=string}
// @Router /ai/search/list [post]
func (h *Handler) SearchList(c *gin.Context) {
	cr, ok := bindJSON[BaseRequest](c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	rewrite, err := h.svc.RewriteSearch(ctx, cr.Content)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	if rewrite.Intent != "search" {
		res.FailWithMsg("当前输入不是文章搜索意图", c)
		return
	}
	result, err := h.svc.SearchArticles(ctx, rewrite)
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(result, c)
}

// SearchLLM 基于检索结果生成 AI 回答，SSE 流式返回。
// @Summary AI 文章搜索（LLM 流式回答）
// @Tags ai
// @Accept json
// @Produce text/event-stream
// @Security BearerAuth
// @Param body body BaseRequest true "请求体"
// @Success 200 {string} string "SSE 流"
// @Router /ai/search/llm [post]
func (h *Handler) SearchLLM(c *gin.Context) {
	cr, ok := bindJSON[BaseRequest](c)
	if !ok {
		return
	}
	prepareSSE(c)
	ctx := c.Request.Context()

	rewrite, err := h.svc.RewriteSearch(ctx, cr.Content)
	if err != nil {
		res.SSEFail(err.Error(), c)
		return
	}
	if rewrite.Intent != "search" {
		res.SSEOk(BaseResponse{Content: rewrite.Content}, c)
		return
	}
	result, err := h.svc.SearchArticles(ctx, rewrite)
	if err != nil {
		res.SSEFail(err.Error(), c)
		return
	}
	contentChan, errChan, err := h.svc.StreamAnswer(cr.Content, result.List)
	if err != nil {
		res.SSEFail(err.Error(), c)
		return
	}
	drainStream(c, contentChan, errChan)
}

// drainStream 把 LLM 的流式输出逐条写成 SSE。
func drainStream(c *gin.Context, contentChan chan string, errChan chan error) {
	for contentChan != nil || errChan != nil {
		select {
		case text, ok := <-contentChan:
			if !ok {
				contentChan = nil
				continue
			}
			res.SSEOk(BaseResponse{Content: text}, c)
		case streamErr, ok := <-errChan:
			if !ok {
				errChan = nil
				continue
			}
			if streamErr != nil {
				res.SSEFail(streamErr.Error(), c)
				return
			}
		}
	}
}

// toScoringResponse 把视图转成旧契约的 JSON 形状。
func toScoringResponse(view app.ScoreView) ScoringResponse {
	resp := ScoringResponse{
		HasScore:       view.HasScore,
		TotalScore:     view.TotalScore,
		ScoreLevel:     view.ScoreLevel,
		MainIssues:     view.MainIssues,
		OverallComment: view.OverallComment,
	}
	if view.ArticleID != 0 {
		id := ctype.ID(view.ArticleID)
		resp.ArticleID = &id
	}
	if view.HasScore {
		resp.AITotalScore = view.AITotalScore
		resp.ArticleType = view.ArticleType
		if view.RecordID != 0 {
			id := ctype.ID(view.RecordID)
			resp.RecordID = &id
		}
		if !view.CreatedAt.IsZero() {
			at := view.CreatedAt
			resp.CreatedAt = &at
		}
		if len(view.Dimensions) > 0 {
			dims := make([]ScoreDimension, 0, len(view.Dimensions))
			for _, dim := range view.Dimensions {
				dims = append(dims, ScoreDimension{Name: dim.Name, Score: dim.Score, Reason: dim.Reason})
			}
			resp.Dimensions = dims
		}
	}
	return resp
}

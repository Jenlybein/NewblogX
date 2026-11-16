// Package http 是内容过滤模块的 HTTP 适配器：后台「审核 → 关键词风险」读面。
//
// 这一票只有**只读**端点。处置动作（删除 / 忽略 / 封禁 / 禁言）是 #398 的活，
// 所以这里没有任何写方法 —— 也就不存在"某条写路由忘了挂 AdminMiddleware"的口子。
package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/filter/app"
	"myblogx/internal/filter/domain"
	"myblogx/internal/platform/http/res"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// Handler 是关键词风险的 HTTP 处理器。
type Handler struct {
	svc *app.Service
}

// NewHandler 构造处理器。
func NewHandler(svc *app.Service) *Handler {
	if svc == nil {
		panic("filter/http: 用例服务未注入")
	}
	return &Handler{svc: svc}
}

// ListHits 返回风险页的一页记录与顶部命中用户 Top（管理员）。
//
// 筛选参数非法时返回"参数错误"而不是静默退化成"不筛选"：后台传错一个字就该看到
// 报错，而不是拿到一份看起来正常、其实没过滤的数据。
// @Summary 关键词风险列表（管理员）
// @Tags content-filter
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码"
// @Param limit query int false "每页条数"
// @Param surface query string false "文本面" Enums(article,comment,nickname,abstract,chat,collection)
// @Param status query string false "处理状态" Enums(pending,ignored,deleted)
// @Param user_id query string false "用户 ID"
// @Success 200 {object} kernel.Envelope[HitListResponse]
// @Router /content-filter/hits [get]
func (h *Handler) ListHits(c *gin.Context) {
	req, err := bindListRequest(c)
	if err != nil {
		res.Fail(err, c)
		return
	}
	result, err := h.svc.List(c.Request.Context(), req)
	if err != nil {
		res.Fail(err, c)
		return
	}
	res.OkWithData(toListResponse(result), c)
}

// bindListRequest 绑定并校验查询参数。
func bindListRequest(c *gin.Context) (app.ListInput, error) {
	var raw HitListRequest
	if err := c.ShouldBindQuery(&raw); err != nil {
		return app.ListInput{}, apperr.New(apperr.CodeCommonInvalidArgument)
	}
	out := app.ListInput{
		UserID: raw.UserID,
		Page:   raw.Page,
		Limit:  raw.Limit,
	}
	if raw.Surface != "" {
		surface, ok := domain.ParseSurface(raw.Surface)
		if !ok {
			return app.ListInput{}, apperr.New(apperr.CodeFilterSurfaceInvalid)
		}
		out.Surface = surface
	}
	if raw.Status != "" {
		status, ok := domain.ParseStatus(raw.Status)
		if !ok {
			return app.ListInput{}, apperr.New(apperr.CodeFilterStatusInvalid)
		}
		out.Status = status
	}
	return out, nil
}

// toListResponse 把用例结果翻译成契约形状。
func toListResponse(result app.ListResult) HitListResponse {
	list := make([]HitItem, 0, len(result.Items))
	for _, row := range result.Items {
		list = append(list, toHitItem(row))
	}
	top := make([]TopUserItem, 0, len(result.TopUsers))
	for _, row := range result.TopUsers {
		top = append(top, TopUserItem{
			UserID:       row.UserID,
			UserNickname: row.Nickname,
			HitCount:     row.HitCount,
		})
	}
	return HitListResponse{List: list, Count: int(result.Total), TopUsers: top}
}

// toHitItem 把领域记录翻译成契约形状。
func toHitItem(row domain.Hit) HitItem {
	words := make([]HitWord, 0, len(row.Words))
	for _, word := range row.Words {
		snippets := make([]HitSnippet, 0, len(word.Snippets))
		for _, snippet := range word.Snippets {
			snippets = append(snippets, HitSnippet{
				Start: snippet.Start,
				End:   snippet.End,
				Text:  snippet.Text,
			})
		}
		words = append(words, HitWord{
			Word:     word.Word,
			Tier:     string(word.Tier),
			Category: string(word.Category),
			Snippets: snippets,
		})
	}
	item := HitItem{
		ID:           row.ID,
		Surface:      string(row.Surface),
		ContentID:    row.ContentID,
		UserID:       row.UserID,
		UserNickname: row.UserNickname,
		Status:       string(row.Status),
		HitCount:     row.HitCount,
		Words:        words,
		HandlerID:    row.HandlerID,
		Note:         row.Note,
	}
	if !row.CreatedAt.IsZero() {
		item.CreatedAt = row.CreatedAt.Format(timeLayout)
	}
	if !row.HandledAt.IsZero() {
		item.HandledAt = row.HandledAt.Format(timeLayout)
	}
	return item
}

// timeLayout 是后台列表统一的时间格式（与评论管理列表一致）。
const timeLayout = "2006-01-02 15:04:05"

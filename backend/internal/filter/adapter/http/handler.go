// Package http 是内容过滤模块的 HTTP 适配器：后台「审核 → 关键词风险」的读面与处置。
//
// 整段路由只给管理员（router/content_filter_router.go）：看得到的是"谁发了什么
// 命中词"，本身就是敏感信息；处置动作还会删内容，更不能让普通登录用户碰到。
package http

import (
	"github.com/gin-gonic/gin"

	"myblogx/internal/apperr"
	"myblogx/internal/filter/app"
	"myblogx/internal/filter/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/res"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
	"myblogx/utils/jwts"
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
// @Param status query string false "处理状态" Enums(pending,ignored,deleted,account)
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

// WordStats 返回词云数据（管理员）：最近 90 天每个命中词的出现处数。
//
// 字号口径在后端定（ADR-0032 决策 9）：前端只负责按 count 画大小，不自己算窗口。
// 读不到统计时返回空集而不是报错 —— 词云是词表页的辅助展示，不该让整页不可用。
// @Summary 词云统计（管理员）
// @Tags content-filter
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[WordStatsResponse]
// @Router /content-filter/word-stats [get]
func (h *Handler) WordStats(c *gin.Context) {
	result := h.svc.WordStats(c.Request.Context())
	items := make([]WordStatItem, 0, len(result.Words))
	for _, stat := range result.Words {
		items = append(items, WordStatItem{
			Word:     stat.Word,
			Count:    stat.Count,
			Tier:     string(stat.Tier),
			Category: string(stat.Category),
		})
	}
	res.OkWithData(WordStatsResponse{Days: result.Days, Items: items}, c)
}

// HandleHit 处置一条关键词风险记录（管理员）。
//
// 动作有四个（票 398 + 399）：delete / ignore 是内容动作；ban / mute 是账号动作，
// 前者封禁（账号禁用），后者禁言（时长见 mute_days，默认 7 天）。
// 内容动作按文本面软删内容（昵称 / 简介面重置资料），私信面没有可删内容会返回
// 明确错误。同一记录重复处置是幂等的（already_handled）。
// @Summary 处置一条关键词风险记录（管理员）
// @Tags content-filter
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "命中记录 ID"
// @Param payload body HandleHitRequest true "处置动作、禁言天数与备注"
// @Success 200 {object} kernel.Envelope[HandleHitResponse]
// @Router /content-filter/hits/{id}/handle [post]
func (h *Handler) HandleHit(c *gin.Context) {
	id, err := parseHitID(c)
	if err != nil {
		res.Fail(err, c)
		return
	}
	var raw HandleHitRequest
	if err := c.ShouldBindJSON(&raw); err != nil {
		res.Fail(apperr.New(apperr.CodeCommonInvalidArgument), c)
		return
	}
	actor, err := handlerID(c)
	if err != nil {
		res.Fail(err, c)
		return
	}
	got, err := h.svc.Handle(c.Request.Context(), app.HandleInput{
		ID:        id,
		Action:    domain.Action(raw.Action),
		MuteDays:  raw.MuteDays,
		Note:      raw.Note,
		HandlerID: actor,
	})
	if err != nil {
		res.Fail(err, c)
		return
	}
	res.OkWithData(HandleHitResponse{
		Hit:            toHitItem(got.Hit),
		AlreadyHandled: got.AlreadyHandled,
	}, c)
}

// HandleBatchHits 批量处置关键词风险记录（管理员）。
//
// 语义是**逐条结果**（见 app.Service.HandleBatch 的注释）：请求级非法（动作错 /
// 备注超长 / 一条没选）整批拒绝；通过之后逐条执行，每条成败各自如实返回，
// 成功的已生效、失败的保持待处理可重试。HTTP 状态恒为 200，业务码在包体里。
// @Summary 批量处置关键词风险记录（管理员）
// @Tags content-filter
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param payload body HandleBatchRequest true "记录 ID、动作与备注"
// @Success 200 {object} kernel.Envelope[HandleBatchResponse]
// @Router /content-filter/hits/handle-batch [post]
func (h *Handler) HandleBatchHits(c *gin.Context) {
	var raw HandleBatchRequest
	if err := c.ShouldBindJSON(&raw); err != nil {
		res.Fail(apperr.New(apperr.CodeCommonInvalidArgument), c)
		return
	}
	actor, err := handlerID(c)
	if err != nil {
		res.Fail(err, c)
		return
	}
	got, err := h.svc.HandleBatch(c.Request.Context(), app.BatchHandleInput{
		IDs:       raw.IDs,
		Action:    domain.Action(raw.Action),
		MuteDays:  raw.MuteDays,
		Note:      raw.Note,
		HandlerID: actor,
	})
	if err != nil {
		res.Fail(err, c)
		return
	}
	results := make([]HandleBatchItem, 0, len(got.Items))
	for _, item := range got.Items {
		row := HandleBatchItem{
			ID:             item.ID,
			Ok:             item.Err == nil,
			Message:        errorMessage(item.Err),
			AlreadyHandled: item.AlreadyHandled,
		}
		if item.Hit.Status.Valid() {
			row.Status = string(item.Hit.Status)
		}
		results = append(results, row)
	}
	res.OkWithData(HandleBatchResponse{Results: results}, c)
}

// parseHitID 从路径参数取命中记录 ID；格式不对按参数错误处理。
func parseHitID(c *gin.Context) (ctype.ID, error) {
	id, err := ctype.ParseID(c.Param("id"))
	if err != nil || id.IsZero() {
		return "", apperr.New(apperr.CodeCommonInvalidArgument)
	}
	return id, nil
}

// handlerID 取当前管理员 ID（AdminMiddleware 已保证登录且是管理员，
// 这里只处理"中间件漏挂"的兜底：没有 claims 就按未登录处理）。
func handlerID(c *gin.Context) (ctype.ID, error) {
	claims := jwts.GetClaimsByGin(c)
	if claims == nil || claims.UserID.IsZero() {
		return "", apperr.New(apperr.CodeAuthRequired)
	}
	return claims.UserID, nil
}

// errorMessage 把逐条失败翻译成可读中文；未知错误不透出内部实现细节。
func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	if code, ok := apperr.CodeOf(err); ok {
		return apperr.Message(code)
	}
	return apperr.Message(apperr.CodeCommonServiceFailed)
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


// Package http 是搜索模块的 HTTP 适配器。
package http

import (
	"context"
	"strconv"
	"strings"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/gin-gonic/gin"

	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/search/app"
	"myblogx/internal/search/domain"
	"myblogx/utils/jwts"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// Verifier 校验访问令牌并返回操作者。
type Verifier interface {
	Verify(ctx context.Context, token string) (domain.Viewer, bool)
}

// Handler 是搜索 HTTP 处理器。
type Handler struct {
	counters domain.CounterReader
	prefs    domain.UserPreferences
	es       *elasticsearch.Client
	index    string
	verifier Verifier
	// onlyAdminAuthors 现读「显示非管理员发布文章」开关的反面：为真表示
	// 公开搜索只出管理员作者的文章。做成取值函数而不是构造时的布尔快照，
	// 后台改完下一次请求即生效（GLOSSARY 的「Provider 闭包（现读）」）。
	//
	// 与另外两处同改：backend/app/visibility_wiring.go 的 onlyAdminAuthors，
	// 以及 backend/internal/article/adapter/http/handler.go 的 onlyAdminAuthors 方法。
	onlyAdminAuthors func() bool
}

// NewHandler 构造处理器。
func NewHandler(counters domain.CounterReader, prefs domain.UserPreferences, es *elasticsearch.Client, index string, verifier Verifier, onlyAdminAuthors func() bool) *Handler {
	if onlyAdminAuthors == nil {
		// 没有配置来源时按开关默认（开）处理：宁可多出内容，也不要静默隐藏。
		onlyAdminAuthors = func() bool { return false }
	}
	return &Handler{counters: counters, prefs: prefs, es: es, index: index, verifier: verifier, onlyAdminAuthors: onlyAdminAuthors}
}

// ArticleSearchQuery 是搜索查询参数（与旧契约一致）。
type ArticleSearchQuery struct {
	Page       int      `form:"page"`
	Limit      int      `form:"limit"`
	Key        string   `form:"key"`
	Order      string   `form:"order"`
	Type       int8     `form:"type"`
	Sort       int8     `form:"sort"`
	PageMode   string   `form:"page_mode"`
	CategoryID ctype.ID `form:"category_id"`
	AuthorID   ctype.ID `form:"author_id"`
	UserID     ctype.ID `form:"user_id"`
	Status     int8     `form:"status"`
	TagList    []string `form:"tag_list"`
}

// ArticleSearch 按关键词、标签、分类等条件搜索文章。
// @Summary 文章搜索
// @Tags search
// @Produce json
// @Success 200 {object} kernel.Envelope[domain.Response]
// @Router /search/articles [get]
func (h *Handler) ArticleSearch(c *gin.Context) {
	var q ArticleSearchQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		res.FailWithMsg("参数错误", c)
		return
	}

	req := domain.Request{
		PageInfo:      domain.PageInfo{Page: q.Page, Limit: q.Limit, Key: q.Key, Order: q.Order},
		Type:          q.Type,
		Sort:          q.Sort,
		PageMode:      domain.PageMode(q.PageMode),
		TagIDs:        parseTagIDs(c),
		CategoryID:    q.CategoryID,
		AuthorID:      q.AuthorID,
		Status:        domain.ArticleStatus(q.Status),
		Key:           q.Key,
		LegacyUserID:  q.UserID,
		LegacyTagList: q.TagList,
		// 开关关闭时公开读面（type 1/2/3）由 app 层加 author_is_admin 过滤；
		// type 4/5 由 app 层忽略这个字段。
		OnlyAdminAuthors: h.onlyAdminAuthors(),
	}

	var viewer *domain.Viewer
	if token := jwts.GetTokenByGin(c); token != "" && h.verifier != nil {
		if v, ok := h.verifier.Verify(c.Request.Context(), token); ok {
			viewer = &v
		}
	}

	var likeTagIDs []ctype.ID
	if viewer != nil && req.NormalizeType() == 2 && h.prefs != nil {
		if tags, err := h.prefs.LikeTagIDs(c.Request.Context(), viewer.ID); err == nil {
			likeTagIDs = tags
		}
	}

	result, err := app.Search(req, viewer, likeTagIDs, h.counters, h.es, h.index)
	if err != nil {
		res.FailWithError(err, c)
		return
	}
	res.OkWithData(result, c)
}

// parseTagIDs 兼容 tag_ids=1,2 与 tag_ids=1&tag_ids=2。
func parseTagIDs(c *gin.Context) []ctype.ID {
	rawList := c.QueryArray("tag_ids")
	if len(rawList) == 0 {
		if joined := strings.TrimSpace(c.Query("tag_ids")); joined != "" {
			rawList = strings.Split(joined, ",")
		}
	}
	result := make([]ctype.ID, 0, len(rawList))
	seen := make(map[ctype.ID]struct{}, len(rawList))
	for _, raw := range rawList {
		id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
		if err != nil || id == 0 {
			continue
		}
		value := ctype.NewID(id)
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

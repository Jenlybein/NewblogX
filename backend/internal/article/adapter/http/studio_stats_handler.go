package http

import (
	"myblogx/internal/article/app"
	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// 本文件是个人数据概览的端点（票 354）。
//
// 一个端点给出首屏要的全部数字：状态数量、四个互动计数、最近 5 篇、
// 阅读 Top 5、90 天活跃度。之前前端发六七次请求，任何一个慢都会让首屏一半是空的，
// 且"共 N 篇"这类数字来自不同时刻的快照，彼此对不上。

// StudioStatusCountResponse 是某个发布态的文章数。
type StudioStatusCountResponse struct {
	Status int `json:"status"`
	Count  int `json:"count"`
}

// StudioArticleBriefResponse 是概览里的文章摘要行（最近 / Top 5 共用）。
//
// 只带列表要显示的字段：概览不是详情页，多带正文只会让载荷变大。
type StudioArticleBriefResponse struct {
	ID           ctype.ID `json:"id"`
	Title        string   `json:"title"`
	Cover        string   `json:"cover"`
	ViewCount    int      `json:"view_count"`
	DiggCount    int      `json:"digg_count"`
	CommentCount int      `json:"comment_count"`
	FavorCount   int      `json:"favor_count"`
	CreatedAt    string   `json:"created_at"`
}

// StudioActivityDayResponse 是热力图的一天。
type StudioActivityDayResponse struct {
	// Day 是 `YYYY-MM-DD`（按 Asia/Shanghai 切天）。
	Day string `json:"day"`
	// Count 是当天的创作动作数（成功的新建 + 更新）。
	Count int `json:"count"`
}

// StudioStatsResponse 是个人数据概览的完整读模型。
type StudioStatsResponse struct {
	StatusCounts []StudioStatusCountResponse  `json:"status_counts"`
	SumView      int64                        `json:"sum_view"`
	SumDigg      int64                        `json:"sum_digg"`
	SumComment   int64                        `json:"sum_comment"`
	SumFavor     int64                        `json:"sum_favor"`
	Recent       []StudioArticleBriefResponse `json:"recent"`
	Top          []StudioArticleBriefResponse `json:"top"`
	// Activity 是**稀疏**的：没有动作的日期不出现，由前端补零铺满 13×7。
	Activity []StudioActivityDayResponse `json:"activity"`
	// ActivityDays 是热力图覆盖的天数（前端据此校验网格尺寸，不必硬编码 91）。
	ActivityDays int `json:"activity_days"`
}

// StudioStats 个人数据概览（登录用户读自己的）。
//
// 不接受任何"看别人"的参数：路由层只从登录态取 userID，端口形状里也写死，
// 个人概览不该成为越权读入口。不是管理员专属 —— 每个作者都能看自己的。
// @Summary 个人数据概览
// @Tags article
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[StudioStatsResponse]
// @Router /articles/studio-stats [get]
func (h *Handler) StudioStats(c *gin.Context) {
	stats, err := h.svc.StudioStats(c.Request.Context(), h.viewerOf(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	res.OkWithData(toStudioStatsResponse(stats), c)
}

// toStudioStatsResponse 把领域读模型翻译成响应。
func toStudioStatsResponse(stats *app.StudioStats) StudioStatsResponse {
	out := StudioStatsResponse{
		StatusCounts: make([]StudioStatusCountResponse, 0, len(stats.StatusCounts)),
		Recent:       make([]StudioArticleBriefResponse, 0, len(stats.Recent)),
		Top:          make([]StudioArticleBriefResponse, 0, len(stats.Top)),
		Activity:     make([]StudioActivityDayResponse, 0, len(stats.Activity)),
		ActivityDays: app.StudioStatsDays,
		SumView:      stats.SumView,
		SumDigg:      stats.SumDigg,
		SumComment:   stats.SumComment,
		SumFavor:     stats.SumFavor,
	}
	for status, count := range stats.StatusCounts {
		out.StatusCounts = append(out.StatusCounts, StudioStatusCountResponse{Status: int(status), Count: int(count)})
	}
	for _, a := range stats.Recent {
		out.Recent = append(out.Recent, toStudioBrief(a))
	}
	for _, a := range stats.Top {
		out.Top = append(out.Top, toStudioBrief(a))
	}
	for day, count := range stats.Activity {
		out.Activity = append(out.Activity, StudioActivityDayResponse{Day: day, Count: count})
	}
	return out
}

// toStudioBrief 把文章聚合翻成概览里的一行。
func toStudioBrief(a *domain.Article) StudioArticleBriefResponse {
	counters := a.Counters()
	return StudioArticleBriefResponse{
		ID:           ctype.ID(a.ID()),
		Title:        a.Title(),
		Cover:        a.Cover(),
		ViewCount:    counters.View,
		DiggCount:    counters.Digg,
		CommentCount: counters.Comment,
		FavorCount:   counters.Favor,
		CreatedAt:    a.CreatedAt().Format("2006-01-02T15:04:05Z07:00"),
	}
}

package http

import (
	"time"

	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"
	"myblogx/internal/platform/redis/redis_site"

	"github.com/gin-gonic/gin"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// 本文件是站点概览的**补充计数**端点（票 349）。
//
// 为什么需要它：概览页要显示"累计访问 / 近 7 日访问 / 待审稿件 / 今日新增"这类数字，
// 而它们不该各占一个端点 —— 每多一个端点，前端就多一次往返、多一处可能失败。
// 这里只放**已有端点不提供的计数**：用户总数、文章总数在 /data/sum，趋势在
// /data/growth，AI 用量在 /data/ai-usage，都不重复搬过来。
//
// 口径统一按 Asia/Shanghai 切天（见 data_window.go）。

// flowRetentionDays 取 Redis 计数侧的保留期，不在这里另写一个 30。
//
// 之前两处各写一遍并靠注释提醒"改一处要同时改另一处"——这种提醒没有强制力，
// 漂了之后"今年访问"会静默只算 30 天而看不出错。
const flowRetentionDays = redis_site.MaxDailyFlowDays

// OverviewRequest 是概览的查询参数。
type OverviewRequest struct {
	// Range 是时间窗：today / week / month / year（默认 week）。
	Range string `form:"range" enums:"today,week,month,year"`
}

// OverviewResponse 是概览的补充计数。
type OverviewResponse struct {
	// Range 是收敛后的时间窗。
	Range string `json:"range"`
	// RangeStart / RangeEnd 是窗口边界（ISO8601，东八区），让前端能把
	// "这个数字算的是哪段时间"直接写在提示里，而不是自己再推一遍。
	RangeStart string `json:"range_start"`
	RangeEnd   string `json:"range_end"`
	// Visits 是窗口内的访问量（与「累计访问」同一口径，见 /data/sum）。
	Visits int `json:"visits"`
	// VisitsDays 是 Visits 实际覆盖的天数。
	//
	// 访问量存在 Redis 的按天哈希里，只保留最近 30 天（见 redis_site）。
	// 所以「今年访问」只能给出最近 30 天的和 —— 把这个天数一起返回，
	// 前端就能如实标注"近 30 天访问"，而不是把 30 天的数字说成一年的。
	VisitsDays int `json:"visits_days"`
	// NewUsers 是窗口内新增注册数。
	NewUsers int `json:"new_users"`
	// NewArticles 是窗口内发布的文章数。
	NewArticles int `json:"new_articles"`
}

// OverviewView 站点概览补充计数（管理员）。
//
// 只给计数：任何"已经有端点"的数据都不在这里重复，前端各卡片仍各自调各自的端点。
// @Summary 站点概览补充计数
// @Tags data
// @Produce json
// @Security BearerAuth
// @Param range query string false "时间窗：today / week / month / year（默认 week）" enums:"today,week,month,year"
// @Success 200 {object} kernel.Envelope[OverviewResponse]
// @Router /data/overview [get]
func (h *Handler) OverviewView(c *gin.Context) {
	cr := middleware.GetBindQuery[OverviewRequest](c)

	window := windowOf(cr.Range, time.Now())
	resp := OverviewResponse{
		Range:      window.Range,
		RangeStart: window.Start.Format(time.RFC3339),
		RangeEnd:   window.End.Format(time.RFC3339),
	}

	// 访问量：窗口内求和（累计访问在 /data/sum，不在这里重复）。
	//
	// 天数取窗口与 Redis 保留期的较小值：按天哈希只留 30 天，按一年去求
	// 会把"只有 30 天数据"悄悄当成"一年只有这么多访问"。
	if h.flow != nil {
		days := window.Days
		if days > flowRetentionDays {
			days = flowRetentionDays
		}
		for _, point := range h.flow.Recent(days) {
			resp.Visits += point.Count
		}
		resp.VisitsDays = days
	}

	if signups, err := h.stats.SignupsByDay(reqCtx(c), window.Start, window.End); err == nil {
		for _, count := range signups {
			resp.NewUsers += count
		}
	}

	if count, err := h.stats.PublishedCountBetween(reqCtx(c), window.Start, window.End); err == nil {
		resp.NewArticles = count
	}

	// 待审稿件不在这里：它是 article 模块的数据，分析模块不认识它，
	// 前端直接调已有的审核任务端点。硬塞进来会让两个模块互相依赖。

	res.OkWithData(resp, c)
}

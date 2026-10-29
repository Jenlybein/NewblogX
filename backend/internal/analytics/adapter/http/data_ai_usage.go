package http

import (
	"time"

	"myblogx/internal/analytics/adapter/clickhouse"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// AIUsageRequest 是 AI 用量趋势的查询参数。
type AIUsageRequest struct {
	Range string `form:"range"`
}

// AIUsageView 按周期聚合 AI token 用量（管理员）。
//
// 这个接口同时服务首页的「今日/本周/本月/本年」切换：四个周期共用一套
// 响应形状，前端只换 range，不用为每个周期写一条请求。
// @Summary AI token 用量趋势
// @Tags data
// @Produce json
// @Security BearerAuth
// @Param range query string false "周期：today / week / month / year，默认 week"
// @Success 200 {object} kernel.Envelope[clickhouse.AIUsageSummary]
// @Router /data/ai-usage [get]
func (h *Handler) AIUsageView(c *gin.Context) {
	cr := middleware.GetBindQuery[AIUsageRequest](c)
	rng := cr.Range
	if rng == "" {
		rng = "week"
	}
	summary, err := clickhouse.SumAIUsage(h.logs, rng, time.Now())
	if err != nil {
		res.FailWithMsg("获取 AI 用量失败", c)
		return
	}
	res.OkWithData(summary, c)
}

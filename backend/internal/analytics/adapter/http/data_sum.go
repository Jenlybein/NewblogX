package http

import (
	"time"

	"myblogx/internal/analytics/adapter/clickhouse"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
	"myblogx/internal/platform/http/middleware"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// SumView 站点汇总统计（管理员）。
// @Summary 站点汇总统计
// @Tags data
// @Produce json
// @Security BearerAuth
// @Success 200 {object} kernel.Envelope[SumResponse]
// @Router /data/sum [get]
func (h *Handler) SumView(c *gin.Context) {
	cr := middleware.GetBindQuery[SumRequest](c)
	var data SumResponse

	// 今天 0 点按东八区算（票 349）：服务器在 UTC 时，"今天新增"会在
	// 每天 0–8 点把昨天下午的注册算成今天的。
	now := time.Now().In(statsTimezone)

	// range 只影响"新增"那两个数字的起点（票 349）：累计数（用户 / 文章 / 消息 /
	// 评论 / 累计访问）与窗口无关，它们本来就是全量。默认 week 与旧行为
	// （今天 0 点起）不同，所以注释里写清：概览页的两个访问 KPI 都按窗口看。
	window := windowOf(cr.Range, now)
	data.Range = window.Range

	summary, err := h.stats.Summary(reqCtx(c), window.Start)
	if err != nil {
		res.FailWithMsg("获取汇总数据失败", c)
		return
	}
	data.UserCount = summary.UserCount
	data.ArticleCount = summary.ArticleCount
	data.MessageCount = summary.MessageCount
	data.CommentCount = summary.CommentCount
	data.NewSignCount = summary.NewSignCount

	if loginCount, err := clickhouse.CountDistinctLoginUsersSince(h.logs, window.Start); err != nil {
		// 登录数来自 ClickHouse，没启用时这里会报错；用 0 兜底，不影响其它统计。
		data.NewLoginCount = 0
	} else {
		data.NewLoginCount = loginCount
	}

	if h.flow != nil {
		data.FlowCount = h.flow.Current()
	}

	res.OkWithData(data, c)
}

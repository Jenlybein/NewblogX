package http

import (
	"time"

	"myblogx/internal/analytics/adapter/clickhouse"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
)

// SumView 站点汇总统计（管理员）。
// @Summary 站点汇总统计
// @Tags data
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=SumResponse,msg=string}
// @Router /data/sum [get]
func (h *Handler) SumView(c *gin.Context) {
	var data SumResponse

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	summary, err := h.stats.Summary(reqCtx(c), todayStart)
	if err != nil {
		res.FailWithMsg("获取汇总数据失败", c)
		return
	}
	data.UserCount = summary.UserCount
	data.ArticleCount = summary.ArticleCount
	data.MessageCount = summary.MessageCount
	data.CommentCount = summary.CommentCount
	data.NewSignCount = summary.NewSignCount

	if loginCount, err := clickhouse.CountDistinctLoginUsersSince(h.logs, todayStart); err != nil {
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

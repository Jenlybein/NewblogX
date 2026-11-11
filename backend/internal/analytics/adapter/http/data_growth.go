package http

import (
	"time"

	"myblogx/internal/analytics/domain"
	"myblogx/internal/platform/http/middleware"
	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// GrowthDataView 增长趋势数据（管理员）。
//
// 票 349：窗口由 `range` 决定（today / week / month / year），所有边界按
// Asia/Shanghai 切；三个类型共用同一个窗口，前端每张卡片各自带自己的 range 调用一次。
// @Summary 增长趋势数据
// @Tags data
// @Produce json
// @Security BearerAuth
// @Param type query int true "统计类型：1 网站流量 2 文章发布 3 用户注册"
// @Param range query string false "时间窗：today / week / month / year（默认 week）" enums:"today,week,month,year"
// @Success 200 {object} kernel.Envelope[GrowthDataResponse]
// @Router /data/growth [get]
func (h *Handler) GrowthDataView(c *gin.Context) {
	cr := middleware.GetBindQuery[GrowthDataRequest](c)
	var resp GrowthDataResponse

	window := windowOf(cr.Range, time.Now())
	resp.Range = window.Range

	var (
		statList map[string]int
		err      error
	)
	switch cr.Type {
	case 1:
		resp.DateCountList = h.flowSeries(window)
	case 2:
		resp.DateCountList = make([]DateCountItem, 0, window.Days)
		for i := 0; i < window.Days; i++ {
			start := window.Start.AddDate(0, 0, i)
			end := start.AddDate(0, 0, 1)
			count, countErr := h.stats.PublishedCountBetween(reqCtx(c), start, end)
			if countErr != nil {
				err = countErr
				break
			}
			resp.DateCountList = append(resp.DateCountList, DateCountItem{
				Date:  start.Format("2006-01-02"),
				Count: count,
			})
		}
	case 3:
		statList, err = h.stats.SignupsByDay(reqCtx(c), window.Start, window.End)
	}
	if err != nil {
		res.FailWithMsg("获取增长数据失败", c)
		return
	}

	if cr.Type == 3 {
		resp.DateCountList = make([]DateCountItem, 0, window.Days)
		for i := 0; i < window.Days; i++ {
			date := window.Start.AddDate(0, 0, i).Format("2006-01-02")
			resp.DateCountList = append(resp.DateCountList, DateCountItem{
				Date:  date,
				Count: statList[date],
			})
		}
	}

	// 环比只在"至少两天"时有意义：今天还没过完，与昨天比只是半天的量。
	if len(resp.DateCountList) >= 2 {
		todayCount := resp.DateCountList[len(resp.DateCountList)-1].Count
		yesterdayCount := resp.DateCountList[len(resp.DateCountList)-2].Count
		resp.GrowthNum = todayCount - yesterdayCount
		if yesterdayCount == 0 {
			if todayCount > 0 {
				resp.GrowthRate = 100
			}
		} else {
			resp.GrowthRate = int(float64(resp.GrowthNum) / float64(yesterdayCount) * 100)
		}
	}

	res.OkWithData(resp, c)
}

// flowSeries 把窗口内的访问量铺成逐日序列（缺的日期补 0）。
//
// 访问量来自 Redis 的按天哈希；缓存空时补零而不是返回空数组 ——
// 前端要按窗口长度画趋势图，长度对不上会让图突然变短。
func (h *Handler) flowSeries(window statsWindow) []DateCountItem {
	byDate := make(map[string]int, window.Days)
	if h.flow != nil {
		// Recent 只支持"最近 N 天"，所以取窗口长度再按日期挑。
		for _, point := range h.flow.Recent(window.Days) {
			byDate[point.Date] = point.Count
		}
	}

	out := make([]DateCountItem, 0, window.Days)
	for i := 0; i < window.Days; i++ {
		date := window.Start.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, DateCountItem{Date: date, Count: byDate[date]})
	}
	return out
}

// 让 domain 包被使用：flowSeries 的注释里说明了 FlowPoint 的来源。
var _ = domain.FlowPoint{}

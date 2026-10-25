package http

import (
	"time"

	"myblogx/common/res"
	"myblogx/internal/analytics/domain"
	"myblogx/middleware"

	"github.com/gin-gonic/gin"
)

// GrowthDataView 近七日增长数据（管理员）。
// @Summary 近七日增长数据
// @Tags data
// @Produce json
// @Security BearerAuth
// @Param type query int true "统计类型：1 网站流量 2 文章发布 3 用户注册"
// @Success 200 {object} object{code=int,data=GrowthDataResponse,msg=string}
// @Router /data/growth [get]
func (h *Handler) GrowthDataView(c *gin.Context) {
	cr := middleware.GetBindQuery[GrowthDataRequest](c)
	var resp GrowthDataResponse

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	rangeStart := todayStart.AddDate(0, 0, -6)
	tomorrowStart := todayStart.AddDate(0, 0, 1)

	var (
		statList map[string]int
		err      error
	)
	switch cr.Type {
	case 1:
		resp.DateCountList = make([]DateCountItem, 0, 7)
		var points []domain.FlowPoint
		if h.flow != nil {
			points = h.flow.Recent(7)
		}
		if len(points) == 0 {
			// 访问量来自 Redis；缓存空时补零，保证下面取"今天/昨天"不会越界。
			for i := 0; i < 7; i++ {
				resp.DateCountList = append(resp.DateCountList, DateCountItem{
					Date: rangeStart.AddDate(0, 0, i).Format("2006-01-02"),
				})
			}
			break
		}
		for _, item := range points {
			resp.DateCountList = append(resp.DateCountList, DateCountItem{Date: item.Date, Count: item.Count})
		}
	case 2:
		resp.DateCountList = make([]DateCountItem, 0, 7)
		for i := 0; i < 7; i++ {
			start := rangeStart.AddDate(0, 0, i)
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
		statList, err = h.stats.SignupsByDay(reqCtx(c), rangeStart, tomorrowStart)
	}
	if err != nil {
		res.FailWithMsg("获取增长数据失败", c)
		return
	}

	if cr.Type == 3 {
		dateMap := statList
		resp.DateCountList = make([]DateCountItem, 0, 7)
		for i := 0; i < 7; i++ {
			date := rangeStart.AddDate(0, 0, i).Format("2006-01-02")
			resp.DateCountList = append(resp.DateCountList, DateCountItem{
				Date:  date,
				Count: dateMap[date],
			})
		}
	}

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

	res.OkWithData(resp, c)
}

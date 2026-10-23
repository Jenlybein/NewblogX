package data_api

import (
	"myblogx/common/res"
	"time"

	"github.com/gin-gonic/gin"
)

func (h DataApi) ArticleYearDataView(c *gin.Context) {
	app := h.App
	var resp ArticleYearDataResponse

	now := time.Now()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	rangeStart := currentMonthStart.AddDate(0, -11, 0)

	resp.DateCountList = make([]DateCountItem, 0, 12)
	for i := 0; i < 12; i++ {
		start := rangeStart.AddDate(0, i, 0)
		end := start.AddDate(0, 1, 0)
		count, err := countPublishedArticlesBetween(app.DB, start, end)
		if err != nil {
			app.Logger.Errorf("获取年度文章数据失败: %v", err)
			res.FailWithMsg("获取年度文章数据失败", c)
			return
		}
		resp.DateCountList = append(resp.DateCountList, DateCountItem{
			Date:  start.Format("2006-01"),
			Count: count,
		})
	}

	res.OkWithData(resp, c)
}

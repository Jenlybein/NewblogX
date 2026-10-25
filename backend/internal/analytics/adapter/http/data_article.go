package http

import (
	"myblogx/internal/platform/http/res"
	"time"

	"github.com/gin-gonic/gin"
)

// ArticleYearDataView 近十二月文章发布数据（管理员）。
// @Summary 近十二月文章发布数据
// @Tags data
// @Produce json
// @Security BearerAuth
// @Success 200 {object} object{code=int,data=ArticleYearDataResponse,msg=string}
// @Router /data/article-year [get]
func (h *Handler) ArticleYearDataView(c *gin.Context) {
	var resp ArticleYearDataResponse

	now := time.Now()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	rangeStart := currentMonthStart.AddDate(0, -11, 0)

	resp.DateCountList = make([]DateCountItem, 0, 12)
	for i := 0; i < 12; i++ {
		start := rangeStart.AddDate(0, i, 0)
		end := start.AddDate(0, 1, 0)
		count, err := h.stats.PublishedCountBetween(reqCtx(c), start, end)
		if err != nil {
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

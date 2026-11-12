package http

import (
	"strings"
	"time"

	"myblogx/internal/platform/http/res"

	"github.com/gin-gonic/gin"
	// 供 swag 注解里的 kernel.Envelope 解析用（代码里不直接引用）。
	_ "myblogx/internal/kernel"
)

// GrowthDataView 增长趋势数据（管理员）。
//
// 票 349：窗口由 `range` 决定（today / week / month / year），所有边界按
// Asia/Shanghai 切；三个类型共用同一个窗口，前端每张卡片各自带自己的 range 调用一次。
// #367：week / month / year 改为向前取的滚动窗口，year 按**月**出桶。
// @Summary 增长趋势数据
// @Tags data
// @Produce json
// @Security BearerAuth
// @Param type query int true "统计类型：1 网站流量 2 文章发布 3 用户注册"
// @Param range query string false "时间窗：week / month / year（默认 week；year 按月出点）" enums:"today,week,month,year"
// @Success 200 {object} kernel.Envelope[GrowthDataResponse]
// @Router /data/growth [get]
func (h *Handler) GrowthDataView(c *gin.Context) {
	cr := bindQuery[GrowthDataRequest](c)
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
		resp.DateCountList = make([]DateCountItem, 0, window.Buckets)
		for i := 0; i < window.Buckets; i++ {
			count, countErr := h.stats.PublishedCountBetween(reqCtx(c), window.BucketStart(i), window.BucketEnd(i))
			if countErr != nil {
				err = countErr
				break
			}
			resp.DateCountList = append(resp.DateCountList, DateCountItem{
				Date:  window.BucketLabel(i),
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
		resp.DateCountList = fillBuckets(window, statList)
	}

	// 环比拿最后两个桶比：日粒度是"今天 vs 昨天"，月粒度是"本月 vs 上月"。
	// 后一个桶都是**没过完**的，所以它天然偏低 —— 这是存量口径，不是 bug。
	if len(resp.DateCountList) >= 2 {
		last := resp.DateCountList[len(resp.DateCountList)-1].Count
		prev := resp.DateCountList[len(resp.DateCountList)-2].Count
		resp.GrowthNum = last - prev
		if prev == 0 {
			if last > 0 {
				resp.GrowthRate = 100
			}
		} else {
			resp.GrowthRate = int(float64(resp.GrowthNum) / float64(prev) * 100)
		}
	}

	res.OkWithData(resp, c)
}

// flowSeries 把窗口内的访问量铺成序列（缺的桶补 0）。
//
// 访问量来自 Redis 的按天哈希，保留期只有 flowRetentionDays 天：月粒度时
// 把能拿到的那些天按月求和，拿不到的历史月份是 0 —— 这是数据源的限制，
// 概览页的 visits_days 会如实告诉前端"实际覆盖了几天"。
func (h *Handler) flowSeries(window statsWindow) []DateCountItem {
	byDate := make(map[string]int, flowRetentionDays)
	if h.flow != nil {
		for _, point := range h.flow.Recent(flowRetentionDays) {
			byDate[point.Date] = point.Count
		}
	}
	return fillBuckets(window, byDate)
}

// fillBuckets 把"按日给出的计数"铺进窗口的桶里。
//
// 日粒度直接按键取；月粒度把同一 `YYYY-MM` 前缀的日期求和。
// 铺满整个窗口而不是只回有数据的那几天：前端按桶数画图，长度对不上会让图突然变短。
func fillBuckets(window statsWindow, byDate map[string]int) []DateCountItem {
	out := make([]DateCountItem, 0, window.Buckets)
	for i := 0; i < window.Buckets; i++ {
		label := window.BucketLabel(i)
		if window.unit == unitMonth {
			sum := 0
			for date, count := range byDate {
				if strings.HasPrefix(date, label) {
					sum += count
				}
			}
			out = append(out, DateCountItem{Date: label, Count: sum})
			continue
		}
		out = append(out, DateCountItem{Date: label, Count: byDate[label]})
	}
	return out
}

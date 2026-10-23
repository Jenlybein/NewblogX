package data_api

import (
	"myblogx/common/res"
	"myblogx/models/enum"
	"myblogx/service/log_service"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_site"
	"time"

	"github.com/gin-gonic/gin"
)

func (h DataApi) SumView(c *gin.Context) {
	app := h.App
	var data SumResponse

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	if err := app.DB.Raw(`
		SELECT
			(SELECT COUNT(*) FROM user_models) AS user_count,
			(SELECT COUNT(*) FROM article_models WHERE `+effectiveArticlePublishStatusExpr+` = ?) AS article_count,
			(SELECT COUNT(*) FROM chat_msg_models) AS message_count,
			(SELECT COUNT(*) FROM comment_models) AS comment_count,
			0 AS new_login_count,
			(SELECT COUNT(*) FROM user_models WHERE created_at >= ?) AS new_sign_count
		`,
		enum.ArticleStatusPublished,
		todayStart,
	).Scan(&data).Error; err != nil {
		app.Logger.Errorf("获取后台汇总数据失败: %v", err)
		res.FailWithMsg("获取汇总数据失败", c)
		return
	}
	loginCount, err := log_service.CountDistinctLoginUsersSince(log_service.NewDeps(h.App.Log, h.App.System, h.App.ClickHouseConfig.Enabled, h.App.Logger, h.App.ClickHouse), todayStart)
	if err != nil {
		app.Logger.Errorf("统计今日登录用户失败: %v", err)
	} else {
		data.NewLoginCount = loginCount
	}

	data.FlowCount = redis_site.GetFlow(redis_service.NewDeps(h.App.Redis, h.App.Logger))

	res.OkWithData(data, c)
}

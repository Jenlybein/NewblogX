package top

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/platform/cachex"
	"myblogx/service/top_service"

	"github.com/gin-gonic/gin"
)

func (h TopApi) ArticleTopListView(c *gin.Context) {
	cr := middleware.GetBindQuery[ArticleTopListRequest](c)

	if cr.Type == 1 && cr.UserID == 0 {
		res.FailWithMsg("请选择作者", c)
		return
	}

	queryService := top_service.NewQueryService(h.App.DB, cachex.NewDeps(h.App.Redis, h.App.Logger))
	list, err := queryService.ListArticles(cr.Type, cr.UserID)
	if err != nil {
		res.FailWithMsg("查询置顶文章失败", c)
		return
	}

	res.OkWithList(list, len(list), c)
}

package top

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/platform/cachex"
	"myblogx/service/top_service"

	"github.com/gin-gonic/gin"
)

// ArticleTopListView 查询置顶文章列表。
// @Summary 置顶文章列表
// @Tags article
// @Produce json
// @Param type query int true "置顶类型(1:用户 2:管理员)"
// @Param user_id query int false "用户 ID(type=1 时必填)"
// @Success 200 {object} object{code=int,data=object{list=[]top_service.ArticleTopListItem,count=int},msg=string}
// @Router /articles/top [get]
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

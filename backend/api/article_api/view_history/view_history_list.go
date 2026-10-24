package view_history

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

// ArticleViewHistoryView 分页查询文章浏览历史。
// @Summary 文章浏览历史
// @Tags article
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码"
// @Param limit query int false "每页条数"
// @Param key query string false "关键词"
// @Param order query string false "排序"
// @Param user_id query int false "目标用户 ID(type=2 时使用)"
// @Param type query int true "查询类型(1:自己 2:别人)"
// @Success 200 {object} object{code=int,data=object{list=[]view_history.ArticleViewHistoryResponse,has_more=bool},msg=string}
// @Router /articles/history [get]
func (h ViewHistoryApi) ArticleViewHistoryView(c *gin.Context) {
	cr := middleware.GetBindQuery[ArticleViewHistoryRequest](c)
	claims := jwts.GetClaimsByGin(c)

	switch cr.Type {
	case 1:
		cr.UserID = claims.UserID
	case 2:
	}

	_list, hasMore, err := common.ListQueryHasMore(models.UserArticleViewHistoryModel{
		UserID: cr.UserID,
	}, common.Options{
		DB:       h.App.DB,
		PageInfo: cr.PageInfo,
		Preloads: []string{"UserModel", "ArticleModel"},
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	var list = make([]ArticleViewHistoryResponse, 0)
	for _, item := range _list {
		list = append(list, ArticleViewHistoryResponse{
			UpdatedAt: item.UpdatedAt,
			Title:     item.ArticleModel.Title,
			Cover:     item.ArticleModel.Cover,
			Nickname:  item.UserModel.Nickname,
			Avatar:    item.UserModel.Avatar,
			UserID:    item.UserID,
			ArticleID: item.ArticleID,
		})
	}

	res.OkWithHasMoreList(list, hasMore, c)
}

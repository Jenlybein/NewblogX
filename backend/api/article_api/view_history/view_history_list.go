package view_history

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

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

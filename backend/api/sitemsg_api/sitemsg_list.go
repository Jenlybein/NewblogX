package sitemsg_api

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/enum/message_enum"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

func (h *SitemsgApi) SitemsgListView(c *gin.Context) {
	app := h.App
	cr := middleware.GetBindQuery[SitemsgListRequest](c)

	var typeList []message_enum.Type
	switch cr.T {
	case 1:
		typeList = append(typeList, message_enum.CommentArticleType, message_enum.CommentReplyType)
	case 2:
		typeList = append(typeList, message_enum.DiggArticleType, message_enum.DiggCommentType, message_enum.FavorArticleType)
	case 3:
		typeList = append(typeList, message_enum.SystemType)
	}

	claims := jwts.MustGetClaimsByGin(c)

	list, hasMore, err := common.ListQueryHasMore(models.ArticleMessageModel{
		ReceiverID: claims.UserID,
	}, common.Options{
		DB:       app.DB,
		PageInfo: cr.PageInfo,
		Where:    app.DB.Where("type in ?", typeList),
	})
	if err != nil {
		res.FailWithError(err, c)
		return
	}

	res.OkWithHasMoreList(list, hasMore, c)
}

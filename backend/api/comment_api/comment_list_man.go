// 获取评论管理列表
package comment_api

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/platform/cachex"
	"myblogx/service/comment_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

type CommentManListRequest struct {
	common.PageInfo
	ArticleID ctype.ID           `form:"article_id"`
	UserID    ctype.ID           `form:"user_id"`
	Status    enum.CommentStatus `form:"status" binding:"oneof=1 2 3"`
	Type      int8               `form:"type" binding:"required,oneof=1 2 3"`
	// 1 查我文章下的评论 2 查我发的评论 3 管理员查所有评论
}

type CommentManListResponse struct {
	comment_service.ManageCommentItem
}

func (h CommentApi) CommentManListView(c *gin.Context) {
	cr := middleware.GetBindQuery[CommentManListRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	switch cr.Type {
	case 1: // 查我文章下的评论
		cr.Status = enum.CommentStatusPublished
	case 2: // 查我发的评论
		cr.UserID = claims.UserID
	case 3: // 管理员查所有评论
		if !claims.IsAdmin() {
			res.FailWithMsg("权限错误", c)
			return
		}
	}
	queryService := comment_service.NewQueryService(h.App.DB, cachex.NewDeps(h.App.Redis, h.App.Logger))
	commentList, count, err := queryService.ListManagedComments(comment_service.ManageCommentQuery{
		Type:      cr.Type,
		Status:    cr.Status,
		ViewerID:  claims.UserID,
		ArticleID: cr.ArticleID,
		UserID:    cr.UserID,
		PageInfo:  cr.PageInfo,
	})
	if err != nil {
		res.FailWithMsg("查询评论失败 "+err.Error(), c)
		return
	}

	res.OkWithList(commentList, count, c)
}

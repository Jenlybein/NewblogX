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

// CommentManListView 获取评论管理列表。
// @Summary 评论管理列表
// @Tags comment
// @Produce json
// @Security BearerAuth
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param key query string false "关键词"
// @Param order query string false "排序"
// @Param article_id query int false "文章 ID"
// @Param user_id query int false "用户 ID（type=2 时忽略，取当前登录用户）"
// @Param status query int false "评论状态：1 审核中 2 已发布 3 已删除"
// @Param type query int true "查询类型：1 我文章下的评论 2 我发的评论 3 管理员查所有评论"
// @Success 200 {object} object{code=int,data=object{list=[]comment_service.ManageCommentItem,count=int},msg=string}
// @Router /comments/man [get]
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

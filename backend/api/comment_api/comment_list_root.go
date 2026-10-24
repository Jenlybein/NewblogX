// 获取文章一级评论列表
package comment_api

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/platform/cachex"
	"myblogx/service/comment_service"

	"github.com/gin-gonic/gin"
)

type CommentRootListRequest struct {
	common.PageInfo
	ArticleID ctype.ID `form:"article_id" binding:"required"`
}

type CommentRootListResponse struct {
	comment_service.RootCommentItem
}

// CommentRootListView 获取文章一级评论列表。
// @Summary 一级评论列表
// @Tags comment
// @Produce json
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param key query string false "关键词"
// @Param order query string false "排序"
// @Param article_id query int true "文章 ID"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /comments [get]
func (h CommentApi) CommentRootListView(c *gin.Context) {
	cr := middleware.GetBindQuery[CommentRootListRequest](c)
	queryService := comment_service.NewQueryService(h.App.DB, cachex.NewDeps(h.App.Redis, h.App.Logger))

	var article models.ArticleModel
	if err := h.App.DB.Select("id").Take(&article, cr.ArticleID).Error; err != nil {
		res.FailWithMsg("文章不存在", c)
		return
	}

	rows, hasMore, err := queryService.ListPublishedRootComments(cr.ArticleID, cr.PageInfo, commentViewerIDFromGin(c))
	if err != nil {
		res.FailWithMsg("查询一级评论失败 "+err.Error(), c)
		return
	}

	res.OkWithData(map[string]any{
		"list":     rows,
		"has_more": hasMore,
	}, c)
}

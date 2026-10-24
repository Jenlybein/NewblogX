package comment_api

import (
	"myblogx/common"
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/platform/cachex"
	"myblogx/service/comment_service"

	"github.com/gin-gonic/gin"
)

type CommentReplyListRequest struct {
	common.PageInfo
	ArticleID ctype.ID `form:"article_id" binding:"required"`
	RootID    ctype.ID `form:"root_id" binding:"required"`
}

type CommentReplyListResponse struct {
	comment_service.ReplyCommentItem
}

// CommentReplyListView 获取一级评论下的二级评论列表。
// @Summary 二级评论列表
// @Tags comment
// @Produce json
// @Param limit query int false "每页条数"
// @Param page query int false "页码"
// @Param key query string false "关键词"
// @Param order query string false "排序"
// @Param article_id query int true "文章 ID"
// @Param root_id query int true "一级评论 ID"
// @Success 200 {object} object{code=int,data=object{},msg=string}
// @Router /comments/replies [get]
func (h CommentApi) CommentReplyListView(c *gin.Context) {
	cr := middleware.GetBindQuery[CommentReplyListRequest](c)
	queryService := comment_service.NewQueryService(h.App.DB, cachex.NewDeps(h.App.Redis, h.App.Logger))

	// 查询一级评论
	var root models.CommentModel
	if err := h.App.DB.Select("id", "article_id", "reply_id", "root_id", "reply_count").
		Take(&root, "id = ? and article_id = ? and status = ?", cr.RootID, cr.ArticleID, enum.CommentStatusPublished).Error; err != nil {
		res.FailWithMsg("一级评论不存在", c)
		return
	}
	if root.ReplyId != 0 || root.RootID != 0 {
		res.FailWithMsg("必须是一级评论", c)
		return
	}

	rows, hasMore, err := queryService.ListPublishedReplyComments(cr.ArticleID, cr.RootID, cr.PageInfo, commentViewerIDFromGin(c))
	if err != nil {
		res.FailWithMsg("查询二级评论失败 "+err.Error(), c)
		return
	}
	rootReplyCount := root.ReplyCount
	rootCounters := queryService.CounterReader.Batch([]ctype.ID{cr.RootID})
	rootReplyCount += rootCounters.ReplyMap[cr.RootID]
	res.OkWithData(map[string]any{
		"root_id":     cr.RootID,
		"reply_count": rootReplyCount,
		"list":        rows,
		"has_more":    hasMore,
	}, c)
}

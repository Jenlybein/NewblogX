package comment_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_article"
	"myblogx/service/redis_service/redis_comment"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
)

func (h CommentApi) CommentRemoveView(c *gin.Context) {
	cr := middleware.GetBindUri[models.IDRequest](c)
	claims := jwts.MustGetClaimsByGin(c)
	db := h.App.DB
	logger := h.App.Logger

	var target models.CommentModel
	if err := db.Select("id", "article_id", "user_id", "reply_id", "root_id", "status").
		Take(&target, cr.ID).Error; err != nil {
		res.FailWithMsg("评论不存在", c)
		return
	}

	// 权限：管理员可删全部；评论作者可删自己；文章作者可删文章下评论。
	if !claims.IsAdmin() && target.UserID != claims.UserID {
		var article models.ArticleModel
		if err := db.Select("id").
			Take(&article, "id = ? AND author_id = ?", target.ArticleID, claims.UserID).Error; err != nil {
			res.FailWithMsg("无权限删除该评论", c)
			return
		}
	}

	isRoot := target.ReplyId == 0 && target.RootID == 0

	var deleteList []models.CommentModel
	query := db.Model(&models.CommentModel{}).
		Select("id", "article_id", "root_id", "status")
	if isRoot {
		if err := query.Where("id = ? OR root_id = ?", target.ID, target.ID).Find(&deleteList).Error; err != nil {
			res.FailWithMsg("查询待删除评论失败", c)
			return
		}
	} else {
		deleteList = append(deleteList, models.CommentModel{
			Model:     models.Model{ID: target.ID},
			ArticleID: target.ArticleID,
			RootID:    target.RootID,
			Status:    target.Status,
		})
	}

	if len(deleteList) == 0 {
		res.FailWithMsg("评论不存在", c)
		return
	}

	deleteIDs := make([]ctype.ID, 0, len(deleteList))
	articleDelta := 0
	for _, item := range deleteList {
		deleteIDs = append(deleteIDs, item.ID)
		if item.Status == enum.CommentStatusPublished {
			articleDelta--
		}
	}
	// 软删除
	if err := db.Delete(&models.CommentModel{}, "id IN ?", deleteIDs).Error; err != nil {
		res.FailWithMsg("删除评论失败", c)
		return
	}
	if err := db.Delete(&models.CommentDiggModel{}, "comment_id IN ?", deleteIDs).Error; err != nil {
		logger.Errorf("删除评论点赞记录失败: 评论ID列表=%v 错误=%v", deleteIDs, err)
	}
	for _, commentID := range deleteIDs {
		if err := redis_comment.DelCacheDigg(redis_service.NewDeps(h.App.Redis, h.App.Logger), commentID); err != nil {
			logger.Errorf("删除评论点赞缓存失败: 评论ID=%d 错误=%v", commentID, err)
		}
	}

	// 删除已发布评论时，回滚文章评论数缓存。
	if articleDelta != 0 {
		if err := redis_article.SetCacheComment(redis_service.NewDeps(h.App.Redis, h.App.Logger), target.ArticleID, articleDelta); err != nil {
			logger.Errorf("回写文章评论缓存失败: 文章ID=%d 增量=%d 错误=%v", target.ArticleID, articleDelta, err)
		}
	}

	// 删除一级评论时，删除根评论回复数缓存。
	if isRoot {
		if err := redis_comment.DelCacheReply(redis_service.NewDeps(h.App.Redis, h.App.Logger), target.ID); err != nil {
			logger.Errorf("删除评论回复缓存失败: 根评论ID=%d 错误=%v", target.ID, err)
		}
	}

	// 删除已发布二级评论时，回滚根评论回复数缓存。
	if !isRoot && target.Status == enum.CommentStatusPublished && target.RootID != 0 {
		if err := redis_comment.SetCacheReply(redis_service.NewDeps(h.App.Redis, h.App.Logger), target.RootID, -1); err != nil {
			logger.Errorf("回写根评论回复缓存失败: 根评论ID=%d 增量=-1 错误=%v", target.RootID, err)
		}
	}

	res.OkWithMsg("删除评论成功", c)
}

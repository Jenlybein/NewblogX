package comment_api

import (
	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/enum"
	dbservice "myblogx/service/db_service"
	"myblogx/service/message_service"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_comment"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h CommentApi) CommentDiggView(c *gin.Context) {
	id := middleware.GetBindUri[models.IDRequest](c)
	db := h.App.DB
	logger := h.App.Logger

	var comment models.CommentModel
	if err := db.Preload("ArticleModel", func(db *gorm.DB) *gorm.DB { return db.Select("id", "title") }).Take(&comment, "id = ? and status = ?", id.ID, enum.CommentStatusPublished).Error; err != nil {
		res.FailWithMsg("评论不存在", c)
		return
	}

	claims := jwts.MustGetClaimsByGin(c)
	var digg models.CommentDiggModel
	if err := db.Take(&digg, "comment_id = ? and user_id = ?", id.ID, claims.UserID).Error; err == nil {
		// 取消点赞必须以条件删除的命中结果为准，避免并发下重复成功。
		// 取消点赞必须看本次 Delete 是否真的删掉了活记录，避免并发下双成功。
		deleteResult := db.Where(map[string]any{
			"comment_id": id.ID,
			"user_id":    claims.UserID,
		}).Delete(&models.CommentDiggModel{})
		if deleteResult.Error != nil {
			res.FailWithMsg("取消点赞失败", c)
			return
		}
		if deleteResult.RowsAffected == 0 {
			res.FailWithMsg("点赞状态已变化，请刷新后重试", c)
			return
		}
		if err := redis_comment.SetCacheDigg(redis_service.NewDeps(h.App.Redis, h.App.Logger), id.ID, -1); err != nil {
			logger.Errorf("回写评论点赞缓存失败: 评论ID=%d 错误=%v", id.ID, err)
		}
		res.OkWithMsg("取消点赞成功", c)
		return
	} else if err != gorm.ErrRecordNotFound {
		res.FailWithMsg("查询点赞记录失败", c)
		return
	}

	// 点赞成功与否只看本次恢复/新建是否真正落库。
	createdOrRestored, err := dbservice.RestoreOrCreateUnique(db, &models.CommentDiggModel{
		CommentID: id.ID,
		UserID:    claims.UserID,
	}, []string{"comment_id", "user_id"})
	if err != nil {
		res.FailWithMsg("点赞失败", c)
		return
	}
	if !createdOrRestored {
		res.FailWithMsg("请勿重复点赞", c)
		return
	}
	if err := redis_comment.SetCacheDigg(redis_service.NewDeps(h.App.Redis, h.App.Logger), id.ID, 1); err != nil {
		logger.Errorf("写入评论点赞缓存失败: 评论ID=%d 错误=%v", id.ID, err)
	}
	go message_service.InsertCommentDiggMessage(db, logger, message_service.CommentDiggMessage{
		ReceiverID:   comment.UserID,
		ActionUserID: claims.UserID,
		CommentID:    comment.ID,
		Content:      comment.Content,
		ArticleID:    comment.ArticleID,
		ArticleTitle: comment.ArticleModel.Title,
	})
	res.OkWithMsg("点赞成功", c)
}

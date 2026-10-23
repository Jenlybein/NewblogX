package article_api

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"myblogx/common/res"
	"myblogx/middleware"
	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"
	"myblogx/service/ai_service/article_score_service"
	"myblogx/service/message_service"
	"myblogx/service/read_service"
	"myblogx/utils/jwts"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func normalizeReviewListPage(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}

func (h ArticleApi) ArticleReviewTaskListView(c *gin.Context) {
	cr := middleware.GetBindQuery[ArticleReviewTaskListRequest](c)
	page, limit := normalizeReviewListPage(cr.Page, cr.Limit)

	query := h.App.DB.Model(&models.ArticleReviewTaskModel{})
	if cr.Status != "" {
		query = query.Where("status = ?", cr.Status)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		res.FailWithMsg("查询审核任务失败", c)
		return
	}

	var tasks []models.ArticleReviewTaskModel
	if err := query.
		Order("article_review_task_models.created_at DESC, article_review_task_models.id DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&tasks).Error; err != nil {
		res.FailWithMsg("查询审核任务失败", c)
		return
	}

	list := make([]ArticleReviewTaskItem, 0, len(tasks))
	for _, task := range tasks {
		list = append(list, ArticleReviewTaskItem{
			ID:            task.ID,
			ArticleID:     task.ArticleID,
			AuthorID:      task.AuthorID,
			ArticleTitle:  task.ArticleTitle,
			AuthorName:    task.AuthorName,
			PublishStatus: task.ArticlePublishStatus,
			Stage:         task.Stage,
			Source:        task.Source,
			Status:        task.Status,
			Reason:        task.Reason,
			CreatedAt:     task.CreatedAt,
			ReviewedAt:    task.ReviewedAt,
			ReviewedBy:    task.ReviewedBy,
		})
	}

	res.OkWithData(ArticleReviewTaskListResponse{
		Count: count,
		List:  list,
	}, c)
}

func (h ArticleApi) ArticleReviewTaskHandleView(c *gin.Context) {
	id := middleware.GetBindUri[models.IDRequest](c)
	cr := middleware.GetBindJson[ArticleReviewHandleRequest](c)
	claims := jwts.MustGetClaimsByGin(c)

	if cr.Status == enum.ArticleStatusRejected && strings.TrimSpace(cr.Reason) == "" {
		res.FailWithMsg("驳回原因不能为空", c)
		return
	}

	var task models.ArticleReviewTaskModel
	if err := h.App.DB.Take(&task, "id = ?", id.ID).Error; err != nil {
		res.FailWithMsg("审核任务不存在", c)
		return
	}
	if task.Status != models.ArticleReviewTaskPending {
		res.FailWithMsg("审核任务已处理", c)
		return
	}

	var article models.ArticleModel
	if err := h.App.DB.Take(&article, "id = ?", task.ArticleID).Error; err != nil {
		res.FailWithMsg("文章不存在", c)
		return
	}

	now := time.Now()
	taskStatus := models.ArticleReviewTaskApproved
	action := "approve"
	if cr.Status == enum.ArticleStatusRejected {
		taskStatus = models.ArticleReviewTaskRejected
		action = "reject"
	}

	if err := h.App.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&task).Updates(map[string]any{
			"status":                 taskStatus,
			"reason":                 strings.TrimSpace(cr.Reason),
			"reviewed_at":            &now,
			"reviewed_by":            &claims.UserID,
			"article_publish_status": cr.Status,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&article).Updates(map[string]any{
			"publish_status": cr.Status,
			"reviewed_at":    &now,
			"reviewed_by":    &claims.UserID,
		}).Error; err != nil {
			return err
		}
		log := models.ArticleReviewLogModel{
			TaskID:     task.ID,
			ArticleID:  article.ID,
			OperatorID: claims.UserID,
			Action:     action,
			FromStatus: string(models.ArticleReviewTaskPending),
			ToStatus:   string(taskStatus),
			Reason:     strings.TrimSpace(cr.Reason),
		}
		return tx.Create(&log).Error
	}); err != nil {
		res.FailWithMsg("处理审核任务失败", c)
		return
	}

	if err := read_service.SyncArticleFavorSnapshots(h.App.DB, []ctype.ID{article.ID}); err != nil {
		h.App.Logger.Errorf("同步文章收藏快照失败: 文章ID=%d 错误=%v", article.ID, err)
	}
	if cr.Status == enum.ArticleStatusPublished {
		article_score_service.EnsureArticleScoreIfMissingAsync(h.App.DB, h.App.Logger, h.App.RuntimeSite, article.ID)
	}

	switch cr.Status {
	case enum.ArticleStatusPublished:
		go message_service.InsertSystemMessage(h.App.DB, h.App.Logger, message_service.SystemMessage{
			ReceiverID:   article.AuthorID,
			ActionUserID: &claims.UserID,
			Content:      fmt.Sprintf("您的文章《%s》审核通过！", article.Title),
			LinkTitle:    article.Title,
			LinkHerf:     fmt.Sprintf("/article/%d", article.ID),
		})
	case enum.ArticleStatusRejected:
		go message_service.InsertSystemMessage(h.App.DB, h.App.Logger, message_service.SystemMessage{
			ReceiverID:   article.AuthorID,
			ActionUserID: &claims.UserID,
			Content:      fmt.Sprintf("您的文章《%s》审核未通过，请修改后再提交。\n驳回原因：%s", article.Title, strings.TrimSpace(cr.Reason)),
			LinkTitle:    article.Title,
			LinkHerf:     fmt.Sprintf("/article/%d", article.ID),
		})
	}

	res.OkWithMsg("审核处理成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "article_review_task_handle",
		TargetType: "article_review_task",
		TargetID:   strconv.FormatUint(uint64(task.ID), 10),
		Success:    true,
		Message:    "审核任务处理成功",
		RequestBody: map[string]any{
			"status": cr.Status,
			"reason": strings.TrimSpace(cr.Reason),
		},
		UseRawRequestBody: true,
		UseRawRequestHead: true,
	})
}

func (h ArticleApi) ArticleAdminVisibilityView(c *gin.Context) {
	cr := middleware.GetBindUri[ArticleAdminVisibilityURI](c)

	nextVisibility := enum.ArticleVisibilityVisible
	if cr.Visibility == "hide" {
		nextVisibility = enum.ArticleVisibilityAdminHidden
	}

	var article models.ArticleModel
	if err := h.App.DB.Take(&article, "id = ?", cr.ID).Error; err != nil {
		res.FailWithMsg("文章不存在", c)
		return
	}

	if err := h.App.DB.Model(&article).Update("visibility_status", nextVisibility).Error; err != nil {
		res.FailWithMsg("更新文章可见性失败", c)
		return
	}

	res.OkWithMsg("更新文章可见性成功", c)
	middleware.EmitActionAuditFromGin(c, middleware.GinAuditInput{
		ActionName: "article_admin_visibility",
		TargetType: "article",
		TargetID:   strconv.FormatUint(uint64(article.ID), 10),
		Success:    true,
		Message:    "更新文章可见性成功",
		RequestBody: map[string]any{
			"visibility": nextVisibility,
		},
		UseRawRequestHead: true,
	})
}

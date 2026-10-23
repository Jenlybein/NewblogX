package article_api

import (
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/models/enum"

	"gorm.io/gorm"
)

func normalizeArticleVisibilityStatus(status enum.ArticleVisibilityStatus) enum.ArticleVisibilityStatus {
	switch status {
	case enum.ArticleVisibilityUserHidden, enum.ArticleVisibilityAdminHidden:
		return status
	default:
		return enum.ArticleVisibilityVisible
	}
}

func resolvePublishStatus(requestStatus enum.ArticleStatus, skipExamining bool) enum.ArticleStatus {
	switch requestStatus {
	case enum.ArticleStatusDraft:
		return enum.ArticleStatusDraft
	case enum.ArticleStatusExamining:
		if skipExamining {
			return enum.ArticleStatusPublished
		}
		return enum.ArticleStatusExamining
	default:
		return requestStatus
	}
}

func cancelPendingReviewTasks(tx *gorm.DB, articleID, operatorID ctype.ID, articlePublishStatus enum.ArticleStatus, reason string) error {
	if tx == nil || articleID == 0 {
		return nil
	}

	var tasks []models.ArticleReviewTaskModel
	if err := tx.Where("article_id = ? AND status = ?", articleID, models.ArticleReviewTaskPending).Find(&tasks).Error; err != nil {
		return err
	}
	if len(tasks) == 0 {
		return nil
	}

	now := time.Now()
	taskIDs := make([]ctype.ID, 0, len(tasks))
	logs := make([]models.ArticleReviewLogModel, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
		logs = append(logs, models.ArticleReviewLogModel{
			TaskID:     task.ID,
			ArticleID:  task.ArticleID,
			OperatorID: operatorID,
			Action:     "cancel",
			FromStatus: string(task.Status),
			ToStatus:   string(models.ArticleReviewTaskCanceled),
			Reason:     reason,
		})
	}
	if err := tx.Model(&models.ArticleReviewTaskModel{}).
		Where("id IN ?", taskIDs).
		Updates(map[string]any{
			"status":                 models.ArticleReviewTaskCanceled,
			"reason":                 reason,
			"reviewed_at":            &now,
			"reviewed_by":            &operatorID,
			"article_publish_status": articlePublishStatus,
		}).Error; err != nil {
		return err
	}
	if len(logs) > 0 {
		return tx.Create(&logs).Error
	}
	return nil
}

func createReviewTask(tx *gorm.DB, article models.ArticleModel, source models.ArticleReviewTaskSource, operatorID ctype.ID) (*models.ArticleReviewTaskModel, error) {
	if tx == nil {
		return nil, nil
	}
	var author models.UserModel
	if err := tx.Select("nickname").Take(&author, "id = ?", article.AuthorID).Error; err != nil {
		return nil, err
	}
	task := &models.ArticleReviewTaskModel{
		ArticleID:            article.ID,
		AuthorID:             article.AuthorID,
		ArticleTitle:         article.Title,
		AuthorName:           author.Nickname,
		ArticlePublishStatus: article.EffectivePublishStatus(),
		Stage:                models.ArticleReviewTaskStageManual,
		Source:               source,
		Status:               models.ArticleReviewTaskPending,
	}
	if err := tx.Create(task).Error; err != nil {
		return nil, err
	}
	log := models.ArticleReviewLogModel{
		TaskID:     task.ID,
		ArticleID:  article.ID,
		OperatorID: operatorID,
		Action:     "submit",
		FromStatus: "",
		ToStatus:   string(models.ArticleReviewTaskPending),
	}
	if err := tx.Create(&log).Error; err != nil {
		return nil, err
	}
	return task, nil
}

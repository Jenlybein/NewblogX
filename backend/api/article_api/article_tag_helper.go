package article_api

import (
	"time"

	"myblogx/models"
	"myblogx/models/ctype"
	"myblogx/service/redis_service"
	"myblogx/service/redis_service/redis_tag"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func validateArticleCategory(db *gorm.DB, userID ctype.ID, categoryID *ctype.ID) error {
	if categoryID == nil {
		return nil
	}

	var category models.CategoryModel
	return db.Take(&category, "id = ? AND user_id = ?", *categoryID, userID).Error
}

func loadEnabledTagsByIDs(db *gorm.DB, tagIDs []ctype.ID) ([]models.TagModel, error) {
	uniqueIDs := normalizeTagIDs(tagIDs)
	if len(uniqueIDs) == 0 {
		return []models.TagModel{}, nil
	}

	var tagList []models.TagModel
	if err := db.Where("id IN ? AND is_enabled = ?", uniqueIDs, true).Find(&tagList).Error; err != nil {
		return nil, err
	}

	if len(tagList) != len(uniqueIDs) {
		return nil, errArticleTagInvalid
	}
	return tagList, nil
}

func normalizeTagIDs(tagIDs []ctype.ID) []ctype.ID {
	uniqueIDs := make([]ctype.ID, 0, len(tagIDs))
	seen := make(map[ctype.ID]struct{}, len(tagIDs))
	for _, tagID := range tagIDs {
		if tagID == 0 {
			continue
		}
		if _, ok := seen[tagID]; ok {
			continue
		}
		seen[tagID] = struct{}{}
		uniqueIDs = append(uniqueIDs, tagID)
	}
	return uniqueIDs
}

func extractTagIDs(tags []models.TagModel) []ctype.ID {
	ids := make([]ctype.ID, 0, len(tags))
	for _, tag := range tags {
		ids = append(ids, tag.ID)
	}
	return ids
}

func loadArticleTagIDs(db *gorm.DB, articleID ctype.ID) ([]ctype.ID, error) {
	var relationList []models.ArticleTagModel
	if err := db.Select("tag_id").Where("article_id = ?", articleID).Find(&relationList).Error; err != nil {
		return nil, err
	}

	tagIDs := make([]ctype.ID, 0, len(relationList))
	for _, item := range relationList {
		tagIDs = append(tagIDs, item.TagID)
	}
	return tagIDs, nil
}

func buildTagArticleCountDelta(oldTagIDs, newTagIDs []ctype.ID) map[ctype.ID]int {
	deltaMap := make(map[ctype.ID]int)
	oldSet := make(map[ctype.ID]struct{}, len(oldTagIDs))
	newSet := make(map[ctype.ID]struct{}, len(newTagIDs))

	for _, tagID := range normalizeTagIDs(oldTagIDs) {
		oldSet[tagID] = struct{}{}
	}
	for _, tagID := range normalizeTagIDs(newTagIDs) {
		newSet[tagID] = struct{}{}
	}

	for tagID := range newSet {
		if _, ok := oldSet[tagID]; !ok {
			deltaMap[tagID]++
		}
	}
	for tagID := range oldSet {
		if _, ok := newSet[tagID]; !ok {
			deltaMap[tagID]--
		}
	}

	return deltaMap
}

func applyTagArticleCountDelta(deps redis_service.Deps, deltaMap map[ctype.ID]int) {
	for tagID, delta := range deltaMap {
		if delta == 0 {
			continue
		}
		if err := redis_tag.SetCacheArticleCount(deps, tagID, delta); err != nil && deps.Logger != nil {
			deps.Logger.Errorf("标签文章数缓存更新失败: 标签ID=%d 增量=%d 错误=%v", tagID, delta, err)
		}
	}
}

func syncArticleTags(tx *gorm.DB, articleID ctype.ID, newTagIDs []ctype.ID) error {
	newTagIDs = normalizeTagIDs(newTagIDs)

	var relationList []models.ArticleTagModel
	if err := tx.Unscoped().
		Where("article_id = ?", articleID).
		Find(&relationList).Error; err != nil {
		return err
	}

	currentMap := make(map[ctype.ID]models.ArticleTagModel, len(relationList))
	for _, item := range relationList {
		currentMap[item.TagID] = item
	}

	for _, tagID := range newTagIDs {
		relation, ok := currentMap[tagID]
		if ok && relation.DeletedAt.Valid {
			if err := tx.Unscoped().Model(&relation).Updates(map[string]any{
				"deleted_at": nil,
				"updated_at": time.Now(),
			}).Error; err != nil {
				return err
			}
			continue
		}
		if ok {
			continue
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "article_id"},
				{Name: "tag_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"deleted_at": nil,
				"updated_at": time.Now(),
			}),
		}).Create(&models.ArticleTagModel{
			ArticleID: articleID,
			TagID:     tagID,
		}).Error; err != nil {
			return err
		}
	}

	newSet := make(map[ctype.ID]struct{}, len(newTagIDs))
	for _, tagID := range newTagIDs {
		newSet[tagID] = struct{}{}
	}
	for _, relation := range relationList {
		if relation.DeletedAt.Valid {
			continue
		}
		if _, ok := newSet[relation.TagID]; ok {
			continue
		}
		if err := tx.Delete(&relation).Error; err != nil {
			return err
		}
	}

	return nil
}

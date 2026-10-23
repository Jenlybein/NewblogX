package data_api

import (
	"myblogx/models/enum"
	"time"

	"gorm.io/gorm"
)

const effectiveArticlePublishStatusExpr = "publish_status"

func applyPublishedArticleWhere(query *gorm.DB) *gorm.DB {
	return query.Where(effectiveArticlePublishStatusExpr+" = ?", enum.ArticleStatusPublished)
}

func countPublishedArticlesBetween(db *gorm.DB, start, end time.Time) (int, error) {
	var count int64
	query := db.Table("article_models").
		Where("created_at >= ? AND created_at < ?", start, end)
	if err := applyPublishedArticleWhere(query).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

package article_service

import (
	"fmt"

	"myblogx/models"

	"gorm.io/gorm"
)

func DeleteArticles(tx *gorm.DB, list []models.ArticleModel, unscoped bool) error {
	if tx == nil || len(list) == 0 {
		return nil
	}

	return tx.Transaction(func(inner *gorm.DB) error {
		for _, article := range list {
			if err := deleteSingleArticle(inner, article, unscoped); err != nil {
				return err
			}
		}
		return nil
	})
}

func deleteSingleArticle(tx *gorm.DB, article models.ArticleModel, unscoped bool) error {
	deleteQuery := func() *gorm.DB {
		q := tx.Session(&gorm.Session{})
		if unscoped {
			return q.Unscoped()
		}
		return q
	}

	var commentList []models.CommentModel
	if err := tx.Where("article_id = ?", article.ID).Find(&commentList).Error; err != nil {
		return err
	}
	if err := deleteQuery().Where("article_id = ?", article.ID).Delete(&models.CommentModel{}).Error; err != nil {
		return err
	}

	var diggList []models.ArticleDiggModel
	if err := tx.Where("article_id = ?", article.ID).Find(&diggList).Error; err != nil {
		return err
	}
	if err := deleteQuery().Where("article_id = ?", article.ID).Delete(&models.ArticleDiggModel{}).Error; err != nil {
		return err
	}

	var favoriteList []models.UserArticleFavorModel
	if err := tx.Where("article_id = ?", article.ID).Find(&favoriteList).Error; err != nil {
		return err
	}
	if err := deleteQuery().Where("article_id = ?", article.ID).Delete(&models.UserArticleFavorModel{}).Error; err != nil {
		return err
	}

	var topList []models.UserTopArticleModel
	if err := tx.Where("article_id = ?", article.ID).Find(&topList).Error; err != nil {
		return err
	}
	if err := deleteQuery().Where("article_id = ?", article.ID).Delete(&models.UserTopArticleModel{}).Error; err != nil {
		return err
	}

	var viewList []models.UserArticleViewHistoryModel
	if err := tx.Where("article_id = ?", article.ID).Find(&viewList).Error; err != nil {
		return err
	}
	if err := deleteQuery().Where("article_id = ?", article.ID).Delete(&models.UserArticleViewHistoryModel{}).Error; err != nil {
		return err
	}

	var articleTagList []models.ArticleTagModel
	if err := tx.Where("article_id = ?", article.ID).Find(&articleTagList).Error; err != nil {
		return err
	}
	if err := deleteQuery().Where("article_id = ?", article.ID).Delete(&models.ArticleTagModel{}).Error; err != nil {
		return err
	}
	if err := deleteQuery().Delete(&article).Error; err != nil {
		return err
	}

	return nil
}

func DeleteArticleByID(tx *gorm.DB, articleID any, scope string, args ...any) error {
	var article models.ArticleModel
	query := tx
	if scope != "" {
		query = query.Where(scope, args...)
	}
	if err := query.Take(&article, articleID).Error; err != nil {
		return fmt.Errorf("查询文章失败: %w", err)
	}
	return DeleteArticles(tx, []models.ArticleModel{article}, false)
}

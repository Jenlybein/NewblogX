package user_service

import (
	"fmt"

	"myblogx/models"
	"myblogx/models/ctype"

	"gorm.io/gorm"
)

// StatApplyArticleDelta 更新作者的文章数量与文章累计浏览数冗余统计。
// articleCountDelta 用于创建/删除文章，articleVisitedCountDelta 用于文章被阅读累计变化。
func StatApplyArticleDelta(tx *gorm.DB, authorID ctype.ID, articleCountDelta, articleVisitedCountDelta int) error {
	if tx == nil {
		return fmt.Errorf("数据库事务不能为空")
	}
	if authorID.IsZero() || (articleCountDelta == 0 && articleVisitedCountDelta == 0) {
		return nil
	}

	if err := StatEnsureRows(tx, authorID); err != nil {
		return err
	}

	updates := map[string]any{}
	if articleCountDelta != 0 {
		if articleCountDelta > 0 {
			updates["article_count"] = gorm.Expr("article_count + ?", articleCountDelta)
		} else {
			updates["article_count"] = gorm.Expr(
				"CASE WHEN article_count + ? < 0 THEN 0 ELSE article_count + ? END",
				articleCountDelta,
				articleCountDelta,
			)
		}
	}
	if articleVisitedCountDelta != 0 {
		if articleVisitedCountDelta > 0 {
			updates["article_visited_count"] = gorm.Expr("article_visited_count + ?", articleVisitedCountDelta)
		} else {
			updates["article_visited_count"] = gorm.Expr(
				"CASE WHEN article_visited_count + ? < 0 THEN 0 ELSE article_visited_count + ? END",
				articleVisitedCountDelta,
				articleVisitedCountDelta,
			)
		}
	}

	result := tx.Model(&models.UserStatModel{}).
		Where("user_id = ?", authorID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("作者文章统计更新失败: author_id=%d", authorID)
	}
	return nil
}

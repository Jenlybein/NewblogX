package mysql

import (
	"context"

	"myblogx/internal/article/domain"
	"myblogx/internal/kernel/ctype"

	"gorm.io/gorm"
)

// AuthorInteractionStats 是某作者全部**已发布**文章的累计互动数。
type AuthorInteractionStats struct {
	Digg    int64
	Favor   int64
	Comment int64
}

// AuthorStats 汇总作者已发布文章的赞 / 收藏 / 评论总数。
//
// 公开主页的「累计被赞 / 累计被收藏 / 累计被评论」就是这三个数。口径只算
// `publish_status = 已发布` 且未软删的文章：草稿、审核中、已拒绝的文章读者看不到，
// 把它们的互动数算进作者的公开主页只会让人困惑"我什么时候被赞过"。
//
// 放在 article 模块的适配器里而不是 user 模块直接查表：表属于文章模块，
// 汇总口径也应该只有一处；user 侧只通过端口消费结论。
func AuthorStats(ctx context.Context, db *gorm.DB, authorID ctype.ID) (AuthorInteractionStats, error) {
	if db == nil || authorID.IsZero() {
		return AuthorInteractionStats{}, nil
	}

	var row AuthorInteractionStats
	err := db.WithContext(ctx).Model(&articleRow{}).
		Select("COALESCE(SUM(digg_count), 0) AS digg, "+
			"COALESCE(SUM(favor_count), 0) AS favor, "+
			"COALESCE(SUM(comment_count), 0) AS comment").
		Where("author_id = ? AND publish_status = ?", authorID, domain.StatusPublished).
		Scan(&row).Error
	if err != nil {
		return AuthorInteractionStats{}, err
	}
	return row, nil
}

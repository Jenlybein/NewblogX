package mysql

import (
	"gorm.io/gorm"
)

// 本文件承载「把 Redis 增量计数刷回文章/标签表」的写入。
//
// 这些 SQL 原本在 service/cron_service 里，用 `s.db.Model(&models.ArticleModel{})`
// 直接写。迁到本包后：表结构由本包的 row 定义，列名受类型约束，
// cron 侧只负责"取增量、调这里、失败回补"，不再持有任何 SQL。

// ArticleCounter 是被刷盘的文章计数列。
//
// 用类型而不是裸字符串：列名会被拼进 UPDATE 表达式，任何"来自变量"的列名
// 都是注入面。这里是编译期常量集合，调用方传不进任意值。
type ArticleCounter string

// 文章计数的四个列。
const (
	ArticleFavorCount   ArticleCounter = "favor_count"
	ArticleDiggCount    ArticleCounter = "digg_count"
	ArticleViewCount    ArticleCounter = "view_count"
	ArticleCommentCount ArticleCounter = "comment_count"
)

// FlushArticleCount 按增量更新一篇文章的计数。
//
// 返回 hit=false 表示文章不存在（已被删除）。这种情况**不返回 error**：
// 调用方若把它当失败就会把增量回补到 Redis，下一轮再撞同一条、无限重试。
// 正确处置是丢弃这条增量并记一条 Warn。
//
// 负数带 0 下限：取消点赞/收藏过多时不该出现负计数，那在页面上比少 1 更难解释。
func FlushArticleCount(db *gorm.DB, col ArticleCounter, articleID uint64, delta int) (hit bool, err error) {
	if db == nil {
		return false, nil
	}
	expr := "CASE WHEN " + string(col) + " + ? < 0 THEN 0 ELSE " + string(col) + " + ? END"
	res := db.Model(&articleRow{}).
		Where("id = ?", articleID).
		UpdateColumn(string(col), gorm.Expr(expr, delta, delta))
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// FlushTagArticleCount 按增量更新标签的文章数。
//
// hit=false 的处置同 FlushArticleCount（标签被删则丢弃增量）。
func FlushTagArticleCount(db *gorm.DB, tagID uint64, delta int) (hit bool, err error) {
	if db == nil {
		return false, nil
	}
	const expr = "CASE WHEN article_count + ? < 0 THEN 0 ELSE article_count + ? END"
	res := db.Model(&tagRow{}).
		Where("id = ?", tagID).
		UpdateColumn("article_count", gorm.Expr(expr, delta, delta))
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

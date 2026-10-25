package mysql

import (
	"gorm.io/gorm"
)

// FlushCommentCount 按增量更新一条评论的计数列。
//
// 与文章侧同一套约定：列名用类型约束（会被拼进 SQL 表达式），
// hit=false 表示评论已不存在 —— 调用方应丢弃增量而不是回补重试，
// 否则一条已删除的评论会让任务无限撞同一行。
func FlushCommentCount(db *gorm.DB, col CommentCounter, commentID uint64, delta int) (hit bool, err error) {
	if db == nil {
		return false, nil
	}
	expr := "CASE WHEN " + string(col) + " + ? < 0 THEN 0 ELSE " + string(col) + " + ? END"
	res := db.Model(&commentRow{}).
		Where("id = ?", commentID).
		UpdateColumn(string(col), gorm.Expr(expr, delta, delta))
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// CommentCounter 是被刷盘的评论计数列。
type CommentCounter string

// 评论计数的两个列。
const (
	CommentReplyCount CommentCounter = "reply_count"
	CommentDiggCount  CommentCounter = "digg_count"
)

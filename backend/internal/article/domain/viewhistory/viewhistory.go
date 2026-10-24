// Package viewhistory 是"用户浏览文章历史"子域。
//
// 语义上是一次 upsert：同一 (userID, articleID) 只保留一条记录，
// 再次浏览刷新时间并顶到最前，而不是追加一条。与库里的
// uk_user_article_history 唯一键一致。
package viewhistory

import (
	"context"
	"time"

	"myblogx/internal/article/domain"
)

// Record 是一条浏览记录。
type Record struct {
	articleID domain.ArticleID
	userID    domain.UserID
	viewedAt  time.Time
}

// NewRecord 生成浏览记录。
func NewRecord(userID domain.UserID, articleID domain.ArticleID, now time.Time) *Record {
	return &Record{articleID: articleID, userID: userID, viewedAt: now}
}

// ArticleID 返回文章。
func (r *Record) ArticleID() domain.ArticleID { return r.articleID }

// UserID 返回用户。
func (r *Record) UserID() domain.UserID { return r.userID }

// ViewedAt 返回浏览时间。
func (r *Record) ViewedAt() time.Time { return r.viewedAt }

// Touch 刷新浏览时间（重复浏览时调用）。
func (r *Record) Touch(now time.Time) { r.viewedAt = now }

// Repository 是浏览历史的持久化契约。
type Repository interface {
	// Upsert 写入或刷新一条浏览记录。
	Upsert(ctx context.Context, r *Record) error
	// List 分页列出某用户的浏览历史，按时间倒序。
	List(ctx context.Context, userID domain.UserID, offset, limit int) ([]*Record, int64, error)
	// Delete 删除某用户指定的若干条记录。
	Delete(ctx context.Context, userID domain.UserID, articleIDs []domain.ArticleID) error
	// DeleteByArticle 删除某文章的全部记录（删文章级联用）。
	DeleteByArticle(ctx context.Context, articleID domain.ArticleID) error
}
